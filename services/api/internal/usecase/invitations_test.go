package usecase_test

import (
	"testing"
	"time"

	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/domain"
	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/usecase"
)

func TestResendInvitation(t *testing.T) {
	t.Run("renews the invitation and e-mails an account that still has to set its password", func(t *testing.T) {
		e := newEnv(t)
		_, invitation, subject := e.addInvited("u-new", e.companyA, "new@a.test", domain.RoleAdmin, true)
		e.clock.Advance(24 * time.Hour)

		result, err := e.resend().Execute(ctx(), usecase.ResendInvitationInput{Actor: e.asAdminA(), InvitationID: invitation.ID})
		assertNoError(t, err)

		if result.EmailDelivery != usecase.EmailSent || len(e.idp.Emails) != 1 || e.idp.Emails[0].Subject != subject {
			t.Fatalf("delivery = %s, emails = %+v", result.EmailDelivery, e.idp.Emails)
		}
		stored := e.db.Invitations[invitation.ID]
		if stored.SendCount != 2 || !stored.ExpiresAt.Equal(e.clock.T.Add(e.settings.InvitationTTL)) {
			t.Fatalf("invitation = %+v", stored)
		}
		e.assertAudit(t, domain.AuditInvitationResent)
	})

	t.Run("an account that already verified its e-mail needs no e-mail", func(t *testing.T) {
		e := newEnv(t)
		_, invitation, _ := e.addInvited("u-new", e.companyA, "new@a.test", domain.RoleAdmin, false)
		result, err := e.resend().Execute(ctx(), usecase.ResendInvitationInput{Actor: e.asAdminA(), InvitationID: invitation.ID})
		assertNoError(t, err)
		if result.EmailDelivery != usecase.EmailNotNeeded || len(e.idp.Emails) != 0 {
			t.Fatalf("delivery = %s", result.EmailDelivery)
		}
	})

	t.Run("an e-mail that does not leave still renews the invitation", func(t *testing.T) {
		e := newEnv(t)
		_, invitation, _ := e.addInvited("u-new", e.companyA, "new@a.test", domain.RoleAdmin, true)
		e.idp.Fail("SendActionEmail", errBoom)
		result, err := e.resend().Execute(ctx(), usecase.ResendInvitationInput{Actor: e.asAdminA(), InvitationID: invitation.ID})
		assertNoError(t, err)
		if result.EmailDelivery != usecase.EmailNotSent {
			t.Fatalf("delivery = %s", result.EmailDelivery)
		}
		assertIs(t, result.EmailFailure, errBoom)
		if e.db.Invitations[invitation.ID].SendCount != 2 {
			t.Fatal("the invitation must be renewed")
		}
	})

	t.Run("a provider that is down changes nothing", func(t *testing.T) {
		e := newEnv(t)
		_, invitation, _ := e.addInvited("u-new", e.companyA, "new@a.test", domain.RoleAdmin, true)
		for _, operation := range []string{"FindAccountByEmail", "AddToRoleGroup"} {
			e.idp.Fail(operation, errBoom)
			_, err := e.resend().Execute(ctx(), usecase.ResendInvitationInput{Actor: e.asAdminA(), InvitationID: invitation.ID})
			assertIs(t, err, usecase.ErrIdentityProviderUnavailable)
			e.idp.Heal(operation)
		}
		if e.db.Invitations[invitation.ID].SendCount != 1 {
			t.Fatal("the invitation must be untouched")
		}
		e.assertNoAudit(t)
	})

	t.Run("a database failure reports the error and sends no e-mail", func(t *testing.T) {
		e := newEnv(t)
		_, invitation, _ := e.addInvited("u-new", e.companyA, "new@a.test", domain.RoleAdmin, true)
		e.db.Fail("UpdateInvitation", errBoom)
		_, err := e.resend().Execute(ctx(), usecase.ResendInvitationInput{Actor: e.asAdminA(), InvitationID: invitation.ID})
		assertIs(t, err, errBoom)
		if len(e.idp.Emails) != 0 {
			t.Fatal("no e-mail for a renewal that was not stored")
		}
	})

	t.Run("closed, foreign and unauthorized", func(t *testing.T) {
		e := newEnv(t)
		_, open, _ := e.addInvited("u-open", e.companyA, "open@a.test", domain.RoleAdmin, true)
		_, closed, _ := e.addInvited("u-closed", e.companyA, "closed@a.test", domain.RoleAdmin, true)
		accepted := closed
		_ = accepted.Accept(e.clock.T)
		e.db.Invitations[closed.ID] = accepted

		_, err := e.resend().Execute(ctx(), usecase.ResendInvitationInput{Actor: e.asAdminA(), InvitationID: closed.ID})
		assertIs(t, err, domain.ErrInvitationClosed)

		_, err = e.resend().Execute(ctx(), usecase.ResendInvitationInput{Actor: e.asAdminB(), InvitationID: open.ID})
		assertIs(t, err, usecase.ErrNotFound)

		_, err = e.resend().Execute(ctx(), usecase.ResendInvitationInput{Actor: e.asOperatorA(), InvitationID: open.ID})
		assertIs(t, err, usecase.ErrForbidden)

		_, err = e.resend().Execute(ctx(), usecase.ResendInvitationInput{Actor: e.asSuper(), InvitationID: open.ID})
		assertNoError(t, err)
	})

	t.Run("inactive company", func(t *testing.T) {
		e := newEnv(t)
		_, invitation, _ := e.addInvited("u-new", e.companyA, "new@a.test", domain.RoleAdmin, true)
		company := e.db.Companies["co-a"]
		company.Status = domain.CompanyInactive
		e.db.Companies["co-a"] = company
		_, err := e.resend().Execute(ctx(), usecase.ResendInvitationInput{Actor: e.asSuper(), InvitationID: invitation.ID})
		assertIs(t, err, usecase.ErrCompanyInactive)
	})
}

func TestCancelInvitation(t *testing.T) {
	t.Run("deletes the account this invitation created and nobody used", func(t *testing.T) {
		e := newEnv(t)
		user, invitation, subject := e.addInvited("u-new", e.companyA, "new@a.test", domain.RoleAdmin, true)

		assertNoError(t, e.cancel().Execute(ctx(), usecase.CancelInvitationInput{Actor: e.asAdminA(), InvitationID: invitation.ID}))

		if _, ok := e.idp.Accounts[subject]; ok {
			t.Fatal("the account created by the invitation must be deleted")
		}
		if _, ok := e.idp.InGroup(subject); ok {
			t.Fatal("the account must leave the group")
		}
		stored := e.db.Users[user.ID]
		if !stored.IsDeleted() {
			t.Fatal("the invited user must be soft deleted")
		}
		if e.db.Invitations[invitation.ID].CancelledAt == nil {
			t.Fatal("the invitation must be cancelled")
		}
		if len(e.db.Identities) != len(e.db.Users)-1 {
			t.Fatalf("identity of the cancelled user must be unlinked: %v", e.db.Identities)
		}
		e.assertAudit(t, domain.AuditInvitationCancelled)
	})

	t.Run("keeps an account that already existed in the realm", func(t *testing.T) {
		e := newEnv(t)
		_, invitation, subject := e.addInvited("u-new", e.companyA, "new@a.test", domain.RoleAdmin, false)

		assertNoError(t, e.cancel().Execute(ctx(), usecase.CancelInvitationInput{Actor: e.asAdminA(), InvitationID: invitation.ID}))

		if _, ok := e.idp.Accounts[subject]; !ok {
			t.Fatal("a pre-existing account must never be deleted")
		}
		if _, ok := e.idp.InGroup(subject); ok {
			t.Fatal("the account must leave the product group")
		}
		if e.idp.CallsOf("DeleteCreatedAccount") != 0 {
			t.Fatal("DeleteCreatedAccount must not run for a reused account")
		}
	})

	t.Run("an account that was used meanwhile is kept without failing", func(t *testing.T) {
		e := newEnv(t)
		_, invitation, subject := e.addInvited("u-new", e.companyA, "new@a.test", domain.RoleAdmin, true)
		e.idp.Accounts[subject].Used = true

		assertNoError(t, e.cancel().Execute(ctx(), usecase.CancelInvitationInput{Actor: e.asAdminA(), InvitationID: invitation.ID}))

		if _, ok := e.idp.Accounts[subject]; !ok {
			t.Fatal("a used account must survive")
		}
	})

	t.Run("provider failure leaves the invitation open and a retry finishes", func(t *testing.T) {
		e := newEnv(t)
		user, invitation, _ := e.addInvited("u-new", e.companyA, "new@a.test", domain.RoleAdmin, true)
		in := usecase.CancelInvitationInput{Actor: e.asAdminA(), InvitationID: invitation.ID}

		e.idp.Fail("RemoveFromProductGroups", errBoom)
		assertIs(t, e.cancel().Execute(ctx(), in), usecase.ErrIdentityProviderUnavailable)
		if !e.db.Invitations[invitation.ID].IsOpen() || e.db.Users[user.ID].IsDeleted() {
			t.Fatal("nothing may be stored when the provider failed")
		}

		e.idp.Heal("RemoveFromProductGroups")
		assertNoError(t, e.cancel().Execute(ctx(), in))
		if e.db.Invitations[invitation.ID].IsOpen() {
			t.Fatal("retry must cancel the invitation")
		}
	})

	t.Run("database failure after the provider cleanup can be retried", func(t *testing.T) {
		e := newEnv(t)
		user, invitation, _ := e.addInvited("u-new", e.companyA, "new@a.test", domain.RoleAdmin, true)
		in := usecase.CancelInvitationInput{Actor: e.asAdminA(), InvitationID: invitation.ID}

		e.db.Fail("UpdateInvitation", errBoom)
		assertIs(t, e.cancel().Execute(ctx(), in), errBoom)
		if e.db.Users[user.ID].IsDeleted() {
			t.Fatal("the transaction must roll back the user delete")
		}

		e.db.Heal("UpdateInvitation")
		assertNoError(t, e.cancel().Execute(ctx(), in))
	})

	t.Run("closed, foreign and unauthorized", func(t *testing.T) {
		e := newEnv(t)
		_, open, _ := e.addInvited("u-open", e.companyA, "open@a.test", domain.RoleAdmin, true)

		assertIs(t, e.cancel().Execute(ctx(), usecase.CancelInvitationInput{Actor: e.asAdminB(), InvitationID: open.ID}), usecase.ErrNotFound)
		assertIs(t, e.cancel().Execute(ctx(), usecase.CancelInvitationInput{Actor: e.asOperatorA(), InvitationID: open.ID}), usecase.ErrForbidden)

		assertNoError(t, e.cancel().Execute(ctx(), usecase.CancelInvitationInput{Actor: e.asAdminA(), InvitationID: open.ID}))
		// the invitation is gone with its user: a second cancel is a 404
		assertIs(t, e.cancel().Execute(ctx(), usecase.CancelInvitationInput{Actor: e.asAdminA(), InvitationID: open.ID}), usecase.ErrNotFound)
	})

	t.Run("an accepted invitation cannot be cancelled", func(t *testing.T) {
		e := newEnv(t)
		_, invitation, _ := e.addInvited("u-new", e.companyA, "new@a.test", domain.RoleAdmin, true)
		accepted := invitation
		_ = accepted.Accept(e.clock.T)
		e.db.Invitations[invitation.ID] = accepted
		assertIs(t, e.cancel().Execute(ctx(), usecase.CancelInvitationInput{Actor: e.asAdminA(), InvitationID: invitation.ID}), domain.ErrInvitationClosed)
	})
}
