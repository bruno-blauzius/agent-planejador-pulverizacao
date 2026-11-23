// Package keycloak is the outbound adapter for the Keycloak Admin REST API
// (ADR 0015, D5). It implements usecase.AccountDirectory, usecase.ProductAccess
// and usecase.InviteSender using only net/http from the standard library.
//
// URLs: the Admin API is always called on Config.BaseURL, the internal address
// (http://keycloak:8080 inside the inceres-infra network, http://localhost:8180
// on a developer machine). The "iss" claim of tokens uses the public address
// the browser sees, which can differ; Config.IssuerURL carries it and is what
// Account.Issuer reports, so (iss, sub) links match the tokens (ADR 0015, D4).
//
// Nothing here logs or returns tokens, secrets, passwords or response bodies.
package keycloak

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/domain"
)

// Defaults for the optional settings.
const (
	DefaultProvisionerClientID = "agentes-agro-provisioner"
	DefaultProductClientID     = "agentes-agro"
	DefaultGroupBasePath       = "/agentes-agro"
	DefaultRequestTimeout      = 10 * time.Second
	DefaultActionLifespan      = 72 * time.Hour
	DefaultMaxGETRetries       = 2
	DefaultRetryBackoff        = 200 * time.Millisecond
)

// Config is the immutable configuration of the adapter. Build it in the
// composition root (or with ConfigFromEnv) and pass it to New.
type Config struct {
	// BaseURL is the internal Keycloak address used for every Admin API call.
	BaseURL string
	// Realm is the realm that holds the product (inceres).
	Realm string
	// IssuerURL is the public base address used to build the "iss" of the
	// realm (<IssuerURL>/realms/<Realm>). Empty means BaseURL.
	IssuerURL string
	// ClientID and ClientSecret authenticate the provisioner (client credentials).
	ClientID     string
	ClientSecret string
	// ProductClientID is the OIDC client of the product (sessions, action e-mail).
	ProductClientID string
	// GroupBasePath is the product group tree, e.g. /agentes-agro.
	GroupBasePath string
	// RoleGroups maps each role to the name of its sub group under GroupBasePath.
	// Empty means the names of infra/keycloak/agentes-agro.json.
	RoleGroups map[domain.Role]string
	// RedirectURI is where the action e-mail link leads once done. It must be
	// one of the valid redirect URIs of ProductClientID.
	RedirectURI string
	// ActionLifespan is the life of the e-mail link (mirrors API_INVITATION_TTL).
	ActionLifespan time.Duration

	// RequestTimeout bounds each HTTP exchange.
	RequestTimeout time.Duration
	// MaxGETRetries is how many times an idempotent GET is repeated after a
	// network error or 5xx. Writes are never retried. Zero means DefaultMaxGETRetries;
	// use a negative value to disable retries.
	MaxGETRetries int
	// RetryBackoff is the base wait between GET retries (linear).
	RetryBackoff time.Duration

	// HTTPClient and Logger are optional (tests inject them).
	HTTPClient *http.Client
	Logger     *slog.Logger
}

// DefaultRoleGroups returns the sub group name of each role.
func DefaultRoleGroups() map[domain.Role]string {
	return map[domain.Role]string{
		domain.RoleSuperAdmin:       "super-admins",
		domain.RoleAdmin:            "admins",
		domain.RolePlanningOperator: "planning-operators",
	}
}

// withDefaults fills optional fields and validates the result.
func (c Config) withDefaults() (Config, error) {
	c.BaseURL = strings.TrimRight(strings.TrimSpace(c.BaseURL), "/")
	c.IssuerURL = strings.TrimRight(strings.TrimSpace(c.IssuerURL), "/")
	if c.IssuerURL == "" {
		c.IssuerURL = c.BaseURL
	}
	if c.ClientID == "" {
		c.ClientID = DefaultProvisionerClientID
	}
	if c.ProductClientID == "" {
		c.ProductClientID = DefaultProductClientID
	}
	if c.GroupBasePath == "" {
		c.GroupBasePath = DefaultGroupBasePath
	}
	if len(c.RoleGroups) == 0 {
		c.RoleGroups = DefaultRoleGroups()
	}
	if c.ActionLifespan == 0 {
		c.ActionLifespan = DefaultActionLifespan
	}
	if c.RequestTimeout == 0 {
		c.RequestTimeout = DefaultRequestTimeout
	}
	if c.RetryBackoff == 0 {
		c.RetryBackoff = DefaultRetryBackoff
	}
	switch {
	case c.MaxGETRetries == 0:
		c.MaxGETRetries = DefaultMaxGETRetries
	case c.MaxGETRetries < 0:
		c.MaxGETRetries = 0
	}

	switch {
	case c.Realm == "":
		return c, errors.New("keycloak realm is required")
	case c.Realm == "master":
		return c, errors.New("keycloak realm must not be master")
	case c.ClientSecret == "":
		return c, errors.New("keycloak provisioner client secret is required")
	case c.ActionLifespan < 0, c.RequestTimeout < 0:
		return c, errors.New("keycloak durations must be positive")
	case !strings.HasPrefix(c.GroupBasePath, "/") || strings.HasSuffix(c.GroupBasePath, "/"):
		return c, fmt.Errorf("keycloak group base path %q must look like /agentes-agro", c.GroupBasePath)
	}
	for _, role := range domain.Roles() {
		if c.RoleGroups[role] == "" || strings.Contains(c.RoleGroups[role], "/") {
			return c, fmt.Errorf("keycloak group name of role %q is missing or has a slash", role)
		}
	}
	if err := checkURL("keycloak base url", c.BaseURL, true); err != nil {
		return c, err
	}
	if err := checkURL("keycloak issuer url", c.IssuerURL, false); err != nil {
		return c, err
	}
	if c.RedirectURI != "" {
		if err := checkURL("keycloak redirect uri", c.RedirectURI, false); err != nil {
			return c, err
		}
	}
	return c, nil
}

// checkURL requires an absolute http(s) URL. For the address that receives
// the provisioner secret, plain http is accepted only for local and
// in-network hosts (same rule as infra/keycloak/apply_product.py).
func checkURL(name, raw string, carriesSecret bool) error {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return fmt.Errorf("%s must be an absolute http(s) URL", name)
	}
	if parsed.User != nil {
		return fmt.Errorf("%s must not embed credentials", name)
	}
	if carriesSecret && parsed.Scheme == "http" {
		switch parsed.Hostname() {
		case "localhost", "127.0.0.1", "::1", "keycloak":
		default:
			return fmt.Errorf("%s must use https outside localhost and the keycloak network host", name)
		}
	}
	return nil
}

// LookupFunc matches os.LookupEnv.
type LookupFunc func(key string) (string, bool)

// ConfigFromEnv reads the adapter configuration. Variables (the first three
// are shared with infra/keycloak/apply_product.py and the repository .env):
//
//	KEYCLOAK_URL                          internal URL (default http://localhost:8180)
//	KEYCLOAK_REALM                        realm (default inceres)
//	KEYCLOAK_PROVISIONER_CLIENT_SECRET    provisioner secret (required)
//	KEYCLOAK_PROVISIONER_CLIENT_ID        default agentes-agro-provisioner
//	KEYCLOAK_ISSUER_URL                   public URL of the realm host (default KEYCLOAK_URL)
//	AGENTES_AGRO_API_URL                  public API URL; the e-mail redirect is <it>/auth/login
//	                                      (default http://localhost:<API_HOST_PORT or 8080>)
//	API_INVITATION_TTL                    lifespan of the action link (default 72h)
func ConfigFromEnv(lookup LookupFunc) (Config, error) {
	get := func(key, fallback string) string {
		if value, ok := lookup(key); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
		return fallback
	}
	cfg := Config{
		BaseURL:      get("KEYCLOAK_URL", "http://localhost:8180"),
		Realm:        get("KEYCLOAK_REALM", "inceres"),
		IssuerURL:    get("KEYCLOAK_ISSUER_URL", ""),
		ClientID:     get("KEYCLOAK_PROVISIONER_CLIENT_ID", DefaultProvisionerClientID),
		ClientSecret: get("KEYCLOAK_PROVISIONER_CLIENT_SECRET", ""),
	}
	apiURL := get("AGENTES_AGRO_API_URL", "http://localhost:"+get("API_HOST_PORT", "8080"))
	// The link of the e-mail must start a login: /auth/callback alone has no
	// code or state and answers 422.
	cfg.RedirectURI = strings.TrimRight(apiURL, "/") + "/auth/login"
	if raw := get("API_INVITATION_TTL", ""); raw != "" {
		ttl, err := time.ParseDuration(raw)
		if err != nil || ttl <= 0 {
			return Config{}, fmt.Errorf("invalid API_INVITATION_TTL %q: must be a positive duration such as 72h", raw)
		}
		cfg.ActionLifespan = ttl
	}
	return cfg.withDefaults()
}
