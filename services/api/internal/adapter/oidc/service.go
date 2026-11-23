package oidc

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"golang.org/x/oauth2"

	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/domain"
	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/usecase"
)

const (
	tokenBytes      = 32 // 256 bits for state, nonce, PKCE verifier and session cookie
	maxReturnTo     = 2048
	maxCookieLength = 512
)

// PrincipalResolver is the use case that turns validated claims into a
// Principal (usecase.ResolvePrincipal).
type PrincipalResolver interface {
	Execute(ctx context.Context, in usecase.ResolvePrincipalInput) (domain.Principal, error)
}

// IdentityFinder links a session to the subject of its identity.
type IdentityFinder interface {
	FindIdentityByUser(ctx context.Context, userID domain.UserID) (domain.Identity, error)
}

// UserSessionRevoker is optional: it lets a back-channel logout token that
// carries only "sub" (no "sid") end every session of that user.
type UserSessionRevoker interface {
	FindUserByIdentity(ctx context.Context, issuer, subject string) (domain.User, error)
	RevokeUserSessions(ctx context.Context, userID domain.UserID, at time.Time) error
}

// Deps are the collaborators of Service, wired in the composition root.
type Deps struct {
	Sessions     usecase.SessionStore
	Resolver     PrincipalResolver
	Identities   IdentityFinder
	UserSessions UserSessionRevoker // optional
	Clock        usecase.Clock
	IDs          usecase.IDGenerator // optional; defaults to random UUIDs
	Logger       *slog.Logger
	HTTPClient   *http.Client // optional (tests inject it)
}

// Service runs the login, session and logout flows against Keycloak.
type Service struct {
	cfg        Config
	prov       *provider
	keys       *Keyring
	sessions   usecase.SessionStore
	resolver   PrincipalResolver
	identities IdentityFinder
	userRevoke UserSessionRevoker
	clock      usecase.Clock
	ids        usecase.IDGenerator
	log        *slog.Logger
	locks      *keyedMutex
}

// NewService validates the configuration and builds the service. It does not
// contact Keycloak; discovery happens on first use.
func NewService(cfg Config, deps Deps) (*Service, error) {
	normalized, err := cfg.Normalize()
	if err != nil {
		return nil, fmt.Errorf("oidc config: %w", err)
	}
	keys, err := NewKeyring(normalized.EncryptionKeyID, normalized.EncryptionKey, normalized.OldKeys)
	if err != nil {
		return nil, fmt.Errorf("oidc keyring: %w", err)
	}
	if deps.Sessions == nil || deps.Resolver == nil || deps.Identities == nil || deps.Clock == nil {
		return nil, errors.New("oidc service needs sessions, resolver, identities and clock")
	}
	logger := deps.Logger
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	ids := deps.IDs
	if ids == nil {
		ids = randomUUIDs{}
	}
	return &Service{
		cfg: normalized, prov: newProvider(normalized, deps.HTTPClient, deps.Clock.Now), keys: keys,
		sessions: deps.Sessions, resolver: deps.Resolver, identities: deps.Identities, userRevoke: deps.UserSessions,
		clock: deps.Clock, ids: ids, log: logger, locks: newKeyedMutex(),
	}, nil
}

// Config returns the effective configuration (cookie names, TTLs, secure flag).
func (s *Service) Config() Config { return s.cfg }

// ---- login ------------------------------------------------------------------

// flowState is what the short lived flow cookie remembers between
// GET /auth/login and GET /auth/callback. The cookie is encrypted and
// authenticated (AES-GCM), so none of it can be read or altered by the browser.
type flowState struct {
	State    string `json:"s"`
	Nonce    string `json:"n"`
	Verifier string `json:"v"`
	ReturnTo string `json:"r"`
	Expires  int64  `json:"e"`
}

// StartLogin prepares the authorization request. It returns the Keycloak URL
// to redirect to and the value of the flow cookie. forceLogin adds
// prompt=login (used right after a product logout, ADR 0015, D6).
func (s *Service) StartLogin(ctx context.Context, returnTo string, forceLogin bool) (authorizationURL, flowCookie string, err error) {
	target, err := ValidateReturnTo(returnTo)
	if err != nil {
		return "", "", err
	}
	state, err := randomToken(tokenBytes)
	if err != nil {
		return "", "", err
	}
	nonce, err := randomToken(tokenBytes)
	if err != nil {
		return "", "", err
	}
	verifier := oauth2.GenerateVerifier()

	authorizationURL, err = s.prov.authorizationURL(ctx, state, nonce, verifier, forceLogin)
	if err != nil {
		return "", "", err
	}
	payload, err := json.Marshal(flowState{
		State: state, Nonce: nonce, Verifier: verifier, ReturnTo: target,
		Expires: s.clock.Now().Add(s.cfg.FlowTTL).Unix(),
	})
	if err != nil {
		return "", "", fmt.Errorf("encode flow state: %w", err)
	}
	flowCookie, err = s.keys.sealFlow(payload)
	if err != nil {
		return "", "", err
	}
	return authorizationURL, flowCookie, nil
}

// FinishLogin completes the callback: it checks the state against the flow
// cookie, exchanges the code at Keycloak, verifies the ID token (nonce) and the
// access token against the realm keys, resolves the user and creates the
// session. It returns the opaque session cookie value and the validated
// return_to.
//
// Errors: usecase.ErrLoginRejected (bad state, expired flow, refused code,
// invalid token), usecase.ErrIdentityProviderUnavailable, and whatever
// ResolvePrincipal returns (user not provisioned, no access, ambiguous role,
// inactive). No session exists after an error.
func (s *Service) FinishLogin(ctx context.Context, code, state, flowCookie string) (sessionToken, returnTo string, err error) {
	flow, err := s.openFlow(flowCookie)
	if err != nil {
		return "", "", err
	}
	if code == "" || state == "" || !sameSecret(state, flow.State) {
		return "", "", fmt.Errorf("%w: state mismatch", usecase.ErrLoginRejected)
	}
	target, err := ValidateReturnTo(flow.ReturnTo)
	if err != nil {
		return "", "", fmt.Errorf("%w: flow return_to", usecase.ErrLoginRejected)
	}

	issued, err := s.prov.exchange(ctx, code, flow.Verifier)
	if errors.Is(err, errGrantRejected) {
		return "", "", fmt.Errorf("%w: code refused", usecase.ErrLoginRejected)
	}
	if err != nil {
		return "", "", err
	}
	if issued.IDToken == "" || issued.AccessToken == "" || issued.RefreshToken == "" {
		return "", "", providerFailure("token response", errors.New("missing token"))
	}

	idToken, err := s.prov.verifyIDToken(ctx, issued.IDToken)
	if err != nil {
		return "", "", s.rejection(err)
	}
	if flow.Nonce == "" || !sameSecret(idToken.Nonce, flow.Nonce) {
		return "", "", fmt.Errorf("%w: nonce mismatch", usecase.ErrLoginRejected)
	}
	access, err := s.prov.verifyAccessToken(ctx, issued.AccessToken)
	if err != nil {
		return "", "", s.rejection(err)
	}
	if access.Subject != idToken.Subject {
		return "", "", fmt.Errorf("%w: subject mismatch", usecase.ErrLoginRejected)
	}

	principal, err := s.resolver.Execute(ctx, usecase.ResolvePrincipalInput{Claims: usecase.Claims{
		Issuer: s.cfg.IssuerURL, Subject: access.Subject, Email: idToken.Email,
		EmailVerified: idToken.EmailVerified, ClientRoles: access.ClientRoles, SyncRole: true,
	}})
	if err != nil {
		return "", "", fmt.Errorf("resolve principal: %w", err)
	}

	token, err := s.createSession(ctx, principal, access, firstNonEmpty(access.SessionID, idToken.SessionID), issued.RefreshToken)
	if err != nil {
		return "", "", err
	}
	return token, target, nil
}

func (s *Service) createSession(ctx context.Context, principal domain.Principal, access verifiedToken, idpSessionID, refreshToken string) (string, error) {
	token, err := randomToken(tokenBytes)
	if err != nil {
		return "", err
	}
	id := s.ids.NewID()
	sealed, keyID, err := s.keys.Seal([]byte(refreshToken), []byte(id))
	if err != nil {
		return "", err
	}
	now := s.clock.Now()
	record := usecase.SessionRecord{
		ID: id, UserID: principal.UserID, Issuer: s.cfg.IssuerURL, IdPSessionID: idpSessionID,
		GrantedRoles: access.ClientRoles, AccessExpiresAt: access.Expiry.Add(-s.cfg.RenewMargin),
		ExpiresAt: now.Add(s.cfg.SessionTTL), RefreshTokenEnc: sealed, EncKeyID: keyID, CreatedAt: now,
	}
	if err := s.sessions.CreateSession(ctx, record, hashToken(token)); err != nil {
		return "", fmt.Errorf("create session: %w", err)
	}
	return token, nil
}

func (s *Service) openFlow(cookie string) (flowState, error) {
	if cookie == "" || len(cookie) > 4096 {
		return flowState{}, fmt.Errorf("%w: missing flow cookie", usecase.ErrLoginRejected)
	}
	plain, err := s.keys.openFlow(cookie)
	if err != nil {
		return flowState{}, fmt.Errorf("%w: unreadable flow cookie", usecase.ErrLoginRejected)
	}
	var flow flowState
	if err := json.Unmarshal(plain, &flow); err != nil {
		return flowState{}, fmt.Errorf("%w: malformed flow cookie", usecase.ErrLoginRejected)
	}
	if s.clock.Now().Unix() > flow.Expires {
		return flowState{}, fmt.Errorf("%w: login took too long", usecase.ErrLoginRejected)
	}
	return flow, nil
}

// rejection turns a verification error into a login rejection, keeping
// provider failures as they are.
func (s *Service) rejection(err error) error {
	if errors.Is(err, errTokenRejected) {
		return fmt.Errorf("%w: token verification failed", usecase.ErrLoginRejected)
	}
	return err
}

// ---- per request authentication ---------------------------------------------

// Authenticator authenticates requests by session cookie or Bearer token. It
// implements httpadapter.Authenticator.
type Authenticator struct{ service *Service }

// NewAuthenticator exposes the authentication step of the service.
func NewAuthenticator(service *Service) *Authenticator { return &Authenticator{service: service} }

// Authenticate returns the Principal of the request. A Bearer token wins over
// a cookie (browsers never send one by themselves). Failures wrap
// usecase.ErrUnauthenticated for a missing or invalid credential.
func (a *Authenticator) Authenticate(r *http.Request) (domain.Principal, error) {
	s := a.service
	if raw, ok := bearerToken(r); ok {
		return s.principalFromBearer(r.Context(), raw)
	}
	cookie, err := r.Cookie(s.cfg.SessionCookieName)
	if err != nil || cookie.Value == "" || len(cookie.Value) > maxCookieLength {
		return domain.Principal{}, usecase.ErrUnauthenticated
	}
	return s.principalFromSession(r.Context(), cookie.Value)
}

func bearerToken(r *http.Request) (string, bool) {
	scheme, token, found := strings.Cut(strings.TrimSpace(r.Header.Get("Authorization")), " ")
	if !found || !strings.EqualFold(scheme, "Bearer") {
		return "", false
	}
	token = strings.TrimSpace(token)
	return token, token != ""
}

func (s *Service) principalFromBearer(ctx context.Context, raw string) (domain.Principal, error) {
	access, err := s.prov.verifyAccessToken(ctx, raw)
	if errors.Is(err, errTokenRejected) {
		return domain.Principal{}, fmt.Errorf("%w: invalid bearer token", usecase.ErrUnauthenticated)
	}
	if err != nil {
		return domain.Principal{}, err
	}
	principal, err := s.resolver.Execute(ctx, usecase.ResolvePrincipalInput{Claims: usecase.Claims{
		Issuer: s.cfg.IssuerURL, Subject: access.Subject, Email: access.Email,
		EmailVerified: access.EmailVerified, ClientRoles: access.ClientRoles,
	}})
	if err != nil {
		return domain.Principal{}, fmt.Errorf("resolve principal: %w", err)
	}
	return principal, nil
}

func (s *Service) principalFromSession(ctx context.Context, cookieValue string) (domain.Principal, error) {
	record, err := s.sessions.FindSessionByTokenHash(ctx, hashToken(cookieValue))
	if errors.Is(err, usecase.ErrNotFound) {
		return domain.Principal{}, usecase.ErrUnauthenticated
	}
	if err != nil {
		return domain.Principal{}, fmt.Errorf("find session: %w", err)
	}
	identity, err := s.identities.FindIdentityByUser(ctx, record.UserID)
	if errors.Is(err, usecase.ErrNotFound) || (err == nil && identity.Issuer != record.Issuer) {
		s.revoke(ctx, record.ID)
		return domain.Principal{}, fmt.Errorf("%w: session without a matching identity", usecase.ErrUnauthenticated)
	}
	if err != nil {
		return domain.Principal{}, fmt.Errorf("find identity: %w", err)
	}

	now := s.clock.Now()
	renewed := false
	if !now.Before(record.AccessExpiresAt) {
		renewed = true
		record, err = s.renewLocked(ctx, hashToken(cookieValue), record, identity.Subject)
		if err != nil {
			return domain.Principal{}, err
		}
	}

	principal, err := s.resolver.Execute(ctx, usecase.ResolvePrincipalInput{Claims: usecase.Claims{
		Issuer: record.Issuer, Subject: identity.Subject, ClientRoles: record.GrantedRoles,
		// Roles are fresh only right after a refresh; between refreshes they are the stored ones.
		SyncRole: renewed,
	}})
	if err != nil {
		return domain.Principal{}, fmt.Errorf("resolve principal: %w", err)
	}
	if principal.UserID != record.UserID {
		s.revoke(ctx, record.ID)
		return domain.Principal{}, fmt.Errorf("%w: identity changed owner", usecase.ErrUnauthenticated)
	}
	if err := s.sessions.TouchSession(ctx, record.ID, now); err != nil {
		s.log.WarnContext(ctx, "touch session failed", "error", err)
	}
	return principal, nil
}

// renewLocked renews the tokens of a session. Renewals of one session are
// serialized: with refresh token rotation a second concurrent use of the same
// refresh token would be refused by Keycloak and kill the session. The lock is
// per process; with several API replicas the realm should keep refresh token
// reuse tolerant (revokeRefreshToken is off in the realm, see ADR 0015).
func (s *Service) renewLocked(ctx context.Context, tokenHash [32]byte, stale usecase.SessionRecord, subject string) (usecase.SessionRecord, error) {
	unlock := s.locks.lock(stale.ID)
	defer unlock()

	// Another request may have renewed while this one waited for the lock.
	current, err := s.sessions.FindSessionByTokenHash(ctx, tokenHash)
	if errors.Is(err, usecase.ErrNotFound) {
		return usecase.SessionRecord{}, usecase.ErrUnauthenticated
	}
	if err != nil {
		return usecase.SessionRecord{}, fmt.Errorf("find session: %w", err)
	}
	if s.clock.Now().Before(current.AccessExpiresAt) {
		return current, nil
	}
	return s.renew(ctx, current, subject)
}

// renew asks Keycloak for new tokens and stores the roles and expiry of the
// new access token. A refused refresh token, a token that no longer verifies or
// a role that disappeared end the session (401). A provider outage does not:
// the session stays and the request fails with 502, so a Keycloak restart does
// not log everybody out.
func (s *Service) renew(ctx context.Context, record usecase.SessionRecord, subject string) (usecase.SessionRecord, error) {
	endSession := func(reason string) (usecase.SessionRecord, error) {
		s.revoke(ctx, record.ID)
		return usecase.SessionRecord{}, fmt.Errorf("%w: %s", usecase.ErrUnauthenticated, reason)
	}
	if len(record.RefreshTokenEnc) == 0 {
		return endSession("session has no refresh token")
	}
	plain, err := s.keys.Open(record.RefreshTokenEnc, record.EncKeyID, []byte(record.ID))
	if err != nil {
		s.log.ErrorContext(ctx, "refresh token does not open; check API_SESSION_ENC_KEY(_OLD_KEYS)", "session_id", record.ID)
		return endSession("refresh token unreadable")
	}

	issued, err := s.prov.refresh(ctx, string(plain))
	if errors.Is(err, errGrantRejected) {
		return endSession("refresh token refused")
	}
	if err != nil {
		return usecase.SessionRecord{}, err
	}
	access, err := s.prov.verifyAccessToken(ctx, issued.AccessToken)
	if errors.Is(err, errTokenRejected) {
		return endSession("renewed access token rejected")
	}
	if err != nil {
		return usecase.SessionRecord{}, err
	}
	if access.Subject != subject {
		return endSession("renewed token belongs to another subject")
	}
	if _, err := domain.ReadAccess(access.ClientRoles); err != nil {
		return endSession("product role no longer granted")
	}

	// Always reseal: it also moves old sessions to the current key (rotation).
	next := string(plain)
	if issued.RefreshToken != "" {
		next = issued.RefreshToken
	}
	sealed, keyID, err := s.keys.Seal([]byte(next), []byte(record.ID))
	if err != nil {
		return usecase.SessionRecord{}, err
	}
	accessExpiry := access.Expiry.Add(-s.cfg.RenewMargin)
	err = s.sessions.RenewSession(ctx, record.ID, access.ClientRoles, accessExpiry, sealed, keyID)
	if errors.Is(err, usecase.ErrNotFound) {
		return usecase.SessionRecord{}, usecase.ErrUnauthenticated
	}
	if err != nil {
		return usecase.SessionRecord{}, fmt.Errorf("renew session: %w", err)
	}
	record.GrantedRoles, record.AccessExpiresAt, record.RefreshTokenEnc, record.EncKeyID = access.ClientRoles, accessExpiry, sealed, keyID
	return record, nil
}

// ---- logout -----------------------------------------------------------------

// Logout ends the product session: local revocation first (the part that
// protects the user), then revocation of the refresh token at Keycloak. The SSO
// session stays open on purpose. It is idempotent: an unknown or already ended
// session is not an error. A Keycloak failure is logged, not returned, because
// the browser session is already gone and the refresh token cannot be used
// without the encrypted copy that was just revoked.
func (s *Service) Logout(ctx context.Context, cookieValue string) error {
	if cookieValue == "" || len(cookieValue) > maxCookieLength {
		return nil
	}
	record, err := s.sessions.FindSessionByTokenHash(ctx, hashToken(cookieValue))
	if errors.Is(err, usecase.ErrNotFound) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("find session: %w", err)
	}
	if err := s.sessions.RevokeSession(ctx, record.ID, s.clock.Now()); err != nil {
		return fmt.Errorf("revoke session: %w", err)
	}
	if len(record.RefreshTokenEnc) == 0 {
		return nil
	}
	plain, err := s.keys.Open(record.RefreshTokenEnc, record.EncKeyID, []byte(record.ID))
	if err != nil {
		s.log.WarnContext(ctx, "logout: refresh token unreadable, not revoked at the provider", "session_id", record.ID)
		return nil
	}
	if err := s.prov.revokeRefreshToken(ctx, string(plain)); err != nil {
		s.log.WarnContext(ctx, "logout: refresh token not revoked at the provider", "session_id", record.ID, "error", err)
	}
	return nil
}

// BackchannelLogout handles the logout_token sent by Keycloak when the SSO
// session ends. It returns usecase.ErrInvalidLogoutToken for a token that
// does not verify. Revocation is idempotent: a session id unknown to this
// product is success.
func (s *Service) BackchannelLogout(ctx context.Context, logoutToken string) error {
	if logoutToken == "" {
		return fmt.Errorf("%w: missing token", usecase.ErrInvalidLogoutToken)
	}
	verified, err := s.prov.verifyLogoutToken(ctx, logoutToken)
	if errors.Is(err, errTokenRejected) {
		return fmt.Errorf("%w: verification failed", usecase.ErrInvalidLogoutToken)
	}
	if err != nil {
		return err
	}
	now := s.clock.Now()
	if verified.SessionID != "" {
		if err := s.sessions.RevokeSessionsByIdPSession(ctx, s.cfg.IssuerURL, verified.SessionID, now); err != nil {
			return fmt.Errorf("revoke sessions by idp session: %w", err)
		}
		return nil
	}
	if s.userRevoke == nil {
		return fmt.Errorf("%w: token without sid", usecase.ErrInvalidLogoutToken)
	}
	user, err := s.userRevoke.FindUserByIdentity(ctx, s.cfg.IssuerURL, verified.Subject)
	if errors.Is(err, usecase.ErrNotFound) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("find user by identity: %w", err)
	}
	if err := s.userRevoke.RevokeUserSessions(ctx, user.ID, now); err != nil {
		return fmt.Errorf("revoke user sessions: %w", err)
	}
	return nil
}

// revoke ends a session locally, logging (not returning) a storage failure:
// the caller is already answering 401.
func (s *Service) revoke(ctx context.Context, id string) {
	if err := s.sessions.RevokeSession(ctx, id, s.clock.Now()); err != nil {
		s.log.ErrorContext(ctx, "revoke session failed", "error", err)
	}
}

func hashToken(token string) [32]byte { return sha256.Sum256([]byte(token)) }

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

// ---- helpers ----------------------------------------------------------------

// keyedMutex serializes work per key without keeping locks of finished work.
type keyedMutex struct {
	mu      sync.Mutex
	entries map[string]*keyedEntry
}

type keyedEntry struct {
	mu   sync.Mutex
	refs int
}

func newKeyedMutex() *keyedMutex { return &keyedMutex{entries: map[string]*keyedEntry{}} }

func (k *keyedMutex) lock(key string) func() {
	k.mu.Lock()
	entry, ok := k.entries[key]
	if !ok {
		entry = &keyedEntry{}
		k.entries[key] = entry
	}
	entry.refs++
	k.mu.Unlock()

	entry.mu.Lock()
	return func() {
		entry.mu.Unlock()
		k.mu.Lock()
		entry.refs--
		if entry.refs == 0 {
			delete(k.entries, key)
		}
		k.mu.Unlock()
	}
}

// randomUUIDs generates version 4 UUIDs (the session id column is a uuid).
type randomUUIDs struct{}

func (randomUUIDs) NewID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic("oidc: crypto/rand unavailable: " + err.Error()) // no safe way to continue without entropy
	}
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	h := hex.EncodeToString(b[:])
	return h[0:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:]
}

// ValidateReturnTo accepts only a path relative to this application, against
// open redirects: it must start with a single "/", and carry no backslash,
// control character, scheme or host. Empty means "/".
func ValidateReturnTo(raw string) (string, error) {
	if raw == "" {
		return "/", nil
	}
	invalid := func() error {
		return fmt.Errorf("%w: only a relative path is accepted", usecase.ErrInvalidReturnTo)
	}
	if len(raw) > maxReturnTo || raw[0] != '/' || strings.HasPrefix(raw, "//") {
		return "", invalid()
	}
	for _, char := range raw {
		if char == '\\' || char < 0x20 || char == 0x7f {
			return "", invalid()
		}
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme != "" || parsed.Host != "" || parsed.User != nil || strings.HasPrefix(parsed.Path, "//") {
		return "", invalid()
	}
	return raw, nil
}
