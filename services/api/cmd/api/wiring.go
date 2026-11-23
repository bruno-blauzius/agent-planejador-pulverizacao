package main

import (
	"log/slog"

	httpadapter "github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/adapter/http"
	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/usecase"
)

// storage is every persistence port the use cases consume. The Postgres
// database implements it; tests plug an in-memory one.
type storage interface {
	usecase.UserFinder
	usecase.UserLister
	usecase.CompanyMembers
	usecase.RoleCounter
	usecase.IdentityStore
	usecase.InvitationFinder
	usecase.CompanyFinder
	usecase.CompanyLister
	usecase.FarmDirectory
	usecase.TxRunner
}

// identityProvider is the whole Keycloak surface the use cases consume.
type identityProvider interface {
	usecase.AccountDirectory
	usecase.ProductAccess
	usecase.InviteSender
}

// dependencies are the already built adapters and values the use cases need.
type dependencies struct {
	Logger   *slog.Logger
	Store    storage
	Identity identityProvider
	Clock    usecase.Clock
	IDs      usecase.IDGenerator
	Settings usecase.Settings
}

// newRouteGroups wires the use cases into the HTTP handlers of the business
// endpoints. It is the only place where use cases are instantiated.
func newRouteGroups(d dependencies) []httpadapter.RouteGroup {
	statusDeps := usecase.UserStatusDeps{
		Users: d.Store, Companies: d.Store, Identities: d.Store, Access: d.Identity,
		Lister: d.Store, Roles: d.Store, Tx: d.Store, Clock: d.Clock,
	}

	users := httpadapter.NewUsersHandler(d.Logger, httpadapter.UsersDeps{
		List:    usecase.NewListUsers(d.Store, d.Settings),
		Summary: usecase.NewGetUsersSummary(d.Store),
		Get:     usecase.NewGetUser(d.Store),
		Update: usecase.NewUpdateUser(usecase.UpdateUserDeps{
			Users: d.Store, Companies: d.Store, Farms: d.Store, Identities: d.Store, Access: d.Identity,
			Lister: d.Store, Roles: d.Store, Tx: d.Store, Clock: d.Clock,
		}),
		Deactivate: usecase.NewDeactivateUser(statusDeps),
		Reactivate: usecase.NewReactivateUser(statusDeps),
		Remove:     usecase.NewDeleteUser(statusDeps),
	})

	invitations := httpadapter.NewInvitationsHandler(d.Logger, httpadapter.InvitationsDeps{
		Invite: usecase.NewInviteUser(usecase.InviteUserDeps{
			Users: d.Store, Companies: d.Store, Farms: d.Store, Accounts: d.Identity, Access: d.Identity,
			Mailer: d.Identity, Tx: d.Store, Clock: d.Clock, IDs: d.IDs, Settings: d.Settings,
		}),
		Resend: usecase.NewResendInvitation(usecase.ResendInvitationDeps{
			Invitations: d.Store, Users: d.Store, Companies: d.Store, Identities: d.Store, Accounts: d.Identity,
			Access: d.Identity, Mailer: d.Identity, Tx: d.Store, Clock: d.Clock, Settings: d.Settings,
		}),
		Cancel: usecase.NewCancelInvitation(usecase.CancelInvitationDeps{
			Invitations: d.Store, Users: d.Store, Identities: d.Store, Accounts: d.Identity,
			Access: d.Identity, Tx: d.Store, Clock: d.Clock,
		}),
	})

	companies := httpadapter.NewCompaniesHandler(d.Logger, httpadapter.CompaniesDeps{
		List:    usecase.NewListCompanies(d.Store, d.Settings),
		Summary: usecase.NewGetCompaniesSummary(d.Store),
		Get:     usecase.NewGetCompany(d.Store),
		Create:  usecase.NewCreateCompany(d.Store, d.Store, d.Clock, d.IDs, d.Settings),
		Update: usecase.NewUpdateCompany(usecase.UpdateCompanyDeps{
			Finder: d.Store, Lister: d.Store, Members: d.Store, Access: d.Identity,
			Tx: d.Store, Clock: d.Clock, Settings: d.Settings,
		}),
		Remove: usecase.NewDeleteCompany(d.Store, d.Store, d.Store, d.Clock),
	})

	me := httpadapter.NewMeHandler(d.Logger, httpadapter.MeDeps{
		Me:            usecase.NewGetMe(d.Store, d.Store, d.Store),
		Profile:       usecase.NewGetProfile(d.Store, d.Store),
		UpdateProfile: usecase.NewUpdateProfile(d.Store, d.Store, d.Store),
	})

	return []httpadapter.RouteGroup{users, invitations, companies, me}
}
