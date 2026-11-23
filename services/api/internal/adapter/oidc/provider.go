package oidc

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	gooidc "github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"

	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/usecase"
)

const (
	// maxResponseBytes caps what is read from Keycloak, so a misbehaving
	// server cannot exhaust memory.
	maxResponseBytes = 1 << 20

	logoutEvent = "http://schemas.openid.net/event/backchannel-logout"

	tokenTypeAccess = "Bearer"
	tokenTypeID     = "ID"
)

// allowedAlgorithms is the fixed list of asymmetric algorithms accepted for
// every token. "none" and HMAC families are never accepted: the realm signs
// with RS256 (infra/keycloak/agentes-agro.json) and anything else is an attack
// or a misconfiguration worth failing loudly.
var allowedAlgorithms = []string{"RS256"}

// errTokenRejected means a token failed verification (signature, issuer,
// audience, authorized party, type, expiry, nonce). The cause is never shown.
var errTokenRejected = errors.New("token rejected")

// errGrantRejected means Keycloak refused the code or refresh token
// (invalid_grant): the credential is dead, not the provider.
var errGrantRejected = errors.New("grant rejected by the identity provider")

// verifiedToken carries the claims of a token that passed verification.
type verifiedToken struct {
	Subject       string
	SessionID     string
	Email         string
	EmailVerified bool
	Nonce         string
	ClientRoles   []string
	Expiry        time.Time
}

// tokenClaims is the subset of claims read after the library verified the
// signature, issuer, audience and expiry.
type tokenClaims struct {
	Type            string                              `json:"typ"`
	AuthorizedParty string                              `json:"azp"`
	SessionID       string                              `json:"sid"`
	Email           string                              `json:"email"`
	EmailVerified   bool                                `json:"email_verified"`
	NotBefore       *int64                              `json:"nbf"`
	IssuedAt        *int64                              `json:"iat"`
	ResourceAccess  map[string]struct{ Roles []string } `json:"resource_access"`
	Events          map[string]json.RawMessage          `json:"events"`
	Nonce           string                              `json:"nonce"`
}

// endpoints are the addresses learned from discovery.
type endpoints struct {
	oauth      *oauth2.Config
	revokeURL  string
	idVerifier *gooidc.IDTokenVerifier
	// accessVerifier and logoutVerifier share the key set of idVerifier.
	accessVerifier *gooidc.IDTokenVerifier
	logoutVerifier *gooidc.IDTokenVerifier
}

// provider talks to Keycloak. Discovery is lazy: the API starts (and answers
// probes) even when Keycloak is down, and the next request retries.
type provider struct {
	cfg  Config
	http *http.Client
	now  func() time.Time

	mu    sync.Mutex
	ready *endpoints
}

func newProvider(cfg Config, client *http.Client, now func() time.Time) *provider {
	if client == nil {
		client = &http.Client{
			// Never follow redirects: client credentials must not travel to another host.
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		}
	}
	copied := *client
	copied.Timeout = cfg.RequestTimeout
	return &provider{cfg: cfg, http: &copied, now: now}
}

// internalBase is the scheme://host that server-to-server calls use.
func (p *provider) internalBase() string {
	if p.cfg.InternalURL != "" {
		return p.cfg.InternalURL
	}
	return strings.TrimSuffix(p.cfg.IssuerURL, "/realms/"+p.cfg.Realm)
}

func (p *provider) publicBase() string {
	return strings.TrimSuffix(p.cfg.IssuerURL, "/realms/"+p.cfg.Realm)
}

// endpoints performs discovery once and caches the result; failures are not cached.
func (p *provider) endpoints(ctx context.Context) (*endpoints, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.ready != nil {
		return p.ready, nil
	}
	found, err := p.discover(ctx)
	if err != nil {
		return nil, err
	}
	p.ready = found
	return found, nil
}

type discoveryDocument struct {
	Issuer             string `json:"issuer"`
	AuthorizationURL   string `json:"authorization_endpoint"`
	TokenURL           string `json:"token_endpoint"`
	JWKSURL            string `json:"jwks_uri"`
	RevocationEndpoint string `json:"revocation_endpoint"`
}

func (p *provider) discover(ctx context.Context) (*endpoints, error) {
	discoveryURL := p.internalBase() + "/realms/" + url.PathEscape(p.cfg.Realm) + "/.well-known/openid-configuration"
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, discoveryURL, http.NoBody)
	if err != nil {
		return nil, providerFailure("build discovery request", err)
	}
	body, err := p.do(request)
	if err != nil {
		return nil, providerFailure("discovery", err)
	}
	var doc discoveryDocument
	if err := json.Unmarshal(body, &doc); err != nil {
		return nil, providerFailure("decode discovery", errors.New("malformed document"))
	}
	// The issuer check is the anchor of trust: whatever address answered, the
	// realm must announce the public issuer the tokens will carry.
	if doc.Issuer != p.cfg.IssuerURL {
		return nil, providerFailure("discovery", fmt.Errorf("issuer %q does not match the configured %q", doc.Issuer, p.cfg.IssuerURL))
	}
	tokenURL, err := p.reAddress(doc.TokenURL, p.internalBase())
	if err != nil {
		return nil, providerFailure("token endpoint", err)
	}
	jwksURL, err := p.reAddress(doc.JWKSURL, p.internalBase())
	if err != nil {
		return nil, providerFailure("jwks endpoint", err)
	}
	authURL, err := p.reAddress(doc.AuthorizationURL, p.publicBase())
	if err != nil {
		return nil, providerFailure("authorization endpoint", err)
	}
	revokeURL := ""
	if doc.RevocationEndpoint != "" {
		if revokeURL, err = p.reAddress(doc.RevocationEndpoint, p.internalBase()); err != nil {
			return nil, providerFailure("revocation endpoint", err)
		}
	}

	keySet := gooidc.NewRemoteKeySet(gooidc.ClientContext(context.WithoutCancel(ctx), p.http), jwksURL)
	verifierConfig := func() *gooidc.Config {
		return &gooidc.Config{ClientID: p.cfg.ClientID, SupportedSigningAlgs: allowedAlgorithms, Now: p.now}
	}
	return &endpoints{
		oauth: &oauth2.Config{
			ClientID:     p.cfg.ClientID,
			ClientSecret: p.cfg.ClientSecret,
			Endpoint:     oauth2.Endpoint{AuthURL: authURL, TokenURL: tokenURL, AuthStyle: oauth2.AuthStyleInHeader},
			RedirectURL:  p.cfg.RedirectURL,
			Scopes:       []string{gooidc.ScopeOpenID, "email"},
		},
		revokeURL:      revokeURL,
		idVerifier:     gooidc.NewVerifier(p.cfg.IssuerURL, keySet, verifierConfig()),
		accessVerifier: gooidc.NewVerifier(p.cfg.IssuerURL, keySet, verifierConfig()),
		logoutVerifier: gooidc.NewVerifier(p.cfg.IssuerURL, keySet, verifierConfig()),
	}, nil
}

// reAddress keeps the path (and query) of an endpoint announced by discovery
// and moves it to base. Server-to-server endpoints go to the internal host,
// the authorization endpoint (opened by the browser) to the public host.
func (p *provider) reAddress(raw, base string) (string, error) {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Path == "" || !strings.HasPrefix(parsed.Path, "/realms/"+p.cfg.Realm+"/") {
		return "", errors.New("endpoint outside the realm")
	}
	return base + parsed.EscapedPath(), nil
}

// do runs a request and returns the body of a 200 answer. Errors carry only
// the status, never the body (it may echo user data or secrets).
func (p *provider) do(request *http.Request) ([]byte, error) {
	response, err := p.http.Do(request)
	if err != nil {
		return nil, errors.New("request failed: " + sanitizeTransportError(err))
	}
	defer response.Body.Close() //nolint:errcheck // nothing to do when closing a read body fails
	body, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes))
	if err != nil {
		return nil, errors.New("read response failed")
	}
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %d", response.StatusCode)
	}
	return body, nil
}

// sanitizeTransportError drops the URL that net/http embeds in its errors:
// authorization codes and token values never travel in query strings here, but
// the rule stays simple, no URLs in logs.
func sanitizeTransportError(err error) string {
	var urlError *url.Error
	if errors.As(err, &urlError) {
		return urlError.Err.Error()
	}
	return err.Error()
}

func providerFailure(operation string, err error) error {
	return fmt.Errorf("%w: %s: %w", usecase.ErrIdentityProviderUnavailable, operation, err)
}

// authorizationURL builds the redirect to Keycloak (code flow, PKCE S256).
func (p *provider) authorizationURL(ctx context.Context, state, nonce, verifier string, forceLogin bool) (string, error) {
	found, err := p.endpoints(ctx)
	if err != nil {
		return "", err
	}
	options := []oauth2.AuthCodeOption{oauth2.S256ChallengeOption(verifier), gooidc.Nonce(nonce)}
	if forceLogin {
		options = append(options, oauth2.SetAuthURLParam("prompt", "login"))
	}
	return found.oauth.AuthCodeURL(state, options...), nil
}

// tokens is what the token endpoint returned.
type tokens struct {
	AccessToken  string
	RefreshToken string
	IDToken      string
}

// exchange trades the authorization code for tokens at Keycloak. A refused
// code is errGrantRejected; anything else means the provider failed.
func (p *provider) exchange(ctx context.Context, code, verifier string) (tokens, error) {
	found, err := p.endpoints(ctx)
	if err != nil {
		return tokens{}, err
	}
	token, err := found.oauth.Exchange(p.clientContext(ctx), code, oauth2.VerifierOption(verifier))
	if err != nil {
		return tokens{}, classifyTokenError("exchange code", err)
	}
	rawID, _ := token.Extra("id_token").(string)
	return tokens{AccessToken: token.AccessToken, RefreshToken: token.RefreshToken, IDToken: rawID}, nil
}

// refresh renews the tokens with the refresh token. Keycloak rotates the
// refresh token when the realm says so; a missing one in the answer means the
// old one stays valid and the returned value is empty.
func (p *provider) refresh(ctx context.Context, refreshToken string) (tokens, error) {
	found, err := p.endpoints(ctx)
	if err != nil {
		return tokens{}, err
	}
	// The expired placeholder forces the token source to renew immediately.
	source := found.oauth.TokenSource(p.clientContext(ctx), &oauth2.Token{RefreshToken: refreshToken, Expiry: time.Unix(1, 0)})
	token, err := source.Token()
	if err != nil {
		return tokens{}, classifyTokenError("refresh token", err)
	}
	rawID, _ := token.Extra("id_token").(string)
	renewed := token.RefreshToken
	if renewed == refreshToken {
		renewed = "" // oauth2 reuses the old value when the answer has none
	}
	return tokens{AccessToken: token.AccessToken, RefreshToken: renewed, IDToken: rawID}, nil
}

func (p *provider) clientContext(ctx context.Context) context.Context {
	return context.WithValue(ctx, oauth2.HTTPClient, p.http)
}

func classifyTokenError(operation string, err error) error {
	var retrieve *oauth2.RetrieveError
	if errors.As(err, &retrieve) && retrieve.Response != nil && retrieve.Response.StatusCode == http.StatusBadRequest {
		return fmt.Errorf("%s: %w", operation, errGrantRejected)
	}
	if errors.As(err, &retrieve) && retrieve.Response != nil {
		return providerFailure(operation, fmt.Errorf("unexpected status %d", retrieve.Response.StatusCode))
	}
	return providerFailure(operation, errors.New(sanitizeTransportError(err)))
}

// revokeRefreshToken revokes the refresh token at Keycloak (RFC 7009). It does
// not end the SSO session.
func (p *provider) revokeRefreshToken(ctx context.Context, refreshToken string) error {
	found, err := p.endpoints(ctx)
	if err != nil {
		return err
	}
	if found.revokeURL == "" {
		return providerFailure("revoke refresh token", errors.New("realm announces no revocation endpoint"))
	}
	form := url.Values{"token": {refreshToken}, "token_type_hint": {"refresh_token"}}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, found.revokeURL, strings.NewReader(form.Encode()))
	if err != nil {
		return providerFailure("build revocation request", err)
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.SetBasicAuth(url.QueryEscape(p.cfg.ClientID), url.QueryEscape(p.cfg.ClientSecret))
	if _, err := p.do(request); err != nil {
		return providerFailure("revoke refresh token", err)
	}
	return nil
}

// verifyAccessToken verifies an access token issued to this client: signature
// by the realm keys, issuer, audience, expiry, not-before, authorized party
// and token type (an ID token is never an access token).
func (p *provider) verifyAccessToken(ctx context.Context, raw string) (verifiedToken, error) {
	found, err := p.endpoints(ctx)
	if err != nil {
		return verifiedToken{}, err
	}
	return p.verify(ctx, found.accessVerifier, raw, tokenTypeAccess)
}

// verifyIDToken verifies the ID token of a login.
func (p *provider) verifyIDToken(ctx context.Context, raw string) (verifiedToken, error) {
	found, err := p.endpoints(ctx)
	if err != nil {
		return verifiedToken{}, err
	}
	return p.verify(ctx, found.idVerifier, raw, tokenTypeID)
}

func (p *provider) verify(ctx context.Context, verifier *gooidc.IDTokenVerifier, raw, wantType string) (verifiedToken, error) {
	token, err := verifier.Verify(gooidc.ClientContext(ctx, p.http), raw)
	if err != nil {
		return verifiedToken{}, classifyVerifyError(err)
	}
	var claims tokenClaims
	if err := token.Claims(&claims); err != nil {
		return verifiedToken{}, errTokenRejected
	}
	now := p.now()
	switch {
	case token.Subject == "":
		return verifiedToken{}, errTokenRejected
	case claims.Type != wantType:
		return verifiedToken{}, errTokenRejected
	case claims.AuthorizedParty != p.cfg.ClientID:
		return verifiedToken{}, errTokenRejected
	case claims.NotBefore != nil && time.Unix(*claims.NotBefore, 0).After(now.Add(p.cfg.ClockSkew)):
		return verifiedToken{}, errTokenRejected
	case claims.IssuedAt != nil && time.Unix(*claims.IssuedAt, 0).After(now.Add(p.cfg.ClockSkew)):
		return verifiedToken{}, errTokenRejected
	}
	return verifiedToken{
		Subject:       token.Subject,
		SessionID:     claims.SessionID,
		Email:         claims.Email,
		EmailVerified: claims.EmailVerified,
		Nonce:         claims.Nonce,
		ClientRoles:   claims.ResourceAccess[p.cfg.ClientID].Roles,
		Expiry:        token.Expiry,
	}, nil
}

// verifyLogoutToken verifies a back-channel logout token (OIDC Back-Channel
// Logout 1.0, section 2.6): realm signature, issuer, audience, expiry, the
// logout event, no nonce, and at least a sid or a sub.
func (p *provider) verifyLogoutToken(ctx context.Context, raw string) (verifiedToken, error) {
	found, err := p.endpoints(ctx)
	if err != nil {
		return verifiedToken{}, err
	}
	token, err := found.logoutVerifier.Verify(gooidc.ClientContext(ctx, p.http), raw)
	if err != nil {
		return verifiedToken{}, classifyVerifyError(err)
	}
	var claims tokenClaims
	if err := token.Claims(&claims); err != nil {
		return verifiedToken{}, errTokenRejected
	}
	_, hasEvent := claims.Events[logoutEvent]
	switch {
	case !hasEvent, claims.Nonce != "":
		return verifiedToken{}, errTokenRejected
	case claims.SessionID == "" && token.Subject == "":
		return verifiedToken{}, errTokenRejected
	case claims.IssuedAt != nil && time.Unix(*claims.IssuedAt, 0).After(p.now().Add(p.cfg.ClockSkew)):
		return verifiedToken{}, errTokenRejected
	}
	return verifiedToken{Subject: token.Subject, SessionID: claims.SessionID, Expiry: token.Expiry}, nil
}

// classifyVerifyError separates "the realm keys could not be fetched"
// (provider failure, retry later) from "this token is bad".
func classifyVerifyError(err error) error {
	var expired *gooidc.TokenExpiredError
	if errors.As(err, &expired) {
		return errTokenRejected
	}
	if strings.Contains(err.Error(), "fetching keys") {
		return providerFailure("fetch signing keys", errors.New("keys unavailable"))
	}
	return errTokenRejected
}

// sameSecret compares two secrets without leaking where they differ.
func sameSecret(a, b string) bool {
	return len(a) == len(b) && subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}
