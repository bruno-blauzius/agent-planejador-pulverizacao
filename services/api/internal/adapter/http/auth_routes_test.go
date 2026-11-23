package httpadapter_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	httpadapter "github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/adapter/http"
	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/usecase"
)

type stubAuthFlow struct{}

func (stubAuthFlow) StartLogin(context.Context, string, bool) (string, string, error) {
	return "http://idp.test/auth?state=x", "flow-cookie", nil
}

func (stubAuthFlow) FinishLogin(context.Context, string, string, string) (string, string, error) {
	return "session-token", "/farms", nil
}
func (stubAuthFlow) Logout(context.Context, string) error { return nil }
func (stubAuthFlow) BackchannelLogout(_ context.Context, token string) error {
	if token == "" {
		return usecase.ErrInvalidLogoutToken
	}
	return nil
}

// The /auth/* endpoints are registered as public routes next to the protected ones.
func TestAuthRoutesAreRegisteredAndPublic(t *testing.T) {
	handler := httpadapter.NewRouter(httpadapter.RouterDeps{
		Logger:             discardLogger(),
		Liveness:           fakeProbe{report: up},
		Startup:            fakeProbe{report: up},
		Readiness:          fakeReadiness{report: up},
		CORSAllowedOrigins: []string{allowedOrigin},
		Groups: []httpadapter.RouteGroup{httpadapter.NewAuthHandlers(httpadapter.AuthHandlerDeps{
			Logger:         discardLogger(),
			Flow:           stubAuthFlow{},
			Cookies:        httpadapter.AuthCookies{Session: "agro_session", Flow: "agro_oidc", Reauth: "agro_reauth"},
			AllowedOrigins: []string{allowedOrigin},
		})},
	})

	tests := []struct {
		name, method, path, body string
		headers                  map[string]string
		want                     int
	}{
		{"login redirects to the provider", http.MethodGet, "/auth/login", "", nil, http.StatusFound},
		{"callback sets the session and redirects", http.MethodGet, "/auth/callback?code=c&state=s", "", nil, http.StatusFound},
		{"logout from a listed origin", http.MethodPost, "/auth/logout", "", map[string]string{"Origin": allowedOrigin}, http.StatusNoContent},
		{"logout from a foreign origin", http.MethodPost, "/auth/logout", "", map[string]string{"Origin": "https://evil.example"}, http.StatusForbidden},
		{"back-channel logout without a token", http.MethodPost, "/auth/backchannel-logout", "logout_token=", map[string]string{"Content-Type": "application/x-www-form-urlencoded"}, http.StatusBadRequest},
		{"back-channel logout with a token", http.MethodPost, "/auth/backchannel-logout", "logout_token=abc", map[string]string{"Content-Type": "application/x-www-form-urlencoded"}, http.StatusOK},
		{"wrong method on login", http.MethodDelete, "/auth/login", "", nil, http.StatusMethodNotAllowed},
		{"/auth/me stays protected (not served by this group)", http.MethodGet, "/auth/me", "", nil, http.StatusNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request := httptest.NewRequestWithContext(context.Background(), tt.method, tt.path, strings.NewReader(tt.body))
			for name, value := range tt.headers {
				request.Header.Set(name, value)
			}
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, request)
			if recorder.Code != tt.want {
				t.Errorf("status = %d, want %d, body %s", recorder.Code, tt.want, recorder.Body)
			}
		})
	}
}
