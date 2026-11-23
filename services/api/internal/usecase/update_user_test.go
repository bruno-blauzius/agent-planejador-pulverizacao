package usecase_test

import (
	"context"
	"slices"
	"testing"

	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/domain"
	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/usecase"
)

func rolePtr(r domain.Role) *domain.Role              { return &r }
func companyPtr(c domain.CompanyID) *domain.CompanyID { return &c }
func scopePtr(s domain.FarmScope) *domain.FarmScope   { return &s }
func stringPtr(s string) *string                      { return &s }

func TestUpdateUserChangesRole(t *testing.T) {
	e := newEnv(t)
	subject := e.subjectOf(t, e.operatorA)

	listing, err := e.update().Execute(ctx(), usecase.UpdateUserInput{
		Actor: e.asAdminA(), UserID: e.operatorA.ID, Role: rolePtr(domain.RoleAdmin),
	})
	assertNoError(t, err)

	if listing.User.Role != domain.RoleAdmin || e.db.Users[e.operatorA.ID].Role != domain.RoleAdmin {
		t.Fatalf("projection = %s", e.db.Users[e.operatorA.ID].Role)
	}
	if role, _ := e.idp.InGroup(subject); role != domain.RoleAdmin {
		t.Fatalf("group = %s, want admin (moved between subgroups)", role)
	}
	if !slices.Contains(e.db.SessionLog, "expire_access:u-op-a") {
		t.Fatalf("sessions must be marked to renew the token: %v", e.db.SessionLog)
	}
	e.assertAudit(t, domain.AuditUserRoleChanged)
	if got := e.db.Audit[0].Metadata; got["from"] != "planning_operator" || got["to"] != "admin" {
		t.Fatalf("audit metadata = %v", got)
	}
}

func TestUpdateUserRoleOfAnInactiveUserIsOnlyAProjection(t *testing.T) {
	e := newEnv(t)
	subject := e.subjectOf(t, e.operatorA)
	inactive := e.db.Users[e.operatorA.ID]
	inactive.Status = domain.UserInactive
	e.db.Users[inactive.ID] = inactive
	delete(e.idp.Groups, subject) // deactivation took the account out of the product groups
	e.idp.Calls = nil

	listing, err := e.update().Execute(ctx(), usecase.UpdateUserInput{
		Actor: e.asAdminA(), UserID: e.operatorA.ID, Role: rolePtr(domain.RoleAdmin),
	})
	assertNoError(t, err)

	if listing.User.Role != domain.RoleAdmin {
		t.Fatalf("projection = %s", listing.User.Role)
	}
	if _, inGroup := e.idp.InGroup(subject); inGroup || e.idp.CallsOf("MoveToRoleGroup") != 0 {
		t.Fatalf("the provider must not be touched: calls = %v", e.idp.Calls)
	}
	e.assertAudit(t, domain.AuditUserRoleChanged)
}

func TestUpdateUserRoleChangeCompensation(t *testing.T) {
	t.Run("provider failure leaves the database untouched", func(t *testing.T) {
		e := newEnv(t)
		e.idp.Fail("MoveToRoleGroup", errBoom)
		_, err := e.update().Execute(ctx(), usecase.UpdateUserInput{Actor: e.asAdminA(), UserID: e.operatorA.ID, Role: rolePtr(domain.RoleAdmin)})
		assertIs(t, err, usecase.ErrIdentityProviderUnavailable)
		if e.db.Users[e.operatorA.ID].Role != domain.RolePlanningOperator || len(e.db.SessionLog) != 0 {
			t.Fatal("no registration by halves")
		}
		e.assertNoAudit(t)
	})

	for _, operation := range []string{"UpdateUser", "ExpireUserAccess", "Record", "Commit"} {
		t.Run("database fails at "+operation+": the group move is reverted", func(t *testing.T) {
			e := newEnv(t)
			subject := e.subjectOf(t, e.operatorA)
			e.db.Fail(operation, errBoom)
			_, err := e.update().Execute(ctx(), usecase.UpdateUserInput{Actor: e.asAdminA(), UserID: e.operatorA.ID, Role: rolePtr(domain.RoleAdmin)})
			assertIs(t, err, errBoom)
			if role, _ := e.idp.InGroup(subject); role != domain.RolePlanningOperator {
				t.Fatalf("group = %s, want the original role back", role)
			}
			if e.db.Users[e.operatorA.ID].Role != domain.RolePlanningOperator {
				t.Fatal("projection must roll back")
			}
		})
	}

	t.Run("failed revert is reported with the cause", func(t *testing.T) {
		e := newEnv(t)
		e.db.Fail("UpdateUser", errBoom)
		uc := usecase.NewUpdateUser(usecase.UpdateUserDeps{
			Users: e.db, Companies: e.db, Farms: e.db, Identities: e.db, Access: &failSecondMove{ProductAccess: e.idp},
			Lister: e.db, Roles: e.db, Tx: e.db, Clock: e.clock,
		})
		_, err := uc.Execute(ctx(), usecase.UpdateUserInput{Actor: e.asAdminA(), UserID: e.operatorA.ID, Role: rolePtr(domain.RoleAdmin)})
		assertIs(t, err, errBoom)
		assertIs(t, err, usecase.ErrIdentityProviderUnavailable)
	})
}

// failSecondMove lets the first group move through and fails the second one,
// which is the compensating move back.
type failSecondMove struct {
	usecase.ProductAccess
	moves int
}

func (f *failSecondMove) MoveToRoleGroup(c context.Context, subject string, from, to domain.Role) error {
	f.moves++
	if f.moves == 2 {
		return usecase.ErrIdentityProviderUnavailable
	}
	return f.ProductAccess.MoveToRoleGroup(c, subject, from, to)
}

func TestUpdateUserAuthorization(t *testing.T) {
	tests := []struct {
		name  string
		actor func(*env) domain.Principal
		input func(*env) usecase.UpdateUserInput
		want  error
	}{
		{"admin edits a user of the own company", (*env).asAdminA, func(e *env) usecase.UpdateUserInput {
			return usecase.UpdateUserInput{UserID: e.operatorA.ID, Name: stringPtr("Olga Souza")}
		}, nil},
		{"admin of A cannot reach a user of B (404, not 403)", (*env).asAdminA, func(e *env) usecase.UpdateUserInput {
			return usecase.UpdateUserInput{UserID: e.adminB.ID, Name: stringPtr("Hacked")}
		}, usecase.ErrNotFound},
		{"admin cannot grant super admin", (*env).asAdminA, func(e *env) usecase.UpdateUserInput {
			return usecase.UpdateUserInput{UserID: e.operatorA.ID, Role: rolePtr(domain.RoleSuperAdmin)}
		}, usecase.ErrForbidden},
		{"admin cannot move a user to another company", (*env).asAdminA, func(e *env) usecase.UpdateUserInput {
			return usecase.UpdateUserInput{UserID: e.operatorA.ID, CompanyID: companyPtr("co-b")}
		}, usecase.ErrForbidden},
		{"operator has no users.manage", (*env).asOperatorA, func(e *env) usecase.UpdateUserInput {
			return usecase.UpdateUserInput{UserID: e.operatorA.ID, Name: stringPtr("Self")}
		}, usecase.ErrForbidden},
		{"nobody changes the own role", (*env).asAdminA, func(e *env) usecase.UpdateUserInput {
			return usecase.UpdateUserInput{UserID: e.adminA.ID, Role: rolePtr(domain.RolePlanningOperator)}
		}, usecase.ErrForbidden},
		{"a user may rename themselves", (*env).asAdminA, func(e *env) usecase.UpdateUserInput {
			return usecase.UpdateUserInput{UserID: e.adminA.ID, Name: stringPtr("Ana Maria")}
		}, nil},
		{"super admin moves a user across companies", (*env).asSuper, func(e *env) usecase.UpdateUserInput {
			return usecase.UpdateUserInput{UserID: e.operatorA.ID, CompanyID: companyPtr("co-b")}
		}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newEnv(t)
			in := tt.input(e)
			in.Actor = tt.actor(e)
			_, err := e.update().Execute(ctx(), in)
			if tt.want == nil {
				assertNoError(t, err)
				return
			}
			assertIs(t, err, tt.want)
		})
	}
}

func TestUpdateUserSuperAdminOnlyInOperatorCompany(t *testing.T) {
	e := newEnv(t)
	_, err := e.update().Execute(ctx(), usecase.UpdateUserInput{Actor: e.asSuper(), UserID: e.operatorA.ID, Role: rolePtr(domain.RoleSuperAdmin)})
	assertValidationOn(t, err, "role")
}

func TestUpdateUserAdminCannotTouchSuperAdmin(t *testing.T) {
	e := newEnv(t)
	// an admin of the operator company still must not manage the super admin
	opAdmin := e.addUser("u-op-admin", e.operatorCo, "Admin Operador", "adm@agro.test", domain.RoleAdmin, domain.AllFarms())
	_, err := e.update().Execute(ctx(), usecase.UpdateUserInput{
		Actor: principal(opAdmin, adminPermissions), UserID: e.superAdmin.ID, Name: stringPtr("X"),
	})
	assertIs(t, err, usecase.ErrForbidden)
}

func TestUpdateUserScope(t *testing.T) {
	selectedA, _ := domain.NewSelectedFarms([]domain.FarmID{"farm-a1"})
	selectedB, _ := domain.NewSelectedFarms([]domain.FarmID{"farm-b1"})

	t.Run("operator gets selected farms of the own company", func(t *testing.T) {
		e := newEnv(t)
		listing, err := e.update().Execute(ctx(), usecase.UpdateUserInput{Actor: e.asAdminA(), UserID: e.operatorA.ID, FarmScope: scopePtr(selectedA)})
		assertNoError(t, err)
		if !listing.User.FarmScope.Equal(selectedA) {
			t.Fatalf("scope = %+v", listing.User.FarmScope)
		}
		e.assertAudit(t, domain.AuditUserScopeChanged)
		if !slices.Contains(e.db.SessionLog, "expire_access:u-op-a") {
			t.Fatal("a scope change must renew the sessions")
		}
		if e.idp.CallsOf("MoveToRoleGroup") != 0 {
			t.Fatal("a scope change does not touch the provider")
		}
	})
	t.Run("farm of another company is rejected", func(t *testing.T) {
		e := newEnv(t)
		_, err := e.update().Execute(ctx(), usecase.UpdateUserInput{Actor: e.asAdminA(), UserID: e.operatorA.ID, FarmScope: scopePtr(selectedB)})
		assertValidationOn(t, err, "farm_scope.farm_ids")
	})
	t.Run("selected farms need the operator role", func(t *testing.T) {
		e := newEnv(t)
		_, err := e.update().Execute(ctx(), usecase.UpdateUserInput{Actor: e.asSuper(), UserID: e.adminA.ID, FarmScope: scopePtr(selectedA)})
		assertValidationOn(t, err, "farm_scope")
	})
	t.Run("leaving the operator role resets the scope to all farms", func(t *testing.T) {
		e := newEnv(t)
		operator := e.db.Users[e.operatorA.ID]
		operator.FarmScope = selectedA
		e.db.Users[operator.ID] = operator

		listing, err := e.update().Execute(ctx(), usecase.UpdateUserInput{Actor: e.asAdminA(), UserID: e.operatorA.ID, Role: rolePtr(domain.RoleAdmin)})
		assertNoError(t, err)
		if listing.User.FarmScope.IsSelected() {
			t.Fatal("an admin cannot keep a farm selection")
		}
	})
	t.Run("moving to another company resets the scope", func(t *testing.T) {
		e := newEnv(t)
		operator := e.db.Users[e.operatorA.ID]
		operator.FarmScope = selectedA
		e.db.Users[operator.ID] = operator

		listing, err := e.update().Execute(ctx(), usecase.UpdateUserInput{Actor: e.asSuper(), UserID: e.operatorA.ID, CompanyID: companyPtr("co-b")})
		assertNoError(t, err)
		if listing.User.CompanyID != "co-b" || listing.User.FarmScope.IsSelected() {
			t.Fatalf("user = %+v", listing.User)
		}
		e.assertAudit(t, domain.AuditUserScopeChanged, domain.AuditUserCompanyChanged)
	})
	t.Run("moving into an inactive company", func(t *testing.T) {
		e := newEnv(t)
		company := e.db.Companies["co-b"]
		company.Status = domain.CompanyInactive
		e.db.Companies["co-b"] = company
		_, err := e.update().Execute(ctx(), usecase.UpdateUserInput{Actor: e.asSuper(), UserID: e.operatorA.ID, CompanyID: companyPtr("co-b")})
		assertIs(t, err, usecase.ErrCompanyInactive)
	})
	t.Run("unknown company is a validation error", func(t *testing.T) {
		e := newEnv(t)
		_, err := e.update().Execute(ctx(), usecase.UpdateUserInput{Actor: e.asSuper(), UserID: e.operatorA.ID, CompanyID: companyPtr("nope")})
		assertValidationOn(t, err, "company_id")
	})
}

func TestUpdateUserNoChangeWritesNothing(t *testing.T) {
	e := newEnv(t)
	listing, err := e.update().Execute(ctx(), usecase.UpdateUserInput{
		Actor: e.asAdminA(), UserID: e.operatorA.ID, Name: stringPtr(e.operatorA.Name), Role: rolePtr(e.operatorA.Role),
	})
	assertNoError(t, err)
	if listing.User.ID != e.operatorA.ID || len(e.idp.Calls) != 0 || len(e.db.Audit) != 0 || len(e.db.SessionLog) != 0 {
		t.Fatal("an identical update must be a no-op")
	}
}

func TestUpdateUserInvalidName(t *testing.T) {
	e := newEnv(t)
	_, err := e.update().Execute(ctx(), usecase.UpdateUserInput{Actor: e.asAdminA(), UserID: e.operatorA.ID, Name: stringPtr("  ")})
	assertValidationOn(t, err, "name")
}
