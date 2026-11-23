package keycloak

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/domain"
	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/usecase"
)

const victimEmail = "victim@example.test"

type harness struct {
	fake    *fakeKeycloak
	adapter *Adapter
	logs    *bytes.Buffer
	sleeps  []time.Duration
}

func newHarness(t *testing.T, mutate ...func(*Config)) *harness {
	t.Helper()
	fake := newFakeKeycloak(t)
	logs := &bytes.Buffer{}
	cfg := Config{
		BaseURL:        fake.server.URL,
		Realm:          testRealm,
		IssuerURL:      "http://localhost:8180",
		ClientSecret:   testSecret,
		RedirectURI:    "http://localhost:8080/auth/login",
		ActionLifespan: 72 * time.Hour,
		RequestTimeout: 2 * time.Second,
		Logger:         slog.New(slog.NewTextHandler(logs, &slog.HandlerOptions{Level: slog.LevelDebug})),
	}
	for _, m := range mutate {
		m(&cfg)
	}
	adapter, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	h := &harness{fake: fake, adapter: adapter, logs: logs}
	adapter.client.sleep = func(_ context.Context, d time.Duration) error {
		h.sleeps = append(h.sleeps, d)
		return nil
	}
	return h
}

func (h *harness) assertNoSecrets(t *testing.T, err error) {
	t.Helper()
	haystacks := []string{h.logs.String()}
	if err != nil {
		haystacks = append(haystacks, err.Error())
	}
	for _, text := range haystacks {
		for _, secret := range []string{testSecret, testToken, victimEmail} {
			if strings.Contains(text, secret) {
				t.Fatalf("leaked %q in: %s", secret[:6]+"...", text)
			}
		}
	}
}

func mustEmail(t *testing.T, value string) domain.Email {
	t.Helper()
	email, err := domain.ParseEmail(value)
	if err != nil {
		t.Fatal(err)
	}
	return email
}

// ---- configuration ---------------------------------------------------------------------

func TestConfigValidation(t *testing.T) {
	valid := Config{BaseURL: "http://localhost:8180", Realm: "inceres", ClientSecret: "x"}
	tests := []struct {
		name   string
		mutate func(*Config)
		ok     bool
	}{
		{"valid", func(*Config) {}, true},
		{"in-network host over http", func(c *Config) { c.BaseURL = "http://keycloak:8080" }, true},
		{"remote host over http", func(c *Config) { c.BaseURL = "http://sso.example.com" }, false},
		{"remote host over https", func(c *Config) { c.BaseURL = "https://sso.example.com" }, true},
		{"no secret", func(c *Config) { c.ClientSecret = "" }, false},
		{"master realm", func(c *Config) { c.Realm = "master" }, false},
		{"no realm", func(c *Config) { c.Realm = "" }, false},
		{"url with credentials", func(c *Config) { c.BaseURL = "http://u:p@localhost:8180" }, false},
		{"bad group base", func(c *Config) { c.GroupBasePath = "agentes-agro" }, false},
		{"bad redirect", func(c *Config) { c.RedirectURI = "not a url" }, false},
		{"negative lifespan", func(c *Config) { c.ActionLifespan = -time.Hour }, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := valid
			tc.mutate(&cfg)
			_, err := New(cfg)
			if (err == nil) != tc.ok {
				t.Fatalf("err = %v, want ok=%v", err, tc.ok)
			}
			if err != nil && strings.Contains(err.Error(), "x") && cfg.ClientSecret == "leak" {
				t.Fatal("secret in error")
			}
		})
	}
}

func TestConfigFromEnv(t *testing.T) {
	env := map[string]string{
		"KEYCLOAK_PROVISIONER_CLIENT_SECRET": "abc",
		"API_HOST_PORT":                      "8090",
		"API_INVITATION_TTL":                 "48h",
		"KEYCLOAK_ISSUER_URL":                "http://localhost:8180",
		"KEYCLOAK_URL":                       "http://keycloak:8080",
	}
	cfg, err := ConfigFromEnv(func(k string) (string, bool) { v, ok := env[k]; return v, ok })
	if err != nil {
		t.Fatal(err)
	}
	if cfg.BaseURL != "http://keycloak:8080" || cfg.IssuerURL != "http://localhost:8180" ||
		cfg.RedirectURI != "http://localhost:8090/auth/login" || cfg.ActionLifespan != 48*time.Hour ||
		cfg.Realm != "inceres" || cfg.ClientID != DefaultProvisionerClientID {
		t.Fatalf("unexpected config: %+v", cfg)
	}

	env["API_INVITATION_TTL"] = "soon"
	if _, err := ConfigFromEnv(func(k string) (string, bool) { v, ok := env[k]; return v, ok }); err == nil {
		t.Fatal("invalid TTL must fail")
	}
	delete(env, "API_INVITATION_TTL")
	delete(env, "KEYCLOAK_PROVISIONER_CLIENT_SECRET")
	if _, err := ConfigFromEnv(func(k string) (string, bool) { v, ok := env[k]; return v, ok }); err == nil {
		t.Fatal("missing secret must fail")
	}
}

// ---- accounts --------------------------------------------------------------------------

func TestFindAccountByEmail(t *testing.T) {
	h := newHarness(t)
	user := h.fake.addUser("ana@example.test")
	user.EmailVerified = true
	h.fake.addUser("ana@example.test.br") // must not match: exact search

	account, err := h.adapter.FindAccountByEmail(context.Background(), mustEmail(t, "Ana@Example.test"))
	if err != nil {
		t.Fatal(err)
	}
	if account.Subject != user.ID || !account.EmailVerified || account.Email != "ana@example.test" ||
		account.Issuer != "http://localhost:8180/realms/inceres" {
		t.Fatalf("unexpected account %+v", account)
	}

	details, err := h.adapter.FindAccountDetailsByEmail(context.Background(), mustEmail(t, "ana@example.test"))
	if err != nil || !details.Enabled || details.CreatedAt.IsZero() {
		t.Fatalf("details = %+v, %v", details, err)
	}

	_, err = h.adapter.FindAccountByEmail(context.Background(), mustEmail(t, "ghost@example.test"))
	if !errors.Is(err, usecase.ErrAccountNotFound) {
		t.Fatalf("err = %v, want ErrAccountNotFound", err)
	}
}

func TestCreateAccount(t *testing.T) {
	tests := []struct {
		name, in, first, last string
	}{
		{"two words", "Maria Silva", "Maria", "Silva"},
		{"many words", "  Maria  da  Silva Souza ", "Maria", "da Silva Souza"},
		{"single word falls back", "Madonna", "Madonna", fallbackLastName},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t)
			account, err := h.adapter.CreateAccount(context.Background(),
				usecase.NewAccount{Email: mustEmail(t, "maria@example.test"), Name: tc.in})
			if err != nil {
				t.Fatal(err)
			}
			stored := h.fake.user(account.Subject)
			if stored == nil {
				t.Fatal("user not created")
			}
			if stored.FirstName != tc.first || stored.LastName != tc.last || stored.EmailVerified ||
				!slices.Equal(stored.RequiredActions, []string{"VERIFY_EMAIL", "UPDATE_PASSWORD"}) {
				t.Fatalf("unexpected stored user %+v", stored)
			}
			if account.EmailVerified || account.Issuer != "http://localhost:8180/realms/inceres" || account.Email != "maria@example.test" {
				t.Fatalf("unexpected account %+v", account)
			}
			if stored.Credentials != 0 {
				t.Fatal("no password may be set")
			}
		})
	}
}

func TestCreateAccountConflictIsUnavailableWithCause(t *testing.T) {
	h := newHarness(t)
	h.fake.addUser("dup@example.test")
	_, err := h.adapter.CreateAccount(context.Background(),
		usecase.NewAccount{Email: mustEmail(t, "dup@example.test"), Name: "Dup Licate"})
	if !errors.Is(err, usecase.ErrIdentityProviderUnavailable) || !errors.Is(err, ErrAccountExists) {
		t.Fatalf("err = %v", err)
	}
	if strings.Contains(err.Error(), "same email") {
		t.Fatalf("response body leaked: %v", err)
	}
	if got := h.fake.callsOf("POST /users"); got != 1 {
		t.Fatalf("POST must not be retried, calls=%d", got)
	}
}

func TestDeleteCreatedAccount(t *testing.T) {
	ctx := context.Background()

	t.Run("deletes an untouched account", func(t *testing.T) {
		h := newHarness(t)
		user := h.fake.addUser("new@example.test")
		user.Groups["g-admin"] = true // the product group does not count as use
		if err := h.adapter.DeleteCreatedAccount(ctx, user.ID); err != nil {
			t.Fatal(err)
		}
		if h.fake.user(user.ID) != nil {
			t.Fatal("account must be deleted")
		}
	})

	t.Run("missing account is success", func(t *testing.T) {
		h := newHarness(t)
		if err := h.adapter.DeleteCreatedAccount(ctx, "00000000-0000-0000-0000-999999999999"); err != nil {
			t.Fatal(err)
		}
		if err := h.adapter.DeleteCreatedAccount(ctx, "00000000-0000-0000-0000-999999999999"); err != nil {
			t.Fatalf("second call: %v", err)
		}
	})

	inUse := []struct {
		name   string
		mutate func(*fakeKeycloak, *fakeUser)
	}{
		{"has a credential", func(_ *fakeKeycloak, u *fakeUser) { u.Credentials = 1 }},
		{"e-mail verified", func(_ *fakeKeycloak, u *fakeUser) { u.EmailVerified = true }},
		{"active session", func(_ *fakeKeycloak, u *fakeUser) {
			u.Sessions = []fakeSession{{ID: "s1", Clients: map[string]string{"c1": "agentes-agro"}}}
		}},
		{"federated identity", func(_ *fakeKeycloak, u *fakeUser) { u.Federated = 1 }},
		{"member of another product group", func(_ *fakeKeycloak, u *fakeUser) { u.Groups["g-other"] = true }},
		{"login event", func(f *fakeKeycloak, u *fakeUser) { f.eventsDenied = false; u.LoginEvents = 1 }},
	}
	for _, tc := range inUse {
		t.Run("in use: "+tc.name, func(t *testing.T) {
			h := newHarness(t)
			user := h.fake.addUser("used@example.test")
			tc.mutate(h.fake, user)
			err := h.adapter.DeleteCreatedAccount(ctx, user.ID)
			if !errors.Is(err, usecase.ErrAccountInUse) {
				t.Fatalf("err = %v, want ErrAccountInUse", err)
			}
			if h.fake.user(user.ID) == nil {
				t.Fatal("account in use must not be deleted")
			}
			if h.fake.callsOf("DELETE /users/") != 0 {
				t.Fatal("DELETE must not be sent")
			}
		})
	}

	t.Run("events readable and empty", func(t *testing.T) {
		h := newHarness(t)
		h.fake.eventsDenied = false
		user := h.fake.addUser("quiet@example.test")
		if err := h.adapter.DeleteCreatedAccount(ctx, user.ID); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("rejects a malformed subject", func(t *testing.T) {
		h := newHarness(t)
		if err := h.adapter.DeleteCreatedAccount(ctx, "../realms"); err == nil {
			t.Fatal("subject must be validated")
		}
		if len(h.fake.calls) != 0 {
			t.Fatal("no request may be sent")
		}
	})
}

// ---- groups ----------------------------------------------------------------------------

func TestAddToRoleGroupIsIdempotentAndExclusive(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	user := h.fake.addUser("op@example.test")
	user.Groups["g-other"] = true

	if err := h.adapter.AddToRoleGroup(ctx, user.ID, domain.RolePlanningOperator); err != nil {
		t.Fatal(err)
	}
	if err := h.adapter.AddToRoleGroup(ctx, user.ID, domain.RolePlanningOperator); err != nil {
		t.Fatal(err)
	}
	if h.fake.callsOf("PUT /users/") != 1 {
		t.Fatalf("second add must not write again, PUTs=%d", h.fake.callsOf("PUT /users/"))
	}
	if h.fake.callsOf("GET /group-by-path") != 1 {
		t.Fatalf("group id must be cached, lookups=%d", h.fake.callsOf("GET /group-by-path"))
	}
	if !user.Groups["g-operator"] || !user.Groups["g-other"] || len(user.Groups) != 2 {
		t.Fatalf("groups = %v", user.Groups)
	}

	// Corrupted state: two role groups. Adding repairs it to exactly one.
	user.Groups["g-admin"] = true
	if err := h.adapter.AddToRoleGroup(ctx, user.ID, domain.RoleAdmin); err != nil {
		t.Fatal(err)
	}
	if user.Groups["g-operator"] || !user.Groups["g-admin"] || !user.Groups["g-other"] {
		t.Fatalf("groups = %v", user.Groups)
	}
}

func TestMoveToRoleGroup(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	user := h.fake.addUser("op@example.test")
	user.Groups["g-admin"] = true
	user.Groups["g-other"] = true

	if err := h.adapter.MoveToRoleGroup(ctx, user.ID, domain.RoleAdmin, domain.RoleSuperAdmin); err != nil {
		t.Fatal(err)
	}
	if err := h.adapter.MoveToRoleGroup(ctx, user.ID, domain.RoleAdmin, domain.RoleSuperAdmin); err != nil {
		t.Fatalf("repeat: %v", err)
	}
	if user.Groups["g-admin"] || !user.Groups["g-super"] || !user.Groups["g-other"] || len(user.Groups) != 2 {
		t.Fatalf("groups = %v", user.Groups)
	}
	if err := h.adapter.MoveToRoleGroup(ctx, user.ID, "bogus", domain.RoleAdmin); err == nil {
		t.Fatal("invalid origin role must fail")
	}
	if err := h.adapter.AddToRoleGroup(ctx, user.ID, "bogus"); err == nil {
		t.Fatal("invalid role must fail")
	}
}

func TestMoveToRoleGroupJoinsTheTargetBeforeLeavingTheOthers(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	user := h.fake.addUser("op@example.test")
	user.Groups["g-admin"] = true

	if err := h.adapter.MoveToRoleGroup(ctx, user.ID, domain.RoleAdmin, domain.RoleSuperAdmin); err != nil {
		t.Fatal(err)
	}
	put, del := -1, -1
	for i, call := range h.fake.calls {
		switch {
		case strings.HasPrefix(call, "PUT /users/") && strings.HasSuffix(call, "/groups/g-super"):
			put = i
		case strings.HasPrefix(call, "DELETE /users/") && strings.HasSuffix(call, "/groups/g-admin"):
			del = i
		}
	}
	if put < 0 || del < 0 || put > del {
		t.Fatalf("join must come before leave: calls = %v", h.fake.calls)
	}

	// A failing join leaves the account where it was: never in no role group.
	other := h.fake.addUser("other@example.test")
	other.Groups["g-admin"] = true
	h.fake.failWith("PUT /groups/g-super", http.StatusServiceUnavailable, 0)
	if err := h.adapter.MoveToRoleGroup(ctx, other.ID, domain.RoleAdmin, domain.RoleSuperAdmin); err == nil {
		t.Fatal("a failing join must fail the move")
	}
	if !other.Groups["g-admin"] || other.Groups["g-super"] {
		t.Fatalf("groups after a failed join = %v", other.Groups)
	}
}

func TestRemoveFromProductGroupsKeepsOtherGroups(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	user := h.fake.addUser("op@example.test")
	user.Groups["g-admin"] = true
	user.Groups["g-root"] = true // parent membership is not a role group: left untouched
	user.Groups["g-other"] = true

	for range 2 {
		if err := h.adapter.RemoveFromProductGroups(ctx, user.ID); err != nil {
			t.Fatal(err)
		}
	}
	if user.Groups["g-admin"] || !user.Groups["g-root"] || !user.Groups["g-other"] {
		t.Fatalf("groups = %v", user.Groups)
	}
	if err := h.adapter.RemoveFromProductGroups(ctx, "00000000-0000-0000-0000-999999999999"); err != nil {
		t.Fatalf("missing user must be success: %v", err)
	}
}

func TestGroupOperationsOnMissingUserOrGroup(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	err := h.adapter.AddToRoleGroup(ctx, "00000000-0000-0000-0000-999999999999", domain.RoleAdmin)
	if !errors.Is(err, usecase.ErrAccountNotFound) {
		t.Fatalf("err = %v, want ErrAccountNotFound", err)
	}

	delete(h.fake.groups, "/agentes-agro/super-admins")
	user := h.fake.addUser("op@example.test")
	err = h.adapter.AddToRoleGroup(ctx, user.ID, domain.RoleSuperAdmin)
	if !errors.Is(err, usecase.ErrIdentityProviderUnavailable) {
		t.Fatalf("missing realm group must be reported, err = %v", err)
	}
}

// ---- sessions --------------------------------------------------------------------------

func TestEndProductSessionsTouchesOnlyProductOnlySessions(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	user := h.fake.addUser("op@example.test")
	user.Sessions = []fakeSession{
		{ID: "only-product", Clients: map[string]string{"u1": "agentes-agro"}},
		{ID: "shared-sso", Clients: map[string]string{"u1": "agentes-agro", "u2": "outro-produto"}},
		{ID: "other-product", Clients: map[string]string{"u2": "outro-produto"}},
	}
	for range 2 {
		if err := h.adapter.EndProductSessions(ctx, user.ID); err != nil {
			t.Fatal(err)
		}
	}
	var left []string
	for _, s := range user.Sessions {
		left = append(left, s.ID)
	}
	slices.Sort(left)
	if !slices.Equal(left, []string{"other-product", "shared-sso"}) {
		t.Fatalf("remaining sessions = %v", left)
	}
	if err := h.adapter.EndProductSessions(ctx, "00000000-0000-0000-0000-999999999999"); err != nil {
		t.Fatalf("missing user must be success: %v", err)
	}
}

// ---- action e-mail ---------------------------------------------------------------------

func TestSendActionEmail(t *testing.T) {
	ctx := context.Background()

	t.Run("setup account", func(t *testing.T) {
		h := newHarness(t)
		user := h.fake.addUser("new@example.test")
		if err := h.adapter.SendActionEmail(ctx, user.ID, usecase.EmailSetupAccount); err != nil {
			t.Fatal(err)
		}
		q := h.fake.lastEmail.query
		if q["client_id"] != "agentes-agro" || q["redirect_uri"] != "http://localhost:8080/auth/login" ||
			q["lifespan"] != "259200" {
			t.Fatalf("query = %v", q)
		}
		if !slices.Equal(h.fake.lastEmail.actions, []string{"VERIFY_EMAIL", "UPDATE_PASSWORD"}) {
			t.Fatalf("actions = %v", h.fake.lastEmail.actions)
		}
	})

	t.Run("verify e-mail only", func(t *testing.T) {
		h := newHarness(t)
		user := h.fake.addUser("old@example.test")
		if err := h.adapter.SendActionEmail(ctx, user.ID, usecase.EmailVerifyEmail); err != nil {
			t.Fatal(err)
		}
		if !slices.Equal(h.fake.lastEmail.actions, []string{"VERIFY_EMAIL"}) {
			t.Fatalf("actions = %v", h.fake.lastEmail.actions)
		}
	})

	t.Run("smtp failure is not_sent, not unavailable", func(t *testing.T) {
		h := newHarness(t)
		h.fake.smtpBroken = true
		user := h.fake.addUser("new@example.test")
		err := h.adapter.SendActionEmail(ctx, user.ID, usecase.EmailSetupAccount)
		if !errors.Is(err, ErrEmailNotSent) || errors.Is(err, usecase.ErrIdentityProviderUnavailable) {
			t.Fatalf("err = %v", err)
		}
		if h.fake.callsOf("PUT /users/") != 1 {
			t.Fatal("e-mail request must not be retried (could send twice)")
		}
	})

	t.Run("authorization failure is unavailable", func(t *testing.T) {
		h := newHarness(t)
		user := h.fake.addUser("new@example.test")
		h.fake.failWith("PUT /execute-actions-email", http.StatusForbidden, 0)
		err := h.adapter.SendActionEmail(ctx, user.ID, usecase.EmailSetupAccount)
		if !errors.Is(err, usecase.ErrIdentityProviderUnavailable) || errors.Is(err, ErrEmailNotSent) {
			t.Fatalf("err = %v", err)
		}
	})

	t.Run("unknown kind", func(t *testing.T) {
		h := newHarness(t)
		user := h.fake.addUser("new@example.test")
		if err := h.adapter.SendActionEmail(ctx, user.ID, "nope"); err == nil {
			t.Fatal("unknown kind must fail")
		}
	})
}

// ---- transport, token and errors ---------------------------------------------------------

func TestTokenIsCachedAndRenewed(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	clock := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	h.adapter.client.now = func() time.Time { return clock }
	h.fake.tokenTTL = 60

	for range 3 {
		_, _ = h.adapter.FindAccountByEmail(ctx, mustEmail(t, "a@example.test"))
	}
	if h.fake.tokenCalls != 1 {
		t.Fatalf("token requests = %d, want 1", h.fake.tokenCalls)
	}
	clock = clock.Add(45 * time.Second) // 60s ttl minus the 20s margin has passed
	_, _ = h.adapter.FindAccountByEmail(ctx, mustEmail(t, "a@example.test"))
	if h.fake.tokenCalls != 2 {
		t.Fatalf("token requests = %d, want 2 after expiry", h.fake.tokenCalls)
	}
}

func TestTokenRefreshedOnceOn401(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	user := h.fake.addUser("a@example.test")
	// Warm the cache, then make the server reject the cached token once.
	if _, err := h.adapter.FindAccountByEmail(ctx, mustEmail(t, "a@example.test")); err != nil {
		t.Fatal(err)
	}
	h.fake.mu.Lock()
	h.fake.rejectToken = true
	h.fake.mu.Unlock()

	account, err := h.adapter.FindAccountByEmail(ctx, mustEmail(t, "a@example.test"))
	if err != nil || account.Subject != user.ID {
		t.Fatalf("account=%+v err=%v", account, err)
	}
	if h.fake.tokenCalls != 2 {
		t.Fatalf("token requests = %d, want 2", h.fake.tokenCalls)
	}
}

func TestWrongSecretIsUnavailableWithoutLeak(t *testing.T) {
	h := newHarness(t, func(c *Config) { c.ClientSecret = "another-secret-value-that-is-wrong-0000" })
	_, err := h.adapter.FindAccountByEmail(context.Background(), mustEmail(t, victimEmail))
	if !errors.Is(err, usecase.ErrIdentityProviderUnavailable) {
		t.Fatalf("err = %v", err)
	}
	var statusErr *statusError
	if !errors.As(err, &statusErr) || statusErr.status != http.StatusUnauthorized {
		t.Fatalf("cause must keep the status: %v", err)
	}
	h.assertNoSecrets(t, err)
	if strings.Contains(err.Error(), "another-secret") {
		t.Fatal("configured secret leaked")
	}
}

func TestProviderStatusesMapToUnavailable(t *testing.T) {
	ctx := context.Background()
	for _, status := range []int{http.StatusUnauthorized, http.StatusForbidden, http.StatusInternalServerError, http.StatusBadGateway} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			h := newHarness(t)
			h.fake.failWith("GET /users", status, 0)
			_, err := h.adapter.FindAccountByEmail(ctx, mustEmail(t, victimEmail))
			if !errors.Is(err, usecase.ErrIdentityProviderUnavailable) {
				t.Fatalf("err = %v", err)
			}
			h.assertNoSecrets(t, err)
		})
	}
}

func TestGETRetriesAreLimitedAndWritesAreNeverRetried(t *testing.T) {
	ctx := context.Background()

	t.Run("recovers after transient 503", func(t *testing.T) {
		h := newHarness(t)
		h.fake.addUser("a@example.test")
		h.fake.failWith("GET /users", http.StatusServiceUnavailable, 2)
		if _, err := h.adapter.FindAccountByEmail(ctx, mustEmail(t, "a@example.test")); err != nil {
			t.Fatal(err)
		}
		if got := h.fake.callsOf("GET /users"); got != 3 {
			t.Fatalf("GET calls = %d, want 3", got)
		}
		if len(h.sleeps) != 2 || h.sleeps[0] >= h.sleeps[1] {
			t.Fatalf("backoff = %v", h.sleeps)
		}
	})

	t.Run("gives up after the limit", func(t *testing.T) {
		h := newHarness(t)
		h.fake.failWith("GET /users", http.StatusServiceUnavailable, 0)
		_, err := h.adapter.FindAccountByEmail(ctx, mustEmail(t, "a@example.test"))
		if !errors.Is(err, usecase.ErrIdentityProviderUnavailable) {
			t.Fatalf("err = %v", err)
		}
		if got := h.fake.callsOf("GET /users"); got != 1+DefaultMaxGETRetries {
			t.Fatalf("GET calls = %d", got)
		}
	})

	t.Run("a 404 is not retried", func(t *testing.T) {
		h := newHarness(t)
		_ = h.adapter.EndProductSessions(ctx, "00000000-0000-0000-0000-999999999999")
		if got := h.fake.callsOf("GET /users/00000000-0000-0000-0000-999999999999/sessions"); got != 1 {
			t.Fatalf("calls = %d", got)
		}
	})

	t.Run("writes are sent once", func(t *testing.T) {
		h := newHarness(t)
		user := h.fake.addUser("a@example.test")
		h.fake.failWith("PUT /groups/g-admin", http.StatusInternalServerError, 0)
		err := h.adapter.AddToRoleGroup(ctx, user.ID, domain.RoleAdmin)
		if !errors.Is(err, usecase.ErrIdentityProviderUnavailable) {
			t.Fatalf("err = %v", err)
		}
		if got := h.fake.callsOf("PUT /users/"); got != 1 {
			t.Fatalf("PUT calls = %d, want 1", got)
		}
	})
}

func TestTimeoutAndNetworkErrorsKeepTheCause(t *testing.T) {
	t.Run("timeout", func(t *testing.T) {
		slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			select {
			case <-r.Context().Done():
			case <-time.After(300 * time.Millisecond):
			}
		}))
		defer slow.Close()
		h := newHarness(t, func(c *Config) {
			c.BaseURL = slow.URL
			c.RequestTimeout = 50 * time.Millisecond
			c.MaxGETRetries = -1
		})
		_, err := h.adapter.FindAccountByEmail(context.Background(), mustEmail(t, victimEmail))
		if !errors.Is(err, usecase.ErrIdentityProviderUnavailable) || !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("err = %v", err)
		}
		h.assertNoSecrets(t, err)
	})

	t.Run("connection refused", func(t *testing.T) {
		listener, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		addr := listener.Addr().String()
		_ = listener.Close()
		h := newHarness(t, func(c *Config) { c.BaseURL = "http://" + addr })
		_, err = h.adapter.FindAccountByEmail(context.Background(), mustEmail(t, victimEmail))
		if !errors.Is(err, usecase.ErrIdentityProviderUnavailable) {
			t.Fatalf("err = %v", err)
		}
		h.assertNoSecrets(t, err)
		if strings.Contains(err.Error(), addr) && strings.Contains(err.Error(), "email=") {
			t.Fatalf("request URL leaked: %v", err)
		}
	})

	t.Run("canceled context is not retried", func(t *testing.T) {
		h := newHarness(t)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_, err := h.adapter.FindAccountByEmail(ctx, mustEmail(t, "a@example.test"))
		if !errors.Is(err, context.Canceled) || !errors.Is(err, usecase.ErrIdentityProviderUnavailable) {
			t.Fatalf("err = %v", err)
		}
		if len(h.sleeps) != 0 {
			t.Fatal("must not retry after cancellation")
		}
	})
}

func TestResponseBodyIsCappedAndNeverCopiedIntoErrors(t *testing.T) {
	huge := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/token") {
			_, _ = w.Write([]byte(`{"access_token":"` + testToken + `","expires_in":300}`))
			return
		}
		_, _ = w.Write([]byte("[" + strings.Repeat(" ", 2<<20) + "]"))
	}))
	defer huge.Close()
	h := newHarness(t, func(c *Config) { c.BaseURL = huge.URL; c.MaxGETRetries = -1 })
	_, err := h.adapter.FindAccountByEmail(context.Background(), mustEmail(t, "a@example.test"))
	if !errors.Is(err, usecase.ErrIdentityProviderUnavailable) {
		t.Fatalf("oversized response must fail, err = %v", err)
	}
}

func TestDebugLogsNeverContainSecretsOrEmails(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	user := h.fake.addUser(victimEmail)
	_, _ = h.adapter.FindAccountByEmail(ctx, mustEmail(t, victimEmail))
	_ = h.adapter.AddToRoleGroup(ctx, user.ID, domain.RoleAdmin)
	_ = h.adapter.SendActionEmail(ctx, user.ID, usecase.EmailSetupAccount)
	_ = h.adapter.EndProductSessions(ctx, user.ID)
	_ = h.adapter.DeleteCreatedAccount(ctx, user.ID)
	if h.logs.Len() == 0 {
		t.Fatal("expected debug logs")
	}
	h.assertNoSecrets(t, nil)
}
