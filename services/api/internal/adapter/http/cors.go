package httpadapter

import (
	"net/http"
	"strings"
)

const (
	corsAllowedMethods = "GET, POST, PUT, PATCH, DELETE, OPTIONS"
	corsAllowedHeaders = "Authorization, Content-Type, " + requestIDHeader
	corsExposedHeaders = requestIDHeader + ", " + invitationEmailHeader
	corsPreflightTTL   = "600" // seconds the browser may cache a preflight answer
)

// corsMiddleware lets browsers from the allowed origins call the API. Origins
// are matched exactly; an empty list disables CORS and the middleware becomes
// a pass-through. Allowed origins may send credentials (the session cookie). Requests from other origins get no CORS headers, so the
// browser blocks them while server-side clients keep working.
func corsMiddleware(allowedOrigins []string, next http.Handler) http.Handler {
	if len(allowedOrigins) == 0 {
		return next
	}
	allowed := make(map[string]struct{}, len(allowedOrigins))
	for _, origin := range allowedOrigins {
		allowed[origin] = struct{}{}
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// The response depends on Origin, so caches must key on it.
		w.Header().Add("Vary", "Origin")

		origin := r.Header.Get("Origin")
		if _, ok := allowed[origin]; !ok {
			next.ServeHTTP(w, r)
			return
		}

		w.Header().Set("Access-Control-Allow-Origin", origin)
		// Credentials are needed for the session cookie; the origin is always
		// an exact match from the list, never a wildcard.
		w.Header().Set("Access-Control-Allow-Credentials", "true")
		w.Header().Set("Access-Control-Expose-Headers", corsExposedHeaders)

		if isPreflight(r) {
			w.Header().Add("Vary", "Access-Control-Request-Method")
			w.Header().Add("Vary", "Access-Control-Request-Headers")
			w.Header().Set("Access-Control-Allow-Methods", corsAllowedMethods)
			w.Header().Set("Access-Control-Allow-Headers", corsAllowedHeaders)
			w.Header().Set("Access-Control-Max-Age", corsPreflightTTL)
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func isPreflight(r *http.Request) bool {
	return r.Method == http.MethodOptions &&
		strings.TrimSpace(r.Header.Get("Access-Control-Request-Method")) != ""
}
