package httpadapter_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	httpadapter "github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/adapter/http"
	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/domain"
	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/usecase"
	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/usecase/fakes"
)

const (
	issuerURL = "http://idp.test/realms/inceres"

	operatorCompanyID = "00000000-0000-4000-8000-0000000000a0"
	companyAID        = "00000000-0000-4000-8000-0000000000a1"
	companyBID        = "00000000-0000-4000-8000-0000000000b1"

	superAdminID = "00000000-0000-4000-8000-000000000001"
	adminAID     = "00000000-0000-4000-8000-000000000002"
	adminBID     = "00000000-0000-4000-8000-000000000003"
	operatorAID  = "00000000-0000-4000-8000-000000000004"

	farmA1 = "00000000-0000-4000-8000-0000000000f1"
	farmB1 = "00000000-0000-4000-8000-0000000000f2"

	allowedOrigin = "http://localhost:3000"
	missingID     = "00000000-0000-4000-8000-00000000dead"
)

var (
	adminPermissions = []domain.Permission{
		domain.PermDashboardView, domain.PermUsersManage, domain.PermRegistryWrite,
		domain.PermApplicationsRegister, domain.PermApplicationsConfirm, domain.PermPlannerUse,
	}
	superPermissions = []domain.Permission{
		domain.PermDashboardView, domain.PermCompaniesManage, domain.PermUsersManage, domain.PermUsersManageAll,
		domain.PermRegistryWrite, domain.PermApplicationsRegister, domain.PermApplicationsConfirm, domain.PermPlannerUse,
	}
	operatorPermissions = []domain.Permission{domain.PermApplicationsRegister, domain.PermPlannerUse}
)

// fakeAuthenticator turns the X-Test-User header into the Principal of a user
// of the in-memory database. X-Test-Auth-Error injects the failures a real
// authenticator can return.
type fakeAuthenticator struct{ db *fakes.Memory }

func (a fakeAuthenticator) Authenticate(r *http.Request) (domain.Principal, error) {
	switch r.Header.Get("X-Test-Auth-Error") {
	case "":
	case "not_provisioned":
		return domain.Principal{}, usecase.ErrUserNotProvisioned
	case "no_access":
		return domain.Principal{}, domain.ErrNoAccessToProduct
	case "ambiguous":
		return domain.Principal{}, domain.ErrAmbiguousRole
	case "inactive":
		return domain.Principal{}, usecase.ErrAccessDenied
	case "provider_down":
		return domain.Principal{}, usecase.ErrIdentityProviderUnavailable
	default:
		return domain.Principal{}, errors.New("secret database password leaked")
	}
	user, ok := a.db.Users[domain.UserID(r.Header.Get("X-Test-User"))]
	if !ok {
		return domain.Principal{}, usecase.ErrUnauthenticated
	}
	permissions := operatorPermissions
	switch user.Role {
	case domain.RoleSuperAdmin:
		permissions = superPermissions
	case domain.RoleAdmin:
		permissions = adminPermissions
	case domain.RolePlanningOperator:
	}
	return domain.Principal{
		UserID: user.ID, CompanyID: user.CompanyID, Name: user.Name, Email: user.Email,
		Role: user.Role, Permissions: permissions, FarmScope: user.FarmScope,
	}, nil
}

// apiEnv is a small world behind the complete router: a platform operator
// company with a super admin, client companies A and B with an admin each, and
// an operator in A.
type apiEnv struct {
	t       *testing.T
	db      *fakes.Memory
	idp     *fakes.Provider
	clock   *fakes.FixedClock
	handler http.Handler
	logs    *bytes.Buffer
}

func newAPIEnv(t *testing.T) *apiEnv {
	t.Helper()
	e := &apiEnv{
		t:     t,
		db:    fakes.NewMemory(),
		idp:   fakes.NewProvider(issuerURL),
		clock: &fakes.FixedClock{T: time.Date(2026, 11, 22, 12, 0, 0, 0, time.UTC)},
		logs:  &bytes.Buffer{},
	}
	e.addCompany(operatorCompanyID, "Agentes Agro", "00000000000191", true)
	e.addCompany(companyAID, "Fazenda A", "11222333000181", false)
	e.addCompany(companyBID, "Fazenda B", "45997418000153", false)
	e.addUser(superAdminID, companyOf(e, operatorCompanyID), "Sofia Super", "sofia@agro.test", domain.RoleSuperAdmin)
	e.addUser(adminAID, companyOf(e, companyAID), "Ana Admin", "ana@a.test", domain.RoleAdmin)
	e.addUser(adminBID, companyOf(e, companyBID), "Beto Admin", "beto@b.test", domain.RoleAdmin)
	e.addUser(operatorAID, companyOf(e, companyAID), "Olga Operadora", "olga@a.test", domain.RolePlanningOperator)
	e.db.AddFarm(companyAID, farmA1, "Santa Rita")
	e.db.AddFarm(companyBID, farmB1, "Boa Esperança")

	settings := usecase.Settings{
		InvitationTTL: 72 * time.Hour, TrialDuration: 14 * 24 * time.Hour,
		LastAccessInterval: 5 * time.Minute, DefaultPageSize: 20, MaxPageSize: 100,
	}
	logger := slog.New(slog.NewTextHandler(e.logs, nil))
	e.handler = httpadapter.NewRouter(httpadapter.RouterDeps{
		Logger:             logger,
		Liveness:           usecase.CheckLiveness{},
		Startup:            fakeProbe{report: up},
		Readiness:          fakeReadiness{report: up},
		ProbeTimeout:       time.Second,
		CORSAllowedOrigins: []string{allowedOrigin},
		Authenticator:      fakeAuthenticator{db: e.db},
		Groups:             e.groups(logger, settings),
	})
	return e
}

// groups wires the real use cases over the in-memory fakes, like the composition root does.
func (e *apiEnv) groups(logger *slog.Logger, settings usecase.Settings) []httpadapter.RouteGroup {
	db, idp, clock := e.db, e.idp, e.clock
	ids := &sequentialUUIDs{}
	statusDeps := usecase.UserStatusDeps{
		Users: db, Companies: db, Identities: db, Access: idp, Lister: db, Roles: db, Tx: db, Clock: clock,
	}
	return []httpadapter.RouteGroup{
		httpadapter.NewUsersHandler(logger, httpadapter.UsersDeps{
			List: usecase.NewListUsers(db, settings), Summary: usecase.NewGetUsersSummary(db), Get: usecase.NewGetUser(db),
			Update: usecase.NewUpdateUser(usecase.UpdateUserDeps{
				Users: db, Companies: db, Farms: db, Identities: db, Access: idp, Lister: db, Roles: db, Tx: db, Clock: clock,
			}),
			Deactivate: usecase.NewDeactivateUser(statusDeps), Reactivate: usecase.NewReactivateUser(statusDeps),
			Remove: usecase.NewDeleteUser(statusDeps),
		}),
		httpadapter.NewInvitationsHandler(logger, httpadapter.InvitationsDeps{
			Invite: usecase.NewInviteUser(usecase.InviteUserDeps{
				Users: db, Companies: db, Farms: db, Accounts: idp, Access: idp, Mailer: idp, Tx: db, Clock: clock, IDs: ids, Settings: settings,
			}),
			Resend: usecase.NewResendInvitation(usecase.ResendInvitationDeps{
				Invitations: db, Users: db, Companies: db, Identities: db, Accounts: idp, Access: idp, Mailer: idp, Tx: db, Clock: clock, Settings: settings,
			}),
			Cancel: usecase.NewCancelInvitation(usecase.CancelInvitationDeps{
				Invitations: db, Users: db, Identities: db, Accounts: idp, Access: idp, Tx: db, Clock: clock,
			}),
		}),
		httpadapter.NewCompaniesHandler(logger, httpadapter.CompaniesDeps{
			List: usecase.NewListCompanies(db, settings), Summary: usecase.NewGetCompaniesSummary(db), Get: usecase.NewGetCompany(db),
			Create: usecase.NewCreateCompany(db, db, clock, ids, settings),
			Update: usecase.NewUpdateCompany(usecase.UpdateCompanyDeps{
				Finder: db, Lister: db, Members: db, Access: idp, Tx: db, Clock: clock, Settings: settings,
			}),
			Remove: usecase.NewDeleteCompany(db, db, db, clock),
		}),
		httpadapter.NewMeHandler(logger, httpadapter.MeDeps{
			Me: usecase.NewGetMe(db, db, db), Profile: usecase.NewGetProfile(db, db), UpdateProfile: usecase.NewUpdateProfile(db, db, db),
		}),
	}
}

// sequentialUUIDs makes ids that pass the UUID check of the edge: 00000000-0000-4000-8000-0000000001NN.
type sequentialUUIDs struct{ n int }

func (s *sequentialUUIDs) NewID() string {
	s.n++
	return "00000000-0000-4000-8000-" + strings.Repeat("0", 8) + padded(1000+s.n)
}

func padded(n int) string {
	digits := []byte("0000")
	for i := 3; i >= 0 && n > 0; i-- {
		digits[i] = byte('0' + n%10)
		n /= 10
	}
	return string(digits)
}

func companyOf(e *apiEnv, id string) domain.Company { return e.db.Companies[domain.CompanyID(id)] }

func (e *apiEnv) addCompany(id, name, document string, operator bool) {
	e.db.Companies[domain.CompanyID(id)] = domain.Company{
		ID: domain.CompanyID(id), Name: name, Document: domain.Document(document), City: "Sorriso", State: "MT",
		Plan: domain.PlanPro, Status: domain.CompanyActive, IsPlatformOperator: operator, CreatedAt: e.clock.T,
	}
}

func (e *apiEnv) addUser(id string, company domain.Company, name, email string, role domain.Role) domain.User {
	user := domain.User{
		ID: domain.UserID(id), CompanyID: company.ID, Name: name, Email: domain.Email(email),
		Role: role, Status: domain.UserActive, FarmScope: domain.AllFarms(), CreatedAt: e.clock.T,
	}
	account := e.idp.SeedAccount(user.Email, true)
	e.idp.Groups[account.Subject] = role
	e.db.PutUser(user, &domain.Identity{UserID: user.ID, Issuer: issuerURL, Subject: account.Subject})
	return user
}

// addInvited creates an invited user with an open invitation.
func (e *apiEnv) addInvited(id string, company domain.Company, email string) (domain.User, domain.Invitation) {
	user := domain.User{
		ID: domain.UserID(id), CompanyID: company.ID, Name: "Convidado", Email: domain.Email(email),
		Role: domain.RolePlanningOperator, Status: domain.UserInvited, FarmScope: domain.AllFarms(), CreatedAt: e.clock.T,
	}
	account := e.idp.SeedAccount(user.Email, false)
	e.idp.Accounts[account.Subject].Used = false
	e.idp.Groups[account.Subject] = user.Role
	e.db.PutUser(user, &domain.Identity{UserID: user.ID, Issuer: issuerURL, Subject: account.Subject})
	invitation := domain.NewInvitation(domain.InvitationID(id[:len(id)-4]+"9999"), user, domain.UserID(adminAID), true, e.clock.T, e.clock.T.Add(72*time.Hour))
	e.db.Invitations[invitation.ID] = invitation
	return user, invitation
}

// ---- requests --------------------------------------------------------------------------

type reply struct {
	status int
	header http.Header
	raw    []byte
}

func (r reply) object(t *testing.T) map[string]any {
	t.Helper()
	var body map[string]any
	if err := json.Unmarshal(r.raw, &body); err != nil {
		t.Fatalf("response is not a JSON object: %v\n%s", err, r.raw)
	}
	return body
}

// errorCode returns error.code of the response.
func (r reply) errorCode(t *testing.T) string {
	t.Helper()
	detail, ok := r.object(t)["error"].(map[string]any)
	if !ok {
		t.Fatalf("response has no error envelope: %s", r.raw)
	}
	code, _ := detail["code"].(string)
	return code
}

// errorFields returns the field names listed in error.details.
func (r reply) errorFields(t *testing.T) []string {
	t.Helper()
	detail, _ := r.object(t)["error"].(map[string]any)
	list, _ := detail["details"].([]any)
	var fields []string
	for _, item := range list {
		entry, _ := item.(map[string]any)
		field, _ := entry["field"].(string)
		fields = append(fields, field)
	}
	return fields
}

// call sends a request as the given user ("" is anonymous). A string body is
// sent as is; anything else is marshalled to JSON.
func (e *apiEnv) call(as, method, path string, body any, modify ...func(*http.Request)) reply {
	e.t.Helper()
	var reader *bytes.Reader
	switch value := body.(type) {
	case nil:
		reader = bytes.NewReader(nil)
	case string:
		reader = bytes.NewReader([]byte(value))
	default:
		encoded, err := json.Marshal(value)
		if err != nil {
			e.t.Fatalf("encode request: %v", err)
		}
		reader = bytes.NewReader(encoded)
	}
	request := httptest.NewRequestWithContext(context.Background(), method, path, reader)
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	if as != "" {
		request.Header.Set("X-Test-User", as)
	}
	for _, change := range modify {
		change(request)
	}
	recorder := httptest.NewRecorder()
	e.handler.ServeHTTP(recorder, request)
	return reply{status: recorder.Code, header: recorder.Header(), raw: recorder.Body.Bytes()}
}

func withHeader(name, value string) func(*http.Request) {
	return func(r *http.Request) { r.Header.Set(name, value) }
}
