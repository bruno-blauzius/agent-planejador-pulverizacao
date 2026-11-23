package oidc

import (
	"context"
	"crypto"
	"crypto/hmac"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/domain"
	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/usecase"
)

const (
	testRealm     = "inceres"
	testClientID  = "agentes-agro"
	testSecret    = "test-client-secret"
	testPublic    = "http://sso.example.test:8180"
	testIssuer    = testPublic + "/realms/" + testRealm
	testRedirect  = "http://api.example.test:8080/auth/callback"
	testSubject   = "11111111-2222-3333-4444-555555555555"
	testUserID    = domain.UserID("user-1")
	realmPrefix   = "/realms/" + testRealm
	protocolPath  = realmPrefix + "/protocol/openid-connect"
	adminRoleName = "admin"
)

var epoch = time.Date(2026, 11, 22, 12, 0, 0, 0, time.UTC)

// ---- clock, ids -----------------------------------------------------------------

type testClock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *testClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *testClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}

type sequentialUUIDs struct {
	mu sync.Mutex
	n  int
}

func (s *sequentialUUIDs) NewID() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.n++
	return fmt.Sprintf("00000000-0000-4000-8000-%012d", s.n)
}

// ---- fake identity provider (Keycloak) ----------------------------------------------

// grant is what a user obtained at the fake authorization endpoint.
type grant struct {
	subject   string
	sid       string
	email     string
	verified  bool
	roles     []string
	challenge string
	nonce     string
}

type fakeIdP struct {
	t   *testing.T
	srv *httptest.Server
	key *rsa.PrivateKey
	now func() time.Time

	mu            sync.Mutex
	codes         map[string]grant
	refreshTokens map[string]grant
	counter       int
	// observable behaviour
	tokenCalls, refreshCalls, revokeCalls, jwksCalls int
	revoked                                          []string
	// knobs
	refreshStatus int      // forced status of refresh grants (0 = normal)
	revokeStatus  int      // forced status of the revocation endpoint
	jwksStatus    int      // forced status of the certs endpoint
	rotate        bool     // issue a new refresh token on each refresh
	refreshRoles  []string // roles of renewed access tokens (nil = keep)
	idTokenNonce  string   // overrides the nonce of ID tokens
	accessMutator func(claims map[string]any)
	idMutator     func(claims map[string]any)
	discoveryIss  string // overrides the issuer announced by discovery
}

func newFakeIdP(t *testing.T, now func() time.Time) *fakeIdP {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	idp := &fakeIdP{t: t, key: key, now: now, codes: map[string]grant{}, refreshTokens: map[string]grant{}}
	mux := http.NewServeMux()
	mux.HandleFunc("GET "+realmPrefix+"/.well-known/openid-configuration", idp.discovery)
	mux.HandleFunc("GET "+protocolPath+"/certs", idp.certs)
	mux.HandleFunc("POST "+protocolPath+"/token", idp.token)
	mux.HandleFunc("POST "+protocolPath+"/revoke", idp.revoke)
	idp.srv = httptest.NewServer(mux)
	t.Cleanup(idp.srv.Close)
	return idp
}

func (f *fakeIdP) discovery(w http.ResponseWriter, _ *http.Request) {
	iss := testIssuer
	if f.discoveryIss != "" {
		iss = f.discoveryIss
	}
	// Like Keycloak with a fixed hostname: every endpoint is announced on the public address.
	writeTestJSON(w, http.StatusOK, map[string]any{
		"issuer":                 iss,
		"authorization_endpoint": testPublic + protocolPath + "/auth",
		"token_endpoint":         testPublic + protocolPath + "/token",
		"jwks_uri":               testPublic + protocolPath + "/certs",
		"revocation_endpoint":    testPublic + protocolPath + "/revoke",
	})
}

func (f *fakeIdP) certs(w http.ResponseWriter, _ *http.Request) {
	f.mu.Lock()
	f.jwksCalls++
	status := f.jwksStatus
	f.mu.Unlock()
	if status != 0 {
		http.Error(w, "down", status)
		return
	}
	writeTestJSON(w, http.StatusOK, map[string]any{"keys": []map[string]string{{
		"kty": "RSA", "alg": "RS256", "use": "sig", "kid": "test-key",
		"n": b64(f.key.N.Bytes()), "e": b64(big.NewInt(int64(f.key.E)).Bytes()),
	}}})
}

func (f *fakeIdP) checkClientAuth(w http.ResponseWriter, r *http.Request) bool {
	user, pass, ok := r.BasicAuth()
	if !ok || user != testClientID || pass != testSecret {
		writeTestJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized_client"})
		return false
	}
	return true
}

func (f *fakeIdP) token(w http.ResponseWriter, r *http.Request) {
	if !f.checkClientAuth(w, r) {
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	switch r.PostForm.Get("grant_type") {
	case "authorization_code":
		f.tokenCalls++
		code := r.PostForm.Get("code")
		granted, ok := f.codes[code]
		delete(f.codes, code) // single use
		verifier := r.PostForm.Get("code_verifier")
		digest := sha256.Sum256([]byte(verifier))
		if !ok || r.PostForm.Get("redirect_uri") != testRedirect || b64(digest[:]) != granted.challenge {
			writeTestJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_grant"})
			return
		}
		f.counter++
		refresh := fmt.Sprintf("refresh-%d", f.counter)
		f.refreshTokens[refresh] = granted
		f.writeTokens(w, granted, refresh, true)
	case "refresh_token":
		f.refreshCalls++
		if f.refreshStatus != 0 {
			writeTestJSON(w, f.refreshStatus, map[string]string{"error": "invalid_grant"})
			return
		}
		old := r.PostForm.Get("refresh_token")
		granted, ok := f.refreshTokens[old]
		if !ok {
			writeTestJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_grant"})
			return
		}
		if f.refreshRoles != nil {
			granted.roles = f.refreshRoles
			f.refreshTokens[old] = granted
		}
		refresh := old
		if f.rotate {
			delete(f.refreshTokens, old)
			f.counter++
			refresh = fmt.Sprintf("refresh-%d", f.counter)
			f.refreshTokens[refresh] = granted
		}
		f.writeTokens(w, granted, refresh, false)
	default:
		writeTestJSON(w, http.StatusBadRequest, map[string]string{"error": "unsupported_grant_type"})
	}
}

func (f *fakeIdP) writeTokens(w http.ResponseWriter, g grant, refresh string, withID bool) {
	body := map[string]any{
		"access_token": f.signAccess(g), "token_type": "Bearer", "expires_in": 300, "refresh_token": refresh,
	}
	if withID {
		body["id_token"] = f.signID(g)
	}
	writeTestJSON(w, http.StatusOK, body)
}

func (f *fakeIdP) revoke(w http.ResponseWriter, r *http.Request) {
	if !f.checkClientAuth(w, r) {
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.revokeCalls++
	if f.revokeStatus != 0 {
		http.Error(w, "down", f.revokeStatus)
		return
	}
	token := r.PostForm.Get("token")
	f.revoked = append(f.revoked, token)
	delete(f.refreshTokens, token)
	w.WriteHeader(http.StatusOK)
}

// authorize plays the browser at Keycloak: it checks the authorization URL and
// returns the code and state the browser would be redirected back with.
func (f *fakeIdP) authorize(authURL string, g grant) (code, state string) {
	f.t.Helper()
	parsed, err := url.Parse(authURL)
	if err != nil {
		f.t.Fatal(err)
	}
	if got := parsed.Scheme + "://" + parsed.Host + parsed.Path; got != testPublic+protocolPath+"/auth" {
		f.t.Fatalf("authorization endpoint = %q, want the public address", got)
	}
	query := parsed.Query()
	for key, want := range map[string]string{
		"client_id": testClientID, "redirect_uri": testRedirect, "response_type": "code", "code_challenge_method": "S256",
	} {
		if query.Get(key) != want {
			f.t.Fatalf("authorization %s = %q, want %q", key, query.Get(key), want)
		}
	}
	g.challenge, g.nonce = query.Get("code_challenge"), query.Get("nonce")
	if g.challenge == "" || g.nonce == "" || query.Get("state") == "" {
		f.t.Fatal("authorization URL lacks challenge, nonce or state")
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.counter++
	code = fmt.Sprintf("code-%d", f.counter)
	f.codes[code] = g
	return code, query.Get("state")
}

func (f *fakeIdP) counts() (token, refresh, revoke int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.tokenCalls, f.refreshCalls, f.revokeCalls
}

// ---- token minting ---------------------------------------------------------------

func (f *fakeIdP) accessClaims(g grant) map[string]any {
	now := f.now()
	return map[string]any{
		"iss": testIssuer, "sub": g.subject, "aud": []string{testClientID, "account"}, "azp": testClientID,
		"typ": "Bearer", "iat": now.Unix(), "exp": now.Add(5 * time.Minute).Unix(), "sid": g.sid,
		"email": g.email, "email_verified": g.verified,
		"resource_access": map[string]any{testClientID: map[string]any{"roles": g.roles}},
	}
}

func (f *fakeIdP) idClaims(g grant) map[string]any {
	now := f.now()
	nonce := g.nonce
	if f.idTokenNonce != "" { // knobs are set by the test before any request
		nonce = f.idTokenNonce
	}
	return map[string]any{
		"iss": testIssuer, "sub": g.subject, "aud": testClientID, "azp": testClientID, "typ": "ID",
		"iat": now.Unix(), "exp": now.Add(5 * time.Minute).Unix(), "sid": g.sid, "nonce": nonce,
		"email": g.email, "email_verified": g.verified,
	}
}

func (f *fakeIdP) signAccess(g grant) string {
	claims := f.accessClaims(g)
	if f.accessMutator != nil {
		f.accessMutator(claims)
	}
	return f.sign(claims)
}

func (f *fakeIdP) signID(g grant) string {
	claims := f.idClaims(g)
	if f.idMutator != nil {
		f.idMutator(claims)
	}
	return f.sign(claims)
}

func (f *fakeIdP) sign(claims map[string]any) string {
	return signJWT(f.t, f.key, "RS256", claims)
}

// signJWT builds a compact JWT. alg "RS256" signs with key; "none" leaves the
// signature empty; "HS256" signs with the (public) modulus bytes as secret,
// the classic algorithm confusion attack.
func signJWT(t *testing.T, key *rsa.PrivateKey, alg string, claims map[string]any) string {
	t.Helper()
	header, _ := json.Marshal(map[string]string{"alg": alg, "typ": "JWT", "kid": "test-key"})
	payload, err := json.Marshal(claims)
	if err != nil {
		t.Fatal(err)
	}
	signingInput := b64(header) + "." + b64(payload)
	switch alg {
	case "RS256":
		digest := sha256.Sum256([]byte(signingInput))
		signature, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, digest[:])
		if err != nil {
			t.Fatal(err)
		}
		return signingInput + "." + b64(signature)
	case "HS256":
		return signingInput + "." + b64(hmacSHA256(key.N.Bytes(), signingInput))
	default:
		return signingInput + "."
	}
}

func b64(data []byte) string { return base64.RawURLEncoding.EncodeToString(data) }

func writeTestJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

// ---- product side fakes -------------------------------------------------------------

type fakeSessions struct {
	mu       sync.Mutex
	clock    *testClock
	byID     map[string]*usecase.SessionRecord
	byHash   map[[32]byte]string
	renewals int
	touches  int
	failFind error
}

func newFakeSessions(clock *testClock) *fakeSessions {
	return &fakeSessions{clock: clock, byID: map[string]*usecase.SessionRecord{}, byHash: map[[32]byte]string{}}
}

func (s *fakeSessions) CreateSession(_ context.Context, record usecase.SessionRecord, hash [32]byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	copied := record
	s.byID[record.ID], s.byHash[hash] = &copied, record.ID
	return nil
}

func (s *fakeSessions) FindSessionByTokenHash(_ context.Context, hash [32]byte) (usecase.SessionRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.failFind != nil {
		return usecase.SessionRecord{}, s.failFind
	}
	id, ok := s.byHash[hash]
	if !ok {
		return usecase.SessionRecord{}, usecase.ErrNotFound
	}
	record := *s.byID[id]
	if record.RevokedAt != nil || !s.clock.Now().Before(record.ExpiresAt) {
		return usecase.SessionRecord{}, usecase.ErrNotFound
	}
	return record, nil
}

func (s *fakeSessions) RenewSession(_ context.Context, id string, roles []string, accessExpiresAt time.Time, refreshEnc []byte, keyID int16) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	record, ok := s.byID[id]
	if !ok || record.RevokedAt != nil {
		return usecase.ErrNotFound
	}
	s.renewals++
	record.GrantedRoles, record.AccessExpiresAt = roles, accessExpiresAt
	if len(refreshEnc) > 0 {
		record.RefreshTokenEnc, record.EncKeyID = refreshEnc, keyID
	}
	return nil
}

func (s *fakeSessions) TouchSession(context.Context, string, time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.touches++
	return nil
}

func (s *fakeSessions) RevokeSession(_ context.Context, id string, at time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if record, ok := s.byID[id]; ok && record.RevokedAt == nil {
		record.RevokedAt = &at
	}
	return nil
}

func (s *fakeSessions) RevokeSessionsByIdPSession(_ context.Context, issuer, sid string, at time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, record := range s.byID {
		if record.Issuer == issuer && record.IdPSessionID == sid && record.RevokedAt == nil {
			revokedAt := at
			record.RevokedAt = &revokedAt
		}
	}
	return nil
}

func (s *fakeSessions) only() usecase.SessionRecord {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.byID) != 1 {
		panic(fmt.Sprintf("expected exactly one session, have %d", len(s.byID)))
	}
	for _, record := range s.byID {
		return *record
	}
	return usecase.SessionRecord{}
}

func (s *fakeSessions) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.byID)
}

func (s *fakeSessions) revokedCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	for _, record := range s.byID {
		if record.RevokedAt != nil {
			n++
		}
	}
	return n
}

// fakeResolver mirrors ResolvePrincipal: role from the claim roles, user by subject.
type fakeResolver struct {
	mu    sync.Mutex
	users map[string]domain.UserID // subject -> user
	err   error
	calls []usecase.Claims
}

func (r *fakeResolver) Execute(_ context.Context, in usecase.ResolvePrincipalInput) (domain.Principal, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, in.Claims)
	if r.err != nil {
		return domain.Principal{}, r.err
	}
	access, err := domain.ReadAccess(in.Claims.ClientRoles)
	if err != nil {
		return domain.Principal{}, err
	}
	userID, ok := r.users[in.Claims.Subject]
	if !ok {
		return domain.Principal{}, usecase.ErrUserNotProvisioned
	}
	return domain.Principal{UserID: userID, CompanyID: "co-1", Role: access.Role, Permissions: access.Permissions}, nil
}

func (r *fakeResolver) lastClaims() usecase.Claims {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.calls[len(r.calls)-1]
}

type fakeIdentities struct {
	byUser map[domain.UserID]domain.Identity
}

func (f fakeIdentities) FindIdentityByUser(_ context.Context, id domain.UserID) (domain.Identity, error) {
	identity, ok := f.byUser[id]
	if !ok {
		return domain.Identity{}, usecase.ErrNotFound
	}
	return identity, nil
}

// ---- harness --------------------------------------------------------------------

type harness struct {
	t        *testing.T
	clock    *testClock
	idp      *fakeIdP
	sessions *fakeSessions
	resolver *fakeResolver
	service  *Service
	auth     *Authenticator
	cfg      Config
}

func testKey(b byte) []byte { return []byte(strings.Repeat(string([]byte{b}), 32)) }

func testConfig(internalURL string) Config {
	return Config{
		IssuerURL: testIssuer, InternalURL: internalURL, Realm: testRealm, ClientID: testClientID, ClientSecret: testSecret,
		RedirectURL: testRedirect, EncryptionKey: testKey('k'), EncryptionKeyID: 1,
	}
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	clock := &testClock{t: epoch}
	idp := newFakeIdP(t, clock.Now)
	h := &harness{t: t, clock: clock, idp: idp, sessions: newFakeSessions(clock), cfg: testConfig(idp.srv.URL)}
	h.resolver = &fakeResolver{users: map[string]domain.UserID{testSubject: testUserID}}
	h.build(h.cfg)
	return h
}

func (h *harness) build(cfg Config) {
	h.t.Helper()
	service, err := NewService(cfg, Deps{
		Sessions: h.sessions, Resolver: h.resolver,
		Identities: fakeIdentities{byUser: map[domain.UserID]domain.Identity{testUserID: {UserID: testUserID, Issuer: testIssuer, Subject: testSubject}}},
		Clock:      h.clock, IDs: &sequentialUUIDs{},
	})
	if err != nil {
		h.t.Fatal(err)
	}
	h.service, h.auth, h.cfg = service, NewAuthenticator(service), service.Config()
}

func adminGrant() grant {
	return grant{subject: testSubject, sid: "sid-1", email: "ana@agro.test", verified: true, roles: []string{adminRoleName, "users.manage", "dashboard.view"}}
}

// login runs the whole login against the fake IdP and returns the session cookie value.
func (h *harness) login(g grant) string {
	h.t.Helper()
	authURL, flow, err := h.service.StartLogin(context.Background(), "/farms", false)
	if err != nil {
		h.t.Fatalf("StartLogin: %v", err)
	}
	code, state := h.idp.authorize(authURL, g)
	token, returnTo, err := h.service.FinishLogin(context.Background(), code, state, flow)
	if err != nil {
		h.t.Fatalf("FinishLogin: %v", err)
	}
	if returnTo != "/farms" {
		h.t.Fatalf("returnTo = %q", returnTo)
	}
	return token
}

func (h *harness) request(cookie string) *http.Request {
	r := httptest.NewRequestWithContext(background(), http.MethodGet, "/auth/me", http.NoBody)
	if cookie != "" {
		r.AddCookie(&http.Cookie{Name: h.cfg.SessionCookieName, Value: cookie})
	}
	return r
}

func (h *harness) bearer(token string) *http.Request {
	r := httptest.NewRequestWithContext(background(), http.MethodGet, "/auth/me", http.NoBody)
	r.Header.Set("Authorization", "Bearer "+token)
	return r
}

func hmacSHA256(key []byte, message string) []byte {
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(message))
	return mac.Sum(nil)
}
