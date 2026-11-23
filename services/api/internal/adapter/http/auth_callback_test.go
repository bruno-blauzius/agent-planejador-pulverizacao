package httpadapter_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	httpadapter "github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/adapter/http"
	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/domain"
	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/usecase"
)

// callbackFlow answers FinishLogin with a fixed outcome.
type callbackFlow struct {
	stubAuthFlow
	returnTo string
	err      error
}

func (f callbackFlow) FinishLogin(context.Context, string, string, string) (string, string, error) {
	if f.err != nil {
		return "", "", f.err
	}
	return "session-token", f.returnTo, nil
}

func callbackRecorder(t *testing.T, webURL string, flow callbackFlow, query string) *httptest.ResponseRecorder {
	t.Helper()
	handlers := httpadapter.NewAuthHandlers(httpadapter.AuthHandlerDeps{
		Logger:  discardLogger(),
		Flow:    flow,
		Cookies: httpadapter.AuthCookies{Session: "agro_session", Flow: "agro_oidc", Reauth: "agro_reauth"},
		WebURL:  webURL,
	})
	request := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/auth/callback"+query, nil)
	recorder := httptest.NewRecorder()
	handlers.Callback(recorder, request)
	return recorder
}

func TestCallbackSuccessRedirectsToTheFrontBase(t *testing.T) {
	tests := []struct {
		name, webURL, returnTo, want string
	}{
		{"absolute front url", "http://localhost:3000", "/farms", "http://localhost:3000/farms"},
		{"trailing slash is trimmed", "http://localhost:3000/", "/farms?tab=1", "http://localhost:3000/farms?tab=1"},
		{"root", "https://app.example.test", "/", "https://app.example.test/"},
		{"no web url keeps it relative", "", "/farms", "/farms"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			response := callbackRecorder(t, tt.webURL, callbackFlow{returnTo: tt.returnTo}, "?code=c&state=s")
			if response.Code != http.StatusFound || response.Header().Get("Location") != tt.want {
				t.Fatalf("status %d, location %q, want %q", response.Code, response.Header().Get("Location"), tt.want)
			}
			if !strings.Contains(strings.Join(response.Header().Values("Set-Cookie"), ";"), "agro_session=session-token") {
				t.Error("the session cookie must be set on success")
			}
		})
	}
}

func TestCallbackFailuresRedirectToTheLoginScreen(t *testing.T) {
	tests := []struct {
		name, webURL, query string
		err                 error
		want                string
	}{
		{"not provisioned", "http://localhost:3000", "?code=c&state=s", usecase.ErrUserNotProvisioned, "http://localhost:3000/login?error=user_not_provisioned"},
		{"no access", "http://localhost:3000", "?code=c&state=s", domain.ErrNoAccessToProduct, "http://localhost:3000/login?error=no_access_to_product"},
		{"ambiguous role", "http://localhost:3000", "?code=c&state=s", domain.ErrAmbiguousRole, "http://localhost:3000/login?error=ambiguous_role"},
		{"inactive", "http://localhost:3000", "?code=c&state=s", usecase.ErrAccessDenied, "http://localhost:3000/login?error=forbidden"},
		{"no permission", "http://localhost:3000", "?code=c&state=s", usecase.ErrForbidden, "http://localhost:3000/login?error=forbidden"},
		{"provider down", "http://localhost:3000", "?code=c&state=s", usecase.ErrIdentityProviderUnavailable, "http://localhost:3000/login?error=identity_provider_unavailable"},
		{"login rejected", "http://localhost:3000", "?code=c&state=s", usecase.ErrLoginRejected, "http://localhost:3000/login?error=unavailable"},
		{"unknown failure leaks nothing", "http://localhost:3000", "?code=c&state=s", errors.New("secret database password"), "http://localhost:3000/login?error=unavailable"},
		{"provider error parameter", "http://localhost:3000", "?error=access_denied&state=x", nil, "http://localhost:3000/login?error=unavailable"},
		{"relative without web url", "", "?code=c&state=s", usecase.ErrUserNotProvisioned, "/login?error=user_not_provisioned"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			response := callbackRecorder(t, tt.webURL, callbackFlow{err: tt.err}, tt.query)
			if response.Code != http.StatusFound || response.Header().Get("Location") != tt.want {
				t.Fatalf("status %d, location %q, want %q", response.Code, response.Header().Get("Location"), tt.want)
			}
			cookies := strings.Join(response.Header().Values("Set-Cookie"), ";")
			if strings.Contains(cookies, "agro_session=") {
				t.Errorf("no session cookie may be set: %s", cookies)
			}
			if !strings.Contains(cookies, "agro_oidc=") {
				t.Errorf("the flow cookie must be discarded: %s", cookies)
			}
			if strings.Contains(response.Body.String(), "secret") || response.Body.Len() != 0 {
				t.Errorf("the body must be empty: %q", response.Body.String())
			}
		})
	}
}
