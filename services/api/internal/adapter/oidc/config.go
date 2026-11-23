// Package oidc is the adapter that makes the API an OpenID Connect client of
// the Keycloak realm (ADR 0015, D6: BFF pattern).
//
// Everything that proves identity happens in Keycloak: the authorization code
// is exchanged at its token endpoint, ID and access tokens are verified
// against the signing keys of the realm (JWKS), refresh tokens are renewed and
// revoked at its endpoints. The API never sees a password and never trusts a
// token it did not verify.
//
// URLs: the browser talks to the public address (the "iss" of the tokens);
// the API talks to Keycloak server-to-server on Config.InternalURL
// (http://keycloak:8080 inside the inceres-infra network). The discovery
// document is read from the internal address and must announce the configured
// public issuer; its server-to-server endpoints are then re-addressed to the
// internal host, so a realm that answers with public URLs still works inside
// the network without ever weakening the issuer check.
//
// Nothing here logs tokens, codes, cookies or response bodies.
package oidc

import (
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Defaults for the optional settings.
const (
	DefaultClientID       = "agentes-agro"
	DefaultRealm          = "inceres"
	DefaultSessionCookie  = "agro_session"
	DefaultFlowCookie     = "agro_oidc"
	DefaultReauthCookie   = "agro_reauth"
	DefaultSessionTTL     = 7 * 24 * time.Hour
	DefaultFlowTTL        = 10 * time.Minute
	DefaultRequestTimeout = 10 * time.Second
	DefaultClockSkew      = 30 * time.Second
	DefaultRenewMargin    = 10 * time.Second

	encryptionKeyBytes     = 32
	defaultEncryptionKeyID = 1
	maxEncryptionKeyID     = 1<<15 - 1
)

// Config is the immutable configuration of the adapter.
type Config struct {
	// IssuerURL is the public issuer of the realm, exactly as it appears in
	// the "iss" claim (http://localhost:8180/realms/inceres).
	IssuerURL string
	// InternalURL is the Keycloak base address for server-to-server calls
	// (http://keycloak:8080). Empty means the host of IssuerURL.
	InternalURL string
	// Realm is the realm that holds the product (inceres).
	Realm string
	// ClientID and ClientSecret identify the confidential client agentes-agro.
	ClientID     string
	ClientSecret string
	// RedirectURL is <API public URL>/auth/callback (registered in the client).
	RedirectURL string
	// WebURL is the public base URL of the web front (AGENTES_AGRO_WEB_URL). The
	// callback redirects to it; empty keeps redirects relative (same host).
	WebURL string

	// EncryptionKey (32 bytes) protects refresh tokens and the login flow
	// cookie; EncryptionKeyID is stored with each sealed token. OldKeys keep
	// older ids readable while a rotation completes.
	EncryptionKey   []byte
	EncryptionKeyID int16
	OldKeys         map[int16][]byte

	// SessionTTL is the absolute life of a browser session (cookie Max-Age).
	// Keycloak decides the real life: a failed renewal ends the session first.
	SessionTTL time.Duration
	// FlowTTL bounds the time between GET /auth/login and the callback.
	FlowTTL time.Duration
	// RequestTimeout bounds each exchange with Keycloak.
	RequestTimeout time.Duration
	// ClockSkew is the tolerance for nbf and iat.
	ClockSkew time.Duration
	// RenewMargin renews the access token this long before it expires.
	RenewMargin time.Duration

	SessionCookieName string
	FlowCookieName    string
	ReauthCookieName  string
}

// Normalize applies defaults and validates a Config (ConfigFromEnv calls it;
// tests call it on a literal).
func (c Config) Normalize() (Config, error) {
	c.IssuerURL = strings.TrimRight(strings.TrimSpace(c.IssuerURL), "/")
	c.InternalURL = strings.TrimRight(strings.TrimSpace(c.InternalURL), "/")
	if c.Realm == "" {
		c.Realm = DefaultRealm
	}
	if c.ClientID == "" {
		c.ClientID = DefaultClientID
	}
	if c.EncryptionKeyID == 0 {
		c.EncryptionKeyID = defaultEncryptionKeyID
	}
	durations := []struct {
		target *time.Duration
		value  time.Duration
	}{
		{&c.SessionTTL, DefaultSessionTTL}, {&c.FlowTTL, DefaultFlowTTL}, {&c.RequestTimeout, DefaultRequestTimeout},
		{&c.ClockSkew, DefaultClockSkew}, {&c.RenewMargin, DefaultRenewMargin},
	}
	for _, d := range durations {
		if *d.target == 0 {
			*d.target = d.value
		}
		if *d.target < 0 {
			return c, errors.New("oidc durations must be positive")
		}
	}
	names := []struct {
		target *string
		value  string
	}{
		{&c.SessionCookieName, DefaultSessionCookie}, {&c.FlowCookieName, DefaultFlowCookie}, {&c.ReauthCookieName, DefaultReauthCookie},
	}
	for _, name := range names {
		if *name.target == "" {
			*name.target = name.value
		}
	}

	switch {
	case c.ClientSecret == "":
		return c, errors.New("oidc client secret is required")
	case c.SessionCookieName == c.FlowCookieName || c.SessionCookieName == c.ReauthCookieName || c.FlowCookieName == c.ReauthCookieName:
		return c, errors.New("oidc cookie names must be distinct")
	}
	if err := checkURL("oidc issuer url", c.IssuerURL); err != nil {
		return c, err
	}
	if !strings.HasSuffix(c.IssuerURL, "/realms/"+c.Realm) {
		return c, fmt.Errorf("oidc issuer url must end with /realms/%s", c.Realm)
	}
	if c.InternalURL != "" {
		if err := checkURL("oidc internal url", c.InternalURL); err != nil {
			return c, err
		}
	}
	if err := checkURL("oidc redirect url", c.RedirectURL); err != nil {
		return c, err
	}
	c.WebURL = strings.TrimRight(strings.TrimSpace(c.WebURL), "/")
	if c.WebURL != "" {
		if err := checkURL("web url", c.WebURL); err != nil {
			return c, err
		}
	}
	if _, err := NewKeyring(c.EncryptionKeyID, c.EncryptionKey, c.OldKeys); err != nil {
		return c, err
	}
	return c, nil
}

func checkURL(name, raw string) error {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return fmt.Errorf("%s must be an absolute http(s) URL", name)
	}
	if parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return fmt.Errorf("%s must not carry credentials, query or fragment", name)
	}
	return nil
}

// LookupFunc matches os.LookupEnv.
type LookupFunc func(key string) (string, bool)

// ConfigFromEnv reads the adapter configuration. Variables (the Keycloak ones
// are shared with adapter/keycloak and infra/keycloak):
//
//	KEYCLOAK_URL                        internal URL (default http://localhost:8180)
//	KEYCLOAK_ISSUER_URL                 public host of the realm (default KEYCLOAK_URL)
//	KEYCLOAK_REALM                      realm (default inceres)
//	KEYCLOAK_AGENTES_AGRO_CLIENT_ID     client id (default agentes-agro)
//	KEYCLOAK_AGENTES_AGRO_CLIENT_SECRET client secret (required)
//	AGENTES_AGRO_API_URL                public API URL; redirect is <it>/auth/callback
//	                                    (default http://localhost:<API_HOST_PORT or 8080>)
//	AGENTES_AGRO_WEB_URL                public front URL; the callback redirects to it
//	                                    (default empty: relative redirects, front and API on one host)
//	API_SESSION_ENC_KEY                 base64 of 32 random bytes (required)
//	API_SESSION_ENC_KEY_ID              numeric id of that key (default 1)
//	API_SESSION_ENC_OLD_KEYS            optional "id:base64,id:base64" kept during rotation
//	API_SESSION_TTL                     absolute session life (default 168h)
//	API_SESSION_FLOW_TTL                login round trip limit (default 10m)
//	API_OIDC_TIMEOUT                    timeout of each Keycloak call (default 10s)
func ConfigFromEnv(lookup LookupFunc) (Config, error) {
	get := func(key, fallback string) string {
		if value, ok := lookup(key); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
		return fallback
	}
	internal := strings.TrimRight(get("KEYCLOAK_URL", "http://localhost:8180"), "/")
	public := strings.TrimRight(get("KEYCLOAK_ISSUER_URL", internal), "/")
	realm := get("KEYCLOAK_REALM", DefaultRealm)
	apiURL := strings.TrimRight(get("AGENTES_AGRO_API_URL", "http://localhost:"+get("API_HOST_PORT", "8080")), "/")

	cfg := Config{
		IssuerURL:    public + "/realms/" + realm,
		InternalURL:  internal,
		Realm:        realm,
		ClientID:     get("KEYCLOAK_AGENTES_AGRO_CLIENT_ID", DefaultClientID),
		ClientSecret: get("KEYCLOAK_AGENTES_AGRO_CLIENT_SECRET", ""),
		RedirectURL:  apiURL + "/auth/callback",
		WebURL:       get("AGENTES_AGRO_WEB_URL", ""),
	}

	key, err := decodeKey("API_SESSION_ENC_KEY", get("API_SESSION_ENC_KEY", ""))
	if err != nil {
		return Config{}, err
	}
	cfg.EncryptionKey = key
	if raw := get("API_SESSION_ENC_KEY_ID", ""); raw != "" {
		id, err := strconv.ParseInt(raw, 10, 16)
		if err != nil || id < 1 || id > maxEncryptionKeyID {
			return Config{}, fmt.Errorf("invalid API_SESSION_ENC_KEY_ID %q: must be an integer between 1 and %d", raw, maxEncryptionKeyID)
		}
		cfg.EncryptionKeyID = int16(id)
	}
	if raw := get("API_SESSION_ENC_OLD_KEYS", ""); raw != "" {
		old, err := parseOldKeys(raw)
		if err != nil {
			return Config{}, err
		}
		cfg.OldKeys = old
	}
	timings := []struct {
		key    string
		target *time.Duration
	}{
		{"API_SESSION_TTL", &cfg.SessionTTL}, {"API_SESSION_FLOW_TTL", &cfg.FlowTTL}, {"API_OIDC_TIMEOUT", &cfg.RequestTimeout},
	}
	for _, timing := range timings {
		raw := get(timing.key, "")
		if raw == "" {
			continue
		}
		value, err := time.ParseDuration(raw)
		if err != nil || value <= 0 {
			return Config{}, fmt.Errorf("invalid %s %q: must be a positive duration such as 168h", timing.key, raw)
		}
		*timing.target = value
	}
	return cfg.Normalize()
}

func decodeKey(name, raw string) ([]byte, error) {
	if raw == "" {
		return nil, fmt.Errorf("%s is required (base64 of %d random bytes)", name, encryptionKeyBytes)
	}
	key, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		return nil, fmt.Errorf("%s must be standard base64", name)
	}
	return key, nil
}

func parseOldKeys(raw string) (map[int16][]byte, error) {
	keys := map[int16][]byte{}
	for _, entry := range strings.Split(raw, ",") {
		idText, encoded, ok := strings.Cut(strings.TrimSpace(entry), ":")
		id, err := strconv.ParseInt(idText, 10, 16)
		if !ok || err != nil || id < 1 {
			return nil, errors.New("API_SESSION_ENC_OLD_KEYS must look like 1:<base64>,2:<base64>")
		}
		key, err := decodeKey("API_SESSION_ENC_OLD_KEYS", encoded)
		if err != nil {
			return nil, err
		}
		keys[int16(id)] = key
	}
	return keys, nil
}
