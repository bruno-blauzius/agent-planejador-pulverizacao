package httpadapter_test

import (
	"net/http"
	"slices"
	"testing"

	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/domain"
)

func TestCurrentUser(t *testing.T) {
	e := newAPIEnv(t)

	tests := []struct {
		name            string
		as              string
		wantRole        string
		wantPermissions []string
	}{
		{"super admin", superAdminID, "super_admin", []string{"companies.manage", "users.manage_all", "users.manage"}},
		{"admin", adminAID, "admin", []string{"users.manage", "dashboard.view"}},
		{"planning operator", operatorAID, "planning_operator", []string{"planner.use", "applications.register"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := e.call(tt.as, http.MethodGet, "/auth/me", nil)
			if got.status != http.StatusOK {
				t.Fatalf("status = %d, body %s", got.status, got.raw)
			}
			body := got.object(t)
			permissions, _ := body["permissions"].([]any)
			names := make([]string, len(permissions))
			for i, permission := range permissions {
				names[i], _ = permission.(string)
			}
			if body["role"] != tt.wantRole || body["id"] != tt.as || body["primary_farm"] != nil {
				t.Errorf("me = %v", body)
			}
			for _, want := range tt.wantPermissions {
				if !slices.Contains(names, want) {
					t.Errorf("permissions %v miss %q", names, want)
				}
			}
			if _, ok := body["farm_scope"].(map[string]any); !ok {
				t.Errorf("farm_scope missing: %v", body)
			}
		})
	}

	t.Run("primary farm is returned when set", func(t *testing.T) {
		primary := e.db.Users[operatorAID]
		farm := domain.FarmID(farmA1)
		primary.PrimaryFarmID = &farm
		e.db.Users[primary.ID] = primary
		got := e.call(operatorAID, http.MethodGet, "/auth/me", nil)
		farmBody, _ := got.object(t)["primary_farm"].(map[string]any)
		if farmBody["id"] != farmA1 || farmBody["name"] != "Santa Rita" {
			t.Errorf("primary_farm = %v", farmBody)
		}
	})
}

func TestProfile(t *testing.T) {
	t.Run("any role reads the profile", func(t *testing.T) {
		e := newAPIEnv(t)
		for _, id := range []string{superAdminID, adminAID, operatorAID} {
			got := e.call(id, http.MethodGet, "/me/profile", nil)
			body := got.object(t)
			if got.status != http.StatusOK || body["id"] != id || body["password_changed_at"] != nil {
				t.Errorf("profile of %s: status %d, %v", id, got.status, body)
			}
			if _, present := body["password_changed_at"]; !present {
				t.Errorf("password_changed_at must be present (null) in %s", got.raw)
			}
		}
	})

	t.Run("changes the name and ignores administrator-owned fields", func(t *testing.T) {
		e := newAPIEnv(t)
		got := e.call(operatorAID, http.MethodPatch, "/me/profile",
			map[string]any{"name": "  Olga Nova  ", "email": "hacker@x.test", "role": "admin", "company_id": companyBID})
		body := got.object(t)
		if got.status != http.StatusOK || body["name"] != "Olga Nova" || body["email"] != "olga@a.test" || body["role"] != "planning_operator" {
			t.Fatalf("status = %d, profile = %v", got.status, body)
		}
		company, _ := body["company"].(map[string]any)
		if company["id"] != companyAID {
			t.Errorf("company changed: %v", company)
		}
	})

	tests := []struct {
		name string
		body any
		want int
	}{
		{"missing name", map[string]any{}, 422},
		{"blank name", map[string]any{"name": "   "}, 422},
		{"unknown field", map[string]any{"name": "X", "nickname": "Y"}, 422},
		{"invalid json", `{"name":`, 422},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newAPIEnv(t)
			got := e.call(operatorAID, http.MethodPatch, "/me/profile", tt.body)
			if got.status != tt.want || got.errorCode(t) != "validation_failed" {
				t.Errorf("status = %d, body %s", got.status, got.raw)
			}
		})
	}
}
