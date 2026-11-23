package httpadapter

import (
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/usecase"
)

// AuthCookies names and tunes the cookies of the flow. The composition root
// fills it from the OIDC configuration.
type AuthCookies struct {
	Session    string        // opaque session id (agro_session)
	Flow       string        // encrypted state of an ongoing login (agro_oidc)
	Reauth     string        // marks "login right after logout" (prompt=login)
	SessionTTL time.Duration // Max-Age of the session cookie
	FlowTTL    time.Duration // Max-Age of the flow cookie
}

// AuthHandlerDeps are the collaborators of AuthHandlers.
type AuthHandlerDeps struct {
	Logger         *slog.Logger
	Flow           usecase.AuthFlow
	Cookies        AuthCookies
	AllowedOrigins []string // same list as CORS; used as the CSRF check of logout
	// WebURL is the public base URL of the web front (no trailing slash needed).
	// The callback is a browser navigation, so it answers with redirects to
	// <WebURL><return_to> and <WebURL>/login?error=<code>. Empty keeps them
	// relative, which works when the front and the API share a host.
	WebURL string
}

// AuthHandlers serves /auth/login, /auth/callback, /auth/logout and
// /auth/backchannel-logout (ADR 0015, D6). /auth/me is served elsewhere.
type AuthHandlers struct {
	logger  *slog.Logger
	flow    usecase.AuthFlow
	cookies AuthCookies
	origins map[string]struct{}
	webURL  string
}

const (
	authCookiePath          = "/auth"
	reauthCookieLife        = 24 * time.Hour
	maxBackchannelBodyBytes = 64 << 10
	logoutTokenField        = "logout_token"
	maxAuthParamLength      = 4096
)

// NewAuthHandlers builds the handlers.
func NewAuthHandlers(deps AuthHandlerDeps) *AuthHandlers {
	origins := make(map[string]struct{}, len(deps.AllowedOrigins))
	for _, origin := range deps.AllowedOrigins {
		origins[origin] = struct{}{}
	}
	return &AuthHandlers{
		logger: deps.Logger, flow: deps.Flow, cookies: deps.Cookies, origins: origins,
		webURL: strings.TrimRight(deps.WebURL, "/"),
	}
}

// Login handles GET /auth/login: validates return_to and redirects to Keycloak.
func (h *AuthHandlers) Login(w http.ResponseWriter, r *http.Request) {
	setNoStore(w)
	_, forceLogin := cookieValue(r, h.cookies.Reauth)
	authorizationURL, flowCookie, err := h.flow.StartLogin(r.Context(), r.URL.Query().Get("return_to"), forceLogin)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	http.SetCookie(w, h.cookie(h.cookies.Flow, flowCookie, authCookiePath, h.cookies.FlowTTL))
	redirect(w, authorizationURL)
}

// Callback handles GET /auth/callback: finishes the login and sets the session
// cookie. It is a browser navigation, so a failure redirects to the login screen
// of the front (/login?error=<code>) instead of answering JSON.
func (h *AuthHandlers) Callback(w http.ResponseWriter, r *http.Request) {
	setNoStore(w)
	// The flow cookie is single use, whatever the outcome.
	http.SetCookie(w, h.expiredCookie(h.cookies.Flow, authCookiePath))

	query := r.URL.Query()
	if query.Get("error") != "" {
		// The user cancelled or Keycloak refused; the details stay out of the answer.
		h.failLogin(w, r, usecase.ErrLoginRejected)
		return
	}
	code, state := query.Get("code"), query.Get("state")
	if len(code) > maxAuthParamLength || len(state) > maxAuthParamLength {
		h.failLogin(w, r, usecase.ErrLoginRejected)
		return
	}
	flowCookie, _ := cookieValue(r, h.cookies.Flow)
	sessionToken, returnTo, err := h.flow.FinishLogin(r.Context(), code, state, flowCookie)
	if err != nil {
		h.failLogin(w, r, err)
		return
	}
	http.SetCookie(w, h.cookie(h.cookies.Session, sessionToken, "/", h.cookies.SessionTTL))
	http.SetCookie(w, h.expiredCookie(h.cookies.Reauth, authCookiePath))
	// returnTo was validated as a relative path at /auth/login; the base is fixed configuration.
	redirect(w, h.webURL+returnTo)
}

// Logout handles POST /auth/logout: CSRF check by Origin, then ends the product
// session. It never ends the SSO session; the next login asks for the password again.
func (h *AuthHandlers) Logout(w http.ResponseWriter, r *http.Request) {
	setNoStore(w)
	if !h.originAllowed(r) {
		writeError(w, h.logger, http.StatusForbidden, codeForbidden, "origem não permitida para esta ação")
		return
	}
	value, _ := cookieValue(r, h.cookies.Session)
	if err := h.flow.Logout(r.Context(), value); err != nil {
		h.fail(w, r, err)
		return
	}
	http.SetCookie(w, h.expiredCookie(h.cookies.Session, "/"))
	http.SetCookie(w, h.cookie(h.cookies.Reauth, "1", authCookiePath, reauthCookieLife))
	w.WriteHeader(http.StatusNoContent)
}

// BackchannelLogout handles POST /auth/backchannel-logout, called by Keycloak.
// Authenticity comes from the signed logout_token, not from cookies or Origin.
func (h *AuthHandlers) BackchannelLogout(w http.ResponseWriter, r *http.Request) {
	setNoStore(w)
	r.Body = http.MaxBytesReader(w, r.Body, maxBackchannelBodyBytes)
	if err := r.ParseForm(); err != nil {
		h.invalidLogoutToken(w)
		return
	}
	err := h.flow.BackchannelLogout(r.Context(), r.PostForm.Get(logoutTokenField))
	switch {
	case errors.Is(err, usecase.ErrInvalidLogoutToken):
		h.invalidLogoutToken(w)
	case err != nil:
		h.fail(w, r, err)
	default:
		w.WriteHeader(http.StatusOK)
	}
}

func (h *AuthHandlers) invalidLogoutToken(w http.ResponseWriter) {
	writeError(w, h.logger, http.StatusBadRequest, codeValidationFailed, "logout_token inválido")
}

// fail answers a flow error with the contract's envelope.
func (h *AuthHandlers) fail(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, usecase.ErrInvalidReturnTo):
		err = validationError("dados inválidos", fieldDetail{Field: "return_to", Message: "use um caminho relativo da aplicação, como /farms"})
	case errors.Is(err, usecase.ErrLoginRejected):
		err = validationError("login inválido ou expirado; tente entrar de novo")
	}
	writeFailure(w, r, h.logger, err)
}

// Codes the login screen of the front understands in /login?error=<code>.
// loginFailedCode is the front-only "unavailable" reason: the honest answer for
// a rejected or expired login and for internal failures, with no detail leaked.
const loginFailedCode = "unavailable"

var loginScreenCodes = map[string]struct{}{
	"user_not_provisioned":  {},
	"no_access_to_product":  {},
	"ambiguous_role":        {},
	codeForbidden:           {},
	codeProviderUnavailable: {},
}

// failLogin sends the browser to the login screen with the reason as a code.
// The session is never created here and internal causes are only logged.
func (h *AuthHandlers) failLogin(w http.ResponseWriter, r *http.Request, err error) {
	failure := classify(err)
	if failure.Status >= http.StatusInternalServerError && failure.Code != codeProviderUnavailable {
		h.logger.ErrorContext(r.Context(), "login callback failed",
			"error", err, "request_id", RequestIDFromContext(r.Context()))
	}
	code := loginFailedCode
	if _, known := loginScreenCodes[failure.Code]; known {
		code = failure.Code
	}
	redirect(w, h.webURL+"/login?error="+url.QueryEscape(code))
}

// originAllowed is the CSRF check of cookie writes: Origin, or Referer when the
// browser sent no Origin, must be one of the allowed origins.
func (h *AuthHandlers) originAllowed(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		referer, err := url.Parse(r.Header.Get("Referer"))
		if err != nil || referer.Scheme == "" || referer.Host == "" {
			return false
		}
		origin = referer.Scheme + "://" + referer.Host
	}
	_, ok := h.origins[origin]
	return ok
}

func (h *AuthHandlers) cookie(name, value, path string, ttl time.Duration) *http.Cookie {
	return h.newCookie(name, value, path, int(ttl.Seconds()))
}

func (h *AuthHandlers) expiredCookie(name, path string) *http.Cookie {
	return h.newCookie(name, "", path, -1)
}

// newCookie builds a cookie that is always HttpOnly, Secure and SameSite=Lax.
// Browsers treat http://localhost as a secure context (Safari excepted), so
// there is no switch to turn Secure off.
func (h *AuthHandlers) newCookie(name, value, path string, maxAge int) *http.Cookie {
	return &http.Cookie{
		Name: name, Value: value, Path: path, MaxAge: maxAge,
		HttpOnly: true, Secure: true, SameSite: http.SameSiteLaxMode,
	}
}

func cookieValue(r *http.Request, name string) (string, bool) {
	cookie, err := r.Cookie(name)
	if err != nil || cookie.Value == "" {
		return "", false
	}
	return cookie.Value, true
}

func setNoStore(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")
}

// redirect answers 302 without the HTML body http.Redirect adds.
func redirect(w http.ResponseWriter, location string) {
	w.Header().Set("Location", location)
	w.WriteHeader(http.StatusFound)
}
