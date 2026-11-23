package httpadapter_test

import (
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/domain"
	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/usecase"
)

func TestListUsers(t *testing.T) {
	e := newAPIEnv(t)

	t.Run("admin sees only the own company with the default page", func(t *testing.T) {
		got := e.call(adminAID, http.MethodGet, "/users", nil)
		if got.status != http.StatusOK {
			t.Fatalf("status = %d, body %s", got.status, got.raw)
		}
		body := got.object(t)
		if body["total"] != float64(2) || body["page"] != float64(1) || body["page_size"] != float64(20) {
			t.Errorf("page = %v", body)
		}
		if got.header.Get("Cache-Control") != "no-store" {
			t.Errorf("Cache-Control = %q, want no-store", got.header.Get("Cache-Control"))
		}
	})

	t.Run("company_id of another company is ignored for admins", func(t *testing.T) {
		got := e.call(adminAID, http.MethodGet, "/users?company_id="+companyBID, nil)
		if got.status != http.StatusOK || got.object(t)["total"] != float64(2) {
			t.Errorf("status = %d, body %s", got.status, got.raw)
		}
	})

	t.Run("super admin sees everyone and may filter by company", func(t *testing.T) {
		if all := e.call(superAdminID, http.MethodGet, "/users", nil); all.object(t)["total"] != float64(4) {
			t.Errorf("all users: %s", all.raw)
		}
		only := e.call(superAdminID, http.MethodGet, "/users?company_id="+strings.ToUpper(companyBID), nil)
		if only.status != http.StatusOK || only.object(t)["total"] != float64(1) {
			t.Errorf("company filter: status %d, %s", only.status, only.raw)
		}
	})

	t.Run("filters and pagination", func(t *testing.T) {
		byRole := e.call(adminAID, http.MethodGet, "/users?role=planning_operator", nil)
		if byRole.object(t)["total"] != float64(1) {
			t.Errorf("role filter: %s", byRole.raw)
		}
		bySearch := e.call(adminAID, http.MethodGet, "/users?search=olga", nil)
		if bySearch.object(t)["total"] != float64(1) {
			t.Errorf("search filter: %s", bySearch.raw)
		}
		second := e.call(adminAID, http.MethodGet, "/users?page=2&page_size=1", nil).object(t)
		items, _ := second["items"].([]any)
		if len(items) != 1 || second["page"] != float64(2) || second["page_size"] != float64(1) || second["total"] != float64(2) {
			t.Errorf("page 2 = %v", second)
		}
	})

	t.Run("invalid parameters answer 422 listing every field", func(t *testing.T) {
		got := e.call(adminAID, http.MethodGet, "/users?page=0&page_size=abc&company_id=nope", nil)
		if got.status != http.StatusUnprocessableEntity || got.errorCode(t) != "validation_failed" {
			t.Fatalf("status = %d, body %s", got.status, got.raw)
		}
		fields := got.errorFields(t)
		for _, want := range []string{"page", "page_size", "company_id"} {
			if !slices.Contains(fields, want) {
				t.Errorf("details %v miss %q", fields, want)
			}
		}
		if tooBig := e.call(adminAID, http.MethodGet, "/users?page_size=101", nil); tooBig.status != http.StatusUnprocessableEntity {
			t.Errorf("page_size above max: status %d", tooBig.status)
		}
		if unknown := e.call(adminAID, http.MethodGet, "/users?role=wizard", nil); unknown.status != http.StatusUnprocessableEntity {
			t.Errorf("unknown role: status %d", unknown.status)
		}
	})

	t.Run("operator lacks users.manage", func(t *testing.T) {
		got := e.call(operatorAID, http.MethodGet, "/users", nil)
		if got.status != http.StatusForbidden || got.errorCode(t) != "forbidden" {
			t.Errorf("status = %d, body %s", got.status, got.raw)
		}
	})
}

func TestUsersSummary(t *testing.T) {
	e := newAPIEnv(t)
	got := e.call(adminAID, http.MethodGet, "/users/summary", nil)
	if got.status != http.StatusOK {
		t.Fatalf("status = %d, body %s", got.status, got.raw)
	}
	body := got.object(t)
	byRole, _ := body["by_role"].(map[string]any)
	if body["total_users"] != float64(2) || byRole["admin"] != float64(1) || byRole["super_admin"] != float64(0) || byRole["planning_operator"] != float64(1) {
		t.Errorf("summary = %v", body)
	}
	if forbidden := e.call(operatorAID, http.MethodGet, "/users/summary", nil); forbidden.status != http.StatusForbidden {
		t.Errorf("operator: status %d", forbidden.status)
	}
	if bad := e.call(superAdminID, http.MethodGet, "/users/summary?company_id=x", nil); bad.status != http.StatusUnprocessableEntity {
		t.Errorf("bad company_id: status %d", bad.status)
	}
}

func TestGetUser(t *testing.T) {
	e := newAPIEnv(t)
	got := e.call(adminAID, http.MethodGet, "/users/"+operatorAID, nil)
	if got.status != http.StatusOK {
		t.Fatalf("status = %d, body %s", got.status, got.raw)
	}
	body := got.object(t)
	company, _ := body["company"].(map[string]any)
	scope, _ := body["farm_scope"].(map[string]any)
	if body["id"] != operatorAID || body["role"] != "planning_operator" || body["status"] != "active" ||
		company["id"] != companyAID || company["name"] != "Fazenda A" || scope["mode"] != "all" || body["invitation"] != nil {
		t.Errorf("user = %v", body)
	}

	tests := []struct {
		name string
		as   string
		path string
		want int
	}{
		{"user of another company is 404, never 403", adminAID, "/users/" + adminBID, http.StatusNotFound},
		{"unknown id", adminAID, "/users/" + missingID, http.StatusNotFound},
		{"malformed id", adminAID, "/users/not-a-uuid", http.StatusNotFound},
		{"operator may not read", operatorAID, "/users/" + adminAID, http.StatusForbidden},
		{"super admin reads across companies", superAdminID, "/users/" + adminBID, http.StatusOK},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := e.call(tt.as, http.MethodGet, tt.path, nil); got.status != tt.want {
				t.Errorf("status = %d, want %d, body %s", got.status, tt.want, got.raw)
			}
		})
	}
}

func TestUpdateUser(t *testing.T) {
	t.Run("renames and changes role", func(t *testing.T) {
		e := newAPIEnv(t)
		got := e.call(adminAID, http.MethodPatch, "/users/"+operatorAID, map[string]any{"name": "Olga Souza", "role": "admin"})
		if got.status != http.StatusOK {
			t.Fatalf("status = %d, body %s", got.status, got.raw)
		}
		body := got.object(t)
		if body["name"] != "Olga Souza" || body["role"] != "admin" {
			t.Errorf("user = %v", body)
		}
		if e.db.Users[operatorAID].Role != domain.RoleAdmin {
			t.Errorf("role was not stored")
		}
	})

	t.Run("deactivates and reactivates through status", func(t *testing.T) {
		e := newAPIEnv(t)
		off := e.call(adminAID, http.MethodPatch, "/users/"+operatorAID, map[string]any{"status": "inactive"})
		if off.status != http.StatusOK || off.object(t)["status"] != "inactive" {
			t.Fatalf("deactivate: status %d, %s", off.status, off.raw)
		}
		on := e.call(adminAID, http.MethodPatch, "/users/"+operatorAID, map[string]any{"status": "active"})
		if on.status != http.StatusOK || on.object(t)["status"] != "active" {
			t.Fatalf("reactivate: status %d, %s", on.status, on.raw)
		}
	})

	t.Run("accepts selected farms of the company for an operator", func(t *testing.T) {
		e := newAPIEnv(t)
		got := e.call(adminAID, http.MethodPatch, "/users/"+operatorAID,
			map[string]any{"farm_scope": map[string]any{"mode": "selected", "farm_ids": []string{farmA1}}})
		if got.status != http.StatusOK {
			t.Fatalf("status = %d, body %s", got.status, got.raw)
		}
		scope, _ := got.object(t)["farm_scope"].(map[string]any)
		if scope["mode"] != "selected" {
			t.Errorf("scope = %v", scope)
		}
	})

	tests := []struct {
		name       string
		as         string
		path       string
		body       any
		modify     []func(*http.Request)
		wantStatus int
		wantCode   string
		wantField  string
	}{
		{name: "empty object", as: adminAID, path: operatorAID, body: map[string]any{}, wantStatus: 422, wantCode: "validation_failed"},
		{name: "unknown field is refused", as: adminAID, path: operatorAID, body: `{"name":"X","email":"x@y.z"}`, wantStatus: 422, wantCode: "validation_failed", wantField: "email"},
		{name: "wrong type", as: adminAID, path: operatorAID, body: `{"name":42}`, wantStatus: 422, wantCode: "validation_failed", wantField: "name"},
		{name: "invalid json", as: adminAID, path: operatorAID, body: `{"name":`, wantStatus: 422, wantCode: "validation_failed"},
		{name: "two documents", as: adminAID, path: operatorAID, body: `{"name":"A"}{"name":"B"}`, wantStatus: 422, wantCode: "validation_failed"},
		{name: "array body", as: adminAID, path: operatorAID, body: `[]`, wantStatus: 422, wantCode: "validation_failed"},
		{name: "wrong content type", as: adminAID, path: operatorAID, body: `{"name":"X"}`, modify: []func(*http.Request){withHeader("Content-Type", "text/plain")}, wantStatus: 415, wantCode: "unsupported_media_type"},
		{name: "missing content type", as: adminAID, path: operatorAID, body: `{"name":"X"}`, modify: []func(*http.Request){withHeader("Content-Type", "")}, wantStatus: 415, wantCode: "unsupported_media_type"},
		{name: "body too large", as: adminAID, path: operatorAID, body: `{"name":"` + strings.Repeat("a", 2<<20) + `"}`, wantStatus: 413, wantCode: "payload_too_large"},
		{name: "unknown role", as: adminAID, path: operatorAID, body: map[string]any{"role": "wizard"}, wantStatus: 422, wantCode: "validation_failed", wantField: "role"},
		{name: "invalid status", as: adminAID, path: operatorAID, body: map[string]any{"status": "invited"}, wantStatus: 422, wantCode: "validation_failed", wantField: "status"},
		{name: "invalid company id", as: superAdminID, path: operatorAID, body: map[string]any{"company_id": "x"}, wantStatus: 422, wantCode: "validation_failed", wantField: "company_id"},
		{name: "farm of another company", as: adminAID, path: operatorAID, body: map[string]any{"farm_scope": map[string]any{"mode": "selected", "farm_ids": []string{farmB1}}}, wantStatus: 422, wantCode: "validation_failed", wantField: "farm_scope.farm_ids"},
		{name: "farm id that is not a uuid", as: adminAID, path: operatorAID, body: map[string]any{"farm_scope": map[string]any{"mode": "selected", "farm_ids": []string{"farm-1"}}}, wantStatus: 422, wantCode: "validation_failed", wantField: "farm_scope.farm_ids"},
		{name: "selected scope without farms", as: adminAID, path: operatorAID, body: map[string]any{"farm_scope": map[string]any{"mode": "selected"}}, wantStatus: 422, wantCode: "validation_failed", wantField: "farm_scope.farm_ids"},
		{name: "admin cannot grant super admin", as: adminAID, path: operatorAID, body: map[string]any{"role": "super_admin"}, wantStatus: 403, wantCode: "forbidden"},
		{name: "admin cannot change own role", as: adminAID, path: adminAID, body: map[string]any{"role": "planning_operator"}, wantStatus: 403, wantCode: "forbidden"},
		{name: "admin cannot deactivate own account", as: adminAID, path: adminAID, body: map[string]any{"status": "inactive"}, wantStatus: 403, wantCode: "forbidden"},
		{name: "other company is 404", as: adminAID, path: adminBID, body: map[string]any{"name": "X"}, wantStatus: 404, wantCode: "not_found"},
		{name: "operator is 403", as: operatorAID, path: adminAID, body: map[string]any{"name": "X"}, wantStatus: 403, wantCode: "forbidden"},
		{name: "malformed id", as: adminAID, path: "nope", body: map[string]any{"name": "X"}, wantStatus: 404, wantCode: "not_found"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newAPIEnv(t)
			got := e.call(tt.as, http.MethodPatch, "/users/"+tt.path, tt.body, tt.modify...)
			if got.status != tt.wantStatus || got.errorCode(t) != tt.wantCode {
				t.Fatalf("status = %d code = %q, want %d %q, body %.200s", got.status, got.errorCode(t), tt.wantStatus, tt.wantCode, got.raw)
			}
			if tt.wantField != "" && !slices.Contains(got.errorFields(t), tt.wantField) {
				t.Errorf("details %v miss %q", got.errorFields(t), tt.wantField)
			}
		})
	}

	t.Run("identity provider failure on a role change is 502 and changes nothing", func(t *testing.T) {
		e := newAPIEnv(t)
		e.idp.Fail("MoveToRoleGroup", usecase.ErrIdentityProviderUnavailable)
		got := e.call(adminAID, http.MethodPatch, "/users/"+operatorAID, map[string]any{"role": "admin"})
		if got.status != http.StatusBadGateway || got.errorCode(t) != "identity_provider_unavailable" {
			t.Fatalf("status = %d, body %s", got.status, got.raw)
		}
		if e.db.Users[operatorAID].Role != domain.RolePlanningOperator {
			t.Errorf("role changed despite the failure")
		}
	})

	t.Run("a non-selected scope for a role that cannot have farms is 422 from the domain", func(t *testing.T) {
		e := newAPIEnv(t)
		got := e.call(adminAID, http.MethodPatch, "/users/"+operatorAID,
			map[string]any{"role": "admin", "farm_scope": map[string]any{"mode": "selected", "farm_ids": []string{farmA1}}})
		if got.status != http.StatusUnprocessableEntity || !slices.Contains(got.errorFields(t), "farm_scope") {
			t.Errorf("status = %d, body %s", got.status, got.raw)
		}
	})
}

func TestDeleteUser(t *testing.T) {
	t.Run("soft deletes and frees the e-mail", func(t *testing.T) {
		e := newAPIEnv(t)
		got := e.call(adminAID, http.MethodDelete, "/users/"+operatorAID, nil)
		if got.status != http.StatusNoContent || len(got.raw) != 0 {
			t.Fatalf("status = %d, body %q", got.status, got.raw)
		}
		if after := e.call(adminAID, http.MethodGet, "/users/"+operatorAID, nil); after.status != http.StatusNotFound {
			t.Errorf("deleted user: status %d", after.status)
		}
	})

	tests := []struct {
		name     string
		as, path string
		want     int
		code     string
	}{
		{"self", adminAID, adminAID, 403, "forbidden"},
		{"other company", adminAID, adminBID, 404, "not_found"},
		{"operator", operatorAID, adminAID, 403, "forbidden"},
		{"malformed id", adminAID, "x", 404, "not_found"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newAPIEnv(t)
			got := e.call(tt.as, http.MethodDelete, "/users/"+tt.path, nil)
			if got.status != tt.want || got.errorCode(t) != tt.code {
				t.Errorf("status = %d code %q, body %s", got.status, got.errorCode(t), got.raw)
			}
		})
	}

	t.Run("pending invitation must be cancelled instead", func(t *testing.T) {
		e := newAPIEnv(t)
		invited, _ := e.addInvited("00000000-0000-4000-8000-000000000a10", companyOf(e, companyAID), "novo@a.test")
		got := e.call(adminAID, http.MethodDelete, "/users/"+string(invited.ID), nil)
		if got.status != http.StatusConflict || got.errorCode(t) != "invitation_pending" {
			t.Errorf("status = %d, body %s", got.status, got.raw)
		}
	})

	t.Run("provider down is 502", func(t *testing.T) {
		e := newAPIEnv(t)
		e.idp.Fail("RemoveFromProductGroups", usecase.ErrIdentityProviderUnavailable)
		got := e.call(adminAID, http.MethodDelete, "/users/"+operatorAID, nil)
		if got.status != http.StatusBadGateway || got.errorCode(t) != "identity_provider_unavailable" {
			t.Errorf("status = %d, body %s", got.status, got.raw)
		}
	})

	t.Run("last active super admin is protected", func(t *testing.T) {
		e := newAPIEnv(t)
		second := e.addUser("00000000-0000-4000-8000-000000000005", companyOf(e, operatorCompanyID), "Segundo", "segundo@agro.test", domain.RoleSuperAdmin)
		// The first one stays able to call (the fake authenticator ignores status) but no longer counts as active.
		first := e.db.Users[domain.UserID(superAdminID)]
		first.Status = domain.UserInactive
		e.db.Users[first.ID] = first
		got := e.call(superAdminID, http.MethodDelete, "/users/"+string(second.ID), nil)
		if got.status != http.StatusConflict || got.errorCode(t) != "last_super_admin" {
			t.Errorf("status = %d, body %s", got.status, got.raw)
		}
	})
}
