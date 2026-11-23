//go:build integration

package postgres_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/domain"
	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/usecase"
)

func TestCompanyRoundTripAndConstraints(t *testing.T) {
	trialEnd := time.Now().Add(14 * 24 * time.Hour).UTC().Truncate(time.Microsecond)
	company := newCompany(t, func(c *domain.Company) {
		c.Plan, c.Status, c.TrialEndsAt = domain.PlanTrial, domain.CompanyTrial, &trialEnd
	})
	got, err := testDB.FindCompany(ctx(), company.ID)
	must(t, err)
	if got.Version != 1 || got.Document != company.Document || got.TrialEndsAt == nil || !got.TrialEndsAt.Equal(trialEnd) || got.IsPlatformOperator {
		t.Fatalf("stored company differs: %+v", got)
	}

	dup := domain.Company{
		ID: domain.CompanyID(newID()), Name: "Duplicada", Document: company.Document, City: "X", State: "MG",
		Plan: domain.PlanBasic, Status: domain.CompanyActive,
	}
	requireIs(t, testDB.InsertCompany(ctx(), dup), usecase.ErrDocumentAlreadyRegistered)

	// a second platform operator violates the partial unique index: an integrity error, not a mapped one
	operator(t)
	second := newCompanyValue()
	second.IsPlatformOperator = true
	if err := testDB.InsertCompany(ctx(), second); err == nil {
		t.Fatal("two operators must not coexist")
	}
}

func newCompanyValue() domain.Company {
	return domain.Company{
		ID: domain.CompanyID(newID()), Name: "Valor", Document: domain.Document(randomDigits(14)), City: "X", State: "MG",
		Plan: domain.PlanBasic, Status: domain.CompanyActive,
	}
}

func TestCompanyUpdateVersionAndSoftDelete(t *testing.T) {
	company := newCompany(t)
	company.Name = "Renomeada"
	must(t, testDB.UpdateCompany(ctx(), company)) // version 0 means 1

	stale := company
	stale.Name = "Atrasada"
	requireIs(t, testDB.UpdateCompany(ctx(), stale), usecase.ErrVersionConflict)

	current, err := testDB.FindCompany(ctx(), company.ID)
	must(t, err)
	if current.Name != "Renomeada" || current.Version != 2 {
		t.Fatalf("current = %+v", current)
	}
	missing := current
	missing.ID = domain.CompanyID(newID())
	requireIs(t, testDB.UpdateCompany(ctx(), missing), usecase.ErrNotFound)

	current.Status = domain.CompanyInactive
	current.MarkDeleted(time.Now().UTC())
	must(t, testDB.UpdateCompany(ctx(), current))
	_, err = testDB.FindCompany(ctx(), company.ID)
	requireIs(t, err, usecase.ErrNotFound)
	_, err = testDB.FindCompanyListing(ctx(), company.ID)
	requireIs(t, err, usecase.ErrNotFound)
	_, err = testDB.LockCompany(ctx(), company.ID)
	requireIs(t, err, usecase.ErrNotFound)
	requireIs(t, testDB.UpdateCompany(ctx(), current), usecase.ErrNotFound)

	// the CNPJ stays reserved after the company is deleted
	reuse := newCompanyValue()
	reuse.Document = company.Document
	requireIs(t, testDB.InsertCompany(ctx(), reuse), usecase.ErrDocumentAlreadyRegistered)
}

func TestListCompanies(t *testing.T) {
	operator(t)
	tag := newID()[:8]
	named := func(prefix string) func(*domain.Company) {
		return func(c *domain.Company) { c.Name = prefix + " " + tag }
	}
	a := newCompany(t, named("Alfa"))
	newCompany(t, named("Beta"), func(c *domain.Company) { c.Status = domain.CompanyInactive })
	newCompany(t, named("Gama"), func(c *domain.Company) { c.City = "Franca" })
	deleted := newCompany(t, named("Delta"))
	deleted.Status = domain.CompanyInactive
	deleted.MarkDeleted(time.Now().UTC())
	must(t, testDB.UpdateCompany(ctx(), deleted))

	newFarm(t, a)
	newFarm(t, a)
	newUser(t, a)
	newUser(t, a)
	newUser(t, a, func(u *domain.User) { u.Status = domain.UserInactive; u.DeletedAt = ptr(time.Now().UTC()) })

	page, err := testDB.ListCompanies(ctx(), usecase.CompanyFilter{Search: tag}, firstPage(2))
	must(t, err)
	if page.Total != 3 || len(page.Items) != 2 || page.Items[0].Company.Name != "Alfa "+tag {
		t.Fatalf("page = total %d, %d items", page.Total, len(page.Items))
	}
	if page.Items[0].FarmCount != 2 || page.Items[0].UserCount != 2 {
		t.Fatalf("counts = farms %d users %d, want 2 and 2 (deleted users do not count)", page.Items[0].FarmCount, page.Items[0].UserCount)
	}
	second, err := testDB.ListCompanies(ctx(), usecase.CompanyFilter{Search: tag}, usecase.PageRequest{Page: 2, PageSize: 2})
	must(t, err)
	if len(second.Items) != 1 || second.Items[0].Company.Name != "Gama "+tag {
		t.Fatalf("second page = %+v", second.Items)
	}
	inactive, err := testDB.ListCompanies(ctx(), usecase.CompanyFilter{Search: tag, Status: domain.CompanyInactive}, firstPage(10))
	must(t, err)
	if inactive.Total != 1 || inactive.Items[0].Company.Name != "Beta "+tag {
		t.Fatalf("status filter = %d", inactive.Total)
	}
	byCity, err := testDB.ListCompanies(ctx(), usecase.CompanyFilter{Search: "franca"}, firstPage(10))
	must(t, err)
	if byCity.Total < 1 {
		t.Fatal("search must match the city, case-insensitively")
	}
	// the platform operator never shows up, even when searched by name
	op, err := testDB.ListCompanies(ctx(), usecase.CompanyFilter{Search: operator(t).Name}, firstPage(10))
	must(t, err)
	if op.Total != 0 {
		t.Fatalf("operator listed: %+v", op.Items)
	}
	listing, err := testDB.FindCompanyListing(ctx(), a.ID)
	must(t, err)
	if listing.FarmCount != 2 || listing.UserCount != 2 {
		t.Fatalf("listing counts = %+v", listing)
	}
}

func ptr[T any](v T) *T { return &v }

func TestSummarizeCompaniesCountsDeltas(t *testing.T) {
	operator(t)
	before, err := testDB.SummarizeCompanies(ctx())
	must(t, err)

	active := newCompany(t)
	trialEnd := time.Now().Add(time.Hour).UTC()
	newCompany(t, func(c *domain.Company) {
		c.Plan, c.Status, c.TrialEndsAt = domain.PlanTrial, domain.CompanyTrial, &trialEnd
	})
	newCompany(t, func(c *domain.Company) { c.Status = domain.CompanyInactive })
	newFarm(t, active)
	_, err = testPool.Exec(ctx(), `INSERT INTO farms (id, company_id, name, production_type, city, state, perimeter)
		VALUES ($1, $2, 'Com perímetro', 'grains', 'X', 'MG',
		ST_GeomFromText('MULTIPOLYGON(((-47.0 -19.0, -47.0 -19.01, -47.01 -19.01, -47.01 -19.0, -47.0 -19.0)))', 4326))`,
		newID(), string(active.ID))
	must(t, err)
	member := newUser(t, active, func(u *domain.User) { u.Status = domain.UserInvited })
	must(t, testDB.InsertInvitation(ctx(), domain.NewInvitation(domain.InvitationID(newID()), member, member.ID, false, time.Now().UTC(), trialEnd)))
	// the operator's own users and farms are not client data
	newUser(t, operator(t))

	after, err := testDB.SummarizeCompanies(ctx())
	must(t, err)
	if d := after.ActiveCompanies - before.ActiveCompanies; d != 2 { // active + trial; inactive does not count
		t.Fatalf("active delta = %d", d)
	}
	if d := after.TrialCompanies - before.TrialCompanies; d != 1 {
		t.Fatalf("trial delta = %d", d)
	}
	if d := after.FarmCount - before.FarmCount; d != 2 {
		t.Fatalf("farm delta = %d", d)
	}
	if d := after.UserCount - before.UserCount; d != 1 {
		t.Fatalf("user delta = %d (operator users must not count)", d)
	}
	if d := after.PendingInvitations - before.PendingInvitations; d != 1 {
		t.Fatalf("pending invitations delta = %d", d)
	}
	if area := after.MonitoredAreaHa - before.MonitoredAreaHa; area < 100 || area > 130 { // ~1.05 km x 1.11 km
		t.Fatalf("monitored area delta = %.2f ha", area)
	}
}

func TestLockCompanySerializesWriters(t *testing.T) {
	company := newCompany(t)
	locked, release := make(chan struct{}), make(chan struct{})
	holder := make(chan error, 1)
	go func() {
		holder <- testDB.InTx(ctx(), func(c context.Context, tx usecase.Stores) error {
			if _, err := tx.Companies.LockCompany(c, company.ID); err != nil {
				return err
			}
			close(locked)
			<-release
			return nil
		})
	}()
	<-locked

	var acquired atomic.Bool
	waiter := make(chan error, 1)
	go func() {
		waiter <- testDB.InTx(ctx(), func(c context.Context, tx usecase.Stores) error {
			_, err := tx.Companies.LockCompany(c, company.ID)
			acquired.Store(true)
			return err
		})
	}()
	time.Sleep(400 * time.Millisecond)
	if acquired.Load() {
		t.Fatal("second writer got the lock while the first still holds it")
	}
	close(release)
	must(t, <-holder)
	must(t, <-waiter)
	if !acquired.Load() {
		t.Fatal("second writer never acquired the lock")
	}
}

func TestTransactionCommitsAndRollsBack(t *testing.T) {
	company := newCompany(t)
	boom := errors.New("boom")
	rolled := domain.User{ID: domain.UserID(newID()), CompanyID: company.ID, Name: "Revertido",
		Email: domain.Email(newID() + "@test.local"), Role: domain.RoleAdmin, Status: domain.UserActive, FarmScope: domain.AllFarms()}
	err := testDB.InTx(ctx(), func(c context.Context, tx usecase.Stores) error {
		if err := tx.Users.InsertUser(c, rolled); err != nil {
			return err
		}
		if err := tx.Audit.Record(c, audit(company, rolled.ID, "user.created")); err != nil {
			return err
		}
		return boom
	})
	requireIs(t, err, boom)
	if _, err := testDB.FindUser(ctx(), allCompanies(), rolled.ID); !errors.Is(err, usecase.ErrNotFound) {
		t.Fatal("rolled back user must not exist")
	}
	if n := auditCount(t, rolled.ID); n != 0 {
		t.Fatalf("rolled back audit rows = %d", n)
	}

	committed := rolled
	committed.ID, committed.Email = domain.UserID(newID()), domain.Email(newID()+"@test.local")
	must(t, testDB.InTx(ctx(), func(c context.Context, tx usecase.Stores) error {
		if err := tx.Users.InsertUser(c, committed); err != nil {
			return err
		}
		return tx.Audit.Record(c, audit(company, committed.ID, "user.created"))
	}))
	if _, err := testDB.FindUser(ctx(), allCompanies(), committed.ID); err != nil {
		t.Fatalf("committed user must exist: %v", err)
	}
	if n := auditCount(t, committed.ID); n != 1 {
		t.Fatalf("committed audit rows = %d", n)
	}
}

// A handled constraint error inside a transaction must not poison it: each
// write runs in a savepoint.
func TestTransactionSurvivesHandledConstraintError(t *testing.T) {
	company := newCompany(t)
	taken := newUser(t, company)
	fresh := domain.User{ID: domain.UserID(newID()), CompanyID: company.ID, Name: "Novo",
		Email: domain.Email(newID() + "@test.local"), Role: domain.RoleAdmin, Status: domain.UserActive, FarmScope: domain.AllFarms()}

	must(t, testDB.InTx(ctx(), func(c context.Context, tx usecase.Stores) error {
		clash := fresh
		clash.ID, clash.Email = domain.UserID(newID()), taken.Email
		if err := tx.Users.InsertUser(c, clash); !errors.Is(err, usecase.ErrEmailAlreadyRegistered) {
			t.Errorf("clash = %v", err)
		}
		return tx.Users.InsertUser(c, fresh) // the transaction is still usable
	}))
	if _, err := testDB.FindUser(ctx(), allCompanies(), fresh.ID); err != nil {
		t.Fatalf("write after a handled error must be committed: %v", err)
	}
}
