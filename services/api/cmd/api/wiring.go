package main

import (
	"log/slog"

	httpadapter "github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/adapter/http"
	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/domain"
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

// registryStorage is what the farm and owner use cases consume (Postgres in
// production, an in-memory registry in tests).
type registryStorage interface {
	usecase.FarmReader
	usecase.OwnerReader
	usecase.CropReader
	usecase.FieldReader
	usecase.PerimeterMeasurer
	usecase.MunicipalityLocator
	usecase.OwnerDocumentReader
	usecase.RegistryTxRunner
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
	Registry registryStorage
	// Terrain is the relief persistence; TerrainRules and TerrainWakeup tune and trigger it.
	Terrain       terrainStorage
	TerrainRules  domain.TerrainRules
	TerrainWakeup usecase.TerrainWakeup
	// Conditions keeps rain gauges, readings, inspections, soil profiles and the
	// thresholds; Weather reads the weather model (cached); Trafficability tunes them.
	Conditions     conditionsStorage
	Weather        usecase.WeatherProvider
	Trafficability usecase.TrafficabilitySettings
	// PerimeterParser reads uploaded perimeter files.
	PerimeterParser usecase.PerimeterParser
	// Documents keeps the files of owner documents (S3, or disabled without a bucket).
	Documents usecase.DocumentStorage
	Identity  identityProvider
	Clock     usecase.Clock
	IDs       usecase.IDGenerator
	Settings  usecase.Settings
}

// trafficabilityDeps are the ports of the trafficability core, shared by the
// GET route and the daily soil history executor, so both compute the same verdict.
func (d dependencies) trafficabilityDeps() usecase.GetFieldTrafficabilityDeps {
	return usecase.GetFieldTrafficabilityDeps{
		Fields: d.Conditions, Gauges: d.Conditions, Thresholds: d.Conditions, Terrain: d.Terrain,
		Weather: d.Weather, Clock: d.Clock, TerrainRules: d.TerrainRules, Settings: d.Trafficability,
	}
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

	farmWrite := usecase.FarmWriteDeps{
		Farms: d.Registry, Measurer: d.Registry, Tx: d.Registry, Clock: d.Clock, IDs: d.IDs, Areas: d.Settings.Areas,
	}
	fieldWrite := usecase.FieldWriteDeps{
		Farms: d.Registry, Fields: d.Registry, Crops: d.Registry, Measurer: d.Registry, Tx: d.Registry,
		Clock: d.Clock, IDs: d.IDs, Areas: d.Settings.Areas,
	}
	farms := httpadapter.NewFarmsHandler(d.Logger, httpadapter.FarmsDeps{
		List:            usecase.NewListFarms(d.Registry, d.Settings),
		Get:             usecase.NewGetFarm(d.Registry),
		Create:          usecase.NewCreateFarm(farmWrite),
		Update:          usecase.NewUpdateFarm(farmWrite),
		ImportPerimeter: usecase.NewImportPerimeter(d.PerimeterParser, d.Registry, d.Registry),
		ListFields:      usecase.NewListFields(d.Registry, d.Registry),
		CreateField:     usecase.NewCreateField(fieldWrite),
		UpdateField:     usecase.NewUpdateField(fieldWrite),
		ListCrops:       usecase.NewListCrops(d.Registry),
	})

	owners := httpadapter.NewOwnersHandler(d.Logger, httpadapter.OwnersDeps{
		List:         usecase.NewListOwners(d.Registry, d.Settings),
		Get:          usecase.NewGetOwner(d.Registry),
		Create:       usecase.NewCreateOwner(d.Registry, d.Registry, d.Clock, d.IDs),
		Update:       usecase.NewUpdateOwner(d.Registry, d.Registry, d.Clock),
		ReplaceLinks: usecase.NewReplaceOwnerLinks(d.Registry, d.Registry, d.Clock),
	})

	documentDeps := usecase.OwnerDocumentDeps{
		Owners: d.Registry, Documents: d.Registry, Storage: d.Documents, Tx: d.Registry,
		Clock: d.Clock, IDs: d.IDs, Settings: d.Settings,
	}
	documents := httpadapter.NewOwnerDocumentsHandler(d.Logger, httpadapter.OwnerDocumentsDeps{
		Upload:   usecase.NewUploadOwnerDocument(documentDeps),
		Download: usecase.NewDownloadOwnerDocument(documentDeps),
		Remove:   usecase.NewRemoveOwnerDocument(documentDeps),
	}, d.Settings.MaxDocumentBytes)

	terrainDeps := usecase.AdjustFieldTerrainDeps{
		Terrain: d.Terrain, Tx: d.Terrain, Clock: d.Clock, Rules: d.TerrainRules,
	}
	terrain := httpadapter.NewTerrainHandler(d.Logger, httpadapter.TerrainDeps{
		GetFarm: usecase.NewGetFarmTerrain(d.Registry, d.Terrain, d.TerrainRules),
		Calculate: usecase.NewCalculateFarmTerrain(usecase.CalculateFarmTerrainDeps{
			Terrain: d.Terrain, Tx: d.Terrain, Wakeup: d.TerrainWakeup, Clock: d.Clock, IDs: d.IDs, Rules: d.TerrainRules,
		}),
		GetField:         usecase.NewGetFieldTerrain(d.Terrain, d.TerrainRules),
		Adjust:           usecase.NewAdjustFieldTerrain(terrainDeps),
		RemoveAdjustment: usecase.NewRemoveFieldTerrainAdjustment(terrainDeps),
	})

	conditionsDeps := usecase.FieldConditionsDeps{
		Farms: d.Registry, Gauges: d.Conditions, Fields: d.Conditions, Tx: d.Conditions,
		Clock: d.Clock, IDs: d.IDs, Settings: d.Trafficability,
	}
	trafficability := httpadapter.NewTrafficabilityHandler(d.Logger, httpadapter.TrafficabilityDeps{
		ListGauges:        usecase.NewListRainGauges(d.Registry, d.Conditions),
		CreateGauge:       usecase.NewCreateRainGauge(conditionsDeps),
		UpdateGauge:       usecase.NewUpdateRainGauge(conditionsDeps),
		ListReadings:      usecase.NewListRainReadings(conditionsDeps),
		RecordReading:     usecase.NewRecordRainReading(conditionsDeps),
		RecordInspection:  usecase.NewRecordSoilInspection(conditionsDeps),
		GetSoilProfile:    usecase.NewGetSoilProfile(d.Conditions),
		UpdateSoilProfile: usecase.NewUpdateSoilProfile(conditionsDeps),
		GetTrafficability: usecase.NewGetFieldTrafficability(d.trafficabilityDeps()),
	})

	return []httpadapter.RouteGroup{users, invitations, companies, me, farms, owners, documents, terrain, trafficability}
}
