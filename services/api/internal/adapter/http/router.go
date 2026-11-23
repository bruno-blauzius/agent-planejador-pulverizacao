package httpadapter

import (
	"log/slog"
	"net/http"
	"slices"
	"sort"
	"strings"
	"time"
)

// Routes is what a group of handlers uses to attach its endpoints. Public
// routes need no credential; protected routes run behind RequireAuth.
type Routes interface {
	Public(method, path string, handler http.Handler)
	Protected(method, path string, handler http.Handler)
}

// RouteGroup is a set of endpoints that registers itself on the router.
type RouteGroup interface {
	Register(routes Routes)
}

// RouterDeps holds everything the router needs, wired in the composition root.
type RouterDeps struct {
	Logger       *slog.Logger
	Liveness     LivenessChecker
	Startup      StartupChecker
	Readiness    ReadinessChecker
	ProbeTimeout time.Duration // bounds the context passed to Readiness
	Spec         []byte        // openapi.yaml content; empty disables the docs routes

	// CORSAllowedOrigins are the exact origins allowed to call the API from a
	// browser, with credentials (the session cookie). Empty disables CORS.
	CORSAllowedOrigins []string
	// SessionCookieName is the cookie checked by the CSRF origin guard;
	// empty means DefaultSessionCookieName.
	SessionCookieName string

	// Authenticator protects the routes registered with Routes.Protected.
	Authenticator Authenticator
	// Groups are the business endpoints (users, invitations, companies, me,
	// auth). Nil entries are skipped.
	Groups []RouteGroup
}

// route is a registered pattern, kept to answer 405 for known paths.
type route struct {
	method   string
	segments []string
}

// router registers routes and remembers which methods each path accepts. It
// answers 404/405 itself, because ServeMux replies with plain text and every
// error of this API must use the JSON envelope.
type router struct {
	logger        *slog.Logger
	mux           *http.ServeMux
	authenticator Authenticator
	routes        []route
}

// NewRouter builds the HTTP handler with routes and middlewares.
func NewRouter(deps RouterDeps) http.Handler {
	routes := &router{
		logger:        deps.Logger,
		mux:           http.NewServeMux(),
		authenticator: deps.Authenticator,
	}

	routes.get("/health/live", livenessHandler(deps.Logger, deps.Liveness))
	routes.get("/health/startup", startupHandler(deps.Logger, deps.Startup))
	routes.get("/health/ready", readinessHandler(deps.Logger, deps.Readiness, deps.ProbeTimeout))
	if len(deps.Spec) > 0 {
		routes.get("/openapi.yaml", openAPIHandler(deps.Spec))
		routes.get("/docs", docsHandler())
	}
	for _, group := range deps.Groups {
		if group != nil {
			group.Register(routes)
		}
	}
	routes.mux.HandleFunc("/", routes.notFoundOrMethodNotAllowed)

	var handler http.Handler = routes.mux
	handler = cookieOriginGuard(deps.Logger, deps.SessionCookieName, deps.CORSAllowedOrigins, handler)
	handler = corsMiddleware(deps.CORSAllowedOrigins, handler)
	handler = accessLogMiddleware(deps.Logger, handler)
	handler = requestIDMiddleware(handler)
	return recoverMiddleware(deps.Logger, handler)
}

func (r *router) get(path string, handler http.HandlerFunc) {
	r.Public(http.MethodGet, path, handler)
}

// Public implements Routes.
func (r *router) Public(method, path string, handler http.Handler) {
	r.mux.Handle(method+" "+path, handler)
	r.routes = append(r.routes, route{method: method, segments: splitPath(path)})
}

// Protected implements Routes. Responses carry personal data, so no cache may keep them.
func (r *router) Protected(method, path string, handler http.Handler) {
	protected := RequireAuth(r.authenticator, r.logger)(noStore(handler))
	r.Public(method, path, protected)
}

func noStore(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		next.ServeHTTP(w, req)
	})
}

// notFoundOrMethodNotAllowed is the catch-all: a known path reaching it means
// the method did not match any registered pattern.
func (r *router) notFoundOrMethodNotAllowed(w http.ResponseWriter, req *http.Request) {
	allowed := r.allowedMethods(req.URL.Path)
	if len(allowed) == 0 {
		writeError(w, r.logger, http.StatusNotFound, "not_found", "resource not found")
		return
	}
	w.Header().Set("Allow", strings.Join(allowed, ", "))
	writeError(w, r.logger, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed")
}

// allowedMethods lists the methods of every registered pattern that matches the path.
func (r *router) allowedMethods(path string) []string {
	segments := splitPath(path)
	var methods []string
	for _, candidate := range r.routes {
		if matchesPattern(candidate.segments, segments) {
			methods = append(methods, candidate.method)
		}
	}
	sort.Strings(methods)
	return slices.Compact(methods)
}

// matchesPattern compares segment by segment; a {name} segment matches any non-empty value.
func matchesPattern(pattern, path []string) bool {
	if len(pattern) != len(path) {
		return false
	}
	for i, segment := range pattern {
		wildcard := strings.HasPrefix(segment, "{") && strings.HasSuffix(segment, "}")
		if wildcard && path[i] != "" {
			continue
		}
		if segment != path[i] {
			return false
		}
	}
	return true
}

func splitPath(path string) []string { return strings.Split(path, "/") }
