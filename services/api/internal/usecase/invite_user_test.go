package usecase_test

import (
	"slices"
	"testing"

	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/domain"
	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/usecase"
)

func inviteInput(actor domain.Principal, companyID domain.CompanyID, email string, role domain.Role) usecase.InviteUserInput {
	return usecase.InviteUserInput{
		Actor: actor, Name: "Carla Nova", Email: email, Role: role, CompanyID: companyID, FarmScope: domain.AllFarms(),
	}
}

func TestInviteUserWithNewAccount(t *testing.T) {
	e := newEnv(t)

	result, err := e.invite().Execute(ctx(), inviteInput(e.asAdminA(), "co-a", " Carla@A.test ", domain.RolePlanningOperator))
	assertNoError(t, err)

	if result.EmailDelivery != usecase.EmailSent || result.EmailFailure != nil {
		t.Fatalf("delivery = %s (%v)", result.EmailDelivery, result.EmailFailure)
	}
	user, ok := e.userByEmail("carla@a.test")
	if !ok || user.Status != domain.UserInvited || user.CompanyID != "co-a" || user.Role != domain.RolePlanningOperator {
		t.Fatalf("stored user = %+v (found %v)", user, ok)
	}
	if result.User.Invitation == nil || !result.User.Invitation.AccountCreated || result.User.CompanyName != "Fazenda A" {
		t.Fatalf("result = %+v", result.User)
	}
	if !result.User.Invitation.ExpiresAt.Equal(e.clock.T.Add(e.settings.InvitationTTL)) {
		t.Fatalf("expires_at = %v", result.User.Invitation.ExpiresAt)
	}
	subject := e.subjectOf(t, user)
	if role, ok := e.idp.InGroup(subject); !ok || role != domain.RolePlanningOperator {
		t.Fatalf("group = %v %v", role, ok)
	}
	if len(e.idp.Emails) != 1 || e.idp.Emails[0].Kind != usecase.EmailSetupAccount || e.idp.Emails[0].Subject != subject {
		t.Fatalf("emails = %+v", e.idp.Emails)
	}
	e.assertAudit(t, domain.AuditInvitationCreated)
	// ADR 0015, D5.1: account, group, database, and only then the e-mail.
	order := []string{"FindAccountByEmail", "CreateAccount", "AddToRoleGroup", "SendActionEmail"}
	if !slices.Equal(e.idp.Calls, order) {
		t.Fatalf("provider calls = %v, want %v", e.idp.Calls, order)
	}
}

func TestInviteUserReusesVerifiedAccountOfAnotherProduct(t *testing.T) {
	e := newEnv(t)
	existing := e.idp.SeedAccount("maria@other.test", true)

	result, err := e.invite().Execute(ctx(), inviteInput(e.asAdminA(), "co-a", "maria@other.test", domain.RoleAdmin))
	assertNoError(t, err)

	if result.EmailDelivery != usecase.EmailNotNeeded || len(e.idp.Emails) != 0 {
		t.Fatalf("delivery = %s, emails = %v", result.EmailDelivery, e.idp.Emails)
	}
	if e.idp.CallsOf("CreateAccount") != 0 {
		t.Fatal("an existing account must be reused, not created")
	}
	if got := e.subjectOf(t, result.User.User); got != existing.Subject {
		t.Fatalf("linked subject = %s, want %s", got, existing.Subject)
	}
	if result.User.Invitation.AccountCreated {
		t.Fatal("the invitation did not create the account: it must never delete it")
	}
	if role, _ := e.idp.InGroup(existing.Subject); role != domain.RoleAdmin {
		t.Fatalf("group = %s", role)
	}
	e.assertAudit(t, domain.AuditInvitationCreated, domain.AuditAccountLinked)
}

func TestInviteUserNeverLinksUnverifiedAccount(t *testing.T) {
	e := newEnv(t)
	existing := e.idp.SeedAccount("pending@other.test", false)
	users := len(e.db.Users)

	_, err := e.invite().Execute(ctx(), inviteInput(e.asAdminA(), "co-a", "pending@other.test", domain.RoleAdmin))

	assertIs(t, err, usecase.ErrEmailNotVerified)
	if len(e.db.Users) != users {
		t.Fatal("no user may be created")
	}
	if _, ok := e.idp.InGroup(existing.Subject); ok {
		t.Fatal("an unverified account must not enter the product group")
	}
	if len(e.idp.Emails) != 1 || e.idp.Emails[0].Kind != usecase.EmailVerifyEmail {
		t.Fatalf("the provider must resend the verification: %+v", e.idp.Emails)
	}
	e.assertNoAudit(t)
}

func TestInviteUserRefusesAccountLinkedToAnotherUser(t *testing.T) {
	e := newEnv(t)
	// ana@a.test changed her e-mail in the realm: the account is Ana's, not a new person's.
	subject := e.subjectOf(t, e.adminA)
	e.idp.Accounts[subject].Account.Email = "ana.new@a.test"

	_, err := e.invite().Execute(ctx(), inviteInput(e.asAdminA(), "co-a", "ana.new@a.test", domain.RolePlanningOperator))

	assertIs(t, err, usecase.ErrEmailAlreadyRegistered)
	if role, _ := e.idp.InGroup(subject); role != domain.RoleAdmin {
		t.Fatalf("Ana's group membership must be untouched, got %s", role)
	}
	if e.idp.CallsOf("AddToRoleGroup") != 0 {
		t.Fatal("the group must not be touched for an account owned by someone else")
	}
}

func TestInviteUserRefusesDuplicateEmailWithoutTouchingTheProvider(t *testing.T) {
	e := newEnv(t)
	for _, email := range []string{"ana@a.test", "ANA@a.test", "beto@b.test"} {
		_, err := e.invite().Execute(ctx(), inviteInput(e.asAdminA(), "co-a", email, domain.RoleAdmin))
		assertIs(t, err, usecase.ErrEmailAlreadyRegistered)
	}
	if len(e.idp.Calls) != 0 {
		t.Fatalf("provider was called: %v", e.idp.Calls)
	}
}

func TestInviteUserAuthorization(t *testing.T) {
	tests := []struct {
		name      string
		actor     func(*env) domain.Principal
		companyID domain.CompanyID
		role      domain.Role
		want      error
	}{
		{"admin invites into own company", (*env).asAdminA, "co-a", domain.RolePlanningOperator, nil},
		{"admin cannot invite into another company", (*env).asAdminA, "co-b", domain.RolePlanningOperator, usecase.ErrForbidden},
		{"admin cannot grant super admin", (*env).asAdminA, "co-a", domain.RoleSuperAdmin, usecase.ErrForbidden},
		{"operator has no users.manage", (*env).asOperatorA, "co-a", domain.RolePlanningOperator, usecase.ErrForbidden},
		{"super admin invites into any company", (*env).asSuper, "co-b", domain.RoleAdmin, nil},
		{"super admin into unknown company", (*env).asSuper, "co-zzz", domain.RoleAdmin, usecase.ErrForbidden},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newEnv(t)
			_, err := e.invite().Execute(ctx(), inviteInput(tt.actor(e), tt.companyID, "carla@x.test", tt.role))
			if tt.want == nil {
				assertNoError(t, err)
				return
			}
			if tt.companyID == "co-zzz" {
				assertValidationOn(t, err, "company_id")
			} else {
				assertIs(t, err, tt.want)
			}
			if len(e.idp.Calls) != 0 || len(e.db.Audit) != 0 {
				t.Fatal("a rejected invitation must not reach the provider or the audit")
			}
		})
	}
}

func TestInviteUserSuperAdminOnlyInOperatorCompany(t *testing.T) {
	e := newEnv(t)
	_, err := e.invite().Execute(ctx(), inviteInput(e.asSuper(), "co-a", "x@x.test", domain.RoleSuperAdmin))
	assertValidationOn(t, err, "role")

	_, err = e.invite().Execute(ctx(), inviteInput(e.asSuper(), "op", "y@x.test", domain.RoleSuperAdmin))
	assertNoError(t, err)
}

func TestInviteUserIntoInactiveCompany(t *testing.T) {
	e := newEnv(t)
	company := e.db.Companies["co-a"]
	company.Status = domain.CompanyInactive
	e.db.Companies["co-a"] = company

	_, err := e.invite().Execute(ctx(), inviteInput(e.asSuper(), "co-a", "x@x.test", domain.RoleAdmin))

	assertIs(t, err, usecase.ErrCompanyInactive)
	if len(e.idp.Calls) != 0 {
		t.Fatalf("provider was called: %v", e.idp.Calls)
	}
}

func TestInviteUserValidation(t *testing.T) {
	selected, _ := domain.NewSelectedFarms([]domain.FarmID{"farm-b1"})
	tests := []struct {
		name  string
		input func(*env) usecase.InviteUserInput
		field string
	}{
		{"bad e-mail", func(e *env) usecase.InviteUserInput {
			return inviteInput(e.asAdminA(), "co-a", "not-an-email", domain.RoleAdmin)
		}, "email"},
		{"empty name", func(e *env) usecase.InviteUserInput {
			in := inviteInput(e.asAdminA(), "co-a", "x@x.test", domain.RoleAdmin)
			in.Name = "  "
			return in
		}, "name"},
		{"unknown role", func(e *env) usecase.InviteUserInput {
			return inviteInput(e.asAdminA(), "co-a", "x@x.test", "owner")
		}, "role"},
		{"selected scope for an admin", func(e *env) usecase.InviteUserInput {
			in := inviteInput(e.asAdminA(), "co-a", "x@x.test", domain.RoleAdmin)
			in.FarmScope = selected
			return in
		}, "farm_scope"},
		{"farm of another company", func(e *env) usecase.InviteUserInput {
			in := inviteInput(e.asAdminA(), "co-a", "x@x.test", domain.RolePlanningOperator)
			in.FarmScope = selected
			return in
		}, "farm_scope.farm_ids"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newEnv(t)
			_, err := e.invite().Execute(ctx(), tt.input(e))
			assertValidationOn(t, err, tt.field)
			if len(e.idp.Calls) != 0 {
				t.Fatalf("provider was called: %v", e.idp.Calls)
			}
		})
	}
}

func TestInviteUserWithSelectedFarms(t *testing.T) {
	e := newEnv(t)
	selected, _ := domain.NewSelectedFarms([]domain.FarmID{"farm-a1"})
	in := inviteInput(e.asAdminA(), "co-a", "x@x.test", domain.RolePlanningOperator)
	in.FarmScope = selected

	result, err := e.invite().Execute(ctx(), in)
	assertNoError(t, err)
	if !result.User.User.FarmScope.Equal(selected) {
		t.Fatalf("scope = %+v", result.User.User.FarmScope)
	}
}

// Compensation: the provider and the database are not transactional (ADR 0015, D5.1).
func TestInviteUserCompensation(t *testing.T) {
	t.Run("account creation fails: nothing is left behind", func(t *testing.T) {
		e := newEnv(t)
		e.idp.Fail("CreateAccount", errBoom)
		_, err := e.invite().Execute(ctx(), inviteInput(e.asAdminA(), "co-a", "x@x.test", domain.RoleAdmin))
		assertIs(t, err, usecase.ErrIdentityProviderUnavailable)
		assertNothingLeft(t, e, "x@x.test", len(e.idp.Accounts))
	})

	t.Run("lookup fails: provider unavailable", func(t *testing.T) {
		e := newEnv(t)
		e.idp.Fail("FindAccountByEmail", errBoom)
		_, err := e.invite().Execute(ctx(), inviteInput(e.asAdminA(), "co-a", "x@x.test", domain.RoleAdmin))
		assertIs(t, err, usecase.ErrIdentityProviderUnavailable)
		assertNothingLeft(t, e, "x@x.test", len(e.idp.Accounts))
	})

	t.Run("group assignment fails: the created account is deleted", func(t *testing.T) {
		e := newEnv(t)
		accounts := len(e.idp.Accounts)
		e.idp.Fail("AddToRoleGroup", errBoom)
		_, err := e.invite().Execute(ctx(), inviteInput(e.asAdminA(), "co-a", "x@x.test", domain.RoleAdmin))
		assertIs(t, err, usecase.ErrIdentityProviderUnavailable)
		assertNothingLeft(t, e, "x@x.test", accounts)
	})

	t.Run("group assignment fails for a reused account: the account stays", func(t *testing.T) {
		e := newEnv(t)
		existing := e.idp.SeedAccount("maria@other.test", true)
		e.idp.Fail("AddToRoleGroup", errBoom)
		_, err := e.invite().Execute(ctx(), inviteInput(e.asAdminA(), "co-a", "maria@other.test", domain.RoleAdmin))
		assertIs(t, err, usecase.ErrIdentityProviderUnavailable)
		if _, ok := e.idp.Accounts[existing.Subject]; !ok {
			t.Fatal("a pre-existing account must never be deleted")
		}
		if e.idp.CallsOf("DeleteCreatedAccount") != 0 {
			t.Fatal("DeleteCreatedAccount must not run for a reused account")
		}
	})

	// Each database step fails in turn; the transaction rolls back and the
	// provider is compensated.
	for _, operation := range []string{"InsertUser", "LinkIdentity", "InsertInvitation", "Record", "Commit"} {
		t.Run("database fails at "+operation+": new account is deleted", func(t *testing.T) {
			e := newEnv(t)
			accounts := len(e.idp.Accounts)
			e.db.Fail(operation, errBoom)
			_, err := e.invite().Execute(ctx(), inviteInput(e.asAdminA(), "co-a", "x@x.test", domain.RoleAdmin))
			assertIs(t, err, errBoom)
			assertNothingLeft(t, e, "x@x.test", accounts)
			if len(e.idp.Emails) != 0 {
				t.Fatal("no e-mail may go out for a registration that failed")
			}
		})
		t.Run("database fails at "+operation+": reused account leaves the group, survives", func(t *testing.T) {
			e := newEnv(t)
			existing := e.idp.SeedAccount("maria@other.test", true)
			e.db.Fail(operation, errBoom)
			_, err := e.invite().Execute(ctx(), inviteInput(e.asAdminA(), "co-a", "maria@other.test", domain.RoleAdmin))
			assertIs(t, err, errBoom)
			if _, ok := e.idp.Accounts[existing.Subject]; !ok {
				t.Fatal("a pre-existing account must never be deleted")
			}
			if _, ok := e.idp.InGroup(existing.Subject); ok {
				t.Fatal("the reused account must leave the product group again")
			}
			if _, ok := e.userByEmail("maria@other.test"); ok {
				t.Fatal("no user may remain")
			}
		})
	}

	t.Run("a failed compensation is reported together with the cause", func(t *testing.T) {
		e := newEnv(t)
		e.db.Fail("InsertUser", errBoom)
		e.idp.Fail("DeleteCreatedAccount", usecase.ErrIdentityProviderUnavailable)
		_, err := e.invite().Execute(ctx(), inviteInput(e.asAdminA(), "co-a", "x@x.test", domain.RoleAdmin))
		assertIs(t, err, errBoom)
		assertIs(t, err, usecase.ErrIdentityProviderUnavailable)
	})
}

func assertNothingLeft(t *testing.T, e *env, email string, wantAccounts int) {
	t.Helper()
	if _, ok := e.userByEmail(email); ok {
		t.Fatal("no user may remain after a failed invitation")
	}
	if len(e.db.Invitations) != 0 {
		t.Fatalf("invitations = %v", e.db.Invitations)
	}
	if got := len(e.idp.Accounts); got != wantAccounts {
		t.Fatalf("realm accounts = %d, want %d (what the flow created must be undone)", got, wantAccounts)
	}
	for subject, role := range e.idp.Groups {
		if e.idp.Accounts[subject] == nil {
			t.Fatalf("group %s still holds the deleted account %s", role, subject)
		}
	}
	e.assertNoAudit(t)
}

func TestInviteUserEmailFailureKeepsTheInvitation(t *testing.T) {
	e := newEnv(t)
	e.idp.Fail("SendActionEmail", errBoom)

	result, err := e.invite().Execute(ctx(), inviteInput(e.asAdminA(), "co-a", "x@x.test", domain.RoleAdmin))

	assertNoError(t, err)
	if result.EmailDelivery != usecase.EmailNotSent {
		t.Fatalf("delivery = %s", result.EmailDelivery)
	}
	assertIs(t, result.EmailFailure, errBoom)
	if _, ok := e.userByEmail("x@x.test"); !ok {
		t.Fatal("the invitation must stay valid so it can be resent")
	}
}

func TestInviteUserReinviteAfterDeletionIsAllowed(t *testing.T) {
	e := newEnv(t)
	first, err := e.invite().Execute(ctx(), inviteInput(e.asAdminA(), "co-a", "x@x.test", domain.RoleAdmin))
	assertNoError(t, err)
	assertNoError(t, e.cancel().Execute(ctx(), usecase.CancelInvitationInput{Actor: e.asAdminA(), InvitationID: first.User.Invitation.ID}))

	_, err = e.invite().Execute(ctx(), inviteInput(e.asAdminA(), "co-a", "x@x.test", domain.RoleAdmin))
	assertNoError(t, err)
}
