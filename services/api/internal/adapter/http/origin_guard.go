package httpadapter

import (
	"log/slog"
	"net/http"
	"net/url"
	"strings"
)

// DefaultSessionCookieName is the cookie of the browser session (contract, securitySchemes.sessionCookie).
const DefaultSessionCookieName = "agro_session"

// cookieOriginGuard protects the cookie session against CSRF (ADR 0015, D6):
// a state-changing request that authenticates with the session cookie must
// come from an origin listed for CORS (the Origin header, or the origin of
// Referer when the browser sent none). Bearer clients (no cookie) and safe
// methods are not affected. SameSite=Lax on the cookie is the first barrier;
// this is the second.
func cookieOriginGuard(logger *slog.Logger, cookieName string, allowedOrigins []string, next http.Handler) http.Handler {
	if cookieName == "" {
		cookieName = DefaultSessionCookieName
	}
	allowed := make(map[string]struct{}, len(allowedOrigins))
	for _, origin := range allowedOrigins {
		allowed[origin] = struct{}{}
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isStateChanging(r.Method) && usesSessionCookie(r, cookieName) {
			if _, ok := allowed[requestOrigin(r)]; !ok {
				writeError(w, logger, http.StatusForbidden, codeForbidden, "origem não permitida para esta ação")
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func isStateChanging(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions, http.MethodTrace:
		return false
	default:
		return true
	}
}

// usesSessionCookie is true when the request carries the session cookie and no
// Bearer token. It must agree with the authenticator, where only the Bearer
// scheme takes precedence over the cookie: any other Authorization value does
// not stop the cookie from authenticating, so it must not skip the guard either.
func usesSessionCookie(r *http.Request, cookieName string) bool {
	if hasBearerToken(r) {
		return false
	}
	_, err := r.Cookie(cookieName)
	return err == nil
}

// hasBearerToken mirrors oidc.bearerToken: the Bearer scheme with a non-empty token.
func hasBearerToken(r *http.Request) bool {
	scheme, token, found := strings.Cut(strings.TrimSpace(r.Header.Get("Authorization")), " ")
	return found && strings.EqualFold(scheme, "Bearer") && strings.TrimSpace(token) != ""
}

// requestOrigin is the Origin header or, without it, the origin of Referer.
func requestOrigin(r *http.Request) string {
	if origin := r.Header.Get("Origin"); origin != "" {
		return origin
	}
	referer, err := url.Parse(r.Header.Get("Referer"))
	if err != nil || referer.Scheme == "" || referer.Host == "" {
		return ""
	}
	return referer.Scheme + "://" + referer.Host
}
