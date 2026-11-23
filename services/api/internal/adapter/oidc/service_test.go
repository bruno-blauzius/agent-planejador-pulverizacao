package oidc

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/domain"
	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/usecase"
)

func background() context.Context { return context.Background() }

// ---- login -----------------------------------------------------------------------

func TestStartLoginBuildsAuthorizationRequest(t *testing.T) {
	h := newHarness(t)

	authURL, flow, err := h.service.StartLogin(background(), "/farms", false)
	if err != nil {
		t.Fatal(err)
	}
	parsed, _ := url.Parse(authURL)
	query := parsed.Query()
	if parsed.Host != "sso.example.test:8180" {
		t.Fatalf("browser must go to the public host, got %s", parsed.Host)
	}
	for key, want := range map[string]string{"scope": "openid email", "code_challenge_method": "S256", "response_type": "code"} {
		if query.Get(key) != want {
			t.Errorf("%s = %q, want %q", key, query.Get(key), want)
		}
	}
	if query.Has("prompt") {
		t.Error("a plain login must not force prompt=login")
	}
	// 256 bits of state and nonce.
	for _, name := range []string{"state", "nonce"} {
		raw, err := base64.RawURLEncoding.DecodeString(query.Get(name))
		if err != nil || len(raw) != 32 {
			t.Errorf("%s is not 256 random bits: %q", name, query.Get(name))
		}
	}
	// The flow cookie is sealed: it must not be readable JSON nor leak the state.
	raw, err := base64.RawURLEncoding.DecodeString(flow)
	if err != nil || json.Valid(raw) || strings.Contains(string(raw), query.Get("state")) {
		t.Error("flow cookie is readable")
	}
}

func TestStartLoginForcesLoginAfterLogout(t *testing.T) {
	h := newHarness(t)
	authURL, _, err := h.service.StartLogin(background(), "/", true)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(authURL, "prompt=login") {
		t.Fatalf("missing prompt=login in %s", authURL)
	}
}

func TestStartLoginValuesAreRandomPerAttempt(t *testing.T) {
	h := newHarness(t)
	first, _, _ := h.service.StartLogin(background(), "/", false)
	second, _, _ := h.service.StartLogin(background(), "/", false)
	a, _ := url.Parse(first)
	b, _ := url.Parse(second)
	for _, name := range []string{"state", "nonce", "code_challenge"} {
		if a.Query().Get(name) == b.Query().Get(name) {
			t.Errorf("%s repeated across attempts", name)
		}
	}
}

func TestValidateReturnTo(t *testing.T) {
	tests := []struct {
		name, input, want string
		ok                bool
	}{
		{"empty means root", "", "/", true},
		{"root", "/", "/", true},
		{"simple path", "/farms", "/farms", true},
		{"path with query", "/farms?page=2&q=a%20b", "/farms?page=2&q=a%20b", true},
		{"nested path", "/farms/123/fields", "/farms/123/fields", true},
		{"encoded slashes decode to a host", "/%2F%2Fevil.com", "", false},
		{"encoded segment is fine", "/farms%2Fx", "/farms%2Fx", true},
		{"protocol relative", "//evil.com", "", false},
		{"triple slash", "///evil.com/x", "", false},
		{"backslash host", "/\\evil.com", "", false},
		{"double backslash", "\\\\evil.com", "", false},
		{"absolute https", "https://evil.com", "", false},
		{"absolute http with path", "http://evil.com/farms", "", false},
		{"javascript scheme", "javascript:alert(1)", "", false},
		{"data scheme", "data:text/html,x", "", false},
		{"no leading slash", "evil.com", "", false},
		{"relative segment", "farms", "", false},
		{"tab inside", "/\t/evil.com", "", false},
		{"newline injection", "/ok\r\nSet-Cookie: a=b", "", false},
		{"nul byte", "/ok\x00", "", false},
		{"backslash later", "/ok\\evil", "", false},
		{"too long", "/" + strings.Repeat("a", maxReturnTo), "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ValidateReturnTo(tt.input)
			if tt.ok {
				if err != nil || got != tt.want {
					t.Fatalf("got (%q, %v), want %q", got, err, tt.want)
				}
				return
			}
			if !errors.Is(err, usecase.ErrInvalidReturnTo) {
				t.Fatalf("got (%q, %v), want ErrInvalidReturnTo", got, err)
			}
		})
	}
}

func TestStartLoginRejectsOpenRedirect(t *testing.T) {
	h := newHarness(t)
	for _, target := range []string{"//evil.com", "https://evil.com", "/\\evil.com"} {
		if _, _, err := h.service.StartLogin(background(), target, false); !errors.Is(err, usecase.ErrInvalidReturnTo) {
			t.Errorf("return_to %q: err = %v", target, err)
		}
	}
	if token, _, _ := h.idp.counts(); token != 0 {
		t.Fatal("no call to the provider is expected")
	}
}

func TestFinishLoginCreatesSession(t *testing.T) {
	h := newHarness(t)
	cookie := h.login(adminGrant())

	raw, err := base64.RawURLEncoding.DecodeString(cookie)
	if err != nil || len(raw) != 32 {
		t.Fatalf("session cookie must be 256 random bits, got %q", cookie)
	}
	record := h.sessions.only()
	if record.UserID != testUserID || record.Issuer != testIssuer || record.IdPSessionID != "sid-1" {
		t.Fatalf("record = %+v", record)
	}
	if !strings.Contains(strings.Join(record.GrantedRoles, ","), "admin") {
		t.Fatalf("granted roles = %v", record.GrantedRoles)
	}
	if want := epoch.Add(h.cfg.SessionTTL); !record.ExpiresAt.Equal(want) {
		t.Fatalf("ExpiresAt = %v, want %v", record.ExpiresAt, want)
	}
	if want := epoch.Add(5*time.Minute - h.cfg.RenewMargin); !record.AccessExpiresAt.Equal(want) {
		t.Fatalf("AccessExpiresAt = %v, want %v", record.AccessExpiresAt, want)
	}
	// Only the hash is stored: nothing in the record equals the cookie.
	if strings.Contains(string(record.RefreshTokenEnc), "refresh-") {
		t.Fatal("refresh token stored in clear text")
	}
	plain, err := h.service.keys.Open(record.RefreshTokenEnc, record.EncKeyID, []byte(record.ID))
	if err != nil || !strings.HasPrefix(string(plain), "refresh-") {
		t.Fatalf("refresh token does not open: %v", err)
	}
	if _, err := h.service.keys.Open(record.RefreshTokenEnc, record.EncKeyID, []byte("another-session")); err == nil {
		t.Fatal("ciphertext must be bound to the session id")
	}

	claims := h.resolver.lastClaims()
	if claims.Issuer != testIssuer || claims.Subject != testSubject || claims.Email != "ana@agro.test" || !claims.EmailVerified {
		t.Fatalf("claims = %+v", claims)
	}
	if !strings.Contains(strings.Join(claims.ClientRoles, ","), "admin") {
		t.Fatalf("claims roles = %v", claims.ClientRoles)
	}
}

func TestAuthorizationCodeIsExchangedAtTheProviderWithPKCE(t *testing.T) {
	h := newHarness(t)
	h.login(adminGrant())
	if token, _, _ := h.idp.counts(); token != 1 {
		t.Fatalf("token endpoint calls = %d, want 1", token)
	}
	// The fake endpoint rejects a wrong secret, a wrong redirect_uri and a wrong
	// code_verifier, so reaching this point proves they were all sent correctly.
}

func TestFinishLoginRejections(t *testing.T) {
	type attempt struct {
		name  string
		setup func(h *harness, code, state, flow *string)
	}
	attempts := []attempt{
		{"state mismatch", func(_ *harness, _, state, _ *string) { *state = "forged" }},
		{"empty state", func(_ *harness, _, state, _ *string) { *state = "" }},
		{"empty code", func(_ *harness, code, _, _ *string) { *code = "" }},
		{"missing flow cookie", func(_ *harness, _, _, flow *string) { *flow = "" }},
		{"tampered flow cookie", func(_ *harness, _, _, flow *string) {
			raw := []byte(*flow)
			if raw[len(raw)-1] == 'A' {
				raw[len(raw)-1] = 'B'
			} else {
				raw[len(raw)-1] = 'A'
			}
			*flow = string(raw)
		}},
		{"garbage flow cookie", func(_ *harness, _, _, flow *string) { *flow = "not-a-cookie" }},
		{"expired flow", func(h *harness, _, _, _ *string) { h.clock.Advance(h.cfg.FlowTTL + time.Second) }},
		{"code never issued", func(_ *harness, code, _, _ *string) { *code = "code-from-nowhere" }},
		{"nonce mismatch in ID token", func(h *harness, _, _, _ *string) { h.idp.idTokenNonce = "replayed-nonce" }},
		{"ID token of another client", func(h *harness, _, _, _ *string) {
			h.idp.idMutator = func(c map[string]any) { c["aud"], c["azp"] = "other-client", "other-client" }
		}},
		{"ID token with wrong issuer", func(h *harness, _, _, _ *string) {
			h.idp.idMutator = func(c map[string]any) { c["iss"] = "http://evil.example/realms/inceres" }
		}},
		{"ID token expired", func(h *harness, _, _, _ *string) {
			h.idp.idMutator = func(c map[string]any) { c["exp"] = epoch.Add(-time.Minute).Unix() }
		}},
		{"access token in place of the ID token", func(h *harness, _, _, _ *string) {
			h.idp.idMutator = func(c map[string]any) { c["typ"] = "Bearer" }
		}},
		{"access token of another client", func(h *harness, _, _, _ *string) {
			h.idp.accessMutator = func(c map[string]any) { c["azp"] = "other-client" }
		}},
		{"ID token in place of the access token", func(h *harness, _, _, _ *string) {
			h.idp.accessMutator = func(c map[string]any) { c["typ"] = "ID" }
		}},
		{"subjects differ", func(h *harness, _, _, _ *string) {
			h.idp.accessMutator = func(c map[string]any) { c["sub"] = "someone-else" }
		}},
	}
	for _, tt := range attempts {
		t.Run(tt.name, func(t *testing.T) {
			h := newHarness(t)
			authURL, flow, err := h.service.StartLogin(background(), "/farms", false)
			if err != nil {
				t.Fatal(err)
			}
			code, state := h.idp.authorize(authURL, adminGrant())
			tt.setup(h, &code, &state, &flow)

			token, _, err := h.service.FinishLogin(background(), code, state, flow)
			if !errors.Is(err, usecase.ErrLoginRejected) {
				t.Fatalf("err = %v, want ErrLoginRejected", err)
			}
			if token != "" || h.sessions.count() != 0 {
				t.Fatal("a session was created by a rejected login")
			}
		})
	}
}

func TestFinishLoginRejectsPKCEVerifierOfAnotherAttempt(t *testing.T) {
	h := newHarness(t)
	urlA, _, _ := h.service.StartLogin(background(), "/", false)
	urlB, flowB, _ := h.service.StartLogin(background(), "/", false)
	codeA, _ := h.idp.authorize(urlA, adminGrant())
	_, stateB := h.idp.authorize(urlB, adminGrant())

	// State and flow cookie agree (attempt B) but the code belongs to attempt A:
	// Keycloak refuses because the code_verifier does not match the challenge.
	_, _, err := h.service.FinishLogin(background(), codeA, stateB, flowB)
	if !errors.Is(err, usecase.ErrLoginRejected) || h.sessions.count() != 0 {
		t.Fatalf("err = %v sessions = %d", err, h.sessions.count())
	}
}

func TestFinishLoginCodeIsSingleUse(t *testing.T) {
	h := newHarness(t)
	authURL, flow, _ := h.service.StartLogin(background(), "/", false)
	code, state := h.idp.authorize(authURL, adminGrant())
	if _, _, err := h.service.FinishLogin(background(), code, state, flow); err != nil {
		t.Fatal(err)
	}
	if _, _, err := h.service.FinishLogin(background(), code, state, flow); !errors.Is(err, usecase.ErrLoginRejected) {
		t.Fatalf("replayed callback: err = %v", err)
	}
	if h.sessions.count() != 1 {
		t.Fatalf("sessions = %d, want 1", h.sessions.count())
	}
}

func TestFinishLoginFlowFromAnotherKeyIsRejected(t *testing.T) {
	h := newHarness(t)
	other := newHarness(t)
	other.cfg.EncryptionKey = testKey('x')
	other.build(other.cfg)

	authURL, flow, err := other.service.StartLogin(background(), "/", false)
	if err != nil {
		t.Fatal(err)
	}
	// Same IdP behaviour is irrelevant: the cookie must not even open.
	code, state := other.idp.authorize(authURL, adminGrant())
	if _, _, err := h.service.FinishLogin(background(), code, state, flow); !errors.Is(err, usecase.ErrLoginRejected) {
		t.Fatalf("err = %v", err)
	}
}

func TestFinishLoginProductAccessErrorsPassThroughWithoutSession(t *testing.T) {
	tests := []struct {
		name    string
		roles   []string
		failure error
		want    error
	}{
		{"not provisioned", nil, usecase.ErrUserNotProvisioned, usecase.ErrUserNotProvisioned},
		{"inactive user or company", nil, usecase.ErrAccessDenied, usecase.ErrAccessDenied},
		{"no product role", []string{"offline_access", "uma_authorization"}, nil, domain.ErrNoAccessToProduct},
		{"ambiguous role", []string{"admin", "super_admin"}, nil, domain.ErrAmbiguousRole},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newHarness(t)
			h.resolver.err = tt.failure
			g := adminGrant()
			if tt.roles != nil {
				g.roles = tt.roles
			}
			authURL, flow, _ := h.service.StartLogin(background(), "/", false)
			code, state := h.idp.authorize(authURL, g)

			_, _, err := h.service.FinishLogin(background(), code, state, flow)
			if !errors.Is(err, tt.want) {
				t.Fatalf("err = %v, want %v", err, tt.want)
			}
			if h.sessions.count() != 0 {
				t.Fatal("a session was created")
			}
		})
	}
}

func TestProviderOutageIsReportedNotInvented(t *testing.T) {
	h := newHarness(t)
	h.idp.srv.Close()
	_, _, err := h.service.StartLogin(background(), "/", false)
	if !errors.Is(err, usecase.ErrIdentityProviderUnavailable) {
		t.Fatalf("err = %v", err)
	}
	if _, err := h.auth.Authenticate(h.bearer("a.b.c")); !errors.Is(err, usecase.ErrIdentityProviderUnavailable) {
		t.Fatalf("bearer during outage: err = %v", err)
	}
}

func TestDiscoveryIssuerMustMatchTheConfiguredOne(t *testing.T) {
	h := newHarness(t)
	h.idp.discoveryIss = "http://evil.example/realms/inceres"
	_, _, err := h.service.StartLogin(background(), "/", false)
	if !errors.Is(err, usecase.ErrIdentityProviderUnavailable) {
		t.Fatalf("err = %v", err)
	}
}

func TestDiscoveryIsRetriedAfterAFailure(t *testing.T) {
	h := newHarness(t)
	h.idp.discoveryIss = "http://evil.example/realms/inceres"
	if _, _, err := h.service.StartLogin(background(), "/", false); err == nil {
		t.Fatal("want a failure")
	}
	h.idp.discoveryIss = ""
	if _, _, err := h.service.StartLogin(background(), "/", false); err != nil {
		t.Fatalf("the failure must not be cached: %v", err)
	}
}

// ---- tokens: bearer and algorithms -------------------------------------------------

func TestBearerAcceptsAValidAccessTokenWithoutCreatingASession(t *testing.T) {
	h := newHarness(t)
	token := h.idp.signAccess(adminGrant())

	principal, err := h.auth.Authenticate(h.bearer(token))
	if err != nil {
		t.Fatal(err)
	}
	if principal.UserID != testUserID || principal.Role != domain.RoleAdmin || !principal.Can(domain.PermUsersManage) {
		t.Fatalf("principal = %+v", principal)
	}
	if h.sessions.count() != 0 {
		t.Fatal("a Bearer request must not create a session")
	}
}

func TestBearerRejectsInvalidTokens(t *testing.T) {
	otherKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	mutations := map[string]func(c map[string]any){
		"token issued to another client of the realm": func(c map[string]any) { c["azp"] = "other-client" },
		"audience of another client only":             func(c map[string]any) { c["aud"] = []string{"account"} },
		"no audience":                                 func(c map[string]any) { delete(c, "aud") },
		"wrong issuer":                                func(c map[string]any) { c["iss"] = "http://evil.example/realms/inceres" },
		"issuer of another realm":                     func(c map[string]any) { c["iss"] = testPublic + "/realms/other" },
		"expired":                                     func(c map[string]any) { c["exp"] = epoch.Add(-time.Second).Unix() },
		"not valid yet":                               func(c map[string]any) { c["nbf"] = epoch.Add(10 * time.Minute).Unix() },
		"issued in the future":                        func(c map[string]any) { c["iat"] = epoch.Add(10 * time.Minute).Unix() },
		"ID token used as access token":               func(c map[string]any) { c["typ"] = "ID" },
		"refresh token used as access token":          func(c map[string]any) { c["typ"] = "Refresh" },
		"logout token used as access token":           func(c map[string]any) { c["typ"] = "Logout" },
		"no token type":                               func(c map[string]any) { delete(c, "typ") },
		"no subject":                                  func(c map[string]any) { delete(c, "sub") },
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			h := newHarness(t)
			claims := h.idp.accessClaims(adminGrant())
			mutate(claims)
			_, err := h.auth.Authenticate(h.bearer(h.idp.sign(claims)))
			if !errors.Is(err, usecase.ErrUnauthenticated) {
				t.Fatalf("err = %v, want ErrUnauthenticated", err)
			}
			if len(h.resolver.calls) != 0 {
				t.Fatal("a rejected token must never reach the resolver")
			}
		})
	}

	forged := map[string]func(h *harness, claims map[string]any) string{
		"alg none": func(h *harness, c map[string]any) string { return signJWT(h.t, h.idp.key, "none", c) },
		"HS256 keyed with the public key": func(h *harness, c map[string]any) string {
			return signJWT(h.t, h.idp.key, "HS256", c)
		},
		"signed by a key the realm does not publish": func(h *harness, c map[string]any) string {
			return signJWT(h.t, otherKey, "RS256", c)
		},
		"tampered payload": func(h *harness, c map[string]any) string {
			parts := strings.Split(h.idp.sign(c), ".")
			c["sub"] = "someone-else"
			payload, _ := json.Marshal(c)
			return parts[0] + "." + b64(payload) + "." + parts[2]
		},
		"not a jwt": func(*harness, map[string]any) string { return "garbage" },
	}
	for name, forge := range forged {
		t.Run(name, func(t *testing.T) {
			h := newHarness(t)
			token := forge(h, h.idp.accessClaims(adminGrant()))
			if _, err := h.auth.Authenticate(h.bearer(token)); !errors.Is(err, usecase.ErrUnauthenticated) {
				t.Fatalf("err = %v, want ErrUnauthenticated", err)
			}
		})
	}
}

func TestBearerToleratesASmallClockSkew(t *testing.T) {
	h := newHarness(t)
	claims := h.idp.accessClaims(adminGrant())
	claims["nbf"] = epoch.Add(10 * time.Second).Unix()
	claims["iat"] = epoch.Add(10 * time.Second).Unix()
	if _, err := h.auth.Authenticate(h.bearer(h.idp.sign(claims))); err != nil {
		t.Fatalf("10s ahead is inside the %v tolerance: %v", h.cfg.ClockSkew, err)
	}
}

func TestBearerKeepsProductAccessFailures(t *testing.T) {
	h := newHarness(t)
	h.resolver.err = usecase.ErrUserNotProvisioned
	_, err := h.auth.Authenticate(h.bearer(h.idp.signAccess(adminGrant())))
	if !errors.Is(err, usecase.ErrUserNotProvisioned) || errors.Is(err, usecase.ErrUnauthenticated) {
		t.Fatalf("err = %v", err)
	}
}

func TestKeysUnavailableIsAProviderFailureNotA401(t *testing.T) {
	h := newHarness(t)
	h.idp.jwksStatus = 503
	_, err := h.auth.Authenticate(h.bearer(h.idp.signAccess(adminGrant())))
	if !errors.Is(err, usecase.ErrIdentityProviderUnavailable) {
		t.Fatalf("err = %v", err)
	}
	h.idp.jwksStatus = 0
	if _, err := h.auth.Authenticate(h.bearer(h.idp.signAccess(adminGrant()))); err != nil {
		t.Fatalf("recovers once the keys are back: %v", err)
	}
}

func TestBearerWinsOverCookieAndNoCredentialIs401(t *testing.T) {
	h := newHarness(t)
	if _, err := h.auth.Authenticate(h.request("")); !errors.Is(err, usecase.ErrUnauthenticated) {
		t.Fatalf("no credential: %v", err)
	}
	if _, err := h.auth.Authenticate(h.request("not-a-session")); !errors.Is(err, usecase.ErrUnauthenticated) {
		t.Fatalf("unknown cookie: %v", err)
	}
	r := h.bearer(h.idp.signAccess(adminGrant()))
	r.Header.Set("Authorization", "bearer   "+h.idp.signAccess(adminGrant()))
	if _, err := h.auth.Authenticate(r); err != nil {
		t.Fatalf("scheme is case insensitive: %v", err)
	}
	r.Header.Set("Authorization", "Basic abc")
	if _, err := h.auth.Authenticate(r); !errors.Is(err, usecase.ErrUnauthenticated) {
		t.Fatalf("other scheme: %v", err)
	}
}

// ---- sessions: renewal ---------------------------------------------------------------

func TestSessionAuthenticatesWithoutCallingTheProvider(t *testing.T) {
	h := newHarness(t)
	cookie := h.login(adminGrant())
	_, refreshBefore, _ := h.idp.counts()

	principal, err := h.auth.Authenticate(h.request(cookie))
	if err != nil {
		t.Fatal(err)
	}
	if principal.UserID != testUserID || principal.Role != domain.RoleAdmin {
		t.Fatalf("principal = %+v", principal)
	}
	if _, refreshAfter, _ := h.idp.counts(); refreshAfter != refreshBefore {
		t.Fatal("a valid access token must not trigger a renewal")
	}
	if h.sessions.touches != 1 {
		t.Fatalf("touches = %d", h.sessions.touches)
	}
}

func TestSessionRenewsAtTheProviderWhenTheAccessTokenExpires(t *testing.T) {
	h := newHarness(t)
	cookie := h.login(adminGrant())
	h.idp.refreshRoles = []string{"planning_operator", "planner.use"}
	h.clock.Advance(5 * time.Minute)

	principal, err := h.auth.Authenticate(h.request(cookie))
	if err != nil {
		t.Fatal(err)
	}
	if principal.Role != domain.RolePlanningOperator {
		t.Fatalf("role after renewal = %q: the new token's roles must apply at once", principal.Role)
	}
	if _, refresh, _ := h.idp.counts(); refresh != 1 || h.sessions.renewals != 1 {
		t.Fatalf("refresh calls = %d renewals = %d", refresh, h.sessions.renewals)
	}
	if _, err := h.auth.Authenticate(h.request(cookie)); err != nil {
		t.Fatal(err)
	}
	if _, refresh, _ := h.idp.counts(); refresh != 1 {
		t.Fatal("the renewed token is valid, no second renewal expected")
	}
}

func TestSessionFollowsRefreshTokenRotation(t *testing.T) {
	h := newHarness(t)
	h.idp.rotate = true
	cookie := h.login(adminGrant())
	before := h.sessions.only().RefreshTokenEnc

	for i := 0; i < 3; i++ {
		h.clock.Advance(5 * time.Minute)
		if _, err := h.auth.Authenticate(h.request(cookie)); err != nil {
			t.Fatalf("renewal %d: %v (the rotated refresh token must be stored)", i+1, err)
		}
	}
	if string(h.sessions.only().RefreshTokenEnc) == string(before) {
		t.Fatal("the stored refresh token did not change")
	}
}

func TestConcurrentRequestsRenewOnlyOnce(t *testing.T) {
	h := newHarness(t)
	h.idp.rotate = true // a second use of the old refresh token would be refused
	cookie := h.login(adminGrant())
	h.clock.Advance(5 * time.Minute)

	var wg sync.WaitGroup
	errs := make(chan error, 12)
	for i := 0; i < cap(errs); i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := h.auth.Authenticate(h.request(cookie))
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent request failed: %v", err)
		}
	}
	if _, refresh, _ := h.idp.counts(); refresh != 1 {
		t.Fatalf("refresh calls = %d, want 1 (serialized per session)", refresh)
	}
}

func TestSessionEndsWhenRenewalIsRefused(t *testing.T) {
	h := newHarness(t)
	cookie := h.login(adminGrant())
	h.idp.refreshStatus = 400
	h.clock.Advance(5 * time.Minute)

	if _, err := h.auth.Authenticate(h.request(cookie)); !errors.Is(err, usecase.ErrUnauthenticated) {
		t.Fatalf("err = %v", err)
	}
	if h.sessions.revokedCount() != 1 {
		t.Fatal("the session must be revoked")
	}
	h.idp.refreshStatus = 0
	if _, err := h.auth.Authenticate(h.request(cookie)); !errors.Is(err, usecase.ErrUnauthenticated) {
		t.Fatalf("revoked session came back: %v", err)
	}
}

func TestSessionSurvivesAProviderOutageDuringRenewal(t *testing.T) {
	h := newHarness(t)
	cookie := h.login(adminGrant())
	h.idp.refreshStatus = 503
	h.clock.Advance(5 * time.Minute)

	if _, err := h.auth.Authenticate(h.request(cookie)); !errors.Is(err, usecase.ErrIdentityProviderUnavailable) {
		t.Fatalf("err = %v", err)
	}
	if h.sessions.revokedCount() != 0 {
		t.Fatal("an outage must not log users out")
	}
	h.idp.refreshStatus = 0
	if _, err := h.auth.Authenticate(h.request(cookie)); err != nil {
		t.Fatalf("recovers after the outage: %v", err)
	}
}

func TestSessionEndsWhenTheProductRoleDisappears(t *testing.T) {
	tests := map[string][]string{
		"role removed":   {"offline_access"},
		"role ambiguous": {"admin", "super_admin"},
	}
	for name, roles := range tests {
		t.Run(name, func(t *testing.T) {
			h := newHarness(t)
			cookie := h.login(adminGrant())
			h.idp.refreshRoles = roles
			h.clock.Advance(5 * time.Minute)

			if _, err := h.auth.Authenticate(h.request(cookie)); !errors.Is(err, usecase.ErrUnauthenticated) {
				t.Fatalf("err = %v", err)
			}
			if h.sessions.revokedCount() != 1 {
				t.Fatal("the session must be revoked")
			}
		})
	}
}

func TestSessionEndsWhenTheRenewedTokenIsInvalid(t *testing.T) {
	h := newHarness(t)
	cookie := h.login(adminGrant())
	h.idp.accessMutator = func(c map[string]any) { c["azp"] = "other-client" }
	h.clock.Advance(5 * time.Minute)
	if _, err := h.auth.Authenticate(h.request(cookie)); !errors.Is(err, usecase.ErrUnauthenticated) || h.sessions.revokedCount() != 1 {
		t.Fatalf("err = %v revoked = %d", err, h.sessions.revokedCount())
	}
}

func TestSessionEndsWhenTheRenewedTokenBelongsToAnotherSubject(t *testing.T) {
	h := newHarness(t)
	cookie := h.login(adminGrant())
	h.idp.accessMutator = func(c map[string]any) { c["sub"] = "someone-else" }
	h.clock.Advance(5 * time.Minute)
	if _, err := h.auth.Authenticate(h.request(cookie)); !errors.Is(err, usecase.ErrUnauthenticated) || h.sessions.revokedCount() != 1 {
		t.Fatalf("err = %v revoked = %d", err, h.sessions.revokedCount())
	}
}

func TestSessionExpiresAbsolutely(t *testing.T) {
	h := newHarness(t)
	cookie := h.login(adminGrant())
	h.clock.Advance(h.cfg.SessionTTL + time.Second)
	if _, err := h.auth.Authenticate(h.request(cookie)); !errors.Is(err, usecase.ErrUnauthenticated) {
		t.Fatalf("err = %v", err)
	}
}

func TestSessionKeepsAccessFailuresAsForbidden(t *testing.T) {
	h := newHarness(t)
	cookie := h.login(adminGrant())
	h.resolver.err = usecase.ErrAccessDenied // user deactivated after login
	if _, err := h.auth.Authenticate(h.request(cookie)); !errors.Is(err, usecase.ErrAccessDenied) {
		t.Fatalf("err = %v", err)
	}
}

func TestSessionOfAnotherUserIsRevoked(t *testing.T) {
	h := newHarness(t)
	cookie := h.login(adminGrant())
	h.resolver.users[testSubject] = "user-2" // identity changed owner
	if _, err := h.auth.Authenticate(h.request(cookie)); !errors.Is(err, usecase.ErrUnauthenticated) || h.sessions.revokedCount() != 1 {
		t.Fatalf("err = %v revoked = %d", err, h.sessions.revokedCount())
	}
}

func TestEncryptionKeyRotationKeepsSessionsAlive(t *testing.T) {
	h := newHarness(t)
	cookie := h.login(adminGrant())

	rotated := h.cfg
	rotated.EncryptionKey, rotated.EncryptionKeyID = testKey('n'), 2
	rotated.OldKeys = map[int16][]byte{1: testKey('k')}
	h.build(rotated)

	h.clock.Advance(5 * time.Minute)
	if _, err := h.auth.Authenticate(h.request(cookie)); err != nil {
		t.Fatalf("a session sealed with the old key must renew: %v", err)
	}
	if got := h.sessions.only().EncKeyID; got != 2 {
		t.Fatalf("EncKeyID = %d, want 2 (resealed with the new key)", got)
	}
}

func TestSessionEndsWhenItsKeyIsGone(t *testing.T) {
	h := newHarness(t)
	cookie := h.login(adminGrant())

	lost := h.cfg
	lost.EncryptionKey, lost.EncryptionKeyID = testKey('n'), 2
	h.build(lost)

	h.clock.Advance(5 * time.Minute)
	if _, err := h.auth.Authenticate(h.request(cookie)); !errors.Is(err, usecase.ErrUnauthenticated) || h.sessions.revokedCount() != 1 {
		t.Fatalf("err = %v", err)
	}
}

// ---- logout -------------------------------------------------------------------------

func TestLogoutRevokesTheSessionAndTheRefreshTokenAtTheProvider(t *testing.T) {
	h := newHarness(t)
	cookie := h.login(adminGrant())
	record := h.sessions.only()
	plain, _ := h.service.keys.Open(record.RefreshTokenEnc, record.EncKeyID, []byte(record.ID))

	if err := h.service.Logout(background(), cookie); err != nil {
		t.Fatal(err)
	}
	if h.sessions.revokedCount() != 1 {
		t.Fatal("session not revoked locally")
	}
	if _, _, revokes := h.idp.counts(); revokes != 1 || len(h.idp.revoked) != 1 || h.idp.revoked[0] != string(plain) {
		t.Fatalf("revoked at the provider = %v", h.idp.revoked)
	}
	if _, err := h.auth.Authenticate(h.request(cookie)); !errors.Is(err, usecase.ErrUnauthenticated) {
		t.Fatalf("session usable after logout: %v", err)
	}
	if err := h.service.Logout(background(), cookie); err != nil {
		t.Fatalf("logout must be idempotent: %v", err)
	}
	if _, _, revokes := h.idp.counts(); revokes != 1 {
		t.Fatal("the second logout must not call the provider again")
	}
}

func TestLogoutWithoutAValidSessionIsANoOp(t *testing.T) {
	h := newHarness(t)
	for _, cookie := range []string{"", "unknown", strings.Repeat("a", maxCookieLength+1)} {
		if err := h.service.Logout(background(), cookie); err != nil {
			t.Fatalf("cookie %q: %v", cookie, err)
		}
	}
	if _, _, revokes := h.idp.counts(); revokes != 0 {
		t.Fatal("no provider call expected")
	}
}

func TestLogoutEndsTheLocalSessionEvenWhenTheProviderFails(t *testing.T) {
	h := newHarness(t)
	cookie := h.login(adminGrant())
	h.idp.revokeStatus = 503
	if err := h.service.Logout(background(), cookie); err != nil {
		t.Fatal(err)
	}
	if h.sessions.revokedCount() != 1 {
		t.Fatal("local revocation must not depend on the provider")
	}
}

// ---- back-channel logout --------------------------------------------------------------

func (f *fakeIdP) logoutClaims(sid, sub string) map[string]any {
	now := f.now()
	claims := map[string]any{
		"iss": testIssuer, "aud": testClientID, "iat": now.Unix(), "exp": now.Add(2 * time.Minute).Unix(), "jti": "jti-1",
		"events": map[string]any{logoutEvent: map[string]any{}},
	}
	if sid != "" {
		claims["sid"] = sid
	}
	if sub != "" {
		claims["sub"] = sub
	}
	return claims
}

type userRevokerFake struct {
	revoked []domain.UserID
}

func (u *userRevokerFake) FindUserByIdentity(_ context.Context, _, subject string) (domain.User, error) {
	if subject != testSubject {
		return domain.User{}, usecase.ErrNotFound
	}
	return domain.User{ID: testUserID}, nil
}

func (u *userRevokerFake) RevokeUserSessions(_ context.Context, id domain.UserID, _ time.Time) error {
	u.revoked = append(u.revoked, id)
	return nil
}

func TestBackchannelLogoutRevokesTheSessionsOfTheIdPSession(t *testing.T) {
	h := newHarness(t)
	h.login(adminGrant())
	other := adminGrant()
	other.sid = "sid-2"
	h.login(other)

	if err := h.service.BackchannelLogout(background(), h.idp.sign(h.idp.logoutClaims("sid-1", testSubject))); err != nil {
		t.Fatal(err)
	}
	if h.sessions.revokedCount() != 1 || h.sessions.count() != 2 {
		t.Fatalf("revoked = %d of %d, want exactly the sid-1 session", h.sessions.revokedCount(), h.sessions.count())
	}
	// Idempotent, and an unknown sid is success.
	for _, sid := range []string{"sid-1", "sid-unknown"} {
		if err := h.service.BackchannelLogout(background(), h.idp.sign(h.idp.logoutClaims(sid, ""))); err != nil {
			t.Fatalf("sid %s: %v", sid, err)
		}
	}
}

func TestBackchannelLogoutBySubject(t *testing.T) {
	h := newHarness(t)
	revoker := &userRevokerFake{}
	service, err := NewService(h.cfg, Deps{
		Sessions: h.sessions, Resolver: h.resolver, Identities: fakeIdentities{}, Clock: h.clock, UserSessions: revoker,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := service.BackchannelLogout(background(), h.idp.sign(h.idp.logoutClaims("", testSubject))); err != nil {
		t.Fatal(err)
	}
	if len(revoker.revoked) != 1 || revoker.revoked[0] != testUserID {
		t.Fatalf("revoked = %v", revoker.revoked)
	}
	// Without the optional collaborator a token with no sid is refused.
	if err := h.service.BackchannelLogout(background(), h.idp.sign(h.idp.logoutClaims("", testSubject))); !errors.Is(err, usecase.ErrInvalidLogoutToken) {
		t.Fatalf("err = %v", err)
	}
}

func TestBackchannelLogoutRejectsInvalidTokens(t *testing.T) {
	otherKey, _ := rsa.GenerateKey(rand.Reader, 2048)
	mutations := map[string]func(c map[string]any){
		"wrong audience":       func(c map[string]any) { c["aud"] = "other-client" },
		"wrong issuer":         func(c map[string]any) { c["iss"] = "http://evil.example/realms/inceres" },
		"expired":              func(c map[string]any) { c["exp"] = epoch.Add(-time.Second).Unix() },
		"no logout event":      func(c map[string]any) { delete(c, "events") },
		"another event":        func(c map[string]any) { c["events"] = map[string]any{"http://example/other": map[string]any{}} },
		"carries a nonce":      func(c map[string]any) { c["nonce"] = "n" },
		"no sid and no sub":    func(c map[string]any) { delete(c, "sid") },
		"issued in the future": func(c map[string]any) { c["iat"] = epoch.Add(time.Hour).Unix() },
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			h := newHarness(t)
			h.login(adminGrant())
			claims := h.idp.logoutClaims("sid-1", "")
			mutate(claims)
			err := h.service.BackchannelLogout(background(), h.idp.sign(claims))
			if !errors.Is(err, usecase.ErrInvalidLogoutToken) {
				t.Fatalf("err = %v", err)
			}
			if h.sessions.revokedCount() != 0 {
				t.Fatal("an invalid token revoked a session")
			}
		})
	}

	forgeries := map[string]func(h *harness) string{
		"alg none":  func(h *harness) string { return signJWT(h.t, h.idp.key, "none", h.idp.logoutClaims("sid-1", "")) },
		"HS256":     func(h *harness) string { return signJWT(h.t, h.idp.key, "HS256", h.idp.logoutClaims("sid-1", "")) },
		"other key": func(h *harness) string { return signJWT(h.t, otherKey, "RS256", h.idp.logoutClaims("sid-1", "")) },
		"an ID token": func(h *harness) string {
			return h.idp.signID(adminGrant())
		},
		"empty": func(*harness) string { return "" },
	}
	for name, forge := range forgeries {
		t.Run(name, func(t *testing.T) {
			h := newHarness(t)
			h.login(adminGrant())
			if err := h.service.BackchannelLogout(background(), forge(h)); !errors.Is(err, usecase.ErrInvalidLogoutToken) {
				t.Fatalf("err = %v", err)
			}
			if h.sessions.revokedCount() != 0 {
				t.Fatal("a forged token revoked a session")
			}
		})
	}
}

// ---- keyring and config ----------------------------------------------------------------

func TestKeyringSealsAndOpens(t *testing.T) {
	ring, err := NewKeyring(2, testKey('n'), map[int16][]byte{1: testKey('k')})
	if err != nil {
		t.Fatal(err)
	}
	sealed, id, err := ring.Seal([]byte("secret"), []byte("aad"))
	if err != nil || id != 2 {
		t.Fatalf("seal: %v id=%d", err, id)
	}
	if got, err := ring.Open(sealed, id, []byte("aad")); err != nil || string(got) != "secret" {
		t.Fatalf("open: %q %v", got, err)
	}
	for name, check := range map[string]func() error{
		"wrong aad":    func() error { _, err := ring.Open(sealed, id, []byte("other")); return err },
		"wrong key id": func() error { _, err := ring.Open(sealed, 1, []byte("aad")); return err },
		"unknown key":  func() error { _, err := ring.Open(sealed, 9, []byte("aad")); return err },
		"truncated":    func() error { _, err := ring.Open(sealed[:5], id, []byte("aad")); return err },
		"flipped byte": func() error {
			bad := append([]byte(nil), sealed...)
			bad[len(bad)-1] ^= 1
			_, err := ring.Open(bad, id, []byte("aad"))
			return err
		},
		"flow as token": func() error {
			cookie, _ := ring.sealFlow([]byte("x"))
			raw, _ := base64.RawURLEncoding.DecodeString(cookie)
			_, err := ring.Open(raw[2:], id, nil)
			return err
		},
	} {
		if check() == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
	old, _ := NewKeyring(1, testKey('k'), nil)
	oldSealed, oldID, _ := old.Seal([]byte("legacy"), []byte("aad"))
	if got, err := ring.Open(oldSealed, oldID, []byte("aad")); err != nil || string(got) != "legacy" {
		t.Fatalf("old key must stay readable: %q %v", got, err)
	}
}

func TestKeyringRejectsBadKeys(t *testing.T) {
	if _, err := NewKeyring(1, []byte("short"), nil); err == nil {
		t.Error("short key accepted")
	}
	if _, err := NewKeyring(0, testKey('k'), nil); err == nil {
		t.Error("id 0 accepted")
	}
	if _, err := NewKeyring(1, testKey('k'), map[int16][]byte{1: testKey('n')}); err == nil {
		t.Error("duplicate id accepted")
	}
	if _, err := NewKeyring(1, testKey('k'), map[int16][]byte{2: []byte("short")}); err == nil {
		t.Error("short old key accepted")
	}
}

func envOf(values map[string]string) LookupFunc {
	return func(key string) (string, bool) { v, ok := values[key]; return v, ok }
}

func TestConfigFromEnvDefaults(t *testing.T) {
	cfg, err := ConfigFromEnv(envOf(map[string]string{
		"KEYCLOAK_AGENTES_AGRO_CLIENT_SECRET": "s",
		"API_SESSION_ENC_KEY":                 base64.StdEncoding.EncodeToString(testKey('k')),
	}))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.IssuerURL != "http://localhost:8180/realms/inceres" || cfg.InternalURL != "http://localhost:8180" ||
		cfg.RedirectURL != "http://localhost:8080/auth/callback" || cfg.ClientID != "agentes-agro" ||
		cfg.SessionTTL != DefaultSessionTTL || cfg.EncryptionKeyID != 1 ||
		cfg.SessionCookieName != "agro_session" || cfg.FlowCookieName != "agro_oidc" {
		t.Fatalf("cfg = %+v", cfg)
	}
}

func TestConfigFromEnvSplitsPublicAndInternalAddresses(t *testing.T) {
	cfg, err := ConfigFromEnv(envOf(map[string]string{
		"KEYCLOAK_URL": "http://keycloak:8080", "KEYCLOAK_ISSUER_URL": "https://sso.inceres.com.br/",
		"AGENTES_AGRO_API_URL": "https://api.agro.inceres.com.br/", "KEYCLOAK_AGENTES_AGRO_CLIENT_SECRET": "s",
		"API_SESSION_ENC_KEY": base64.StdEncoding.EncodeToString(testKey('k')), "API_SESSION_ENC_KEY_ID": "7",
		"API_SESSION_ENC_OLD_KEYS": "3:" + base64.StdEncoding.EncodeToString(testKey('o')),
		"API_SESSION_TTL":          "12h", "AGENTES_AGRO_WEB_URL": "https://agro.inceres.com.br/",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.WebURL != "https://agro.inceres.com.br" {
		t.Fatalf("WebURL = %q", cfg.WebURL)
	}
	if cfg.IssuerURL != "https://sso.inceres.com.br/realms/inceres" || cfg.InternalURL != "http://keycloak:8080" ||
		cfg.RedirectURL != "https://api.agro.inceres.com.br/auth/callback" || cfg.EncryptionKeyID != 7 ||
		cfg.SessionTTL != 12*time.Hour || len(cfg.OldKeys) != 1 {
		t.Fatalf("cfg = %+v", cfg)
	}
}

func TestConfigFromEnvRejectsInvalidValues(t *testing.T) {
	good := map[string]string{
		"KEYCLOAK_AGENTES_AGRO_CLIENT_SECRET": "s",
		"API_SESSION_ENC_KEY":                 base64.StdEncoding.EncodeToString(testKey('k')),
	}
	with := func(key, value string) LookupFunc {
		values := map[string]string{}
		for k, v := range good {
			values[k] = v
		}
		if value == "<unset>" {
			delete(values, key)
		} else {
			values[key] = value
		}
		return envOf(values)
	}
	tests := []struct{ name, key, value string }{
		{"no secret", "KEYCLOAK_AGENTES_AGRO_CLIENT_SECRET", "<unset>"},
		{"no key", "API_SESSION_ENC_KEY", "<unset>"},
		{"key not base64", "API_SESSION_ENC_KEY", "%%%"},
		{"key too short", "API_SESSION_ENC_KEY", base64.StdEncoding.EncodeToString([]byte("short"))},
		{"bad key id", "API_SESSION_ENC_KEY_ID", "0"},
		{"bad old keys", "API_SESSION_ENC_OLD_KEYS", "nonsense"},
		{"bad ttl", "API_SESSION_TTL", "-1h"},
		{"bad issuer", "KEYCLOAK_ISSUER_URL", "not a url"},
		{"bad api url", "AGENTES_AGRO_API_URL", "ftp://x"},
		{"bad web url", "AGENTES_AGRO_WEB_URL", "ftp://x"},
		{"web url with query", "AGENTES_AGRO_WEB_URL", "http://web.test/?a=b"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := ConfigFromEnv(with(tt.key, tt.value)); err == nil {
				t.Fatal("expected an error")
			}
		})
	}
}
