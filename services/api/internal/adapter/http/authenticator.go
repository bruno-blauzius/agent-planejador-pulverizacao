package httpadapter

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/domain"
	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/usecase"
)

// Authenticator identifies who is calling, from the session cookie or the
// Bearer token of the request (ADR 0015, D6). Implementations live in
// adapters (OIDC) and are injected by the composition root.
//
// Authenticate returns usecase.ErrUnauthenticated (wrapped is fine) for a missing,
// invalid or expired credential. Access failures after a valid identity
// (domain.ErrNoAccessToProduct, domain.ErrAmbiguousRole,
// usecase.ErrUserNotProvisioned, usecase.ErrAccessDenied) pass through
// unchanged and become 403; the identity provider being down is
// usecase.ErrIdentityProviderUnavailable (502). Anything else is a 500.
type Authenticator interface {
	Authenticate(r *http.Request) (domain.Principal, error)
}

type principalKey struct{}

// ContextWithPrincipal stores the authenticated principal in the context.
func ContextWithPrincipal(ctx context.Context, principal domain.Principal) context.Context {
	return context.WithValue(ctx, principalKey{}, principal)
}

// PrincipalFromContext returns the principal set by RequireAuth.
func PrincipalFromContext(ctx context.Context) (domain.Principal, bool) {
	principal, ok := ctx.Value(principalKey{}).(domain.Principal)
	return principal, ok
}

// RequireAuth authenticates every request before it reaches the handler. The
// handler reads the principal with PrincipalFromContext.
func RequireAuth(authenticator Authenticator, logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if authenticator == nil {
				// Fail closed: a router built without an authenticator never serves protected routes.
				writeFailure(w, r, logger, usecase.ErrUnauthenticated)
				return
			}
			principal, err := authenticator.Authenticate(r)
			if err != nil {
				writeFailure(w, r, logger, err)
				return
			}
			next.ServeHTTP(w, r.WithContext(ContextWithPrincipal(r.Context(), principal)))
		})
	}
}
