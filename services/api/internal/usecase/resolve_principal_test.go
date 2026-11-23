package usecase_test

import (
	"slices"
	"testing"
	"time"

	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/domain"
	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/usecase"
)

func claimsFor(e *env, t *testing.T, user domain.User, roles ...string) usecase.ResolvePrincipalInput {
	t.Helper()
	return usecase.ResolvePrincipalInput{Claims: usecase.Claims{
		Issuer: issuer, Subject: e.subjectOf(t, user), Email: user.Email.String(), EmailVerified: true, ClientRoles: roles,
	}}
}

func TestResolvePrincipalGrantsAccess(t *testing.T) {
	e := newEnv(t)
	in := claimsFor(e, t, e.adminA, "admin", "users.manage", "dashboard.view", "offline_access")

	got, err := e.resolve().Execute(ctx(), in)
	assertNoError(t, err)

	if got.UserID != e.adminA.ID || got.CompanyID != "co-a" || got.Role != domain.RoleAdmin {
		t.Fatalf("principal = %+v", got)
	}
	want := []domain.Permission{domain.PermDashboardView, domain.PermUsersManage}
	if !slices.Equal(got.Permissions, want) {
		t.Fatalf("permissions = %v, want %v (intersection with the enum)", got.Permissions, want)
	}
}

func TestResolvePrincipalDenials(t *testing.T) {
	deactivatedUser := func(e *env) {
		user := e.db.Users[e.adminA.ID]
		user.Status = domain.UserInactive
		e.db.Users[user.ID] = user
	}
	inactiveCompany := func(e *env) {
		company := e.db.Companies["co-a"]
		company.Status = domain.CompanyInactive
		e.db.Companies[company.ID] = company
	}
	deletedUser := func(e *env) {
		user := e.db.Users[e.adminA.ID]
		now := e.clock.T
		user.DeletedAt = &now
		e.db.Users[user.ID] = user
	}

	tests := []struct {
		name  string
		setup func(*env)
		roles []string
		want  error
	}{
		{name: "no product role", roles: []string{"users.manage"}, want: domain.ErrNoAccessToProduct},
		{name: "empty claim", roles: nil, want: domain.ErrNoAccessToProduct},
		{name: "two roles", roles: []string{"admin", "planning_operator"}, want: domain.ErrAmbiguousRole},
		{name: "inactive user", setup: deactivatedUser, roles: []string{"admin"}, want: usecase.ErrAccessDenied},
		{name: "inactive company cuts the access of its users", setup: inactiveCompany, roles: []string{"admin"}, want: usecase.ErrAccessDenied},
		{name: "deleted user", setup: deletedUser, roles: []string{"admin"}, want: usecase.ErrUserNotProvisioned},
		{name: "token says super admin but the company is a client", roles: []string{"super_admin"}, want: usecase.ErrAccessDenied},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newEnv(t)
			if tt.setup != nil {
				tt.setup(e)
			}
			_, err := e.resolve().Execute(ctx(), claimsFor(e, t, e.adminA, tt.roles...))
			assertIs(t, err, tt.want)
		})
	}
}

func TestResolvePrincipalNeverProvisionsOnLogin(t *testing.T) {
	e := newEnv(t)
	stranger := e.idp.SeedAccount("stranger@x.test", true)
	users := len(e.db.Users)

	_, err := e.resolve().Execute(ctx(), usecase.ResolvePrincipalInput{Claims: usecase.Claims{
		Issuer: issuer, Subject: stranger.Subject, Email: "stranger@x.test", EmailVerified: true, ClientRoles: []string{"admin"},
	}})

	assertIs(t, err, usecase.ErrUserNotProvisioned)
	if len(e.db.Users) != users {
		t.Fatal("login must never create a user (no JIT)")
	}
}

func TestResolvePrincipalAmbiguousRoleIsAuditedForKnownUsers(t *testing.T) {
	t.Run("known user", func(t *testing.T) {
		e := newEnv(t)
		_, err := e.resolve().Execute(ctx(), claimsFor(e, t, e.adminA, "admin", "super_admin"))
		assertIs(t, err, domain.ErrAmbiguousRole)
		e.assertAudit(t, domain.AuditAmbiguousRole)
		if e.db.Audit[0].CompanyID != "co-a" {
			t.Fatalf("audit company = %s", e.db.Audit[0].CompanyID)
		}
	})
	t.Run("unknown identity", func(t *testing.T) {
		e := newEnv(t)
		_, err := e.resolve().Execute(ctx(), usecase.ResolvePrincipalInput{Claims: usecase.Claims{
			Issuer: issuer, Subject: "nobody", ClientRoles: []string{"admin", "super_admin"},
		}})
		assertIs(t, err, domain.ErrAmbiguousRole)
		e.assertNoAudit(t)
	})
	t.Run("audit failure is reported, never hidden", func(t *testing.T) {
		e := newEnv(t)
		e.db.Fail("Record", errBoom)
		_, err := e.resolve().Execute(ctx(), claimsFor(e, t, e.adminA, "admin", "super_admin"))
		assertIs(t, err, domain.ErrAmbiguousRole)
		assertIs(t, err, errBoom)
	})
}

func TestResolvePrincipalFirstLoginAcceptsInvitation(t *testing.T) {
	e := newEnv(t)
	user, invitation, subject := e.addInvited("u-new", e.companyA, "new@a.test", domain.RoleAdmin, true)
	e.clock.Advance(time.Hour)

	got, err := e.resolve().Execute(ctx(), usecase.ResolvePrincipalInput{Claims: usecase.Claims{
		Issuer: issuer, Subject: subject, Email: "new@a.test", EmailVerified: true, ClientRoles: []string{"admin", "users.manage"},
	}})
	assertNoError(t, err)

	if got.UserID != user.ID {
		t.Fatalf("principal user = %s", got.UserID)
	}
	if stored := e.db.Users[user.ID]; stored.Status != domain.UserActive || stored.LastAccessAt == nil {
		t.Fatalf("user after first login = %+v", stored)
	}
	if stored := e.db.Invitations[invitation.ID]; stored.AcceptedAt == nil {
		t.Fatal("invitation must be accepted")
	}
	e.assertAudit(t, domain.AuditInvitationAccepted)
}

func TestResolvePrincipalInvitedNeedsVerifiedEmail(t *testing.T) {
	e := newEnv(t)
	user, invitation, subject := e.addInvited("u-new", e.companyA, "new@a.test", domain.RoleAdmin, true)

	_, err := e.resolve().Execute(ctx(), usecase.ResolvePrincipalInput{Claims: usecase.Claims{
		Issuer: issuer, Subject: subject, EmailVerified: false, ClientRoles: []string{"admin"},
	}})

	assertIs(t, err, usecase.ErrAccessDenied)
	if e.db.Users[user.ID].Status != domain.UserInvited || e.db.Invitations[invitation.ID].AcceptedAt != nil {
		t.Fatal("nothing may change before the e-mail is verified")
	}
}

func TestResolvePrincipalInvitedWithoutOpenInvitationIsDenied(t *testing.T) {
	e := newEnv(t)
	_, invitation, subject := e.addInvited("u-new", e.companyA, "new@a.test", domain.RoleAdmin, true)
	cancelled := invitation
	_ = cancelled.Cancel(e.clock.T)
	e.db.Invitations[invitation.ID] = cancelled

	_, err := e.resolve().Execute(ctx(), usecase.ResolvePrincipalInput{Claims: usecase.Claims{
		Issuer: issuer, Subject: subject, EmailVerified: true, ClientRoles: []string{"admin"},
	}})
	assertIs(t, err, usecase.ErrAccessDenied)
}

func TestResolvePrincipalSyncsRoleProjection(t *testing.T) {
	e := newEnv(t)
	// The role was changed directly in the Keycloak console: the fresh token says
	// planning_operator while the projection still says admin.
	in := claimsFor(e, t, e.adminA, "planning_operator", "planner.use")
	in.Claims.SyncRole = true
	_, err := e.resolve().Execute(ctx(), in)
	assertNoError(t, err)

	if got := e.db.Users[e.adminA.ID].Role; got != domain.RolePlanningOperator {
		t.Fatalf("projection = %s, want planning_operator", got)
	}
	e.assertAudit(t, domain.AuditUserRoleSynced)
}

func TestResolvePrincipalDoesNotRewriteTheProjectionFromAStaleToken(t *testing.T) {
	e := newEnv(t)
	// A Bearer token issued before the API promoted the user still says
	// planning_operator: the principal follows the token, the projection stays.
	user := e.db.Users[e.operatorA.ID]
	user.Role = domain.RoleAdmin
	e.db.Users[user.ID] = user

	got, err := e.resolve().Execute(ctx(), claimsFor(e, t, e.operatorA, "planning_operator", "planner.use"))
	assertNoError(t, err)

	if got.Role != domain.RolePlanningOperator {
		t.Fatalf("principal role = %s", got.Role)
	}
	if stored := e.db.Users[user.ID].Role; stored != domain.RoleAdmin {
		t.Fatalf("a stale token rewrote the projection: %s", stored)
	}
	for _, event := range e.db.Audit {
		if event.Action == domain.AuditUserRoleSynced {
			t.Fatal("no role sync must be audited")
		}
	}
}

func TestResolvePrincipalDropsCrossTenantPermissionsOutsideThePlatformOperator(t *testing.T) {
	e := newEnv(t)
	t.Run("an admin with extra permissions in the token gets no cross-tenant scope", func(t *testing.T) {
		got, err := e.resolve().Execute(ctx(), claimsFor(e, t, e.adminA, "admin", "users.manage", "users.manage_all", "companies.manage"))
		assertNoError(t, err)
		if got.Can(domain.PermUsersManageAll) || got.Can(domain.PermCompaniesManage) || got.ManagesAllCompanies() {
			t.Fatalf("permissions = %v", got.Permissions)
		}
		if !got.Can(domain.PermUsersManage) {
			t.Fatalf("regular permissions must stay: %v", got.Permissions)
		}
	})
	t.Run("a super admin of the platform operator keeps them", func(t *testing.T) {
		got, err := e.resolve().Execute(ctx(), claimsFor(e, t, e.superAdmin, "super_admin", "users.manage", "users.manage_all", "companies.manage"))
		assertNoError(t, err)
		if !got.Can(domain.PermUsersManageAll) || !got.Can(domain.PermCompaniesManage) {
			t.Fatalf("permissions = %v", got.Permissions)
		}
	})
}

func TestResolvePrincipalLastAccessIsRateLimited(t *testing.T) {
	e := newEnv(t)
	in := claimsFor(e, t, e.adminA, "admin")

	_, err := e.resolve().Execute(ctx(), in)
	assertNoError(t, err)
	first := e.db.Users[e.adminA.ID]
	if first.LastAccessAt == nil {
		t.Fatal("first request must stamp the last access")
	}

	e.clock.Advance(time.Minute)
	_, err = e.resolve().Execute(ctx(), in)
	assertNoError(t, err)
	if got := e.db.Users[e.adminA.ID]; got.Version != first.Version {
		t.Fatalf("request inside the interval wrote the user again (version %d -> %d)", first.Version, got.Version)
	}

	e.clock.Advance(10 * time.Minute)
	_, err = e.resolve().Execute(ctx(), in)
	assertNoError(t, err)
	if got := e.db.Users[e.adminA.ID]; got.Version == first.Version {
		t.Fatal("request after the interval must stamp again")
	}
}

func TestResolvePrincipalLostRaceOnLastAccessDoesNotFailLogin(t *testing.T) {
	e := newEnv(t)
	e.db.Fail("UpdateUser", usecase.ErrVersionConflict)
	_, err := e.resolve().Execute(ctx(), claimsFor(e, t, e.adminA, "admin"))
	assertNoError(t, err)
}

func TestResolvePrincipalPassesTheFarmScopeOfTheUser(t *testing.T) {
	e := newEnv(t)
	selected, err := domain.NewSelectedFarms([]domain.FarmID{"farm-a1"})
	assertNoError(t, err)
	user := e.db.Users[e.operatorA.ID]
	user.FarmScope = selected
	e.db.Users[user.ID] = user

	got, err := e.resolve().Execute(ctx(), claimsFor(e, t, e.operatorA, "planning_operator", "planner.use"))
	assertNoError(t, err)
	if !got.FarmScope.Equal(selected) {
		t.Fatalf("farm scope = %+v", got.FarmScope)
	}
}
