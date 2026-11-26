package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	httpadapter "github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/adapter/http"
	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/adapter/perimeter"
	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/domain"
	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/platform"
	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/usecase"
	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/usecase/fakes"
)

const (
	testIssuer    = "http://idp.test/realms/inceres"
	superID       = "00000000-0000-4000-8000-000000000001"
	adminID       = "00000000-0000-4000-8000-000000000002"
	operatorCoID  = "00000000-0000-4000-8000-0000000000a0"
	clientCoID    = "00000000-0000-4000-8000-0000000000a1"
	testOrigin    = "http://localhost:3000"
	validCNPJ     = "11.444.777/0001-61"
	secretInError = "s3cr3t-value"
)

// headerAuthenticator turns X-Test-User into the Principal of a stored user.
type headerAuthenticator struct{ db *fakes.Memory }

func (a headerAuthenticator) Authenticate(r *http.Request) (domain.Principal, error) {
	user, ok := a.db.Users[domain.UserID(r.Header.Get("X-Test-User"))]
	if !ok {
		return domain.Principal{}, usecase.ErrUnauthenticated
	}
	permissions := []domain.Permission{domain.PermDashboardView, domain.PermUsersManage}
	if user.Role == domain.RoleSuperAdmin {
		permissions = append(permissions, domain.PermCompaniesManage, domain.PermUsersManageAll)
	}
	return domain.Principal{
		UserID: user.ID, CompanyID: user.CompanyID, Name: user.Name, Email: user.Email,
		Role: user.Role, Permissions: permissions, FarmScope: user.FarmScope,
	}, nil
}

type fakeReadiness struct{ up bool }

func (f fakeReadiness) Execute(context.Context) usecase.HealthReport {
	if f.up {
		return usecase.HealthReport{Status: usecase.StatusUp}
	}
	return usecase.HealthReport{Status: usecase.StatusDown}
}

// composed builds the complete router through the same wiring the process uses
// (newRouteGroups), with in-memory fakes instead of Postgres and Keycloak.
func composed(t *testing.T) (http.Handler, *fakes.Memory) {
	t.Helper()
	db := fakes.NewMemory()
	idp := fakes.NewProvider(testIssuer)
	clock := &fakes.FixedClock{T: time.Date(2026, 11, 22, 12, 0, 0, 0, time.UTC)}
	for _, company := range []domain.Company{
		{ID: operatorCoID, Name: "Agentes Agro", Document: "00000000000191", City: "Sorriso", State: "MT", Plan: domain.PlanPro, Status: domain.CompanyActive, IsPlatformOperator: true, CreatedAt: clock.T},
		{ID: clientCoID, Name: "Fazenda A", Document: "11222333000181", City: "Sorriso", State: "MT", Plan: domain.PlanPro, Status: domain.CompanyActive, CreatedAt: clock.T},
	} {
		db.Companies[company.ID] = company
	}
	for _, user := range []domain.User{
		{ID: superID, CompanyID: operatorCoID, Name: "Sofia", Email: "sofia@agro.test", Role: domain.RoleSuperAdmin, Status: domain.UserActive, FarmScope: domain.AllFarms(), CreatedAt: clock.T},
		{ID: adminID, CompanyID: clientCoID, Name: "Ana", Email: "ana@a.test", Role: domain.RoleAdmin, Status: domain.UserActive, FarmScope: domain.AllFarms(), CreatedAt: clock.T},
	} {
		account := idp.SeedAccount(user.Email, true)
		idp.Groups[account.Subject] = user.Role
		db.PutUser(user, &domain.Identity{UserID: user.ID, Issuer: testIssuer, Subject: account.Subject})
	}

	logger := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))
	registry := fakes.NewRegistry()
	groups := newRouteGroups(dependencies{
		Logger: logger, Store: db, Registry: registry, PerimeterParser: perimeter.NewParser(),
		Terrain: fakes.NewTerrain(registry), TerrainRules: domain.DefaultTerrainRules(), TerrainWakeup: noWakeup{},
		Conditions: fakes.NewFieldConditions(registry), Weather: &fakes.Weather{},
		Trafficability: usecase.TrafficabilitySettings{Location: time.UTC, GaugeMaxStaleness: 36 * time.Hour, InspectionValidity: 12 * time.Hour},
		Documents:      fakes.NewObjectStore(),
		Identity:       idp, Clock: clock, IDs: platform.UUIDGenerator{},
		Settings: usecase.Settings{
			InvitationTTL: 72 * time.Hour, TrialDuration: 14 * 24 * time.Hour,
			LastAccessInterval: 5 * time.Minute, DefaultPageSize: 20, MaxPageSize: 100,
			Areas:            domain.AreaRules{MinFarmAreaHa: 0.1, MinFieldAreaHa: 0.01, ToleranceM2: 10},
			MaxDocumentBytes: 10 << 20,
		},
	})
	return httpadapter.NewRouter(httpadapter.RouterDeps{
		Logger:             logger,
		Liveness:           usecase.CheckLiveness{},
		Startup:            usecase.NewCheckStartup(startedLifecycle()),
		Readiness:          fakeReadiness{up: true},
		ProbeTimeout:       time.Second,
		CORSAllowedOrigins: []string{testOrigin},
		Authenticator:      headerAuthenticator{db: db},
		Groups:             groups,
	}), db
}

func startedLifecycle() *usecase.Lifecycle {
	lifecycle := usecase.NewLifecycle()
	lifecycle.MarkStarted()
	return lifecycle
}

func send(handler http.Handler, as, method, path string, body any) (int, map[string]any) {
	var reader *bytes.Reader
	if body == nil {
		reader = bytes.NewReader(nil)
	} else {
		encoded, _ := json.Marshal(body)
		reader = bytes.NewReader(encoded)
	}
	request := httptest.NewRequestWithContext(context.Background(), method, path, reader)
	request.Header.Set("Content-Type", "application/json")
	if as != "" {
		request.Header.Set("X-Test-User", as)
	}
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	var decoded map[string]any
	_ = json.Unmarshal(recorder.Body.Bytes(), &decoded)
	return recorder.Code, decoded
}

// A story through the complete router: the super admin registers a company,
// the company's admin invites a user, and everything shows up in the listings.
func TestCompositionServesTheAdministrationFlow(t *testing.T) {
	handler, _ := composed(t)

	status, company := send(handler, superID, http.MethodPost, "/companies", map[string]any{
		"name": "Fazenda Nova", "document": validCNPJ, "city": "Lucas do Rio Verde", "state": "MT", "plan": "pro",
	})
	if status != http.StatusCreated {
		t.Fatalf("create company: status %d, %v", status, company)
	}
	companyID, _ := company["id"].(string)

	status, invited := send(handler, adminID, http.MethodPost, "/invitations", map[string]any{
		"name": "Carla Campos", "email": "carla@a.test", "role": "planning_operator",
		"company_id": clientCoID, "farm_scope": map[string]any{"mode": "all"},
	})
	if status != http.StatusCreated || invited["status"] != "invited" {
		t.Fatalf("invite: status %d, %v", status, invited)
	}

	if status, page := send(handler, adminID, http.MethodGet, "/users", nil); status != http.StatusOK || page["total"] != float64(2) {
		t.Errorf("users of the client company: status %d, %v", status, page)
	}
	if status, me := send(handler, adminID, http.MethodGet, "/auth/me", nil); status != http.StatusOK || me["role"] != "admin" {
		t.Errorf("/auth/me: status %d, %v", status, me)
	}
	if status, _ := send(handler, adminID, http.MethodGet, "/companies/"+companyID, nil); status != http.StatusForbidden {
		t.Errorf("an admin must not read companies: status %d", status)
	}
	if status, page := send(handler, superID, http.MethodGet, "/companies", nil); status != http.StatusOK || page["total"] != float64(2) {
		t.Errorf("companies: status %d, %v", status, page)
	}
	if status, _ := send(handler, "", http.MethodGet, "/users", nil); status != http.StatusUnauthorized {
		t.Errorf("anonymous: status %d", status)
	}
	if status, _ := send(handler, "", http.MethodGet, "/health/live", nil); status != http.StatusOK {
		t.Errorf("liveness: status %d", status)
	}
}

func validAppEnv() map[string]string {
	key := base64.StdEncoding.EncodeToString([]byte(strings.Repeat("k", 32)))
	return map[string]string{
		"API_DATABASE_URL":                    "postgres://api_app:" + secretInError + "@agro-postgres:5432/api?sslmode=disable",
		"KEYCLOAK_URL":                        "http://keycloak:8080",
		"KEYCLOAK_ISSUER_URL":                 "http://localhost:8180",
		"KEYCLOAK_PROVISIONER_CLIENT_SECRET":  secretInError,
		"KEYCLOAK_AGENTES_AGRO_CLIENT_SECRET": secretInError,
		"API_SESSION_ENC_KEY":                 key,
	}
}

func TestLoadAppConfig(t *testing.T) {
	key := base64.StdEncoding.EncodeToString([]byte(strings.Repeat("k", 32)))
	valid := map[string]string{
		"API_DATABASE_URL":                    "postgres://api_app:" + secretInError + "@agro-postgres:5432/api?sslmode=disable",
		"KEYCLOAK_URL":                        "http://keycloak:8080",
		"KEYCLOAK_ISSUER_URL":                 "http://localhost:8180",
		"KEYCLOAK_PROVISIONER_CLIENT_SECRET":  secretInError,
		"KEYCLOAK_AGENTES_AGRO_CLIENT_SECRET": secretInError,
		"API_SESSION_ENC_KEY":                 key,
		"API_INVITATION_TTL":                  "48h",
	}
	lookup := func(values map[string]string) platform.LookupFunc {
		return func(name string) (string, bool) {
			value, ok := values[name]
			return value, ok
		}
	}
	without := func(name string) map[string]string {
		copied := map[string]string{}
		for k, v := range valid {
			if k != name {
				copied[k] = v
			}
		}
		return copied
	}

	t.Run("a complete environment loads and shares the invitation ttl", func(t *testing.T) {
		cfg, err := loadAppConfig(lookup(valid))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.Keycloak.ActionLifespan != 48*time.Hour || cfg.InvitationTTL != 48*time.Hour {
			t.Errorf("invitation ttl not shared: keycloak %v, settings %v", cfg.Keycloak.ActionLifespan, cfg.InvitationTTL)
		}
		if cfg.OIDC.IssuerURL != "http://localhost:8180/realms/inceres" || cfg.Keycloak.BaseURL != "http://keycloak:8080" {
			t.Errorf("urls: issuer %q, internal %q", cfg.OIDC.IssuerURL, cfg.Keycloak.BaseURL)
		}
		if _, err := cfg.settings(); err != nil {
			t.Errorf("settings: %v", err)
		}
	})

	t.Run("the startup log carries no secret", func(t *testing.T) {
		cfg, err := loadAppConfig(lookup(valid))
		if err != nil {
			t.Fatal(err)
		}
		var out bytes.Buffer
		slog.New(slog.NewJSONHandler(&out, nil)).Info("api started", "config", cfg.logValue())
		if strings.Contains(out.String(), secretInError) || strings.Contains(out.String(), key) {
			t.Errorf("the log leaked a secret: %s", out.String())
		}
	})

	for _, required := range []string{
		"API_DATABASE_URL", "KEYCLOAK_PROVISIONER_CLIENT_SECRET", "KEYCLOAK_AGENTES_AGRO_CLIENT_SECRET", "API_SESSION_ENC_KEY",
	} {
		t.Run("missing "+required+" aborts with a clear message", func(t *testing.T) {
			_, err := loadAppConfig(lookup(without(required)))
			if err == nil {
				t.Fatalf("expected an error without %s", required)
			}
			if strings.Contains(err.Error(), secretInError) {
				t.Errorf("error leaked a secret: %v", err)
			}
		})
	}
}
