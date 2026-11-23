//go:build integration

package postgres_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/adapter/postgres"
	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/domain"
	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/usecase"
)

func TestUserRoundTripKeepsFarmScope(t *testing.T) {
	company := newCompany(t)
	farmA, farmB := newFarm(t, company), newFarm(t, company)
	user := newUser(t, company, func(u *domain.User) {
		u.Role = domain.RolePlanningOperator
		u.FarmScope = selected(t, farmA, farmB)
		primary := farmA
		u.PrimaryFarmID = &primary
	})

	got, err := testDB.FindUser(ctx(), inCompany(company), user.ID)
	must(t, err)
	if !got.FarmScope.Equal(user.FarmScope) || got.PrimaryFarmID == nil || *got.PrimaryFarmID != farmA {
		t.Fatalf("scope or primary farm lost: %+v", got)
	}
	if got.Version != 1 || got.Email != user.Email || got.Role != user.Role {
		t.Fatalf("stored user differs: %+v", got)
	}

	// shrink the scope: the access row of farm B must go away
	got.FarmScope = selected(t, farmA)
	must(t, testDB.UpdateUser(ctx(), got))
	reloaded, err := testDB.FindUser(ctx(), inCompany(company), user.ID)
	must(t, err)
	if !reloaded.FarmScope.Equal(selected(t, farmA)) || reloaded.Version != 2 {
		t.Fatalf("scope update not applied: %+v", reloaded)
	}

	// back to all farms
	reloaded.Role, reloaded.FarmScope, reloaded.PrimaryFarmID = domain.RoleAdmin, domain.AllFarms(), nil
	must(t, testDB.UpdateUser(ctx(), reloaded))
	final, err := testDB.FindUser(ctx(), inCompany(company), user.ID)
	must(t, err)
	if final.FarmScope.IsSelected() {
		t.Fatal("scope should be all")
	}
	var rows int
	must(t, testPool.QueryRow(ctx(), `SELECT count(*) FROM user_farm_access WHERE user_id = $1`, string(user.ID)).Scan(&rows))
	if rows != 0 {
		t.Fatalf("access rows left behind: %d", rows)
	}
}

func TestUserOfAnotherCompanyIsInvisible(t *testing.T) {
	mine, other := newCompany(t), newCompany(t)
	foreign := newUser(t, other)

	_, err := testDB.FindUser(ctx(), inCompany(mine), foreign.ID)
	requireIs(t, err, usecase.ErrNotFound)
	_, err = testDB.FindUserListing(ctx(), inCompany(mine), foreign.ID)
	requireIs(t, err, usecase.ErrNotFound)
	if _, err := testDB.FindUser(ctx(), allCompanies(), foreign.ID); err != nil {
		t.Fatalf("super admin scope must find it: %v", err)
	}
	if _, err := testDB.FindUser(ctx(), usecase.Scope{}, foreign.ID); !errors.Is(err, usecase.ErrNotFound) {
		t.Fatalf("empty scope must find nothing: %v", err)
	}

	page, err := testDB.ListUsers(ctx(), inCompany(mine), usecase.UserFilter{Search: foreign.Name}, firstPage(10))
	must(t, err)
	if page.Total != 0 || len(page.Items) != 0 {
		t.Fatalf("listing leaked another company: %+v", page)
	}
	// the client may ask for another company, but a single-company scope ignores the widening
	page, err = testDB.ListUsers(ctx(), inCompany(mine), usecase.UserFilter{CompanyID: other.ID}, firstPage(10))
	must(t, err)
	if page.Total != 0 {
		t.Fatalf("company filter widened the scope: %+v", page)
	}
	summary, err := testDB.SummarizeUsers(ctx(), inCompany(mine))
	must(t, err)
	if summary.TotalUsers != 0 {
		t.Fatalf("summary leaked another company: %+v", summary)
	}
}

func TestMalformedIDsAreNotFoundNotErrors(t *testing.T) {
	_, err := testDB.FindUser(ctx(), allCompanies(), "not-a-uuid")
	requireIs(t, err, usecase.ErrNotFound)
	_, err = testDB.FindUser(ctx(), usecase.OnlyCompany("nope"), domain.UserID(newID()))
	requireIs(t, err, usecase.ErrNotFound)
	_, err = testDB.FindCompany(ctx(), "x")
	requireIs(t, err, usecase.ErrNotFound)
	_, err = testDB.FindInvitation(ctx(), allCompanies(), "x")
	requireIs(t, err, usecase.ErrNotFound)
	_, err = testDB.FindFarmRef(ctx(), domain.CompanyID(newID()), "x")
	requireIs(t, err, usecase.ErrNotFound)
	requireIs(t, testDB.CheckFarmsInCompany(ctx(), domain.CompanyID(newID()), []domain.FarmID{"x"}), usecase.ErrFarmNotInCompany)
	page, err := testDB.ListUsers(ctx(), allCompanies(), usecase.UserFilter{CompanyID: "nope"}, firstPage(5))
	must(t, err)
	if page.Total != 0 {
		t.Fatal("malformed company filter must match nothing")
	}
}

func TestSoftDeletedUserDisappears(t *testing.T) {
	company := newCompany(t)
	user := newUser(t, company)
	must(t, testDB.LinkIdentity(ctx(), domain.Identity{UserID: user.ID, Issuer: "https://idp/realms/x", Subject: newID()}))
	stored, err := testDB.FindUser(ctx(), inCompany(company), user.ID)
	must(t, err)

	stored.Status = domain.UserInactive
	stored.MarkDeleted(time.Now().UTC())
	must(t, testDB.UpdateUser(ctx(), stored))

	_, err = testDB.FindUser(ctx(), allCompanies(), user.ID)
	requireIs(t, err, usecase.ErrNotFound)
	_, err = testDB.FindUserByEmail(ctx(), user.Email)
	requireIs(t, err, usecase.ErrNotFound)
	page, err := testDB.ListUsers(ctx(), inCompany(company), usecase.UserFilter{}, firstPage(10))
	must(t, err)
	if page.Total != 0 {
		t.Fatalf("deleted user listed: %+v", page)
	}
	identities, err := testDB.ListCompanyIdentities(ctx(), company.ID)
	must(t, err)
	if len(identities) != 0 {
		t.Fatalf("identity of a deleted user listed: %+v", identities)
	}
	// a deleted user cannot be written again
	requireIs(t, testDB.UpdateUser(ctx(), stored), usecase.ErrNotFound)
	// the e-mail is free for a new invitation
	newUser(t, company, func(u *domain.User) { u.Email = user.Email })
}

func TestUserUniquenessAndForeignKeys(t *testing.T) {
	company, other := newCompany(t), newCompany(t)
	existing := newUser(t, company)

	duplicate := domain.User{
		ID: domain.UserID(newID()), CompanyID: other.ID, Name: "Outro", Email: existing.Email,
		Role: domain.RoleAdmin, Status: domain.UserActive, FarmScope: domain.AllFarms(),
	}
	requireIs(t, testDB.InsertUser(ctx(), duplicate), usecase.ErrEmailAlreadyRegistered)

	foreignFarm := newFarm(t, other)
	crossTenant := domain.User{
		ID: domain.UserID(newID()), CompanyID: company.ID, Name: "Cruzado", Email: "x" + domain.Email(newID()+"@test.local"),
		Role: domain.RolePlanningOperator, Status: domain.UserActive, FarmScope: selected(t, foreignFarm),
	}
	requireIs(t, testDB.InsertUser(ctx(), crossTenant), usecase.ErrFarmNotInCompany)
	if _, err := testDB.FindUserByEmail(ctx(), crossTenant.Email); !errors.Is(err, usecase.ErrNotFound) {
		t.Fatalf("a failed insert must leave nothing behind: %v", err)
	}

	missingCompany := domain.User{
		ID: domain.UserID(newID()), CompanyID: domain.CompanyID(newID()), Name: "Sem empresa",
		Email: domain.Email(newID() + "@test.local"), Role: domain.RoleAdmin, Status: domain.UserActive, FarmScope: domain.AllFarms(),
	}
	var integrity *postgres.IntegrityError
	if err := testDB.InsertUser(ctx(), missingCompany); !errors.As(err, &integrity) || integrity.Kind != "foreign_key" {
		t.Fatalf("missing company must be a foreign key integrity error, got %v", err)
	}
}

func TestIdentityLinking(t *testing.T) {
	company := newCompany(t)
	a, b := newUser(t, company), newUser(t, company)
	issuer, subject := "https://idp/realms/inceres", newID()

	must(t, testDB.LinkIdentity(ctx(), domain.Identity{UserID: a.ID, Issuer: issuer, Subject: subject}))
	must(t, testDB.LinkIdentity(ctx(), domain.Identity{UserID: a.ID, Issuer: issuer, Subject: subject})) // idempotent
	requireIs(t, testDB.LinkIdentity(ctx(), domain.Identity{UserID: b.ID, Issuer: issuer, Subject: subject}), usecase.ErrIdentityAlreadyLinked)
	// the same user cannot hold a second account of the same issuer
	requireIs(t, testDB.LinkIdentity(ctx(), domain.Identity{UserID: a.ID, Issuer: issuer, Subject: newID()}), usecase.ErrIdentityAlreadyLinked)

	got, err := testDB.FindUserByIdentity(ctx(), issuer, subject)
	must(t, err)
	if got.ID != a.ID {
		t.Fatalf("identity resolved to %s", got.ID)
	}
	link, err := testDB.FindIdentityByUser(ctx(), a.ID)
	must(t, err)
	if link.Subject != subject {
		t.Fatalf("link = %+v", link)
	}
	if _, err := testDB.FindIdentityByUser(ctx(), b.ID); !errors.Is(err, usecase.ErrNotFound) {
		t.Fatalf("user without link: %v", err)
	}
	members, err := testDB.ListCompanyIdentities(ctx(), company.ID)
	must(t, err)
	if len(members) != 1 || members[0].UserID != a.ID {
		t.Fatalf("company identities = %+v", members)
	}

	must(t, testDB.UnlinkIdentities(ctx(), a.ID))
	if _, err := testDB.FindUserByIdentity(ctx(), issuer, subject); !errors.Is(err, usecase.ErrNotFound) {
		t.Fatalf("link should be gone: %v", err)
	}
	// freed for another user
	must(t, testDB.LinkIdentity(ctx(), domain.Identity{UserID: b.ID, Issuer: issuer, Subject: subject}))
}

func TestOptimisticVersioning(t *testing.T) {
	company := newCompany(t)
	user := newUser(t, company)
	// a freshly built user carries version 0 and is stored as version 1
	user.Name = "Renomeado"
	must(t, testDB.UpdateUser(ctx(), user))

	stale := user // still at version 0 => 1 expected, DB is at 2
	stale.Name = "Atrasado"
	requireIs(t, testDB.UpdateUser(ctx(), stale), usecase.ErrVersionConflict)

	current, err := testDB.FindUser(ctx(), inCompany(company), user.ID)
	must(t, err)
	if current.Version != 2 || current.Name != "Renomeado" {
		t.Fatalf("current = %+v", current)
	}
	missing := current
	missing.ID = domain.UserID(newID())
	requireIs(t, testDB.UpdateUser(ctx(), missing), usecase.ErrNotFound)
}

func TestUserMovesToAnotherCompany(t *testing.T) {
	from, to := newCompany(t), newCompany(t)
	farm := newFarm(t, from)
	user := newUser(t, from, func(u *domain.User) {
		u.Role, u.FarmScope = domain.RolePlanningOperator, selected(t, farm)
	})
	inv := domain.NewInvitation(domain.InvitationID(newID()), user, user.ID, false, time.Now().UTC(), time.Now().Add(time.Hour).UTC())
	must(t, testDB.InsertInvitation(ctx(), inv))

	stored, err := testDB.FindUser(ctx(), inCompany(from), user.ID)
	must(t, err)
	must(t, stored.Assign(to, domain.RoleAdmin, domain.AllFarms()))
	must(t, testDB.UpdateUser(ctx(), stored))

	if _, err := testDB.FindUser(ctx(), inCompany(from), user.ID); !errors.Is(err, usecase.ErrNotFound) {
		t.Fatal("old company must no longer see the user")
	}
	if _, err := testDB.FindUser(ctx(), inCompany(to), user.ID); err != nil {
		t.Fatalf("new company must see the user: %v", err)
	}
	// the invitation history followed the user (ON UPDATE CASCADE)
	if _, err := testDB.FindInvitation(ctx(), inCompany(to), inv.ID); err != nil {
		t.Fatalf("invitation must follow the user: %v", err)
	}
}

func TestListUsersFiltersSearchAndPaging(t *testing.T) {
	company, other := newCompany(t), newCompany(t)
	tag := newID()[:8]
	names := []string{"Ana " + tag, "Bruno " + tag, "Carla " + tag, "Diego " + tag, "Eva " + tag}
	for i, name := range names {
		newUser(t, company, func(u *domain.User) {
			u.Name = name
			if i == 0 {
				u.Role = domain.RolePlanningOperator
			}
			if i == 1 {
				u.Status = domain.UserInactive
			}
		})
	}
	newUser(t, other, func(u *domain.User) { u.Name = "Zélia " + tag })
	invited := newUser(t, company, func(u *domain.User) { u.Name = "Convidado " + tag; u.Status = domain.UserInvited })
	must(t, testDB.InsertInvitation(ctx(), domain.NewInvitation(domain.InvitationID(newID()), invited, invited.ID, true,
		time.Now().UTC(), time.Now().Add(time.Hour).UTC())))

	page, err := testDB.ListUsers(ctx(), inCompany(company), usecase.UserFilter{Search: tag}, firstPage(4))
	must(t, err)
	if page.Total != 6 || len(page.Items) != 4 || page.Items[0].User.Name != "Ana "+tag {
		t.Fatalf("first page = total %d items %d", page.Total, len(page.Items))
	}
	second, err := testDB.ListUsers(ctx(), inCompany(company), usecase.UserFilter{Search: tag}, usecase.PageRequest{Page: 2, PageSize: 4})
	must(t, err)
	if len(second.Items) != 2 || second.Total != 6 {
		t.Fatalf("second page = %d items", len(second.Items))
	}
	beyond, err := testDB.ListUsers(ctx(), inCompany(company), usecase.UserFilter{Search: tag}, usecase.PageRequest{Page: 9, PageSize: 4})
	must(t, err)
	if len(beyond.Items) != 0 || beyond.Total != 6 {
		t.Fatalf("page beyond the end = %+v", beyond)
	}

	byRole, err := testDB.ListUsers(ctx(), inCompany(company), usecase.UserFilter{Search: tag, Role: domain.RolePlanningOperator}, firstPage(10))
	must(t, err)
	if byRole.Total != 1 || byRole.Items[0].User.Name != "Ana "+tag {
		t.Fatalf("role filter = %+v", byRole.Total)
	}
	byStatus, err := testDB.ListUsers(ctx(), inCompany(company), usecase.UserFilter{Search: tag, Status: domain.UserInvited}, firstPage(10))
	must(t, err)
	if byStatus.Total != 1 || byStatus.Items[0].Invitation == nil || byStatus.Items[0].Invitation.UserID != invited.ID ||
		byStatus.Items[0].Invitation.SendCount != 1 || !byStatus.Items[0].Invitation.AccountCreated {
		t.Fatalf("invited row must carry its open invitation: %+v", byStatus.Items)
	}
	if byStatus.Items[0].CompanyName != company.Name {
		t.Fatalf("company name = %q", byStatus.Items[0].CompanyName)
	}
	byCompanyName, err := testDB.ListUsers(ctx(), allCompanies(), usecase.UserFilter{Search: other.Name}, firstPage(10))
	must(t, err)
	if byCompanyName.Total != 1 || byCompanyName.Items[0].User.Name != "Zélia "+tag {
		t.Fatalf("search by company name = %+v", byCompanyName.Total)
	}
	narrowed, err := testDB.ListUsers(ctx(), allCompanies(), usecase.UserFilter{Search: tag, CompanyID: other.ID}, firstPage(10))
	must(t, err)
	if narrowed.Total != 1 {
		t.Fatalf("super admin company filter = %d", narrowed.Total)
	}
}

func TestListUsersTreatsWildcardsAsLiterals(t *testing.T) {
	company := newCompany(t)
	newUser(t, company, func(u *domain.User) { u.Name = "Cem%por_cento" })
	newUser(t, company, func(u *domain.User) { u.Name = "Cemxporycento" })
	page, err := testDB.ListUsers(ctx(), inCompany(company), usecase.UserFilter{Search: "%por_"}, firstPage(10))
	must(t, err)
	if page.Total != 1 || page.Items[0].User.Name != "Cem%por_cento" {
		t.Fatalf("wildcards must be literal: %d rows", page.Total)
	}
}

func TestSummarizeUsers(t *testing.T) {
	company := newCompany(t)
	newUser(t, company, func(u *domain.User) { u.Role = domain.RolePlanningOperator })
	newUser(t, company)
	newUser(t, company)
	invited := newUser(t, company, func(u *domain.User) { u.Status = domain.UserInvited })
	must(t, testDB.InsertInvitation(ctx(), domain.NewInvitation(domain.InvitationID(newID()), invited, invited.ID, false,
		time.Now().UTC(), time.Now().Add(time.Hour).UTC())))

	summary, err := testDB.SummarizeUsers(ctx(), inCompany(company))
	must(t, err)
	if summary.TotalUsers != 4 || summary.CompanyCount != 1 || summary.PendingInvitations != 1 ||
		summary.ByRole[domain.RoleAdmin] != 3 || summary.ByRole[domain.RolePlanningOperator] != 1 {
		t.Fatalf("summary = %+v", summary)
	}
}

func TestCompanyMembersCounts(t *testing.T) {
	company := newCompany(t)
	newUser(t, company)
	newUser(t, company)
	newUser(t, company, func(u *domain.User) { u.Status = domain.UserInactive })
	counts, err := testDB.CountUsersByStatus(ctx(), company.ID)
	must(t, err)
	if counts[domain.UserActive] != 2 || counts[domain.UserInactive] != 1 || counts[domain.UserInvited] != 0 {
		t.Fatalf("counts = %+v", counts)
	}
}

// ---- last super admin ----------------------------------------------------------------

// The guard counts super admins across the whole product, so this test starts
// from a clean slate: it deactivates every super admin left by other tests.
func TestLastSuperAdminIsProtectedAtomically(t *testing.T) {
	op := operator(t)
	_, err := testPool.Exec(ctx(), `UPDATE users SET status = 'inactive' WHERE role = 'super_admin' AND status = 'active' AND deleted_at IS NULL`)
	must(t, err)
	makeSuper := func() domain.User {
		return newUser(t, op, func(u *domain.User) { u.Role = domain.RoleSuperAdmin })
	}
	a, b := makeSuper(), makeSuper()

	count, err := testDB.CountActiveUsersByRole(ctx(), domain.RoleSuperAdmin)
	must(t, err)
	if count != 2 {
		t.Fatalf("active super admins = %d, want 2", count)
	}

	deactivate := func(user domain.User, hold time.Duration) error {
		return testDB.InTx(ctx(), func(c context.Context, tx usecase.Stores) error {
			stored, err := tx.Users.FindUser(c, allCompanies(), user.ID)
			if err != nil {
				return err
			}
			stored.Status = domain.UserInactive
			if err := tx.Users.UpdateUser(c, stored); err != nil {
				return err
			}
			time.Sleep(hold) // keep the transaction open so the other one must wait for the lock
			return nil
		})
	}

	// two concurrent removals of the last two super admins: exactly one may win
	results := make(chan error, 2)
	var start sync.WaitGroup
	start.Add(1)
	for _, user := range []domain.User{a, b} {
		go func() {
			start.Wait()
			results <- deactivate(user, 400*time.Millisecond)
		}()
	}
	start.Done()
	first, second := <-results, <-results
	wins, refusals := 0, 0
	for _, err := range []error{first, second} {
		switch {
		case err == nil:
			wins++
		case errors.Is(err, usecase.ErrLastSuperAdmin):
			refusals++
		default:
			t.Fatalf("unexpected error: %v", err)
		}
	}
	if wins != 1 || refusals != 1 {
		t.Fatalf("wins=%d refusals=%d (errors: %v / %v)", wins, refusals, first, second)
	}
	count, err = testDB.CountActiveUsersByRole(ctx(), domain.RoleSuperAdmin)
	must(t, err)
	if count != 1 {
		t.Fatalf("exactly one super admin must remain, found %d", count)
	}

	// demoting, deactivating or deleting the one that remains is refused too
	var remaining domain.User
	for _, candidate := range []domain.User{a, b} {
		stored, err := testDB.FindUser(ctx(), allCompanies(), candidate.ID)
		must(t, err)
		if stored.Status == domain.UserActive {
			remaining = stored
		}
	}
	demoted := remaining
	demoted.Role = domain.RoleAdmin
	requireIs(t, testDB.UpdateUser(ctx(), demoted), usecase.ErrLastSuperAdmin)
	deleted := remaining
	deleted.Status = domain.UserInactive
	deleted.MarkDeleted(time.Now().UTC())
	requireIs(t, testDB.UpdateUser(ctx(), deleted), usecase.ErrLastSuperAdmin)

	// a second active super admin makes the same change legal again
	makeSuper()
	deleted = remaining
	deleted.Status = domain.UserInactive
	must(t, testDB.UpdateUser(ctx(), deleted))
}
