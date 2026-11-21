package httpadapter

import (
	"log/slog"
	"net/http"
	"sort"
	"strings"
	"time"
)

// RouterDeps holds everything the router needs, wired in the composition root.
type RouterDeps struct {
	Logger       *slog.Logger
	Liveness     LivenessChecker
	Startup      StartupChecker
	Readiness    ReadinessChecker
	ProbeTimeout time.Duration // bounds the context passed to Readiness
	Spec         []byte        // openapi.yaml content; empty disables the docs routes
}

// router registers GET routes and remembers which methods each path accepts.
// It answers 404/405 itself, because ServeMux replies with plain text and
// every error of this API must use the JSON envelope.
type router struct {
	logger  *slog.Logger
	mux     *http.ServeMux
	methods map[string][]string
}

// NewRouter builds the HTTP handler with routes and middlewares.
func NewRouter(deps RouterDeps) http.Handler {
	routes := &router{
		logger:  deps.Logger,
		mux:     http.NewServeMux(),
		methods: map[string][]string{},
	}

	routes.get("/health/live", livenessHandler(deps.Logger, deps.Liveness))
	routes.get("/health/startup", startupHandler(deps.Logger, deps.Startup))
	routes.get("/health/ready", readinessHandler(deps.Logger, deps.Readiness, deps.ProbeTimeout))
	if len(deps.Spec) > 0 {
		routes.get("/openapi.yaml", openAPIHandler(deps.Spec))
		routes.get("/docs", docsHandler())
	}
	routes.mux.HandleFunc("/", routes.notFoundOrMethodNotAllowed)

	var handler http.Handler = routes.mux
	handler = accessLogMiddleware(deps.Logger, handler)
	handler = requestIDMiddleware(handler)
	return recoverMiddleware(deps.Logger, handler)
}

func (r *router) get(path string, handler http.HandlerFunc) {
	r.mux.HandleFunc("GET "+path, handler)
	r.methods[path] = append(r.methods[path], http.MethodGet)
}

// notFoundOrMethodNotAllowed is the catch-all: a known path reaching it means
// the method did not match any registered pattern.
func (r *router) notFoundOrMethodNotAllowed(w http.ResponseWriter, req *http.Request) {
	allowed, known := r.methods[req.URL.Path]
	if !known {
		writeError(w, r.logger, http.StatusNotFound, "not_found", "resource not found")
		return
	}
	sort.Strings(allowed)
	w.Header().Set("Allow", strings.Join(allowed, ", "))
	writeError(w, r.logger, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed")
}
