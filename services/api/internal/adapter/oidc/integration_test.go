//go:build integration || keycloak

// Integration test against the local Keycloak (infra-docker, realm inceres).
// Run with `make api-test-oidc`. It proves that the token validation happens at
// the real Keycloak: the authorization code is exchanged at its token endpoint
// and the ID and access tokens are verified against its published keys.
//
// Needs `make keycloak` done (client secret in the repository .env) and the
// seed passwords in the .env. The seed users are created only if the login of
// agro-dev-admin does not work yet, and removed afterwards only in that case.
// Passwords are read from the environment and never printed.
package oidc

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"html"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/domain"
	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/usecase"
)

const (
	seedUsername = "agro-dev-admin"
	seedEmail    = "agro-dev-admin@agentes-agro.test"
	repoRootUp   = "../../../../.."
)

// closeQuietly closes a body that was already consumed; a failure cannot change the test outcome.
func closeQuietly(closer io.Closer) {
	if err := closer.Close(); err != nil {
		return // nothing useful to do in a test helper
	}
}

func loadDotEnv() map[string]string {
	values := map[string]string{}
	file, err := os.Open(filepath.Join(repoRootUp, ".env"))
	if err != nil {
		return values
	}
	defer closeQuietly(file)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if key, value, ok := strings.Cut(line, "="); ok {
			values[strings.TrimSpace(key)] = strings.Trim(strings.TrimSpace(value), `"'`)
		}
	}
	return values
}

// recordingResolver accepts any identity of the realm and records the claims,
// so the test can assert what Keycloak really put in the tokens.
type recordingResolver struct{ claims []usecase.Claims }

func (r *recordingResolver) Execute(_ context.Context, in usecase.ResolvePrincipalInput) (domain.Principal, error) {
	r.claims = append(r.claims, in.Claims)
	access, err := domain.ReadAccess(in.Claims.ClientRoles)
	if err != nil {
		return domain.Principal{}, err
	}
	return domain.Principal{UserID: testUserID, CompanyID: "co-it", Role: access.Role, Permissions: access.Permissions}, nil
}

// recordedIdentities returns the subject Keycloak announced at login.
type recordedIdentities struct{ resolver *recordingResolver }

func (r recordedIdentities) FindIdentityByUser(context.Context, domain.UserID) (domain.Identity, error) {
	if len(r.resolver.claims) == 0 {
		return domain.Identity{}, usecase.ErrNotFound
	}
	first := r.resolver.claims[0]
	return domain.Identity{UserID: testUserID, Issuer: first.Issuer, Subject: first.Subject}, nil
}

var (
	formActionPattern = regexp.MustCompile(`(?s)<form[^>]*id="kc-form-login"[^>]*action="([^"]+)"`)
	anyFormAction     = regexp.MustCompile(`(?s)<form[^>]*action="([^"]+)"`)
)

// browserLogin plays the browser at the real Keycloak: it opens the
// authorization URL, parses the login form and posts the credentials. It
// returns the code and state of the redirect back to the API, or ok=false when
// the credentials were refused.
func browserLogin(t *testing.T, authURL, username, password string) (code, state string, ok bool) {
	t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{
		Jar: jar, Timeout: 20 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	ctx := context.Background()

	page := fetch(ctx, t, client, http.MethodGet, authURL, nil)
	for i := 0; i < 5 && page.status >= 300 && page.status < 400; i++ { // some themes redirect once
		page = fetch(ctx, t, client, http.MethodGet, resolve(t, authURL, page.location), nil)
	}
	if page.status != http.StatusOK {
		t.Fatalf("authorization endpoint answered %d (is the redirect URI registered? run `make keycloak`)", page.status)
	}
	match := formActionPattern.FindStringSubmatch(page.body)
	if match == nil {
		match = anyFormAction.FindStringSubmatch(page.body)
	}
	if match == nil {
		t.Fatal("login form not found in the Keycloak page")
	}
	action := resolve(t, authURL, html.UnescapeString(match[1]))

	form := url.Values{"username": {username}, "password": {password}, "credentialId": {""}}
	post := fetch(ctx, t, client, http.MethodPost, action, strings.NewReader(form.Encode()))
	if post.status != http.StatusFound {
		return "", "", false // wrong credentials: Keycloak re-renders the form
	}
	back, err := url.Parse(post.location)
	if err != nil || !strings.HasPrefix(post.location, testRedirectForIntegration) {
		t.Fatalf("login redirected somewhere unexpected: %q", redactQuery(post.location))
	}
	return back.Query().Get("code"), back.Query().Get("state"), true
}

var testRedirectForIntegration string

type page struct {
	status   int
	location string
	body     string
}

func fetch(ctx context.Context, t *testing.T, client *http.Client, method, target string, body io.Reader) page {
	t.Helper()
	request, err := http.NewRequestWithContext(ctx, method, target, body)
	if err != nil {
		t.Fatal(err)
	}
	if body != nil {
		request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	response, err := client.Do(request)
	if err != nil {
		t.Fatalf("%s %s: %v", method, redactQuery(target), errors.Unwrap(err))
	}
	defer closeQuietly(response.Body)
	raw, err := io.ReadAll(io.LimitReader(response.Body, 2<<20))
	if err != nil {
		t.Fatal(err)
	}
	return page{status: response.StatusCode, location: response.Header.Get("Location"), body: string(raw)}
}

func resolve(t *testing.T, base, ref string) string {
	t.Helper()
	baseURL, err := url.Parse(base)
	if err != nil {
		t.Fatal(err)
	}
	refURL, err := url.Parse(ref)
	if err != nil {
		t.Fatal(err)
	}
	return baseURL.ResolveReference(refURL).String()
}

// redactQuery keeps codes and states out of failure messages.
func redactQuery(raw string) string {
	if i := strings.IndexByte(raw, '?'); i >= 0 {
		return raw[:i] + "?<redacted>"
	}
	return raw
}

func runSeed(t *testing.T, flag string) {
	t.Helper()
	args := []string{"run", "--no-project", "python", "infra/keycloak/seed_dev_users.py"}
	if flag != "" {
		args = append(args, flag)
	}
	command := exec.CommandContext(context.Background(), "uv", args...)
	command.Dir = repoRootUp
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("seed %s failed: %v\n%s", flag, err, output) // the script never prints passwords
	}
}

func TestRealKeycloakAuthorizationCodeFlow(t *testing.T) {
	dotenv := loadDotEnv()
	lookup := func(key string) (string, bool) {
		if value, ok := os.LookupEnv(key); ok {
			return value, true
		}
		value, ok := dotenv[key]
		return value, ok
	}
	if secret, _ := lookup("KEYCLOAK_AGENTES_AGRO_CLIENT_SECRET"); secret == "" {
		t.Skip("KEYCLOAK_AGENTES_AGRO_CLIENT_SECRET not set (run `make keycloak` first)")
	}
	password, _ := lookup("KEYCLOAK_SEED_PASSWORD_ADMIN")
	if password == "" {
		t.Skip("KEYCLOAK_SEED_PASSWORD_ADMIN not set (see .env.example, then `make keycloak-seed`)")
	}

	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatal(err)
	}
	withKey := func(name string) (string, bool) {
		switch name {
		case "API_SESSION_ENC_KEY":
			return base64.StdEncoding.EncodeToString(key), true
		case "AGENTES_AGRO_API_URL":
			if value, ok := lookup(name); ok && value != "" {
				return value, true
			}
			port, _ := lookup("API_HOST_PORT")
			if port == "" {
				port = "8080"
			}
			return "http://localhost:" + port, true
		}
		return lookup(name)
	}
	cfg, err := ConfigFromEnv(withKey)
	if err != nil {
		t.Fatal(err)
	}
	testRedirectForIntegration = cfg.RedirectURL

	resolver := &recordingResolver{}
	sessions := newFakeSessions(&testClock{t: time.Now()})
	service, err := NewService(cfg, Deps{
		Sessions: sessions, Resolver: resolver, Identities: recordedIdentities{resolver}, Clock: realClock{},
	})
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	authURL, flow, err := service.StartLogin(ctx, "/farms", false)
	if err != nil {
		t.Fatalf("StartLogin (is Keycloak up at %s?): %v", cfg.InternalURL, err)
	}

	// Seed the users only when the login does not work yet.
	seeded := false
	code, state, ok := browserLogin(t, authURL, seedUsername, password)
	if !ok {
		runSeed(t, "")
		seeded = true
		t.Cleanup(func() { runSeed(t, "--cleanup") })
		authURL, flow, err = service.StartLogin(ctx, "/farms", false)
		if err != nil {
			t.Fatal(err)
		}
		if code, state, ok = browserLogin(t, authURL, seedUsername, password); !ok {
			t.Fatal("Keycloak refused the credentials of the seed user (check KEYCLOAK_SEED_PASSWORD_ADMIN, then `make keycloak-seed ARGS=--reset-passwords`)")
		}
	}
	t.Logf("seed users created by this test: %v", seeded)

	// 1. A forged state never reaches Keycloak's token endpoint.
	if _, _, err := service.FinishLogin(ctx, code, "forged-state", flow); !errors.Is(err, usecase.ErrLoginRejected) {
		t.Fatalf("forged state: %v", err)
	}

	// 2. The real exchange and validation, at Keycloak.
	token, returnTo, err := service.FinishLogin(ctx, code, state, flow)
	if err != nil {
		t.Fatalf("FinishLogin against the real Keycloak: %v", err)
	}
	if returnTo != "/farms" || token == "" {
		t.Fatalf("returnTo=%q token set=%v", returnTo, token != "")
	}
	claims := resolver.claims[0]
	if claims.Issuer != cfg.IssuerURL || claims.Subject == "" || claims.Email != seedEmail {
		t.Fatalf("claims from the real tokens = issuer %q subject set=%v email %q", claims.Issuer, claims.Subject != "", claims.Email)
	}
	hasAdmin := false
	for _, role := range claims.ClientRoles {
		hasAdmin = hasAdmin || role == "admin"
	}
	if !hasAdmin {
		t.Fatalf("resource_access.agentes-agro.roles = %v, want admin", claims.ClientRoles)
	}
	t.Logf("verified at Keycloak: iss=%s roles=%v email_verified=%v", claims.Issuer, claims.ClientRoles, claims.EmailVerified)

	// 4. The session authenticates requests; a forced renewal goes through the
	// real refresh grant and re-reads the roles from the new access token.
	authenticator := NewAuthenticator(service)
	request, _ := http.NewRequestWithContext(ctx, http.MethodGet, "/auth/me", http.NoBody)
	request.AddCookie(&http.Cookie{Name: cfg.SessionCookieName, Value: token})
	principal, err := authenticator.Authenticate(request)
	if err != nil || principal.Role != domain.RoleAdmin {
		t.Fatalf("session authentication: %v %+v", err, principal)
	}
	record := sessions.only()
	sessions.mu.Lock()
	sessions.byID[record.ID].AccessExpiresAt = time.Now().Add(-time.Second)
	sessions.mu.Unlock()
	before := sessions.only().RefreshTokenEnc
	if principal, err = authenticator.Authenticate(request); err != nil || principal.Role != domain.RoleAdmin {
		t.Fatalf("renewal at the real Keycloak: %v", err)
	}
	renewed := sessions.only()
	if sessions.renewals != 1 || !renewed.AccessExpiresAt.After(time.Now()) {
		t.Fatalf("renewals=%d accessExpiresAt=%v", sessions.renewals, renewed.AccessExpiresAt)
	}
	t.Logf("refresh token resealed after renewal: %v", string(before) != string(renewed.RefreshTokenEnc))

	// 5. A token the same realm signed for ANOTHER client is refused. The
	// provisioner is a real client of the realm with a service account.
	if secret, _ := lookup("KEYCLOAK_PROVISIONER_CLIENT_SECRET"); secret != "" {
		foreign := clientCredentialsToken(t, cfg, secret)
		bearer, _ := http.NewRequestWithContext(ctx, http.MethodGet, "/auth/me", http.NoBody)
		bearer.Header.Set("Authorization", "Bearer "+foreign)
		if _, err := authenticator.Authenticate(bearer); !errors.Is(err, usecase.ErrUnauthenticated) {
			t.Fatalf("token of another client of the realm: err = %v, want ErrUnauthenticated", err)
		}
		t.Log("a real token of another client of the realm was refused")
	}

	// 6. Logout revokes the refresh token at Keycloak: using it again fails.
	plain, err := service.keys.Open(renewed.RefreshTokenEnc, renewed.EncKeyID, []byte(renewed.ID))
	if err != nil {
		t.Fatal(err)
	}
	if err := service.Logout(ctx, token); err != nil {
		t.Fatal(err)
	}
	if _, err := authenticator.Authenticate(request); !errors.Is(err, usecase.ErrUnauthenticated) {
		t.Fatalf("session after logout: %v", err)
	}
	if _, err := service.prov.refresh(ctx, string(plain)); !errors.Is(err, errGrantRejected) {
		t.Fatalf("revoked refresh token must be refused by Keycloak, got %v", err)
	}

	// 7. The authorization code is single use, enforced by Keycloak. (Keycloak
	// also kills the session of a replayed code, so this runs last.)
	if _, _, err := service.FinishLogin(ctx, code, state, flow); !errors.Is(err, usecase.ErrLoginRejected) {
		t.Fatalf("code replay: %v", err)
	}
}

type realClock struct{}

func (realClock) Now() time.Time { return time.Now() }

// clientCredentialsToken asks the real realm for a service account token of the
// provisioner client: a valid, signed access token with azp != agentes-agro.
func clientCredentialsToken(t *testing.T, cfg Config, secret string) string {
	t.Helper()
	form := url.Values{"grant_type": {"client_credentials"}, "client_id": {"agentes-agro-provisioner"}, "client_secret": {secret}}
	tokenURL := cfg.InternalURL + "/realms/" + cfg.Realm + "/protocol/openid-connect/token"
	client := &http.Client{Timeout: 15 * time.Second}
	request, err := http.NewRequestWithContext(context.Background(), http.MethodPost, tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response, err := client.Do(request)
	if err != nil {
		t.Fatalf("client credentials: %v", errors.Unwrap(err))
	}
	defer closeQuietly(response.Body)
	raw, readErr := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if readErr != nil {
		t.Fatalf("read provisioner answer: %v", readErr)
	}
	if response.StatusCode != http.StatusOK {
		t.Skipf("provisioner token not available (status %d)", response.StatusCode)
	}
	const marker = `"access_token":"`
	start := strings.Index(string(raw), marker)
	if start < 0 {
		t.Skip("no access_token in the provisioner answer")
	}
	rest := string(raw)[start+len(marker):]
	return rest[:strings.IndexByte(rest, '"')]
}
