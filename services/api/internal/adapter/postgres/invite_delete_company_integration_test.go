//go:build integration

package postgres_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/domain"
	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/usecase"
	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/usecase/fakes"
)

type uuidGenerator struct{}

func (uuidGenerator) NewID() string { return newID() }

func newInviteUser(idp *fakes.Provider) *usecase.InviteUser {
	return usecase.NewInviteUser(usecase.InviteUserDeps{
		Users: testDB, Companies: testDB, Farms: testDB, Accounts: idp, Access: idp, Mailer: idp,
		Tx: testDB, Clock: &fakes.FixedClock{T: time.Now().UTC()}, IDs: uuidGenerator{},
		Settings: usecase.Settings{InvitationTTL: 72 * time.Hour, DefaultPageSize: 20, MaxPageSize: 100},
	})
}

func inviteInto(company domain.Company, actor domain.User) usecase.InviteUserInput {
	return usecase.InviteUserInput{
		Actor: domain.Principal{
			UserID: actor.ID, CompanyID: company.ID, Name: actor.Name, Role: domain.RoleAdmin, FarmScope: domain.AllFarms(),
			Permissions: []domain.Permission{domain.PermUsersManage},
		},
		CompanyID: company.ID, Name: "Convidada Teste", Email: newID() + "@invite.test", Role: domain.RoleAdmin, FarmScope: domain.AllFarms(),
	}
}

func liveUsers(t *testing.T, company domain.Company) int {
	t.Helper()
	var n int
	must(t, testPool.QueryRow(ctx(), `SELECT count(*) FROM users WHERE company_id = $1 AND deleted_at IS NULL`, string(company.ID)).Scan(&n))
	return n
}

// The delete commits while an invitation waits for the company lock: the
// invitation must be refused and leave no user behind.
func TestInviteUserLosesAgainstACommittedCompanyDelete(t *testing.T) {
	company := newCompany(t)
	admin := newUser(t, company)
	must(t, testDB.InTx(ctx(), func(c context.Context, tx usecase.Stores) error {
		return tx.Users.UpdateUser(c, withStatus(admin, domain.UserInactive)) // the company may be deleted without live members
	}))
	before := liveUsers(t, company)

	locked, release := make(chan struct{}), make(chan struct{})
	deleter := make(chan error, 1)
	go func() {
		deleter <- testDB.InTx(ctx(), func(c context.Context, tx usecase.Stores) error {
			current, err := tx.Companies.LockCompany(c, company.ID)
			if err != nil {
				return err
			}
			close(locked)
			<-release
			current.MarkDeleted(time.Now().UTC())
			return tx.Companies.UpdateCompany(c, current)
		})
	}()
	<-locked

	idp := fakes.NewProvider("https://idp/realms/x")
	invited := make(chan error, 1)
	go func() {
		_, err := newInviteUser(idp).Execute(ctx(), inviteInto(company, admin))
		invited <- err
	}()
	time.Sleep(500 * time.Millisecond) // the invitation is now blocked on the company lock
	select {
	case err := <-invited:
		t.Fatalf("the invitation must wait for the company lock, returned: %v", err)
	default:
	}
	close(release)
	must(t, <-deleter)

	if err := <-invited; !errors.Is(err, usecase.ErrNotFound) {
		t.Fatalf("invite into a deleted company = %v, want ErrNotFound", err)
	}
	if after := liveUsers(t, company); after != before {
		t.Fatalf("live users %d -> %d: an invited user slipped into a deleted company", before, after)
	}
	if len(idp.Accounts) != 0 {
		t.Fatalf("the account created for the refused invitation must be removed: %d left", len(idp.Accounts))
	}
}

// Whatever the interleaving, a deleted company never keeps an invited or active user.
func TestInviteUserAndDeleteCompanyNeverLeaveAnOrphanInvitation(t *testing.T) {
	for round := 0; round < 12; round++ {
		company := newCompany(t)
		admin := newUser(t, company)
		must(t, testDB.InTx(ctx(), func(c context.Context, tx usecase.Stores) error {
			return tx.Users.UpdateUser(c, withStatus(admin, domain.UserInactive))
		}))
		idp := fakes.NewProvider("https://idp/realms/x")
		deleteCompany := usecase.NewDeleteCompany(testDB, testDB, testDB, &fakes.FixedClock{T: time.Now().UTC()})
		superAdmin := domain.Principal{UserID: admin.ID, Name: admin.Name, Permissions: []domain.Permission{domain.PermCompaniesManage, domain.PermUsersManageAll}}

		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			_ = deleteCompany.Execute(ctx(), usecase.DeleteCompanyInput{Actor: superAdmin, CompanyID: company.ID})
		}()
		go func() {
			defer wg.Done()
			_, _ = newInviteUser(idp).Execute(ctx(), inviteInto(company, admin))
		}()
		wg.Wait()

		var deleted bool
		must(t, testPool.QueryRow(ctx(), `SELECT deleted_at IS NOT NULL FROM companies WHERE id = $1`, string(company.ID)).Scan(&deleted))
		var invited int
		must(t, testPool.QueryRow(ctx(), `SELECT count(*) FROM users WHERE company_id = $1 AND status = 'invited' AND deleted_at IS NULL`, string(company.ID)).Scan(&invited))
		if deleted && invited != 0 {
			t.Fatalf("round %d: company deleted with %d invited user(s)", round, invited)
		}
	}
}

func withStatus(user domain.User, status domain.UserStatus) domain.User {
	user.Status = status
	return user
}
