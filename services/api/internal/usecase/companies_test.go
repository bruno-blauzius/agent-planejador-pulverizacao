package usecase_test

import (
	"slices"
	"testing"

	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/domain"
	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/usecase"
)

const newCompanyDocument = "33.000.167/0001-01"

func newCompanyInput(actor domain.Principal) usecase.CreateCompanyInput {
	return usecase.CreateCompanyInput{Actor: actor, NewCompanyInput: domain.NewCompanyInput{
		Name: "Fazenda Nova", Document: newCompanyDocument, City: "Lucas do Rio Verde", State: "mt", Plan: domain.PlanPro,
	}}
}

func (e *env) createCompany() *usecase.CreateCompany {
	return usecase.NewCreateCompany(e.db, e.db, e.clock, e.ids, e.settings)
}

func (e *env) deleteCompany() *usecase.DeleteCompany {
	return usecase.NewDeleteCompany(e.db, e.db, e.db, e.clock)
}

func TestCompaniesRequireCompaniesManage(t *testing.T) {
	e := newEnv(t)
	for _, actor := range []domain.Principal{e.asAdminA(), e.asOperatorA()} {
		_, err := usecase.NewListCompanies(e.db, e.settings).Execute(ctx(), usecase.ListCompaniesInput{Actor: actor})
		assertIs(t, err, usecase.ErrForbidden)
		_, err = usecase.NewGetCompaniesSummary(e.db).Execute(ctx(), actor)
		assertIs(t, err, usecase.ErrForbidden)
		_, err = usecase.NewGetCompany(e.db).Execute(ctx(), usecase.GetCompanyInput{Actor: actor, CompanyID: "co-a"})
		assertIs(t, err, usecase.ErrForbidden)
		_, err = e.createCompany().Execute(ctx(), newCompanyInput(actor))
		assertIs(t, err, usecase.ErrForbidden)
		_, err = e.updateCompany().Execute(ctx(), usecase.UpdateCompanyInput{Actor: actor, CompanyID: "co-a", Name: stringPtr("x")})
		assertIs(t, err, usecase.ErrForbidden)
		assertIs(t, e.deleteCompany().Execute(ctx(), usecase.DeleteCompanyInput{Actor: actor, CompanyID: "co-a"}), usecase.ErrForbidden)
	}
}

func TestListCompaniesHidesTheOperator(t *testing.T) {
	e := newEnv(t)
	page, err := usecase.NewListCompanies(e.db, e.settings).Execute(ctx(), usecase.ListCompaniesInput{Actor: e.asSuper()})
	assertNoError(t, err)
	if page.Total != 2 || page.Page != 1 || page.PageSize != 20 {
		t.Fatalf("page = %+v", page)
	}
	for _, item := range page.Items {
		if item.Company.IsPlatformOperator {
			t.Fatal("the operator company must not be listed")
		}
	}

	_, err = usecase.NewGetCompany(e.db).Execute(ctx(), usecase.GetCompanyInput{Actor: e.asSuper(), CompanyID: "op"})
	assertIs(t, err, usecase.ErrNotFound)
}

func TestListCompaniesValidatesFiltersAndPage(t *testing.T) {
	e := newEnv(t)
	uc := usecase.NewListCompanies(e.db, e.settings)
	_, err := uc.Execute(ctx(), usecase.ListCompaniesInput{Actor: e.asSuper(), Status: "paused", Page: -1, PageSize: 1000})
	assertValidationOn(t, err, "status")
	assertValidationOn(t, err, "page")
	assertValidationOn(t, err, "page_size")
}

func TestCreateCompany(t *testing.T) {
	t.Run("stores the company and audits it", func(t *testing.T) {
		e := newEnv(t)
		created, err := e.createCompany().Execute(ctx(), newCompanyInput(e.asSuper()))
		assertNoError(t, err)
		if created.Company.State != "MT" || created.Company.Status != domain.CompanyActive || created.Company.Document != "33000167000101" {
			t.Fatalf("company = %+v", created.Company)
		}
		if _, ok := e.db.Companies[created.Company.ID]; !ok {
			t.Fatal("company not stored")
		}
		e.assertAudit(t, domain.AuditCompanyCreated)
	})
	t.Run("trial plan opens the evaluation window from configuration", func(t *testing.T) {
		e := newEnv(t)
		in := newCompanyInput(e.asSuper())
		in.Plan = domain.PlanTrial
		created, err := e.createCompany().Execute(ctx(), in)
		assertNoError(t, err)
		if created.Company.Status != domain.CompanyTrial || !created.Company.TrialEndsAt.Equal(e.clock.T.Add(e.settings.TrialDuration)) {
			t.Fatalf("company = %+v", created.Company)
		}
	})
	t.Run("duplicate document", func(t *testing.T) {
		e := newEnv(t)
		in := newCompanyInput(e.asSuper())
		in.Document = "11.222.333/0001-81" // company A
		_, err := e.createCompany().Execute(ctx(), in)
		assertIs(t, err, usecase.ErrDocumentAlreadyRegistered)
		e.assertNoAudit(t)
	})
	t.Run("invalid input", func(t *testing.T) {
		e := newEnv(t)
		in := newCompanyInput(e.asSuper())
		in.Document = "123"
		_, err := e.createCompany().Execute(ctx(), in)
		assertValidationOn(t, err, "document")
	})
	t.Run("database failure stores nothing", func(t *testing.T) {
		e := newEnv(t)
		e.db.Fail("Record", errBoom)
		_, err := e.createCompany().Execute(ctx(), newCompanyInput(e.asSuper()))
		assertIs(t, err, errBoom)
		if len(e.db.Companies) != 3 {
			t.Fatal("the transaction must roll back the company")
		}
	})
}

func TestUpdateCompanyDetailsAndPlan(t *testing.T) {
	e := newEnv(t)
	listing, err := e.updateCompany().Execute(ctx(), usecase.UpdateCompanyInput{
		Actor: e.asSuper(), CompanyID: "co-a", Name: stringPtr("Santa Rita Agro"), Plan: planPtr(domain.PlanTrial),
	})
	assertNoError(t, err)
	if listing.Company.Name != "Santa Rita Agro" || listing.Company.Status != domain.CompanyTrial || listing.Company.TrialEndsAt == nil {
		t.Fatalf("company = %+v", listing.Company)
	}
	e.assertAudit(t, domain.AuditCompanyUpdated)
	if len(e.db.SessionLog) != 0 {
		t.Fatal("editing details must not touch sessions")
	}
}

func planPtr(p domain.CompanyPlan) *domain.CompanyPlan       { return &p }
func statusPtr(s domain.CompanyStatus) *domain.CompanyStatus { return &s }

func TestUpdateCompanyInvalidStatusPlanCombination(t *testing.T) {
	e := newEnv(t)
	_, err := e.updateCompany().Execute(ctx(), usecase.UpdateCompanyInput{Actor: e.asSuper(), CompanyID: "co-a", Status: statusPtr(domain.CompanyTrial)})
	assertValidationOn(t, err, "status")
}

func TestUpdateCompanyOperatorAndUnknownAreNotFound(t *testing.T) {
	e := newEnv(t)
	for _, id := range []domain.CompanyID{"op", "nope"} {
		_, err := e.updateCompany().Execute(ctx(), usecase.UpdateCompanyInput{Actor: e.asSuper(), CompanyID: id, Status: statusPtr(domain.CompanyInactive)})
		assertIs(t, err, usecase.ErrNotFound)
	}
	if e.db.Companies["op"].Status != domain.CompanyActive {
		t.Fatal("the operator company can never be deactivated")
	}
}

func TestDeactivatingACompanyCutsAccessOfEveryUser(t *testing.T) {
	e := newEnv(t)
	subjects := []string{e.subjectOf(t, e.adminA), e.subjectOf(t, e.operatorA)}

	listing, err := e.updateCompany().Execute(ctx(), usecase.UpdateCompanyInput{Actor: e.asSuper(), CompanyID: "co-a", Status: statusPtr(domain.CompanyInactive)})
	assertNoError(t, err)

	if listing.Company.Status != domain.CompanyInactive {
		t.Fatalf("status = %s", listing.Company.Status)
	}
	if !slices.Contains(e.db.SessionLog, "revoke_company:co-a") {
		t.Fatalf("API sessions of the company must be revoked: %v", e.db.SessionLog)
	}
	for _, subject := range subjects {
		if !slices.Contains(e.idp.EndedSessions, subject) {
			t.Fatalf("product sessions of %s must end in the realm", subject)
		}
		if _, ok := e.idp.Accounts[subject]; !ok {
			t.Fatal("realm accounts are untouched")
		}
	}
	if len(e.idp.Groups) != 4 {
		t.Fatalf("group memberships must stay so that reactivation restores access: %v", e.idp.Groups)
	}
	e.assertAudit(t, domain.AuditCompanyDeactivated)

	// the very next request of any user of A is denied, even with a valid token
	_, err = e.resolve().Execute(ctx(), claimsFor(e, t, e.adminA, "admin", "users.manage"))
	assertIs(t, err, usecase.ErrAccessDenied)
	// users of B are not affected
	_, err = e.resolve().Execute(ctx(), claimsFor(e, t, e.adminB, "admin", "users.manage"))
	assertNoError(t, err)

	// reactivating the company restores the access without touching users
	_, err = e.updateCompany().Execute(ctx(), usecase.UpdateCompanyInput{Actor: e.asSuper(), CompanyID: "co-a", Status: statusPtr(domain.CompanyActive)})
	assertNoError(t, err)
	_, err = e.resolve().Execute(ctx(), claimsFor(e, t, e.adminA, "admin", "users.manage"))
	assertNoError(t, err)
}

func TestDeactivatingACompanyDoesNotReactivateInactiveUsers(t *testing.T) {
	e := newEnv(t)
	_, err := e.deactivate().Execute(ctx(), status(e.asAdminA(), e.operatorA.ID))
	assertNoError(t, err)
	for _, st := range []domain.CompanyStatus{domain.CompanyInactive, domain.CompanyActive} {
		_, err = e.updateCompany().Execute(ctx(), usecase.UpdateCompanyInput{Actor: e.asSuper(), CompanyID: "co-a", Status: statusPtr(st)})
		assertNoError(t, err)
	}
	if e.db.Users[e.operatorA.ID].Status != domain.UserInactive {
		t.Fatal("company reactivation must not reactivate users")
	}
}

func TestDeactivatingACompanyWithProviderFailure(t *testing.T) {
	e := newEnv(t)
	e.idp.Fail("EndProductSessions", errBoom)
	in := usecase.UpdateCompanyInput{Actor: e.asSuper(), CompanyID: "co-a", Status: statusPtr(domain.CompanyInactive)}

	_, err := e.updateCompany().Execute(ctx(), in)

	assertIs(t, err, usecase.ErrIdentityProviderUnavailable)
	if e.idp.CallsOf("EndProductSessions") != 2 {
		t.Fatalf("every user must be tried even when one fails, got %d calls", e.idp.CallsOf("EndProductSessions"))
	}
	if e.db.Companies["co-a"].Status != domain.CompanyInactive {
		t.Fatal("the database already blocks the company when the provider is down")
	}
	_, resolveErr := e.resolve().Execute(ctx(), claimsFor(e, t, e.adminA, "admin"))
	assertIs(t, resolveErr, usecase.ErrAccessDenied)

	// a retry finishes the cleanup of the sessions
	e.idp.Heal("EndProductSessions")
	_, err = e.updateCompany().Execute(ctx(), in)
	assertNoError(t, err)
	if len(e.idp.EndedSessions) != 2 {
		t.Fatalf("ended sessions = %v", e.idp.EndedSessions)
	}
	e.assertAudit(t, domain.AuditCompanyDeactivated)
}

func TestDeactivatingACompanyDatabaseFailureChangesNothing(t *testing.T) {
	for _, operation := range []string{"UpdateCompany", "RevokeCompanySessions", "Record", "Commit"} {
		t.Run(operation, func(t *testing.T) {
			e := newEnv(t)
			e.db.Fail(operation, errBoom)
			_, err := e.updateCompany().Execute(ctx(), usecase.UpdateCompanyInput{Actor: e.asSuper(), CompanyID: "co-a", Status: statusPtr(domain.CompanyInactive)})
			assertIs(t, err, errBoom)
			if e.db.Companies["co-a"].Status != domain.CompanyActive || len(e.idp.Calls) != 0 {
				t.Fatal("when the database fails nothing else may happen")
			}
		})
	}
}

func TestDeleteCompany(t *testing.T) {
	t.Run("refused while there are active or invited users", func(t *testing.T) {
		e := newEnv(t)
		assertIs(t, e.deleteCompany().Execute(ctx(), usecase.DeleteCompanyInput{Actor: e.asSuper(), CompanyID: "co-a"}), usecase.ErrCompanyHasActiveUsers)

		// only invited users left: still refused
		for _, id := range []domain.UserID{e.adminA.ID, e.operatorA.ID} {
			assertNoError(t, e.delete().Execute(ctx(), status(e.asSuper(), id)))
		}
		e.addInvited("u-new", e.companyA, "new@a.test", domain.RoleAdmin, true)
		assertIs(t, e.deleteCompany().Execute(ctx(), usecase.DeleteCompanyInput{Actor: e.asSuper(), CompanyID: "co-a"}), usecase.ErrCompanyHasActiveUsers)
		if e.db.Companies["co-a"].DeletedAt != nil {
			t.Fatal("company must not be deleted")
		}
	})
	t.Run("soft deletes when only inactive users remain", func(t *testing.T) {
		e := newEnv(t)
		for _, id := range []domain.UserID{e.adminA.ID, e.operatorA.ID} {
			_, err := e.deactivate().Execute(ctx(), status(e.asSuper(), id))
			assertNoError(t, err)
		}
		e.db.Audit = nil

		assertNoError(t, e.deleteCompany().Execute(ctx(), usecase.DeleteCompanyInput{Actor: e.asSuper(), CompanyID: "co-a"}))

		if e.db.Companies["co-a"].DeletedAt == nil {
			t.Fatal("company must be soft deleted")
		}
		if e.db.Companies["co-a"].Status != domain.CompanyInactive {
			t.Fatal("a deleted company must be inactive (ck_companies_deleted)")
		}
		e.assertAudit(t, domain.AuditCompanyDeleted)
		_, err := usecase.NewGetCompany(e.db).Execute(ctx(), usecase.GetCompanyInput{Actor: e.asSuper(), CompanyID: "co-a"})
		assertIs(t, err, usecase.ErrNotFound)
		// the CNPJ stays reserved
		in := newCompanyInput(e.asSuper())
		in.Document = "11.222.333/0001-81"
		_, err = e.createCompany().Execute(ctx(), in)
		assertIs(t, err, usecase.ErrDocumentAlreadyRegistered)
		// and nobody can sign in through it
		_, err = e.resolve().Execute(ctx(), claimsFor(e, t, e.adminA, "admin"))
		if err == nil {
			t.Fatal("users of a deleted company must not sign in")
		}
	})
	t.Run("the operator company is never deleted", func(t *testing.T) {
		e := newEnv(t)
		assertIs(t, e.deleteCompany().Execute(ctx(), usecase.DeleteCompanyInput{Actor: e.asSuper(), CompanyID: "op"}), usecase.ErrNotFound)
	})
	t.Run("count or write failure rolls back", func(t *testing.T) {
		e := newEnv(t)
		for _, id := range []domain.UserID{e.adminA.ID, e.operatorA.ID} {
			_, err := e.deactivate().Execute(ctx(), status(e.asSuper(), id))
			assertNoError(t, err)
		}
		e.db.Fail("Record", errBoom)
		assertIs(t, e.deleteCompany().Execute(ctx(), usecase.DeleteCompanyInput{Actor: e.asSuper(), CompanyID: "co-a"}), errBoom)
		if e.db.Companies["co-a"].DeletedAt != nil {
			t.Fatal("rolled back")
		}
	})
}

func TestCompaniesSummary(t *testing.T) {
	e := newEnv(t)
	summary, err := usecase.NewGetCompaniesSummary(e.db).Execute(ctx(), e.asSuper())
	assertNoError(t, err)
	if summary.ActiveCompanies != 2 || summary.FarmCount != 2 {
		t.Fatalf("summary = %+v", summary)
	}
}
