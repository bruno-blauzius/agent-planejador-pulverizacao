package oidc

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	httpadapter "github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/adapter/http"
	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/usecase"
)

const webOrigin = "http://web.example.test"

// browser is a tiny cookie jar plus helpers to drive the /auth handlers the
// way a browser would.
type browser struct {
	t       *testing.T
	mux     *http.ServeMux
	cookies map[string]*http.Cookie
}

func newBrowser(t *testing.T, h *harness) *browser {
	t.Helper()
	cfg := h.service.Config()
	handlers := httpadapter.NewAuthHandlers(httpadapter.AuthHandlerDeps{
		Logger: slog.New(slog.NewTextHandler(testWriter{t}, nil)), Flow: h.service,
		Cookies: httpadapter.AuthCookies{
			Session: cfg.SessionCookieName, Flow: cfg.FlowCookieName, Reauth: cfg.ReauthCookieName,
			SessionTTL: cfg.SessionTTL, FlowTTL: cfg.FlowTTL,
		},
		AllowedOrigins: []string{webOrigin},
		WebURL:         webOrigin + "/",
	})
	mux := http.NewServeMux()
	mux.HandleFunc("GET /auth/login", handlers.Login)
	mux.HandleFunc("GET /auth/callback", handlers.Callback)
	mux.HandleFunc("POST /auth/logout", handlers.Logout)
	mux.HandleFunc("POST /auth/backchannel-logout", handlers.BackchannelLogout)
	return &browser{t: t, mux: mux, cookies: map[string]*http.Cookie{}}
}

type testWriter struct{ t *testing.T }

func (w testWriter) Write(p []byte) (int, error) {
	w.t.Log(strings.TrimSpace(string(p)))
	return len(p), nil
}

func (b *browser) do(r *http.Request) *httptest.ResponseRecorder {
	b.t.Helper()
	for _, cookie := range b.cookies {
		r.AddCookie(cookie)
	}
	recorder := httptest.NewRecorder()
	b.mux.ServeHTTP(recorder, r)
	for _, set := range recorder.Result().Cookies() {
		if set.MaxAge < 0 {
			delete(b.cookies, set.Name)
			continue
		}
		b.cookies[set.Name] = &http.Cookie{Name: set.Name, Value: set.Value}
	}
	return recorder
}

func (b *browser) get(target string) *httptest.ResponseRecorder {
	return b.do(httptest.NewRequestWithContext(background(), http.MethodGet, target, http.NoBody))
}

func (b *browser) post(target, origin string) *httptest.ResponseRecorder {
	r := httptest.NewRequestWithContext(background(), http.MethodPost, target, http.NoBody)
	if origin != "" {
		r.Header.Set("Origin", origin)
	}
	return b.do(r)
}

func errorCode(t *testing.T, recorder *httptest.ResponseRecorder) string {
	t.Helper()
	var body struct {
		Error struct{ Code string } `json:"error"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("body is not the error envelope: %q", recorder.Body.String())
	}
	return body.Error.Code
}

func cookieNamed(recorder *httptest.ResponseRecorder, name string) *http.Cookie {
	for _, cookie := range recorder.Result().Cookies() {
		if cookie.Name == name {
			return cookie
		}
	}
	return nil
}

// signIn runs login + callback through the handlers and returns the callback response.
func (b *browser) signIn(h *harness, g grant, returnTo string) *httptest.ResponseRecorder {
	b.t.Helper()
	login := b.get("/auth/login?return_to=" + url.QueryEscape(returnTo))
	if login.Code != http.StatusFound {
		b.t.Fatalf("login status = %d body=%s", login.Code, login.Body.String())
	}
	code, state := h.idp.authorize(login.Header().Get("Location"), g)
	return b.get("/auth/callback?code=" + url.QueryEscape(code) + "&state=" + url.QueryEscape(state))
}

func TestHTTPLoginFlowEndToEnd(t *testing.T) {
	h := newHarness(t)
	b := newBrowser(t, h)

	login := b.get("/auth/login?return_to=/farms")
	if login.Code != http.StatusFound || login.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("login: %d %v", login.Code, login.Header())
	}
	flow := cookieNamed(login, "agro_oidc")
	if flow == nil || !flow.HttpOnly || !flow.Secure || flow.SameSite != http.SameSiteLaxMode || flow.Path != "/auth" || flow.MaxAge != int((10*time.Minute).Seconds()) {
		t.Fatalf("flow cookie = %+v", flow)
	}

	code, state := h.idp.authorize(login.Header().Get("Location"), adminGrant())
	callback := b.get("/auth/callback?code=" + code + "&state=" + state)
	if callback.Code != http.StatusFound || callback.Header().Get("Location") != webOrigin+"/farms" {
		t.Fatalf("callback: %d location=%q body=%s", callback.Code, callback.Header().Get("Location"), callback.Body.String())
	}
	session := cookieNamed(callback, "agro_session")
	if session == nil || !session.HttpOnly || !session.Secure || session.SameSite != http.SameSiteLaxMode || session.Path != "/" || session.MaxAge != int(h.cfg.SessionTTL.Seconds()) {
		t.Fatalf("session cookie = %+v", session)
	}
	if cleared := cookieNamed(callback, "agro_oidc"); cleared == nil || cleared.MaxAge >= 0 {
		t.Fatal("the flow cookie must be cleared after the callback")
	}
	if callback.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("callback must not be cached")
	}

	principal, err := h.auth.Authenticate(h.request(session.Value))
	if err != nil || principal.UserID != testUserID {
		t.Fatalf("session does not authenticate: %v", err)
	}
}

func TestHTTPLoginRejectsOpenRedirect(t *testing.T) {
	h := newHarness(t)
	b := newBrowser(t, h)
	for _, target := range []string{"//evil.com", "https://evil.com/x", "/\\evil.com", "javascript:alert(1)"} {
		response := b.get("/auth/login?return_to=" + url.QueryEscape(target))
		if response.Code != http.StatusUnprocessableEntity || errorCode(t, response) != "validation_failed" {
			t.Errorf("return_to %q: %d %s", target, response.Code, response.Body.String())
		}
		if response.Header().Get("Location") != "" || cookieNamed(response, "agro_oidc") != nil {
			t.Errorf("return_to %q: nothing may be redirected or stored", target)
		}
	}
}

func TestHTTPCallbackRejectionsRedirectToLoginWithoutSession(t *testing.T) {
	h := newHarness(t)
	b := newBrowser(t, h)
	login := b.get("/auth/login")
	code, _ := h.idp.authorize(login.Header().Get("Location"), adminGrant())

	for name, target := range map[string]string{
		"forged state":   "/auth/callback?code=" + code + "&state=forged",
		"no parameters":  "/auth/callback",
		"provider error": "/auth/callback?error=access_denied&state=x",
		"huge code":      "/auth/callback?code=" + strings.Repeat("a", 5000) + "&state=x",
	} {
		response := b.get(target)
		if response.Code != http.StatusFound || response.Header().Get("Location") != webOrigin+"/login?error=unavailable" {
			t.Errorf("%s: %d location=%q body=%s", name, response.Code, response.Header().Get("Location"), response.Body.String())
		}
		if cookieNamed(response, "agro_session") != nil {
			t.Errorf("%s: a session cookie was set", name)
		}
		if cleared := cookieNamed(response, "agro_oidc"); cleared == nil || cleared.MaxAge >= 0 {
			t.Errorf("%s: the flow cookie must be discarded", name)
		}
	}
	if h.sessions.count() != 0 {
		t.Fatal("a session was created")
	}
}

func TestHTTPCallbackProductAccessFailures(t *testing.T) {
	tests := []struct {
		name  string
		setup func(h *harness) grant
		want  string
	}{
		{"not provisioned", func(h *harness) grant { h.resolver.err = usecase.ErrUserNotProvisioned; return adminGrant() }, "user_not_provisioned"},
		{"no product role", func(*harness) grant { g := adminGrant(); g.roles = []string{"offline_access"}; return g }, "no_access_to_product"},
		{"ambiguous role", func(*harness) grant { g := adminGrant(); g.roles = []string{"admin", "super_admin"}; return g }, "ambiguous_role"},
		{"inactive", func(h *harness) grant { h.resolver.err = usecase.ErrAccessDenied; return adminGrant() }, "forbidden"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newHarness(t)
			b := newBrowser(t, h)
			response := b.signIn(h, tt.setup(h), "/")
			if response.Code != http.StatusFound || response.Header().Get("Location") != webOrigin+"/login?error="+tt.want {
				t.Fatalf("%d location=%q body=%s", response.Code, response.Header().Get("Location"), response.Body.String())
			}
			if cookieNamed(response, "agro_session") != nil || h.sessions.count() != 0 {
				t.Fatal("no session may exist")
			}
		})
	}
}

func TestHTTPCallbackProviderOutageRedirectsToLogin(t *testing.T) {
	h := newHarness(t)
	b := newBrowser(t, h)
	login := b.get("/auth/login")
	code, state := h.idp.authorize(login.Header().Get("Location"), adminGrant())
	h.idp.srv.Close()
	response := b.get("/auth/callback?code=" + code + "&state=" + state)
	if response.Code != http.StatusFound || response.Header().Get("Location") != webOrigin+"/login?error=identity_provider_unavailable" {
		t.Fatalf("%d location=%q body=%s", response.Code, response.Header().Get("Location"), response.Body.String())
	}
	if cookieNamed(response, "agro_session") != nil {
		t.Fatal("a session cookie was set")
	}
}

func TestHTTPLogoutThenLoginForcesPasswordAgain(t *testing.T) {
	h := newHarness(t)
	b := newBrowser(t, h)
	b.signIn(h, adminGrant(), "/")
	session := b.cookies["agro_session"].Value

	logout := b.post("/auth/logout", webOrigin)
	if logout.Code != http.StatusNoContent {
		t.Fatalf("logout: %d %s", logout.Code, logout.Body.String())
	}
	if cleared := cookieNamed(logout, "agro_session"); cleared == nil || cleared.MaxAge >= 0 || !cleared.HttpOnly {
		t.Fatalf("session cookie not cleared: %+v", cleared)
	}
	if marker := cookieNamed(logout, "agro_reauth"); marker == nil || marker.MaxAge <= 0 || !marker.HttpOnly {
		t.Fatalf("reauth marker missing: %+v", marker)
	}
	if _, err := h.auth.Authenticate(h.request(session)); err == nil {
		t.Fatal("session still valid after logout")
	}
	if _, _, revokes := h.idp.counts(); revokes != 1 {
		t.Fatal("refresh token not revoked at the provider")
	}

	login := b.get("/auth/login")
	if !strings.Contains(login.Header().Get("Location"), "prompt=login") {
		t.Fatalf("login after logout must force prompt=login: %s", login.Header().Get("Location"))
	}
	// A successful login consumes the marker.
	code, state := h.idp.authorize(login.Header().Get("Location"), adminGrant())
	b.get("/auth/callback?code=" + code + "&state=" + state)
	if again := b.get("/auth/login"); strings.Contains(again.Header().Get("Location"), "prompt=login") {
		t.Fatal("the marker must be consumed by a successful login")
	}
}

func TestHTTPLogoutChecksOrigin(t *testing.T) {
	h := newHarness(t)
	b := newBrowser(t, h)
	b.signIn(h, adminGrant(), "/")
	session := b.cookies["agro_session"].Value

	for name, origin := range map[string]string{"foreign": "http://evil.example", "null": "null", "missing": "", "lookalike": webOrigin + ".evil.example"} {
		response := b.post("/auth/logout", origin)
		if response.Code != http.StatusForbidden || errorCode(t, response) != "forbidden" {
			t.Errorf("%s origin: %d", name, response.Code)
		}
	}
	if _, err := h.auth.Authenticate(h.request(session)); err != nil {
		t.Fatalf("a refused CSRF attempt must not end the session: %v", err)
	}

	// Referer is the fallback when the browser sends no Origin.
	r := httptest.NewRequestWithContext(background(), http.MethodPost, "/auth/logout", http.NoBody)
	r.Header.Set("Referer", webOrigin+"/farms")
	if response := b.do(r); response.Code != http.StatusNoContent {
		t.Fatalf("referer fallback: %d", response.Code)
	}
	r = httptest.NewRequestWithContext(background(), http.MethodPost, "/auth/logout", http.NoBody)
	r.Header.Set("Referer", "http://evil.example/farms")
	if response := b.do(r); response.Code != http.StatusForbidden {
		t.Fatalf("foreign referer: %d", response.Code)
	}
}

func TestHTTPLogoutIsIdempotent(t *testing.T) {
	h := newHarness(t)
	b := newBrowser(t, h)
	for i := 0; i < 2; i++ {
		if response := b.post("/auth/logout", webOrigin); response.Code != http.StatusNoContent {
			t.Fatalf("logout #%d without a session: %d", i+1, response.Code)
		}
	}
}

func TestHTTPBackchannelLogout(t *testing.T) {
	h := newHarness(t)
	b := newBrowser(t, h)
	b.signIn(h, adminGrant(), "/")
	session := b.cookies["agro_session"].Value

	post := func(body, contentType string) *httptest.ResponseRecorder {
		r := httptest.NewRequestWithContext(background(), http.MethodPost, "/auth/backchannel-logout", strings.NewReader(body))
		r.Header.Set("Content-Type", contentType)
		recorder := httptest.NewRecorder()
		b.mux.ServeHTTP(recorder, r)
		return recorder
	}
	form := "application/x-www-form-urlencoded"

	for name, body := range map[string]string{
		"missing token":  "",
		"forged token":   "logout_token=" + signJWT(t, h.idp.key, "none", h.idp.logoutClaims("sid-1", "")),
		"garbage":        "logout_token=garbage",
		"oversized body": "logout_token=" + strings.Repeat("a", 70<<10),
	} {
		response := post(body, form)
		if response.Code != http.StatusBadRequest || errorCode(t, response) != "validation_failed" {
			t.Errorf("%s: %d %s", name, response.Code, response.Body.String())
		}
	}
	if _, err := h.auth.Authenticate(h.request(session)); err != nil {
		t.Fatalf("invalid tokens must not end sessions: %v", err)
	}

	valid := "logout_token=" + h.idp.sign(h.idp.logoutClaims("sid-1", ""))
	if response := post(valid, form); response.Code != http.StatusOK || response.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("valid token: %d %v", response.Code, response.Header())
	}
	if _, err := h.auth.Authenticate(h.request(session)); err == nil {
		t.Fatal("the session must be gone after the back-channel logout")
	}
	// Needs no cookie, no Origin: Keycloak calls it server to server.
	if response := post(valid, form); response.Code != http.StatusOK {
		t.Fatalf("repeat: %d", response.Code)
	}
}
