package httpadapter_test

import (
	"net/http"
	"testing"
)

// Every protected route answers 401 without a session, before anything else.
func TestProtectedRoutesRequireAuthentication(t *testing.T) {
	e := newAPIEnv(t)
	routes := []struct{ method, path string }{
		{http.MethodGet, "/auth/me"},
		{http.MethodGet, "/users"},
		{http.MethodGet, "/users/summary"},
		{http.MethodGet, "/users/" + operatorAID},
		{http.MethodPatch, "/users/" + operatorAID},
		{http.MethodDelete, "/users/" + operatorAID},
		{http.MethodPost, "/invitations"},
		{http.MethodPost, "/invitations/" + missingID + "/resend"},
		{http.MethodDelete, "/invitations/" + missingID},
		{http.MethodGet, "/companies"},
		{http.MethodPost, "/companies"},
		{http.MethodGet, "/companies/summary"},
		{http.MethodGet, "/companies/" + companyAID},
		{http.MethodPatch, "/companies/" + companyAID},
		{http.MethodDelete, "/companies/" + companyAID},
		{http.MethodGet, "/me/profile"},
		{http.MethodPatch, "/me/profile"},
	}
	for _, route := range routes {
		t.Run(route.method+" "+route.path, func(t *testing.T) {
			got := e.call("", route.method, route.path, nil)
			if got.status != http.StatusUnauthorized || got.errorCode(t) != "unauthorized" {
				t.Fatalf("status = %d, body %s", got.status, got.raw)
			}
			if got.header.Get("WWW-Authenticate") == "" {
				t.Error("401 must carry WWW-Authenticate")
			}
			if got.header.Get("X-Request-Id") == "" {
				t.Error("401 must carry the request id")
			}
		})
	}
	if unknownUser := e.call(missingID, http.MethodGet, "/users", nil); unknownUser.status != http.StatusUnauthorized {
		t.Errorf("unknown principal: status %d", unknownUser.status)
	}
}

// Access failures after a valid identity keep their own stable codes.
func TestAuthenticatorFailuresAreMapped(t *testing.T) {
	e := newAPIEnv(t)
	tests := []struct {
		injected string
		status   int
		code     string
	}{
		{"not_provisioned", 403, "user_not_provisioned"},
		{"no_access", 403, "no_access_to_product"},
		{"ambiguous", 403, "ambiguous_role"},
		{"inactive", 403, "forbidden"},
		{"provider_down", 502, "identity_provider_unavailable"},
		{"boom", 500, "internal_error"},
	}
	for _, tt := range tests {
		t.Run(tt.injected, func(t *testing.T) {
			got := e.call(adminAID, http.MethodGet, "/auth/me", nil, withHeader("X-Test-Auth-Error", tt.injected))
			if got.status != tt.status || got.errorCode(t) != tt.code {
				t.Fatalf("status = %d code %q, want %d %q", got.status, got.errorCode(t), tt.status, tt.code)
			}
			if contains(string(got.raw), "password") {
				t.Errorf("response leaked an internal message: %s", got.raw)
			}
		})
	}
	if !contains(e.logs.String(), "secret database password leaked") {
		t.Error("the internal cause must be logged for operators")
	}
}

func TestPublicRoutesStayPublic(t *testing.T) {
	e := newAPIEnv(t)
	for _, path := range []string{"/health/live", "/health/startup", "/health/ready"} {
		if got := e.call("", http.MethodGet, path, nil); got.status != http.StatusOK {
			t.Errorf("%s: status %d", path, got.status)
		}
	}
}

func TestMethodAndPathErrorsUseTheJSONEnvelope(t *testing.T) {
	e := newAPIEnv(t)
	tests := []struct {
		name, method, path string
		status             int
		code               string
		allow              string
	}{
		{"unknown path", http.MethodGet, "/nope", 404, "not_found", ""},
		{"known collection, wrong method", http.MethodDelete, "/companies", 405, "method_not_allowed", "GET, POST"},
		{"known item, wrong method", http.MethodPost, "/users/" + operatorAID, 405, "method_not_allowed", "DELETE, GET, PATCH"},
		{"literal path also matched by the item pattern", http.MethodPost, "/users/summary", 405, "method_not_allowed", "DELETE, GET, PATCH"},
		{"obsolete password route is not served", http.MethodPost, "/me/password", 404, "not_found", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := e.call(adminAID, tt.method, tt.path, nil)
			if got.status != tt.status || got.errorCode(t) != tt.code {
				t.Fatalf("status = %d code %q, body %s", got.status, got.errorCode(t), got.raw)
			}
			if got.header.Get("Allow") != tt.allow {
				t.Errorf("Allow = %q, want %q", got.header.Get("Allow"), tt.allow)
			}
		})
	}
}

func TestCORSAllowsCredentialsOnlyForListedOrigins(t *testing.T) {
	e := newAPIEnv(t)

	preflight := e.call("", http.MethodOptions, "/users", nil,
		withHeader("Origin", allowedOrigin), withHeader("Access-Control-Request-Method", "PATCH"))
	if preflight.status != http.StatusNoContent ||
		preflight.header.Get("Access-Control-Allow-Origin") != allowedOrigin ||
		preflight.header.Get("Access-Control-Allow-Credentials") != "true" {
		t.Errorf("preflight: status %d, headers %v", preflight.status, preflight.header)
	}

	allowed := e.call(adminAID, http.MethodGet, "/users", nil, withHeader("Origin", allowedOrigin))
	if allowed.header.Get("Access-Control-Allow-Credentials") != "true" || allowed.header.Get("Access-Control-Allow-Origin") != allowedOrigin {
		t.Errorf("allowed origin headers: %v", allowed.header)
	}

	foreign := e.call(adminAID, http.MethodGet, "/users", nil, withHeader("Origin", "https://evil.example"))
	if foreign.header.Get("Access-Control-Allow-Origin") != "" || foreign.header.Get("Access-Control-Allow-Credentials") != "" {
		t.Errorf("foreign origin must get no CORS headers: %v", foreign.header)
	}
}

func TestSessionCookieWritesNeedAListedOrigin(t *testing.T) {
	e := newAPIEnv(t)
	cookie := withHeader("Cookie", "agro_session=opaque")
	body := map[string]any{"name": "Novo Nome"}

	tests := []struct {
		name   string
		method string
		modify []func(*http.Request)
		want   int
	}{
		{"cookie write without origin", http.MethodPatch, []func(*http.Request){cookie}, 403},
		{"cookie write from a foreign origin", http.MethodPatch, []func(*http.Request){cookie, withHeader("Origin", "https://evil.example")}, 403},
		{"cookie write from a listed origin", http.MethodPatch, []func(*http.Request){cookie, withHeader("Origin", allowedOrigin)}, 200},
		{"cookie read needs no origin", http.MethodGet, []func(*http.Request){cookie}, 200},
		{"bearer write needs no origin", http.MethodPatch, []func(*http.Request){cookie, withHeader("Authorization", "Bearer token")}, 200},
		{"a non-bearer Authorization does not skip the origin check", http.MethodPatch, []func(*http.Request){cookie, withHeader("Authorization", "Basic abc")}, 403},
		{"write without cookie needs no origin", http.MethodPatch, nil, 200},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var payload any
			if tt.method != http.MethodGet {
				payload = body
			}
			got := e.call(operatorAID, tt.method, "/me/profile", payload, tt.modify...)
			if got.status != tt.want {
				t.Errorf("status = %d, want %d, body %s", got.status, tt.want, got.raw)
			}
			if tt.want == 403 && got.errorCode(t) != "forbidden" {
				t.Errorf("code = %q", got.errorCode(t))
			}
		})
	}
}
