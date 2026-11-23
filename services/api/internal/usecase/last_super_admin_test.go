package usecase_test

import (
	"testing"

	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/domain"
	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/usecase"
)

// The only active super admin cannot be deactivated, deleted or demoted; with
// a second one, each of those operations is allowed.
func TestLastSuperAdminIsProtected(t *testing.T) {
	// The acting super admin must be someone else, so a second one is always
	// present in the "allowed" cases and the target is the only one in the "refused" ones.
	operations := map[string]func(e *env, actor domain.Principal, target domain.UserID) error{
		"deactivate": func(e *env, a domain.Principal, id domain.UserID) error {
			_, err := e.deactivate().Execute(ctx(), status(a, id))
			return err
		},
		"delete": func(e *env, a domain.Principal, id domain.UserID) error {
			return e.delete().Execute(ctx(), status(a, id))
		},
		"demote": func(e *env, a domain.Principal, id domain.UserID) error {
			_, err := e.update().Execute(ctx(), usecase.UpdateUserInput{Actor: a, UserID: id, Role: rolePtr(domain.RoleAdmin)})
			return err
		},
	}
	for name, run := range operations {
		t.Run(name+": refused for the last active super admin", func(t *testing.T) {
			e := newEnv(t)
			// an admin of the operator company with manage_all acts on the only super admin
			actor := principal(e.addUser("u-op-admin", e.operatorCo, "Adm", "adm@agro.test", domain.RoleAdmin, domain.AllFarms()), superPermissions)
			err := run(e, actor, e.superAdmin.ID)
			// demoting to admin is also blocked by the invariant only for non-operator companies; here the company is the operator
			assertIs(t, err, usecase.ErrLastSuperAdmin)
			if e.db.Users[e.superAdmin.ID].Status != domain.UserActive || e.db.Users[e.superAdmin.ID].Role != domain.RoleSuperAdmin {
				t.Fatal("nothing may change")
			}
			if len(e.idp.Calls) != 0 || len(e.db.Audit) != 0 {
				t.Fatal("neither the provider nor the audit may be touched")
			}
		})
		t.Run(name+": allowed with a second active super admin", func(t *testing.T) {
			e := newEnv(t)
			second := e.addUser("u-super-2", e.operatorCo, "Segundo", "s2@agro.test", domain.RoleSuperAdmin, domain.AllFarms())
			assertNoError(t, run(e, principal(second, superPermissions), e.superAdmin.ID))
		})
	}

	t.Run("an inactive super admin does not count", func(t *testing.T) {
		e := newEnv(t)
		second := e.addUser("u-super-2", e.operatorCo, "Segundo", "s2@agro.test", domain.RoleSuperAdmin, domain.AllFarms())
		stored := e.db.Users[second.ID]
		stored.Status = domain.UserInactive
		e.db.Users[second.ID] = stored
		actor := principal(e.addUser("u-op-admin", e.operatorCo, "Adm", "adm@agro.test", domain.RoleAdmin, domain.AllFarms()), superPermissions)
		_, err := e.deactivate().Execute(ctx(), status(actor, e.superAdmin.ID))
		assertIs(t, err, usecase.ErrLastSuperAdmin)
	})
	t.Run("reactivating is never blocked", func(t *testing.T) {
		e := newEnv(t)
		second := e.addUser("u-super-2", e.operatorCo, "Segundo", "s2@agro.test", domain.RoleSuperAdmin, domain.AllFarms())
		_, err := e.deactivate().Execute(ctx(), status(e.asSuper(), second.ID))
		assertNoError(t, err)
		_, err = e.reactivate().Execute(ctx(), status(e.asSuper(), second.ID))
		assertNoError(t, err)
	})
	t.Run("a count failure is reported, never ignored", func(t *testing.T) {
		e := newEnv(t)
		second := e.addUser("u-super-2", e.operatorCo, "Segundo", "s2@agro.test", domain.RoleSuperAdmin, domain.AllFarms())
		e.db.Fail("CountActiveUsersByRole", errBoom)
		_, err := e.deactivate().Execute(ctx(), status(principal(second, superPermissions), e.superAdmin.ID))
		assertIs(t, err, errBoom)
	})
}
