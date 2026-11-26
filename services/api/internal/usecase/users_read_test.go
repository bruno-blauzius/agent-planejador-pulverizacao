package usecase_test

import (
	"testing"

	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/domain"
	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/usecase"
)

func userIDs(page usecase.UserPage) []domain.UserID {
	ids := make([]domain.UserID, len(page.Items))
	for i, item := range page.Items {
		ids[i] = item.User.ID
	}
	return ids
}

func TestListUsersTenantIsolation(t *testing.T) {
	e := newEnv(t)
	e.addUser("u-op-b", e.companyB, "Otávio B", "otavio@b.test", domain.RolePlanningOperator, domain.AllFarms())
	list := usecase.NewListUsers(e.db, e.settings)

	t.Run("admin sees only the own company", func(t *testing.T) {
		page, err := list.Execute(ctx(), usecase.ListUsersInput{Actor: e.asAdminA()})
		assertNoError(t, err)
		if page.Total != 2 {
			t.Fatalf("users = %v", userIDs(page))
		}
		for _, item := range page.Items {
			if item.User.CompanyID != "co-a" {
				t.Fatalf("leaked %s from %s", item.User.ID, item.User.CompanyID)
			}
		}
	})
	t.Run("an admin cannot widen the scope with company_id", func(t *testing.T) {
		page, err := list.Execute(ctx(), usecase.ListUsersInput{Actor: e.asAdminA(), CompanyID: "co-b"})
		assertNoError(t, err)
		for _, item := range page.Items {
			if item.User.CompanyID != "co-a" {
				t.Fatalf("company_id from the client widened the scope: %s", item.User.ID)
			}
		}
	})
	t.Run("super admin sees everything and can filter by company", func(t *testing.T) {
		page, err := list.Execute(ctx(), usecase.ListUsersInput{Actor: e.asSuper()})
		assertNoError(t, err)
		if page.Total != 5 {
			t.Fatalf("users = %v", userIDs(page))
		}
		page, err = list.Execute(ctx(), usecase.ListUsersInput{Actor: e.asSuper(), CompanyID: "co-b"})
		assertNoError(t, err)
		if page.Total != 2 {
			t.Fatalf("users of B = %v", userIDs(page))
		}
	})
	t.Run("operator is forbidden", func(t *testing.T) {
		_, err := list.Execute(ctx(), usecase.ListUsersInput{Actor: e.asOperatorA()})
		assertIs(t, err, usecase.ErrForbidden)
	})
}

func TestListUsersFiltersAndPagination(t *testing.T) {
	e := newEnv(t)
	e.addInvited("u-new", e.companyA, "novo@a.test", domain.RoleAdmin, true)
	list := usecase.NewListUsers(e.db, e.settings)

	tests := []struct {
		name  string
		input usecase.ListUsersInput
		want  int
	}{
		{"by role", usecase.ListUsersInput{Role: domain.RoleAdmin}, 2},
		{"by status", usecase.ListUsersInput{Status: domain.UserInvited}, 1},
		{"search in name or e-mail", usecase.ListUsersInput{Search: "olga"}, 1},
		{"search matches the company name", usecase.ListUsersInput{Search: "fazenda a"}, 3},
		{"no match", usecase.ListUsersInput{Search: "zzz"}, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.input.Actor = e.asAdminA()
			page, err := list.Execute(ctx(), tt.input)
			assertNoError(t, err)
			if page.Total != tt.want {
				t.Fatalf("total = %d, want %d", page.Total, tt.want)
			}
		})
	}

	t.Run("pagination", func(t *testing.T) {
		page, err := list.Execute(ctx(), usecase.ListUsersInput{Actor: e.asAdminA(), Page: 2, PageSize: 2})
		assertNoError(t, err)
		if page.Total != 3 || len(page.Items) != 1 || page.Page != 2 || page.PageSize != 2 {
			t.Fatalf("page = %+v", page)
		}
	})
	t.Run("invalid input is a validation error", func(t *testing.T) {
		_, err := list.Execute(ctx(), usecase.ListUsersInput{Actor: e.asAdminA(), Role: "owner", Status: "gone", Page: -1, PageSize: 101})
		for _, field := range []string{"role", "status", "page", "page_size"} {
			assertValidationOn(t, err, field)
		}
	})
	t.Run("an invited row carries its invitation", func(t *testing.T) {
		page, err := list.Execute(ctx(), usecase.ListUsersInput{Actor: e.asAdminA(), Status: domain.UserInvited})
		assertNoError(t, err)
		if page.Items[0].Invitation == nil {
			t.Fatal("invitation missing")
		}
	})
}

func TestUsersSummary(t *testing.T) {
	e := newEnv(t)
	e.addInvited("u-new", e.companyA, "novo@a.test", domain.RoleAdmin, true)
	summary := usecase.NewGetUsersSummary(e.db)

	got, err := summary.Execute(ctx(), usecase.UsersSummaryInput{Actor: e.asAdminA(), CompanyID: "co-b"})
	assertNoError(t, err)
	if got.TotalUsers != 3 || got.CompanyCount != 1 || got.PendingInvitations != 1 || got.ByRole[domain.RoleAdmin] != 2 || got.ByRole[domain.RoleSuperAdmin] != 0 {
		t.Fatalf("admin summary = %+v (company_id from an admin must be ignored)", got)
	}
	if _, ok := got.ByRole[domain.RoleSuperAdmin]; !ok {
		t.Fatal("every role must be present in the summary")
	}

	got, err = summary.Execute(ctx(), usecase.UsersSummaryInput{Actor: e.asSuper()})
	assertNoError(t, err)
	if got.TotalUsers != 5 || got.CompanyCount != 3 {
		t.Fatalf("super summary = %+v", got)
	}

	_, err = summary.Execute(ctx(), usecase.UsersSummaryInput{Actor: e.asOperatorA()})
	assertIs(t, err, usecase.ErrForbidden)
}

func TestGetUser(t *testing.T) {
	e := newEnv(t)
	get := usecase.NewGetUser(e.db)

	listing, err := get.Execute(ctx(), usecase.GetUserInput{Actor: e.asAdminA(), UserID: e.operatorA.ID})
	assertNoError(t, err)
	if listing.CompanyName != "Fazenda A" {
		t.Fatalf("listing = %+v", listing)
	}
	_, err = get.Execute(ctx(), usecase.GetUserInput{Actor: e.asAdminA(), UserID: e.adminB.ID})
	assertIs(t, err, usecase.ErrNotFound)
	_, err = get.Execute(ctx(), usecase.GetUserInput{Actor: e.asSuper(), UserID: e.adminB.ID})
	assertNoError(t, err)
	_, err = get.Execute(ctx(), usecase.GetUserInput{Actor: e.asOperatorA(), UserID: e.adminA.ID})
	assertIs(t, err, usecase.ErrForbidden)
}

func TestGetMe(t *testing.T) {
	e := newEnv(t)
	farm := domain.FarmID("farm-a1")
	operator := e.db.Users[e.operatorA.ID]
	operator.PrimaryFarmID = &farm
	e.db.Users[operator.ID] = operator
	getMe := usecase.NewGetMe(e.db, e.db, e.db)

	me, err := getMe.Execute(ctx(), usecase.GetMeInput{Actor: e.asOperatorA()})
	assertNoError(t, err)
	if me.ID != e.operatorA.ID || me.Company.Name != "Fazenda A" || me.PrimaryFarm == nil || me.PrimaryFarm.Name != "Santa Rita" {
		t.Fatalf("me = %+v", me)
	}
	if me.Role != domain.RolePlanningOperator || len(me.Permissions) != len(operatorPermissions) {
		t.Fatalf("role/permissions must come from the principal (token): %+v", me)
	}

	t.Run("primary farm that vanished is simply absent", func(t *testing.T) {
		delete(e.db.Farms["co-a"], "farm-a1")
		me, err := getMe.Execute(ctx(), usecase.GetMeInput{Actor: e.asOperatorA()})
		assertNoError(t, err)
		if me.PrimaryFarm != nil {
			t.Fatal("primary farm must be nil")
		}
	})
	t.Run("a principal never reads a user of another company", func(t *testing.T) {
		stranger := e.asOperatorA()
		stranger.CompanyID = "co-b" // forged company
		_, err := getMe.Execute(ctx(), usecase.GetMeInput{Actor: stranger})
		assertIs(t, err, usecase.ErrNotFound)
	})
}

func TestProfile(t *testing.T) {
	e := newEnv(t)
	get := usecase.NewGetProfile(e.db, e.db)
	update := usecase.NewUpdateProfile(e.db, e.db, e.db)

	t.Run("any role reads the profile", func(t *testing.T) {
		profile, err := get.Execute(ctx(), usecase.GetProfileInput{Actor: e.asOperatorA()})
		assertNoError(t, err)
		if profile.Email != "olga@a.test" || profile.Company.ID != "co-a" {
			t.Fatalf("profile = %+v", profile)
		}
	})
	t.Run("updates only the name", func(t *testing.T) {
		profile, err := update.Execute(ctx(), usecase.UpdateProfileInput{Actor: e.asOperatorA(), Name: "  Olga Maria  "})
		assertNoError(t, err)
		stored := e.db.Users[e.operatorA.ID]
		if profile.Name != "Olga Maria" || stored.Name != "Olga Maria" || stored.Email != "olga@a.test" || stored.Role != domain.RolePlanningOperator {
			t.Fatalf("profile = %+v, stored = %+v", profile, stored)
		}
	})
	t.Run("empty name", func(t *testing.T) {
		_, err := update.Execute(ctx(), usecase.UpdateProfileInput{Actor: e.asOperatorA(), Name: " "})
		assertValidationOn(t, err, "name")
	})
	t.Run("database failure", func(t *testing.T) {
		e.db.Fail("UpdateUser", errBoom)
		defer e.db.Heal("UpdateUser")
		_, err := update.Execute(ctx(), usecase.UpdateProfileInput{Actor: e.asOperatorA(), Name: "Outra"})
		assertIs(t, err, errBoom)
	})
}

func TestSettingsValidate(t *testing.T) {
	areas := domain.AreaRules{MinFarmAreaHa: 0.1, MinFieldAreaHa: 0.01, ToleranceM2: 10}
	good := usecase.Settings{InvitationTTL: 1, TrialDuration: 1, DefaultPageSize: 10, MaxPageSize: 100, Areas: areas, MaxDocumentBytes: 1}
	if err := good.Validate(); err != nil {
		t.Fatal(err)
	}
	for name, bad := range map[string]usecase.Settings{
		"no ttl":         {TrialDuration: 1, DefaultPageSize: 10, MaxPageSize: 100},
		"no trial":       {InvitationTTL: 1, DefaultPageSize: 10, MaxPageSize: 100},
		"default > max":  {InvitationTTL: 1, TrialDuration: 1, DefaultPageSize: 200, MaxPageSize: 100},
		"no page size":   {InvitationTTL: 1, TrialDuration: 1, MaxPageSize: 100},
		"negative stamp": {InvitationTTL: 1, TrialDuration: 1, LastAccessInterval: -1, DefaultPageSize: 10, MaxPageSize: 100},
		"no farm area":   {InvitationTTL: 1, TrialDuration: 1, DefaultPageSize: 10, MaxPageSize: 100, Areas: domain.AreaRules{MinFieldAreaHa: 1}},
		"no field area":  {InvitationTTL: 1, TrialDuration: 1, DefaultPageSize: 10, MaxPageSize: 100, Areas: domain.AreaRules{MinFarmAreaHa: 1}},
		"negative tolerance": {InvitationTTL: 1, TrialDuration: 1, DefaultPageSize: 10, MaxPageSize: 100,
			Areas: domain.AreaRules{MinFarmAreaHa: 1, MinFieldAreaHa: 1, ToleranceM2: -1}},
	} {
		if bad.Validate() == nil {
			t.Errorf("%s: want error", name)
		}
	}
}
