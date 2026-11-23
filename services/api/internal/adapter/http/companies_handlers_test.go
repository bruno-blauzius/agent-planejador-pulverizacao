package httpadapter_test

import (
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/usecase"
)

func contains(text, part string) bool { return strings.Contains(text, part) }

func newCompanyBody(mutate func(map[string]any)) map[string]any {
	body := map[string]any{"name": "Fazenda Nova", "document": "11.444.777/0001-61", "city": "Lucas do Rio Verde", "state": "mt", "plan": "pro"}
	if mutate != nil {
		mutate(body)
	}
	return body
}

func TestListCompanies(t *testing.T) {
	e := newAPIEnv(t)
	got := e.call(superAdminID, http.MethodGet, "/companies", nil)
	if got.status != http.StatusOK {
		t.Fatalf("status = %d, body %s", got.status, got.raw)
	}
	body := got.object(t)
	items, _ := body["items"].([]any)
	if body["total"] != float64(2) || len(items) != 2 {
		t.Errorf("the platform operator must not be listed: %v", body)
	}
	first, _ := items[0].(map[string]any)
	if first["document"] == nil || first["farm_count"] == nil || first["user_count"] == nil || first["created_at"] == nil {
		t.Errorf("company shape: %v", first)
	}

	if search := e.call(superAdminID, http.MethodGet, "/companies?search=fazenda+a", nil); search.object(t)["total"] != float64(1) {
		t.Errorf("search: %s", search.raw)
	}
	if page := e.call(superAdminID, http.MethodGet, "/companies?page=2&page_size=1", nil); len(page.object(t)["items"].([]any)) != 1 {
		t.Errorf("page 2: %s", page.raw)
	}
	for name, path := range map[string]string{"bad status": "/companies?status=gone", "bad page": "/companies?page=-1", "big page size": "/companies?page_size=500"} {
		if bad := e.call(superAdminID, http.MethodGet, path, nil); bad.status != http.StatusUnprocessableEntity {
			t.Errorf("%s: status %d", name, bad.status)
		}
	}
	if forbidden := e.call(adminAID, http.MethodGet, "/companies", nil); forbidden.status != http.StatusForbidden {
		t.Errorf("admin: status %d", forbidden.status)
	}
}

func TestCompaniesSummary(t *testing.T) {
	e := newAPIEnv(t)
	got := e.call(superAdminID, http.MethodGet, "/companies/summary", nil)
	if got.status != http.StatusOK {
		t.Fatalf("status = %d, body %s", got.status, got.raw)
	}
	for _, field := range []string{
		"active_companies", "trial_companies", "farm_count", "monitored_area_ha", "user_count",
		"pending_invitations", "applications_this_month", "applications_previous_month",
	} {
		if _, ok := got.object(t)[field]; !ok {
			t.Errorf("summary misses %q: %s", field, got.raw)
		}
	}
	if forbidden := e.call(adminAID, http.MethodGet, "/companies/summary", nil); forbidden.status != http.StatusForbidden {
		t.Errorf("admin: status %d", forbidden.status)
	}
}

func TestGetCompany(t *testing.T) {
	e := newAPIEnv(t)
	for _, tt := range []struct {
		name, as, id string
		want         int
	}{
		{"super admin", superAdminID, companyAID, 200},
		{"the platform operator company is hidden", superAdminID, operatorCompanyID, 404},
		{"unknown", superAdminID, missingID, 404},
		{"malformed", superAdminID, "x", 404},
		{"admin may not", adminAID, companyAID, 403},
	} {
		if got := e.call(tt.as, http.MethodGet, "/companies/"+tt.id, nil); got.status != tt.want {
			t.Errorf("%s: status = %d, want %d, body %s", tt.name, got.status, tt.want, got.raw)
		}
	}
}

func TestCreateCompany(t *testing.T) {
	t.Run("creates a company and normalizes the fields", func(t *testing.T) {
		e := newAPIEnv(t)
		got := e.call(superAdminID, http.MethodPost, "/companies", newCompanyBody(nil))
		if got.status != http.StatusCreated {
			t.Fatalf("status = %d, body %s", got.status, got.raw)
		}
		body := got.object(t)
		if body["document"] != "11444777000161" || body["state"] != "MT" || body["status"] != "active" || body["trial_ends_at"] != nil {
			t.Errorf("company = %v", body)
		}
	})

	t.Run("trial plan starts the evaluation", func(t *testing.T) {
		e := newAPIEnv(t)
		got := e.call(superAdminID, http.MethodPost, "/companies", newCompanyBody(func(b map[string]any) { b["plan"] = "trial" }))
		body := got.object(t)
		if got.status != http.StatusCreated || body["status"] != "trial" || body["trial_ends_at"] == nil {
			t.Errorf("status = %d, company = %v", got.status, body)
		}
	})

	tests := []struct {
		name      string
		as        string
		body      any
		want      int
		wantCode  string
		wantField string
	}{
		{"duplicate document", superAdminID, newCompanyBody(func(b map[string]any) { b["document"] = "11222333000181" }), 409, "conflict", ""},
		{"invalid check digit", superAdminID, newCompanyBody(func(b map[string]any) { b["document"] = "11444777000162" }), 422, "validation_failed", "document"},
		{"unknown plan", superAdminID, newCompanyBody(func(b map[string]any) { b["plan"] = "gold" }), 422, "validation_failed", "plan"},
		{"missing city", superAdminID, newCompanyBody(func(b map[string]any) { delete(b, "city") }), 422, "validation_failed", "city"},
		{"unknown field", superAdminID, newCompanyBody(func(b map[string]any) { b["status"] = "active" }), 422, "validation_failed", "status"},
		{"admin may not", adminAID, newCompanyBody(nil), 403, "forbidden", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newAPIEnv(t)
			got := e.call(tt.as, http.MethodPost, "/companies", tt.body)
			if got.status != tt.want || got.errorCode(t) != tt.wantCode {
				t.Fatalf("status = %d code %q, want %d %q, body %.200s", got.status, got.errorCode(t), tt.want, tt.wantCode, got.raw)
			}
			if tt.wantField != "" && !slices.Contains(got.errorFields(t), tt.wantField) {
				t.Errorf("details %v miss %q", got.errorFields(t), tt.wantField)
			}
		})
	}
}

func TestUpdateCompany(t *testing.T) {
	t.Run("changes details and plan", func(t *testing.T) {
		e := newAPIEnv(t)
		got := e.call(superAdminID, http.MethodPatch, "/companies/"+companyAID, map[string]any{"name": "Fazenda A+", "plan": "trial"})
		body := got.object(t)
		if got.status != http.StatusOK || body["name"] != "Fazenda A+" || body["plan"] != "trial" || body["status"] != "trial" {
			t.Errorf("status = %d, company = %v", got.status, body)
		}
	})

	t.Run("inactivating cuts the product sessions of its users", func(t *testing.T) {
		e := newAPIEnv(t)
		got := e.call(superAdminID, http.MethodPatch, "/companies/"+companyAID, map[string]any{"status": "inactive"})
		if got.status != http.StatusOK || got.object(t)["status"] != "inactive" {
			t.Fatalf("status = %d, body %s", got.status, got.raw)
		}
		if len(e.idp.EndedSessions) != 2 {
			t.Errorf("ended sessions = %d, want 2 (admin and operator of the company)", len(e.idp.EndedSessions))
		}
	})

	t.Run("provider down while inactivating is 502", func(t *testing.T) {
		e := newAPIEnv(t)
		e.idp.Fail("EndProductSessions", usecase.ErrIdentityProviderUnavailable)
		got := e.call(superAdminID, http.MethodPatch, "/companies/"+companyAID, map[string]any{"status": "inactive"})
		if got.status != http.StatusBadGateway || got.errorCode(t) != "identity_provider_unavailable" {
			t.Errorf("status = %d, body %s", got.status, got.raw)
		}
	})

	tests := []struct {
		name      string
		as        string
		id        string
		body      any
		want      int
		wantCode  string
		wantField string
	}{
		{"empty object", superAdminID, companyAID, map[string]any{}, 422, "validation_failed", ""},
		{"unknown status", superAdminID, companyAID, map[string]any{"status": "gone"}, 422, "validation_failed", "status"},
		{"trial status needs the trial plan", superAdminID, companyAID, map[string]any{"status": "trial"}, 422, "validation_failed", "status"},
		{"bad state", superAdminID, companyAID, map[string]any{"state": "Mato"}, 422, "validation_failed", "state"},
		{"the platform operator company is hidden", superAdminID, operatorCompanyID, map[string]any{"name": "X"}, 404, "not_found", ""},
		{"admin may not", adminAID, companyAID, map[string]any{"name": "X"}, 403, "forbidden", ""},
		{"malformed id", superAdminID, "x", map[string]any{"name": "X"}, 404, "not_found", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newAPIEnv(t)
			got := e.call(tt.as, http.MethodPatch, "/companies/"+tt.id, tt.body)
			if got.status != tt.want || got.errorCode(t) != tt.wantCode {
				t.Fatalf("status = %d code %q, want %d %q, body %.200s", got.status, got.errorCode(t), tt.want, tt.wantCode, got.raw)
			}
			if tt.wantField != "" && !slices.Contains(got.errorFields(t), tt.wantField) {
				t.Errorf("details %v miss %q", got.errorFields(t), tt.wantField)
			}
		})
	}
}

func TestDeleteCompany(t *testing.T) {
	t.Run("a company with active users cannot be deleted", func(t *testing.T) {
		e := newAPIEnv(t)
		got := e.call(superAdminID, http.MethodDelete, "/companies/"+companyAID, nil)
		if got.status != http.StatusConflict || got.errorCode(t) != "company_has_active_users" {
			t.Errorf("status = %d, body %s", got.status, got.raw)
		}
	})

	t.Run("an empty company is soft deleted", func(t *testing.T) {
		e := newAPIEnv(t)
		created := e.call(superAdminID, http.MethodPost, "/companies", newCompanyBody(nil))
		id, _ := created.object(t)["id"].(string)
		if got := e.call(superAdminID, http.MethodDelete, "/companies/"+id, nil); got.status != http.StatusNoContent {
			t.Fatalf("status = %d, body %s", got.status, got.raw)
		}
		if after := e.call(superAdminID, http.MethodGet, "/companies/"+id, nil); after.status != http.StatusNotFound {
			t.Errorf("deleted company: status %d", after.status)
		}
		if again := e.call(superAdminID, http.MethodPost, "/companies", newCompanyBody(nil)); again.status != http.StatusConflict {
			t.Errorf("the CNPJ stays reserved: status %d", again.status)
		}
	})

	for _, tt := range []struct {
		name, as, id string
		want         int
	}{
		{"admin may not", adminAID, companyAID, 403},
		{"the platform operator company", superAdminID, operatorCompanyID, 404},
		{"malformed id", superAdminID, "x", 404},
	} {
		t.Run(tt.name, func(t *testing.T) {
			e := newAPIEnv(t)
			if got := e.call(tt.as, http.MethodDelete, "/companies/"+tt.id, nil); got.status != tt.want {
				t.Errorf("status = %d, want %d, body %s", got.status, tt.want, got.raw)
			}
		})
	}
}
