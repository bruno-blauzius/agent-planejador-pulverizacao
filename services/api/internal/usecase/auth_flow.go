package usecase

import (
	"context"
	"errors"
)

// Errors of the authentication flow. The OIDC adapter returns them and the
// HTTP adapter maps them; both depend on this package, never on each other.
var (
	// ErrUnauthenticated means the request carries no valid credential (401).
	ErrUnauthenticated = errors.New("unauthenticated")
	// ErrInvalidReturnTo: return_to is not a relative path of this application (422).
	ErrInvalidReturnTo = errors.New("invalid return_to")
	// ErrLoginRejected: state, nonce, code or token of the callback did not
	// check out, or the login took too long (422). No session is created.
	ErrLoginRejected = errors.New("login rejected")
	// ErrInvalidLogoutToken: the back-channel logout token is missing or does
	// not verify (400).
	ErrInvalidLogoutToken = errors.New("invalid logout token")
)

// AuthFlow is the browser login flow, declared here because the HTTP adapter
// consumes it and the OIDC adapter implements it. All the protocol work (state,
// nonce, PKCE, token exchange and validation at Keycloak) happens behind it;
// the handlers only move cookies and redirects.
type AuthFlow interface {
	// StartLogin returns the Keycloak URL to redirect to and the value of the
	// short lived flow cookie. forceLogin adds prompt=login.
	StartLogin(ctx context.Context, returnTo string, forceLogin bool) (authorizationURL, flowCookie string, err error)
	// FinishLogin completes the callback and returns the opaque session
	// cookie value and the validated return_to.
	FinishLogin(ctx context.Context, code, state, flowCookie string) (sessionToken, returnTo string, err error)
	// Logout ends the product session (idempotent).
	Logout(ctx context.Context, sessionCookie string) error
	// BackchannelLogout ends the sessions named by a Keycloak logout token.
	BackchannelLogout(ctx context.Context, logoutToken string) error
}
