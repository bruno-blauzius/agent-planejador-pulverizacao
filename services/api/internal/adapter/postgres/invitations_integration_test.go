//go:build integration

package postgres_test

import (
	"errors"
	"testing"
	"time"

	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/domain"
	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/usecase"
	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/usecase/fakes"
)

func TestInvitationLifecycleAndTenant(t *testing.T) {
	company, other := newCompany(t), newCompany(t)
	inviter := newUser(t, company)
	invitee := newUser(t, company, func(u *domain.User) { u.Status = domain.UserInvited })
	now := time.Now().UTC().Truncate(time.Microsecond)
	invitation := domain.NewInvitation(domain.InvitationID(newID()), invitee, inviter.ID, true, now, now.Add(72*time.Hour))
	must(t, testDB.InsertInvitation(ctx(), invitation))

	// one open invitation per user
	again := domain.NewInvitation(domain.InvitationID(newID()), invitee, inviter.ID, false, now, now.Add(time.Hour))
	if err := testDB.InsertInvitation(ctx(), again); err == nil {
		t.Fatal("a second open invitation for the same user must be refused")
	}

	got, err := testDB.FindInvitation(ctx(), inCompany(company), invitation.ID)
	must(t, err)
	if got.UserID != invitee.ID || got.InvitedBy != inviter.ID || !got.AccountCreated || got.SendCount != 1 || !got.IsOpen() ||
		!got.ExpiresAt.Equal(invitation.ExpiresAt) {
		t.Fatalf("stored invitation differs: %+v", got)
	}
	_, err = testDB.FindInvitation(ctx(), inCompany(other), invitation.ID)
	requireIs(t, err, usecase.ErrNotFound)
	if _, err := testDB.FindInvitation(ctx(), allCompanies(), invitation.ID); err != nil {
		t.Fatalf("super admin scope: %v", err)
	}
	open, err := testDB.FindOpenInvitationByUser(ctx(), invitee.ID)
	must(t, err)
	if open.ID != invitation.ID {
		t.Fatalf("open = %+v", open)
	}

	must(t, got.Resend(now.Add(time.Hour), now.Add(96*time.Hour)))
	must(t, testDB.UpdateInvitation(ctx(), got))
	got, err = testDB.FindInvitation(ctx(), inCompany(company), invitation.ID)
	must(t, err)
	if got.SendCount != 2 || !got.ExpiresAt.Equal(now.Add(96*time.Hour)) {
		t.Fatalf("resend not stored: %+v", got)
	}

	must(t, got.Accept(now.Add(2*time.Hour)))
	must(t, testDB.UpdateInvitation(ctx(), got))
	if _, err := testDB.FindOpenInvitationByUser(ctx(), invitee.ID); !errors.Is(err, usecase.ErrNotFound) {
		t.Fatalf("an accepted invitation is not open: %v", err)
	}
	missing := got
	missing.ID = domain.InvitationID(newID())
	requireIs(t, testDB.UpdateInvitation(ctx(), missing), usecase.ErrNotFound)

	// the invitation of a deleted user is hidden along with the user
	stored, err := testDB.FindUser(ctx(), inCompany(company), invitee.ID)
	must(t, err)
	stored.Status = domain.UserInactive
	stored.MarkDeleted(now)
	must(t, testDB.UpdateUser(ctx(), stored))
	_, err = testDB.FindInvitation(ctx(), inCompany(company), invitation.ID)
	requireIs(t, err, usecase.ErrNotFound)
}

func TestFarmDirectoryIsTenantScoped(t *testing.T) {
	mine, other := newCompany(t), newCompany(t)
	farm, foreign := newFarm(t, mine), newFarm(t, other)

	must(t, testDB.CheckFarmsInCompany(ctx(), mine.ID, []domain.FarmID{farm}))
	must(t, testDB.CheckFarmsInCompany(ctx(), mine.ID, []domain.FarmID{farm, farm})) // repeated ids are one farm
	must(t, testDB.CheckFarmsInCompany(ctx(), mine.ID, nil))
	requireIs(t, testDB.CheckFarmsInCompany(ctx(), mine.ID, []domain.FarmID{farm, foreign}), usecase.ErrFarmNotInCompany)
	requireIs(t, testDB.CheckFarmsInCompany(ctx(), mine.ID, []domain.FarmID{domain.FarmID(newID())}), usecase.ErrFarmNotInCompany)

	ref, err := testDB.FindFarmRef(ctx(), mine.ID, farm)
	must(t, err)
	if ref.ID != farm || ref.Name == "" {
		t.Fatalf("ref = %+v", ref)
	}
	_, err = testDB.FindFarmRef(ctx(), mine.ID, foreign)
	requireIs(t, err, usecase.ErrNotFound)

	// soft deleted farms are not selectable
	_, err = testPool.Exec(ctx(), `UPDATE farms SET deleted_at = now() WHERE id = $1`, string(farm))
	must(t, err)
	requireIs(t, testDB.CheckFarmsInCompany(ctx(), mine.ID, []domain.FarmID{farm}), usecase.ErrFarmNotInCompany)
}

// TestCancelInvitationSoftDeletesInvitedUser runs the real use case on the real
// database: an invited user is soft deleted together with the cancellation, which
// ck_users_deleted_inactive only accepts when the status goes to inactive too.
func TestCancelInvitationSoftDeletesInvitedUser(t *testing.T) {
	company := newCompany(t)
	admin := newUser(t, company)
	invitee := newUser(t, company, func(u *domain.User) { u.Status = domain.UserInvited })
	now := time.Now().UTC().Truncate(time.Microsecond)
	invitation := domain.NewInvitation(domain.InvitationID(newID()), invitee, admin.ID, true, now, now.Add(72*time.Hour))
	must(t, testDB.InsertInvitation(ctx(), invitation))

	idp := fakes.NewProvider("https://idp/realms/x")
	account := idp.SeedAccount(invitee.Email, false)
	idp.Groups[account.Subject] = invitee.Role
	must(t, testDB.LinkIdentity(ctx(), domain.Identity{UserID: invitee.ID, Issuer: account.Issuer, Subject: account.Subject}))

	cancel := usecase.NewCancelInvitation(usecase.CancelInvitationDeps{
		Invitations: testDB, Users: testDB, Identities: testDB, Accounts: idp, Access: idp,
		Tx: testDB, Clock: &fakes.FixedClock{T: now.Add(time.Hour)},
	})
	actor := domain.Principal{
		UserID: admin.ID, CompanyID: company.ID, Name: admin.Name, Role: domain.RoleAdmin, FarmScope: domain.AllFarms(),
		Permissions: []domain.Permission{domain.PermUsersManage},
	}
	must(t, cancel.Execute(ctx(), usecase.CancelInvitationInput{Actor: actor, InvitationID: invitation.ID}))

	var status string
	var deleted bool
	must(t, testPool.QueryRow(ctx(), `SELECT status, deleted_at IS NOT NULL FROM users WHERE id = $1`, string(invitee.ID)).Scan(&status, &deleted))
	if status != string(domain.UserInactive) || !deleted {
		t.Fatalf("user status = %q, deleted = %v", status, deleted)
	}
	_, err := testDB.FindUser(ctx(), allCompanies(), invitee.ID)
	requireIs(t, err, usecase.ErrNotFound)
	_, err = testDB.FindIdentityByUser(ctx(), invitee.ID)
	requireIs(t, err, usecase.ErrNotFound)
	var cancelled bool
	must(t, testPool.QueryRow(ctx(), `SELECT cancelled_at IS NOT NULL FROM invitations WHERE id = $1`, string(invitation.ID)).Scan(&cancelled))
	if !cancelled {
		t.Fatal("the invitation must be cancelled")
	}
	if _, stillInGroup := idp.InGroup(account.Subject); stillInGroup || idp.CallsOf("DeleteCreatedAccount") != 1 {
		t.Fatalf("provider cleanup missing: calls = %v", idp.Calls)
	}
}
