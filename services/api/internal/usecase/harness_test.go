package usecase_test

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/domain"
	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/usecase"
	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/usecase/fakes"
)

const issuer = "http://idp.test/realms/inceres"

var errBoom = errors.New("boom")

var (
	adminPermissions = []domain.Permission{
		domain.PermDashboardView, domain.PermUsersManage, domain.PermRegistryWrite,
		domain.PermApplicationsRegister, domain.PermApplicationsConfirm, domain.PermPlannerUse,
		domain.PermFieldConditionsRecord,
	}
	superPermissions = []domain.Permission{
		domain.PermDashboardView, domain.PermCompaniesManage, domain.PermUsersManage, domain.PermUsersManageAll,
		domain.PermRegistryWrite, domain.PermApplicationsRegister, domain.PermApplicationsConfirm, domain.PermPlannerUse,
		domain.PermFieldConditionsRecord,
	}
	operatorPermissions = []domain.Permission{domain.PermApplicationsRegister, domain.PermPlannerUse, domain.PermFieldConditionsRecord}
)

// env is a small world: a platform operator company with a super admin, two
// client companies (A and B) with an admin each, and an operator in A. Every
// user has an account in the fake realm, inside its role group.
type env struct {
	db       *fakes.Memory
	idp      *fakes.Provider
	clock    *fakes.FixedClock
	ids      *fakes.SequentialIDs
	settings usecase.Settings

	operatorCo, companyA, companyB domain.Company

	superAdmin, adminA, adminB, operatorA domain.User
}

func newEnv(t *testing.T) *env {
	t.Helper()
	e := &env{
		db:    fakes.NewMemory(),
		idp:   fakes.NewProvider(issuer),
		clock: &fakes.FixedClock{T: time.Date(2026, 11, 22, 12, 0, 0, 0, time.UTC)},
		ids:   &fakes.SequentialIDs{},
		settings: usecase.Settings{
			InvitationTTL:      72 * time.Hour,
			TrialDuration:      14 * 24 * time.Hour,
			LastAccessInterval: 5 * time.Minute,
			DefaultPageSize:    20,
			MaxPageSize:        100,
			Areas:              domain.AreaRules{MinFarmAreaHa: 0.1, MinFieldAreaHa: 0.01, ToleranceM2: 10},
			MaxDocumentBytes:   1 << 20,
		},
	}
	e.operatorCo = e.addCompany("op", "Agentes Agro", "00000000000191", true)
	e.companyA = e.addCompany("co-a", "Fazenda A", "11222333000181", false)
	e.companyB = e.addCompany("co-b", "Fazenda B", "45997418000153", false)
	e.superAdmin = e.addUser("u-super", e.operatorCo, "Sofia Super", "sofia@agro.test", domain.RoleSuperAdmin, domain.AllFarms())
	e.adminA = e.addUser("u-admin-a", e.companyA, "Ana Admin", "ana@a.test", domain.RoleAdmin, domain.AllFarms())
	e.adminB = e.addUser("u-admin-b", e.companyB, "Beto Admin", "beto@b.test", domain.RoleAdmin, domain.AllFarms())
	e.operatorA = e.addUser("u-op-a", e.companyA, "Olga Operadora", "olga@a.test", domain.RolePlanningOperator, domain.AllFarms())
	e.db.AddFarm("co-a", "farm-a1", "Santa Rita")
	e.db.AddFarm("co-b", "farm-b1", "Boa Esperança")
	return e
}

func (e *env) addCompany(id, name, document string, operator bool) domain.Company {
	company := domain.Company{
		ID: domain.CompanyID(id), Name: name, Document: domain.Document(document), City: "Sorriso", State: "MT",
		Plan: domain.PlanPro, Status: domain.CompanyActive, IsPlatformOperator: operator, CreatedAt: e.clock.T,
	}
	e.db.Companies[company.ID] = company
	return company
}

// addUser creates an active user with a realm account in the group of its role.
func (e *env) addUser(id string, company domain.Company, name, email string, role domain.Role, scope domain.FarmScope) domain.User {
	user := domain.User{
		ID: domain.UserID(id), CompanyID: company.ID, Name: name, Email: domain.Email(email),
		Role: role, Status: domain.UserActive, FarmScope: scope, CreatedAt: e.clock.T,
	}
	account := e.idp.SeedAccount(user.Email, true)
	e.idp.Groups[account.Subject] = role
	e.db.PutUser(user, &domain.Identity{UserID: user.ID, Issuer: issuer, Subject: account.Subject})
	return user
}

// addInvited creates an invited user with an open invitation and a fresh
// (unused, product-created) realm account in its role group.
func (e *env) addInvited(id string, company domain.Company, email string, role domain.Role, accountCreated bool) (domain.User, domain.Invitation, string) {
	user := domain.User{
		ID: domain.UserID(id), CompanyID: company.ID, Name: "Convidado " + id, Email: domain.Email(email),
		Role: role, Status: domain.UserInvited, FarmScope: domain.AllFarms(), CreatedAt: e.clock.T,
	}
	account := e.idp.SeedAccount(user.Email, !accountCreated)
	e.idp.Accounts[account.Subject].Used = !accountCreated
	e.idp.Groups[account.Subject] = role
	e.db.PutUser(user, &domain.Identity{UserID: user.ID, Issuer: issuer, Subject: account.Subject})
	invitation := domain.NewInvitation(domain.InvitationID("inv-"+id), user, e.adminA.ID, accountCreated, e.clock.T, e.clock.T.Add(e.settings.InvitationTTL))
	e.db.Invitations[invitation.ID] = invitation
	return user, invitation, account.Subject
}

func (e *env) subjectOf(t *testing.T, user domain.User) string {
	t.Helper()
	for _, identity := range e.db.Identities {
		if identity.UserID == user.ID {
			return identity.Subject
		}
	}
	t.Fatalf("user %s has no identity", user.ID)
	return ""
}

func principal(user domain.User, permissions []domain.Permission) domain.Principal {
	return domain.Principal{
		UserID: user.ID, CompanyID: user.CompanyID, Name: user.Name, Email: user.Email,
		Role: user.Role, Permissions: permissions, FarmScope: user.FarmScope,
	}
}

func (e *env) asSuper() domain.Principal     { return principal(e.superAdmin, superPermissions) }
func (e *env) asAdminA() domain.Principal    { return principal(e.adminA, adminPermissions) }
func (e *env) asAdminB() domain.Principal    { return principal(e.adminB, adminPermissions) }
func (e *env) asOperatorA() domain.Principal { return principal(e.operatorA, operatorPermissions) }

// ---- use case factories ----------------------------------------------------

func (e *env) invite() *usecase.InviteUser {
	return usecase.NewInviteUser(usecase.InviteUserDeps{
		Users: e.db, Companies: e.db, Farms: e.db, Accounts: e.idp, Access: e.idp, Mailer: e.idp,
		Tx: e.db, Clock: e.clock, IDs: e.ids, Settings: e.settings,
	})
}

func (e *env) resend() *usecase.ResendInvitation {
	return usecase.NewResendInvitation(usecase.ResendInvitationDeps{
		Invitations: e.db, Users: e.db, Companies: e.db, Identities: e.db, Accounts: e.idp, Access: e.idp,
		Mailer: e.idp, Tx: e.db, Clock: e.clock, Settings: e.settings,
	})
}

func (e *env) cancel() *usecase.CancelInvitation {
	return usecase.NewCancelInvitation(usecase.CancelInvitationDeps{
		Invitations: e.db, Users: e.db, Identities: e.db, Accounts: e.idp, Access: e.idp, Tx: e.db, Clock: e.clock,
	})
}

func (e *env) update() *usecase.UpdateUser {
	return usecase.NewUpdateUser(usecase.UpdateUserDeps{
		Users: e.db, Companies: e.db, Farms: e.db, Identities: e.db, Access: e.idp, Lister: e.db, Roles: e.db, Tx: e.db, Clock: e.clock,
	})
}

func (e *env) statusDeps() usecase.UserStatusDeps {
	return usecase.UserStatusDeps{
		Users: e.db, Companies: e.db, Identities: e.db, Access: e.idp, Lister: e.db, Roles: e.db, Tx: e.db, Clock: e.clock,
	}
}

func (e *env) resolve() *usecase.ResolvePrincipal {
	return usecase.NewResolvePrincipal(e.db, e.db, e.db, e.db, e.clock, e.settings)
}

func (e *env) updateCompany() *usecase.UpdateCompany {
	return usecase.NewUpdateCompany(usecase.UpdateCompanyDeps{
		Finder: e.db, Lister: e.db, Members: e.db, Access: e.idp, Tx: e.db, Clock: e.clock, Settings: e.settings,
	})
}

// ---- assertions ------------------------------------------------------------------

func assertIs(t *testing.T, err, want error) {
	t.Helper()
	if !errors.Is(err, want) {
		t.Fatalf("error = %v, want %v", err, want)
	}
}

func assertNoError(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func assertValidationOn(t *testing.T, err error, field string) {
	t.Helper()
	var verr *domain.ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("error = %v, want ValidationError on %q", err, field)
	}
	for _, f := range verr.Fields {
		if f.Field == field {
			return
		}
	}
	t.Fatalf("validation fields = %v, want %q", verr.Fields, field)
}

func (e *env) assertAudit(t *testing.T, want ...string) {
	t.Helper()
	if got := e.db.AuditActions(); !slices.Equal(got, want) {
		t.Fatalf("audit = %v, want %v", got, want)
	}
}

func (e *env) assertNoAudit(t *testing.T) {
	t.Helper()
	if len(e.db.Audit) != 0 {
		t.Fatalf("unexpected audit events: %v", e.db.AuditActions())
	}
}

func (e *env) userByEmail(email string) (domain.User, bool) {
	for _, user := range e.db.Users {
		if user.Email == domain.Email(email) {
			return user, true
		}
	}
	return domain.User{}, false
}

func ctx() context.Context { return context.Background() }
