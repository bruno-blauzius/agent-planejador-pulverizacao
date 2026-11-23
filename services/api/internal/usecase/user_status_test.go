package usecase_test

import (
	"slices"
	"testing"

	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/domain"
	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/usecase"
)

func (e *env) deactivate() *usecase.DeactivateUser { return usecase.NewDeactivateUser(e.statusDeps()) }
func (e *env) reactivate() *usecase.ReactivateUser { return usecase.NewReactivateUser(e.statusDeps()) }
func (e *env) delete() *usecase.DeleteUser         { return usecase.NewDeleteUser(e.statusDeps()) }

func status(actor domain.Principal, id domain.UserID) usecase.UserStatusInput {
	return usecase.UserStatusInput{Actor: actor, UserID: id}
}

// assertRealmAccountIntact checks rule 1 of ADR 0015, D5: the realm account is
// never deleted or disabled by the product.
func assertRealmAccountIntact(t *testing.T, e *env, subject string) {
	t.Helper()
	if _, ok := e.idp.Accounts[subject]; !ok {
		t.Fatal("the realm account must survive: other products use it")
	}
	if e.idp.CallsOf("DeleteCreatedAccount") != 0 {
		t.Fatal("deactivating or deleting must never delete the realm account")
	}
}

func TestDeactivateUser(t *testing.T) {
	e := newEnv(t)
	subject := e.subjectOf(t, e.operatorA)

	listing, err := e.deactivate().Execute(ctx(), status(e.asAdminA(), e.operatorA.ID))
	assertNoError(t, err)

	if listing.User.Status != domain.UserInactive {
		t.Fatalf("status = %s", listing.User.Status)
	}
	if !slices.Contains(e.db.SessionLog, "revoke_user:u-op-a") {
		t.Fatalf("sessions must be revoked: %v", e.db.SessionLog)
	}
	if _, ok := e.idp.InGroup(subject); ok {
		t.Fatal("the user must leave the product group")
	}
	if !slices.Contains(e.idp.EndedSessions, subject) {
		t.Fatal("the product sessions in the realm must end")
	}
	assertRealmAccountIntact(t, e, subject)
	e.assertAudit(t, domain.AuditUserDeactivated)

	// the database, not the token, denies access: even a still-valid token is refused
	_, err = e.resolve().Execute(ctx(), claimsFor(e, t, e.operatorA, "planning_operator"))
	assertIs(t, err, usecase.ErrAccessDenied)
}

func TestDeactivateUserDatabaseFirst(t *testing.T) {
	e := newEnv(t)
	e.idp.Fail("RemoveFromProductGroups", errBoom)

	_, err := e.deactivate().Execute(ctx(), status(e.asAdminA(), e.operatorA.ID))

	assertIs(t, err, usecase.ErrIdentityProviderUnavailable)
	if e.db.Users[e.operatorA.ID].Status != domain.UserInactive {
		t.Fatal("the database must already block the user when the provider is down")
	}
	_, resolveErr := e.resolve().Execute(ctx(), claimsFor(e, t, e.operatorA, "planning_operator"))
	assertIs(t, resolveErr, usecase.ErrAccessDenied)

	// repeating the call finishes the provider cleanup
	e.idp.Heal("RemoveFromProductGroups")
	_, err = e.deactivate().Execute(ctx(), status(e.asAdminA(), e.operatorA.ID))
	assertNoError(t, err)
	if _, ok := e.idp.InGroup(e.subjectOf(t, e.operatorA)); ok {
		t.Fatal("the retry must remove the user from the group")
	}
	e.assertAudit(t, domain.AuditUserDeactivated) // audited once, not on the retry
}

func TestDeactivateUserDatabaseFailureChangesNothing(t *testing.T) {
	for _, operation := range []string{"UpdateUser", "RevokeUserSessions", "Record", "Commit"} {
		t.Run(operation, func(t *testing.T) {
			e := newEnv(t)
			e.db.Fail(operation, errBoom)
			_, err := e.deactivate().Execute(ctx(), status(e.asAdminA(), e.operatorA.ID))
			assertIs(t, err, errBoom)
			if e.db.Users[e.operatorA.ID].Status != domain.UserActive || len(e.idp.Calls) != 0 {
				t.Fatal("when the database fails the provider must not be touched")
			}
		})
	}
}

func TestStatusChangeRules(t *testing.T) {
	type op struct {
		name string
		run  func(*env, domain.Principal, domain.UserID) error
	}
	operations := []op{
		{"deactivate", func(e *env, a domain.Principal, id domain.UserID) error {
			_, err := e.deactivate().Execute(ctx(), status(a, id))
			return err
		}},
		{"reactivate", func(e *env, a domain.Principal, id domain.UserID) error {
			_, err := e.reactivate().Execute(ctx(), status(a, id))
			return err
		}},
		{"delete", func(e *env, a domain.Principal, id domain.UserID) error {
			return e.delete().Execute(ctx(), status(a, id))
		}},
	}
	for _, o := range operations {
		t.Run(o.name+": nobody acts on themselves", func(t *testing.T) {
			e := newEnv(t)
			assertIs(t, o.run(e, e.asAdminA(), e.adminA.ID), usecase.ErrForbidden)
		})
		t.Run(o.name+": admin of A cannot reach B (404)", func(t *testing.T) {
			e := newEnv(t)
			assertIs(t, o.run(e, e.asAdminA(), e.adminB.ID), usecase.ErrNotFound)
			e.assertNoAudit(t)
		})
		t.Run(o.name+": operator has no users.manage", func(t *testing.T) {
			e := newEnv(t)
			assertIs(t, o.run(e, e.asOperatorA(), e.adminA.ID), usecase.ErrForbidden)
		})
		t.Run(o.name+": admin cannot touch a super admin", func(t *testing.T) {
			e := newEnv(t)
			opAdmin := e.addUser("u-op-admin", e.operatorCo, "Admin Operador", "adm@agro.test", domain.RoleAdmin, domain.AllFarms())
			assertIs(t, o.run(e, principal(opAdmin, adminPermissions), e.superAdmin.ID), usecase.ErrForbidden)
		})
		t.Run(o.name+": unknown user", func(t *testing.T) {
			e := newEnv(t)
			assertIs(t, o.run(e, e.asAdminA(), "nope"), usecase.ErrNotFound)
		})
	}

	t.Run("a pending invitation is not deactivated or reactivated", func(t *testing.T) {
		e := newEnv(t)
		invited, _, _ := e.addInvited("u-new", e.companyA, "new@a.test", domain.RoleAdmin, true)
		_, err := e.deactivate().Execute(ctx(), status(e.asAdminA(), invited.ID))
		assertValidationOn(t, err, "status")
		_, err = e.reactivate().Execute(ctx(), status(e.asAdminA(), invited.ID))
		assertValidationOn(t, err, "status")
	})
	t.Run("super admin deactivates a user of any company", func(t *testing.T) {
		e := newEnv(t)
		_, err := e.deactivate().Execute(ctx(), status(e.asSuper(), e.adminB.ID))
		assertNoError(t, err)
	})
}

func TestReactivateUser(t *testing.T) {
	deactivated := func(e *env) {
		_, err := e.deactivate().Execute(ctx(), status(e.asAdminA(), e.operatorA.ID))
		assertNoError(t, err)
		e.db.Audit, e.idp.Calls, e.idp.EndedSessions = nil, nil, nil
	}

	t.Run("rejoins the group and becomes active", func(t *testing.T) {
		e := newEnv(t)
		deactivated(e)
		subject := e.subjectOf(t, e.operatorA)

		listing, err := e.reactivate().Execute(ctx(), status(e.asAdminA(), e.operatorA.ID))
		assertNoError(t, err)

		if listing.User.Status != domain.UserActive {
			t.Fatalf("status = %s", listing.User.Status)
		}
		if role, _ := e.idp.InGroup(subject); role != domain.RolePlanningOperator {
			t.Fatalf("group = %s", role)
		}
		e.assertAudit(t, domain.AuditUserReactivated)
		_, err = e.resolve().Execute(ctx(), claimsFor(e, t, e.operatorA, "planning_operator"))
		assertNoError(t, err)
	})
	t.Run("already active is a no-op", func(t *testing.T) {
		e := newEnv(t)
		_, err := e.reactivate().Execute(ctx(), status(e.asAdminA(), e.operatorA.ID))
		assertNoError(t, err)
		if len(e.idp.Calls) != 0 || len(e.db.Audit) != 0 {
			t.Fatal("nothing to do")
		}
	})
	t.Run("inactive company", func(t *testing.T) {
		e := newEnv(t)
		deactivated(e)
		company := e.db.Companies["co-a"]
		company.Status = domain.CompanyInactive
		e.db.Companies["co-a"] = company
		_, err := e.reactivate().Execute(ctx(), status(e.asSuper(), e.operatorA.ID))
		assertIs(t, err, usecase.ErrCompanyInactive)
		if len(e.idp.Calls) != 0 {
			t.Fatal("the provider must not be touched")
		}
	})
	t.Run("provider failure leaves the user inactive", func(t *testing.T) {
		e := newEnv(t)
		deactivated(e)
		e.idp.Fail("AddToRoleGroup", errBoom)
		_, err := e.reactivate().Execute(ctx(), status(e.asAdminA(), e.operatorA.ID))
		assertIs(t, err, usecase.ErrIdentityProviderUnavailable)
		if e.db.Users[e.operatorA.ID].Status != domain.UserInactive {
			t.Fatal("no registration by halves")
		}
	})
	t.Run("database failure undoes the group join", func(t *testing.T) {
		e := newEnv(t)
		deactivated(e)
		subject := e.subjectOf(t, e.operatorA)
		e.db.Fail("UpdateUser", errBoom)
		_, err := e.reactivate().Execute(ctx(), status(e.asAdminA(), e.operatorA.ID))
		assertIs(t, err, errBoom)
		if _, ok := e.idp.InGroup(subject); ok {
			t.Fatal("the group join must be reverted")
		}
	})
}

func TestDeleteUser(t *testing.T) {
	t.Run("soft deletes, releases the access, frees the e-mail, keeps the account", func(t *testing.T) {
		e := newEnv(t)
		subject := e.subjectOf(t, e.operatorA)

		assertNoError(t, e.delete().Execute(ctx(), status(e.asAdminA(), e.operatorA.ID)))

		stored := e.db.Users[e.operatorA.ID]
		if !stored.IsDeleted() || stored.Status != domain.UserInactive {
			t.Fatalf("user = %+v", stored)
		}
		if _, ok := e.idp.InGroup(subject); ok {
			t.Fatal("the user must leave the product group")
		}
		if !slices.Contains(e.idp.EndedSessions, subject) || !slices.Contains(e.db.SessionLog, "revoke_user:u-op-a") {
			t.Fatal("every session must end")
		}
		assertRealmAccountIntact(t, e, subject)
		e.assertAudit(t, domain.AuditUserDeactivated, domain.AuditUserDeleted)

		// gone from the product: 404 and no login, and the e-mail can be invited again
		_, err := e.update().Execute(ctx(), usecase.UpdateUserInput{Actor: e.asAdminA(), UserID: e.operatorA.ID, Name: stringPtr("x")})
		assertIs(t, err, usecase.ErrNotFound)
		_, err = e.resolve().Execute(ctx(), usecase.ResolvePrincipalInput{Claims: usecase.Claims{
			Issuer: issuer, Subject: subject, EmailVerified: true, ClientRoles: []string{"planning_operator"},
		}})
		assertIs(t, err, usecase.ErrUserNotProvisioned)
		_, err = e.invite().Execute(ctx(), inviteInput(e.asAdminA(), "co-a", "olga@a.test", domain.RolePlanningOperator))
		assertNoError(t, err)
	})
	t.Run("an already inactive user is deleted without a second deactivation event", func(t *testing.T) {
		e := newEnv(t)
		_, err := e.deactivate().Execute(ctx(), status(e.asAdminA(), e.operatorA.ID))
		assertNoError(t, err)
		assertNoError(t, e.delete().Execute(ctx(), status(e.asAdminA(), e.operatorA.ID)))
		e.assertAudit(t, domain.AuditUserDeactivated, domain.AuditUserDeleted)
	})
	t.Run("a pending invitation must be cancelled instead", func(t *testing.T) {
		e := newEnv(t)
		invited, _, _ := e.addInvited("u-new", e.companyA, "new@a.test", domain.RoleAdmin, true)
		assertIs(t, e.delete().Execute(ctx(), status(e.asAdminA(), invited.ID)), usecase.ErrInvitationPending)
	})
	t.Run("provider failure leaves the user inactive and a retry deletes", func(t *testing.T) {
		e := newEnv(t)
		e.idp.Fail("EndProductSessions", errBoom)
		assertIs(t, e.delete().Execute(ctx(), status(e.asAdminA(), e.operatorA.ID)), usecase.ErrIdentityProviderUnavailable)
		stored := e.db.Users[e.operatorA.ID]
		if stored.IsDeleted() || stored.Status != domain.UserInactive {
			t.Fatalf("after the failure the user must be blocked but not deleted: %+v", stored)
		}
		e.idp.Heal("EndProductSessions")
		assertNoError(t, e.delete().Execute(ctx(), status(e.asAdminA(), e.operatorA.ID)))
		if !e.db.Users[e.operatorA.ID].IsDeleted() {
			t.Fatal("the retry must delete")
		}
	})
	t.Run("final transaction failure keeps the user blocked and can be retried", func(t *testing.T) {
		e := newEnv(t)
		e.db.Fail("UnlinkIdentities", errBoom)
		assertIs(t, e.delete().Execute(ctx(), status(e.asAdminA(), e.operatorA.ID)), errBoom)
		stored := e.db.Users[e.operatorA.ID]
		if stored.IsDeleted() || stored.Status != domain.UserInactive || len(e.db.Identities) == 0 {
			t.Fatal("rolled back: blocked, not deleted, still linked")
		}
		e.db.Heal("UnlinkIdentities")
		assertNoError(t, e.delete().Execute(ctx(), status(e.asAdminA(), e.operatorA.ID)))
	})
}
