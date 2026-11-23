package domain_test

import (
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/domain"
)

var now = time.Date(2026, 11, 22, 12, 0, 0, 0, time.UTC)

func TestReadAccess(t *testing.T) {
	tests := []struct {
		name        string
		claim       []string
		wantRole    domain.Role
		wantPerms   []domain.Permission
		wantErr     error
		wantErrText string
	}{
		{
			name:     "admin with composite permissions",
			claim:    []string{"users.manage", "admin", "dashboard.view", "registry.write"},
			wantRole: domain.RoleAdmin,
			// canonical enum order, regardless of claim order
			wantPerms: []domain.Permission{domain.PermDashboardView, domain.PermUsersManage, domain.PermRegistryWrite},
		},
		{
			name:      "unknown names are ignored",
			claim:     []string{"planning_operator", "planner.use", "offline_access", "realm-admin", "billing.view"},
			wantRole:  domain.RolePlanningOperator,
			wantPerms: []domain.Permission{domain.PermPlannerUse},
		},
		{
			name:      "duplicated role counts once",
			claim:     []string{"admin", "admin", "users.manage"},
			wantRole:  domain.RoleAdmin,
			wantPerms: []domain.Permission{domain.PermUsersManage},
		},
		{name: "no roles", claim: nil, wantErr: domain.ErrNoAccessToProduct},
		{name: "only permissions, no role", claim: []string{"users.manage"}, wantErr: domain.ErrNoAccessToProduct},
		{name: "two roles are ambiguous", claim: []string{"admin", "super_admin", "users.manage"}, wantErr: domain.ErrAmbiguousRole},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := domain.ReadAccess(tt.claim)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("error = %v, want %v", err, tt.wantErr)
			}
			if tt.wantErr != nil {
				return
			}
			if got.Role != tt.wantRole {
				t.Errorf("role = %q, want %q", got.Role, tt.wantRole)
			}
			if !slices.Equal(got.Permissions, tt.wantPerms) {
				t.Errorf("permissions = %v, want %v", got.Permissions, tt.wantPerms)
			}
		})
	}
}

func TestParseEmail(t *testing.T) {
	tests := []struct {
		in      string
		want    domain.Email
		wantErr bool
	}{
		{in: "  Bruno@Fazenda.com.br ", want: "bruno@fazenda.com.br"},
		{in: "a@b.co", want: "a@b.co"},
		{in: "", wantErr: true},
		{in: "sem-arroba", wantErr: true},
		{in: "a@@b.com", wantErr: true},
		{in: "a b@c.com", wantErr: true},
		{in: "@c.com", wantErr: true},
		{in: "a@c", wantErr: true},
		{in: "a@.com", wantErr: true},
		{in: "a@c.", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got, err := domain.ParseEmail(tt.in)
			if (err != nil) != tt.wantErr {
				t.Fatalf("error = %v, wantErr %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("email = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestParseDocument(t *testing.T) {
	tests := []struct {
		in      string
		want    domain.Document
		wantErr bool
	}{
		{in: "11.222.333/0001-81", want: "11222333000181"},
		{in: "11222333000181", want: "11222333000181"},
		{in: "11222333000182", wantErr: true}, // wrong check digit
		{in: "00000000000000", wantErr: true}, // repeated digits
		{in: "123", wantErr: true},
		{in: "", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got, err := domain.ParseDocument(tt.in)
			if (err != nil) != tt.wantErr {
				t.Fatalf("error = %v, wantErr %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("document = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFarmScope(t *testing.T) {
	t.Run("zero value covers all farms", func(t *testing.T) {
		var scope domain.FarmScope
		if scope.Mode() != domain.ScopeAll || !scope.Allows("any") {
			t.Fatalf("zero scope = %v, want all", scope.Mode())
		}
	})
	t.Run("selected limits farms", func(t *testing.T) {
		scope, err := domain.NewSelectedFarms([]domain.FarmID{"f1", "f2"})
		if err != nil {
			t.Fatal(err)
		}
		if !scope.Allows("f1") || scope.Allows("f3") {
			t.Fatal("selected scope must allow only listed farms")
		}
		other, _ := domain.NewSelectedFarms([]domain.FarmID{"f2", "f1"})
		if !scope.Equal(other) || scope.Equal(domain.AllFarms()) {
			t.Fatal("equality must ignore order and compare modes")
		}
	})
	t.Run("selected copies its input", func(t *testing.T) {
		ids := []domain.FarmID{"f1"}
		scope, _ := domain.NewSelectedFarms(ids)
		ids[0] = "tampered"
		if !scope.Allows("f1") {
			t.Fatal("scope must not alias the caller slice")
		}
	})
	invalidCases := []struct {
		name string
		mode string
		ids  []string
	}{
		{"selected without farms", "selected", nil},
		{"selected with duplicate", "selected", []string{"a", "a"}},
		{"selected with empty id", "selected", []string{""}},
		{"all with farms", "all", []string{"a"}},
		{"unknown mode", "some", nil},
	}
	for _, tt := range invalidCases {
		t.Run(tt.name, func(t *testing.T) {
			_, err := domain.ParseFarmScope(tt.mode, tt.ids)
			var verr *domain.ValidationError
			if !errors.As(err, &verr) {
				t.Fatalf("error = %v, want ValidationError", err)
			}
		})
	}
}

func clientCompany() domain.Company {
	return domain.Company{ID: "c1", Name: "Fazenda", Status: domain.CompanyActive, Plan: domain.PlanBasic}
}

func operatorCompany() domain.Company {
	return domain.Company{ID: "op", Name: "Agentes Agro", Status: domain.CompanyActive, IsPlatformOperator: true}
}

func TestCheckAssignment(t *testing.T) {
	selected, _ := domain.NewSelectedFarms([]domain.FarmID{"f1"})
	tests := []struct {
		name      string
		company   domain.Company
		role      domain.Role
		scope     domain.FarmScope
		wantField string
	}{
		{name: "admin in client company", company: clientCompany(), role: domain.RoleAdmin, scope: domain.AllFarms()},
		{name: "super admin in operator company", company: operatorCompany(), role: domain.RoleSuperAdmin, scope: domain.AllFarms()},
		{name: "operator with selected farms", company: clientCompany(), role: domain.RolePlanningOperator, scope: selected},
		{name: "super admin in client company", company: clientCompany(), role: domain.RoleSuperAdmin, scope: domain.AllFarms(), wantField: "role"},
		{name: "admin with selected farms", company: clientCompany(), role: domain.RoleAdmin, scope: selected, wantField: "farm_scope"},
		{name: "unknown role", company: clientCompany(), role: "owner", scope: domain.AllFarms(), wantField: "role"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := domain.CheckAssignment(tt.company, tt.role, tt.scope)
			if tt.wantField == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			var verr *domain.ValidationError
			if !errors.As(err, &verr) || verr.Fields[0].Field != tt.wantField {
				t.Fatalf("error = %v, want validation on %q", err, tt.wantField)
			}
		})
	}
}

func TestNewInvitedUser(t *testing.T) {
	user, err := domain.NewInvitedUser("u1", clientCompany(), "  Maria  ", " Maria@X.com ", domain.RoleAdmin, domain.AllFarms(), now)
	if err != nil {
		t.Fatal(err)
	}
	if user.Name != "Maria" || user.Email != "maria@x.com" || user.Status != domain.UserInvited || user.CompanyID != "c1" {
		t.Fatalf("unexpected user: %+v", user)
	}

	_, err = domain.NewInvitedUser("u2", clientCompany(), " ", "bad", domain.RoleSuperAdmin, domain.AllFarms(), now)
	var verr *domain.ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("error = %v, want ValidationError", err)
	}
	fields := map[string]bool{}
	for _, f := range verr.Fields {
		fields[f.Field] = true
	}
	for _, want := range []string{"name", "email", "role"} {
		if !fields[want] {
			t.Errorf("missing violation for %q in %v", want, verr.Fields)
		}
	}
}

func TestUserStatusTransitions(t *testing.T) {
	tests := []struct {
		name    string
		from    domain.UserStatus
		act     func(*domain.User) error
		want    domain.UserStatus
		wantErr bool
	}{
		{"accept invitation", domain.UserInvited, func(u *domain.User) error { return u.Activate(now) }, domain.UserActive, false},
		{"activate active", domain.UserActive, func(u *domain.User) error { return u.Activate(now) }, domain.UserActive, true},
		{"deactivate active", domain.UserActive, func(u *domain.User) error { return u.Deactivate() }, domain.UserInactive, false},
		{"deactivate invited", domain.UserInvited, func(u *domain.User) error { return u.Deactivate() }, domain.UserInvited, true},
		{"deactivate inactive", domain.UserInactive, func(u *domain.User) error { return u.Deactivate() }, domain.UserInactive, true},
		{"reactivate inactive", domain.UserInactive, func(u *domain.User) error { return u.Reactivate() }, domain.UserActive, false},
		{"reactivate invited", domain.UserInvited, func(u *domain.User) error { return u.Reactivate() }, domain.UserInvited, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			user := domain.User{Status: tt.from}
			err := tt.act(&user)
			if tt.wantErr != errors.Is(err, domain.ErrInvalidTransition) {
				t.Fatalf("error = %v, wantErr %v", err, tt.wantErr)
			}
			if user.Status != tt.want {
				t.Errorf("status = %s, want %s", user.Status, tt.want)
			}
		})
	}
}

func TestUserAssignClearsPrimaryFarm(t *testing.T) {
	farm := domain.FarmID("f1")
	selected, _ := domain.NewSelectedFarms([]domain.FarmID{"f2"})

	t.Run("scope without the primary farm", func(t *testing.T) {
		user := domain.User{CompanyID: "c1", Role: domain.RolePlanningOperator, PrimaryFarmID: &farm}
		if err := user.Assign(clientCompany(), domain.RolePlanningOperator, selected); err != nil {
			t.Fatal(err)
		}
		if user.PrimaryFarmID != nil {
			t.Fatal("primary farm outside the scope must be cleared")
		}
	})
	t.Run("same company, all farms keeps it", func(t *testing.T) {
		user := domain.User{CompanyID: "c1", PrimaryFarmID: &farm}
		if err := user.Assign(clientCompany(), domain.RoleAdmin, domain.AllFarms()); err != nil {
			t.Fatal(err)
		}
		if user.PrimaryFarmID == nil {
			t.Fatal("primary farm must be kept")
		}
	})
	t.Run("invalid assignment changes nothing", func(t *testing.T) {
		user := domain.User{CompanyID: "c1", Role: domain.RoleAdmin}
		if err := user.Assign(clientCompany(), domain.RoleSuperAdmin, domain.AllFarms()); err == nil {
			t.Fatal("want error")
		}
		if user.Role != domain.RoleAdmin {
			t.Fatal("failed assignment must not mutate the user")
		}
	})
}

func TestNewCompany(t *testing.T) {
	valid := domain.NewCompanyInput{Name: " Santa Rita ", Document: "11.222.333/0001-81", City: "Sorriso", State: "mt", Plan: domain.PlanPro}

	t.Run("regular plan is active without trial", func(t *testing.T) {
		company, err := domain.NewCompany("c1", valid, now, 14*24*time.Hour)
		if err != nil {
			t.Fatal(err)
		}
		if company.Status != domain.CompanyActive || company.TrialEndsAt != nil || company.State != "MT" || company.Name != "Santa Rita" || company.IsPlatformOperator {
			t.Fatalf("unexpected company: %+v", company)
		}
	})
	t.Run("trial plan opens the evaluation window", func(t *testing.T) {
		in := valid
		in.Plan = domain.PlanTrial
		company, err := domain.NewCompany("c1", in, now, 14*24*time.Hour)
		if err != nil {
			t.Fatal(err)
		}
		want := now.Add(14 * 24 * time.Hour)
		if company.Status != domain.CompanyTrial || company.TrialEndsAt == nil || !company.TrialEndsAt.Equal(want) {
			t.Fatalf("unexpected trial: %+v", company)
		}
	})
	t.Run("collects every violation", func(t *testing.T) {
		_, err := domain.NewCompany("c1", domain.NewCompanyInput{Name: "x", Document: "1", City: "", State: "M", Plan: "gold"}, now, time.Hour)
		var verr *domain.ValidationError
		if !errors.As(err, &verr) {
			t.Fatalf("error = %v, want ValidationError", err)
		}
	})
}

func TestCompanyPlanAndStatusChanges(t *testing.T) {
	d := 14 * 24 * time.Hour
	t.Run("plan to trial starts evaluation", func(t *testing.T) {
		c := clientCompany()
		if err := c.ChangePlan(domain.PlanTrial, now, d); err != nil {
			t.Fatal(err)
		}
		if c.Status != domain.CompanyTrial || c.TrialEndsAt == nil {
			t.Fatalf("company = %+v", c)
		}
	})
	t.Run("leaving trial activates", func(t *testing.T) {
		c := clientCompany()
		_ = c.ChangePlan(domain.PlanTrial, now, d)
		if err := c.ChangePlan(domain.PlanPro, now, d); err != nil {
			t.Fatal(err)
		}
		if c.Status != domain.CompanyActive || c.TrialEndsAt != nil {
			t.Fatalf("company = %+v", c)
		}
	})
	t.Run("inactive company stays inactive on trial plan", func(t *testing.T) {
		c := clientCompany()
		c.Status = domain.CompanyInactive
		_ = c.ChangePlan(domain.PlanTrial, now, d)
		if c.Status != domain.CompanyInactive {
			t.Fatalf("status = %s", c.Status)
		}
	})
	t.Run("trial status needs trial plan", func(t *testing.T) {
		c := clientCompany()
		if err := c.ChangeStatus(domain.CompanyTrial, now, d); err == nil {
			t.Fatal("want validation error")
		}
	})
	t.Run("unknown status", func(t *testing.T) {
		c := clientCompany()
		if err := c.ChangeStatus("paused", now, d); err == nil {
			t.Fatal("want validation error")
		}
	})
}

func TestCompanyAllowsAccess(t *testing.T) {
	deleted := now
	tests := []struct {
		name    string
		company domain.Company
		want    bool
	}{
		{"active", domain.Company{Status: domain.CompanyActive}, true},
		{"trial", domain.Company{Status: domain.CompanyTrial}, true},
		{"inactive", domain.Company{Status: domain.CompanyInactive}, false},
		{"deleted", domain.Company{Status: domain.CompanyActive, DeletedAt: &deleted}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.company.AllowsAccess(); got != tt.want {
				t.Errorf("AllowsAccess = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestInvitationLifecycle(t *testing.T) {
	user := domain.User{ID: "u1", CompanyID: "c1"}
	expires := now.Add(7 * 24 * time.Hour)

	t.Run("resend renews and counts", func(t *testing.T) {
		inv := domain.NewInvitation("i1", user, "admin", true, now, expires)
		later := now.Add(time.Hour)
		if err := inv.Resend(later, later.Add(7*24*time.Hour)); err != nil {
			t.Fatal(err)
		}
		if inv.SendCount != 2 || !inv.LastSentAt.Equal(later) {
			t.Fatalf("invitation = %+v", inv)
		}
	})
	t.Run("closed invitations reject every transition", func(t *testing.T) {
		accepted := domain.NewInvitation("i1", user, "admin", false, now, expires)
		_ = accepted.Accept(now)
		cancelled := domain.NewInvitation("i2", user, "admin", false, now, expires)
		_ = cancelled.Cancel(now)
		for name, inv := range map[string]domain.Invitation{"accepted": accepted, "cancelled": cancelled} {
			for action, run := range map[string]func(*domain.Invitation) error{
				"resend": func(i *domain.Invitation) error { return i.Resend(now, expires) },
				"accept": func(i *domain.Invitation) error { return i.Accept(now) },
				"cancel": func(i *domain.Invitation) error { return i.Cancel(now) },
			} {
				copyOf := inv
				if err := run(&copyOf); !errors.Is(err, domain.ErrInvitationClosed) {
					t.Errorf("%s then %s: error = %v, want ErrInvitationClosed", name, action, err)
				}
			}
		}
	})
}

func TestPrincipalCan(t *testing.T) {
	p := domain.Principal{Permissions: []domain.Permission{domain.PermUsersManage}}
	if !p.Can(domain.PermUsersManage) || p.Can(domain.PermUsersManageAll) || p.ManagesAllCompanies() {
		t.Fatal("principal permissions mismatch")
	}
}
