package httpadapter

import "net/http"

// Register attaches the browser login flow to the router. All four endpoints
// are public by nature: they are how a credential is obtained or ended, and
// each one authenticates itself (state and PKCE, Origin, signed logout token).
func (h *AuthHandlers) Register(routes Routes) {
	routes.Public(http.MethodGet, "/auth/login", http.HandlerFunc(h.Login))
	routes.Public(http.MethodGet, "/auth/callback", http.HandlerFunc(h.Callback))
	routes.Public(http.MethodPost, "/auth/logout", http.HandlerFunc(h.Logout))
	routes.Public(http.MethodPost, "/auth/backchannel-logout", http.HandlerFunc(h.BackchannelLogout))
}
