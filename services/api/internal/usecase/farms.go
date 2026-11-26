package usecase

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/domain"
)

// Reading the registry needs no capability; writing needs registry.write
// (ADR 0013). Everything happens inside the caller's own company, and a
// planning operator only sees the farms of its scope: a farm outside it is
// ErrNotFound, never ErrForbidden.

// ListFarmsInput is the request of GET /farms.
type ListFarmsInput struct {
	Actor          domain.Principal
	Search         string
	Status         domain.FarmStatus
	ProductionType domain.ProductionType
	Page           int
	PageSize       int
}

// ListFarms lists the farms the caller sees.
type ListFarms struct {
	farms    FarmReader
	settings Settings
}

// NewListFarms builds the use case.
func NewListFarms(farms FarmReader, settings Settings) *ListFarms {
	return &ListFarms{farms: farms, settings: settings}
}

// Execute returns one page of farms.
func (uc *ListFarms) Execute(ctx context.Context, in ListFarmsInput) (FarmPage, error) {
	var problems domain.Violations
	if in.Status != "" && !in.Status.Valid() {
		problems.Add("status", "status desconhecido")
	}
	if in.ProductionType != "" && !in.ProductionType.Valid() {
		problems.Add("production_type", "tipo de produção desconhecido")
	}
	page, err := uc.settings.pageRequest(in.Page, in.PageSize)
	if mergeErr := problems.Merge(err); mergeErr != nil {
		return FarmPage{}, mergeErr
	}
	if err := problems.Err(); err != nil {
		return FarmPage{}, err
	}
	filter := FarmFilter{Search: in.Search, Status: in.Status, ProductionType: in.ProductionType, Visible: in.Actor.FarmScope}
	result, err := uc.farms.ListFarms(ctx, in.Actor.CompanyID, filter, page)
	if err != nil {
		return FarmPage{}, fmt.Errorf("list farms: %w", err)
	}
	return result, nil
}

// GetFarmInput is the request of GET /farms/{farmId}.
type GetFarmInput struct {
	Actor  domain.Principal
	FarmID domain.FarmID
}

// GetFarm returns one farm the caller sees.
type GetFarm struct {
	farms FarmReader
}

// NewGetFarm builds the use case.
func NewGetFarm(farms FarmReader) *GetFarm { return &GetFarm{farms: farms} }

// Execute returns the farm or ErrNotFound.
func (uc *GetFarm) Execute(ctx context.Context, in GetFarmInput) (FarmListing, error) {
	return findVisibleFarm(ctx, uc.farms, in.Actor, in.FarmID)
}

func findVisibleFarm(ctx context.Context, farms FarmReader, actor domain.Principal, id domain.FarmID) (FarmListing, error) {
	if !actor.FarmScope.Allows(id) {
		return FarmListing{}, fmt.Errorf("find farm %s: %w", id, ErrNotFound)
	}
	listing, err := farms.FindFarm(ctx, actor.CompanyID, id)
	if err != nil {
		return FarmListing{}, fmt.Errorf("find farm: %w", err)
	}
	return listing, nil
}

// CreateFarmInput is the request of POST /farms.
type CreateFarmInput struct {
	Actor   domain.Principal
	Details domain.FarmDetails
}

// FarmWriteDeps are the dependencies of CreateFarm and UpdateFarm.
type FarmWriteDeps struct {
	Farms    FarmReader
	Measurer PerimeterMeasurer
	Tx       RegistryTxRunner
	Clock    Clock
	IDs      IDGenerator
	Areas    domain.AreaRules
}

// CreateFarm registers a farm with its owners (wizard of 4 steps).
type CreateFarm struct {
	deps FarmWriteDeps
}

// NewCreateFarm builds the use case.
func NewCreateFarm(deps FarmWriteDeps) *CreateFarm { return &CreateFarm{deps: deps} }

// Execute validates and stores the farm and its owner links in one transaction.
func (uc *CreateFarm) Execute(ctx context.Context, in CreateFarmInput) (FarmListing, error) {
	if err := requireCan(in.Actor, domain.PermRegistryWrite); err != nil {
		return FarmListing{}, err
	}
	now := uc.deps.Clock.Now()
	farm, err := domain.NewFarm(domain.FarmID(uc.deps.IDs.NewID()), in.Actor.CompanyID, in.Details, now)
	if err != nil {
		return FarmListing{}, err
	}
	err = uc.deps.Tx.InRegistryTx(ctx, func(ctx context.Context, tx RegistryStores) error {
		if err := checkFarmOwners(ctx, tx.Owners, farm); err != nil {
			return err
		}
		if err := checkFarmArea(ctx, uc.deps, nil, farm); err != nil {
			return err
		}
		if err := tx.Farms.InsertFarm(ctx, farm); err != nil {
			return fmt.Errorf("insert farm: %w", err)
		}
		return recordFarmEvent(ctx, tx.Audit, in.Actor, now, domain.AuditFarmCreated, farm)
	})
	if err != nil {
		return FarmListing{}, err
	}
	return findVisibleFarm(ctx, uc.deps.Farms, in.Actor, farm.ID)
}

// UpdateFarmInput is the request of PUT /farms/{farmId}.
type UpdateFarmInput struct {
	Actor   domain.Principal
	FarmID  domain.FarmID
	Details domain.FarmDetails
}

// UpdateFarm replaces the editable data of a farm, with the rules of the registration.
type UpdateFarm struct {
	deps FarmWriteDeps
}

// NewUpdateFarm builds the use case.
func NewUpdateFarm(deps FarmWriteDeps) *UpdateFarm { return &UpdateFarm{deps: deps} }

// Execute revises the farm on a locked row, so concurrent link changes of the
// same farm cannot push its shares above 100%.
func (uc *UpdateFarm) Execute(ctx context.Context, in UpdateFarmInput) (FarmListing, error) {
	if err := requireCan(in.Actor, domain.PermRegistryWrite); err != nil {
		return FarmListing{}, err
	}
	if !in.Actor.FarmScope.Allows(in.FarmID) {
		return FarmListing{}, fmt.Errorf("update farm %s: %w", in.FarmID, ErrNotFound)
	}
	now := uc.deps.Clock.Now()
	err := uc.deps.Tx.InRegistryTx(ctx, func(ctx context.Context, tx RegistryStores) error {
		farm, err := tx.Farms.LockFarm(ctx, in.Actor.CompanyID, in.FarmID)
		if err != nil {
			return fmt.Errorf("lock farm: %w", err)
		}
		if err := farm.Revise(in.Details, now); err != nil {
			return err
		}
		if err := checkFarmOwners(ctx, tx.Owners, farm); err != nil {
			return err
		}
		if err := checkFarmArea(ctx, uc.deps, tx.Farms, farm); err != nil {
			return err
		}
		if err := tx.Farms.UpdateFarm(ctx, farm); err != nil {
			return fmt.Errorf("update farm: %w", err)
		}
		return recordFarmEvent(ctx, tx.Audit, in.Actor, now, domain.AuditFarmUpdated, farm)
	})
	if err != nil {
		return FarmListing{}, err
	}
	return findVisibleFarm(ctx, uc.deps.Farms, in.Actor, in.FarmID)
}

// checkFarmArea enforces the area rules on the perimeter of the farm: it has
// its own size in hectares and, on an update (store not nil), still holds the
// fields already registered.
func checkFarmArea(ctx context.Context, deps FarmWriteDeps, store FarmStore, farm domain.Farm) error {
	var areaHa float64
	if farm.Perimeter != nil {
		measure, err := deps.Measurer.MeasurePerimeter(ctx, *farm.Perimeter)
		if err != nil {
			return fmt.Errorf("measure perimeter: %w", err)
		}
		if !measure.Valid {
			return domain.Invalid("location.perimeter", "o perímetro é inválido (polígono com autointerseção)")
		}
		areaHa = measure.AreaHa
	}
	var coverage domain.FarmCoverage
	if store != nil {
		measured, err := store.FarmCoverage(ctx, farm.CompanyID, farm.ID, farm.Perimeter)
		if err != nil {
			return fmt.Errorf("measure farm coverage: %w", err)
		}
		coverage = measured
	}
	return deps.Areas.CheckFarm(farm.Perimeter != nil, areaHa, coverage)
}

// checkFarmOwners reports every linked owner that is not in the company, on
// the index the client sent (the domain keeps the input order).
func checkFarmOwners(ctx context.Context, owners OwnerStore, farm domain.Farm) error {
	ids := farm.OwnerIDs()
	if len(ids) == 0 {
		return nil
	}
	missing, err := owners.MissingOwners(ctx, farm.CompanyID, ids)
	if err != nil {
		return fmt.Errorf("check owners: %w", err)
	}
	var problems domain.Violations
	for i, id := range ids {
		if slices.Contains(missing, id) {
			problems.Add(fmt.Sprintf("owners[%d].owner_id", i), "proprietário não encontrado")
		}
	}
	return problems.Err()
}

func recordFarmEvent(ctx context.Context, audit AuditRecorder, actor domain.Principal, now time.Time, action string, farm domain.Farm) error {
	event := actorEvent(actor, now, farm.CompanyID, action, domain.AuditEntityFarm, string(farm.ID),
		map[string]string{"status": string(farm.Status), "owners": fmt.Sprint(len(farm.Owners))})
	if err := audit.Record(ctx, event); err != nil {
		return fmt.Errorf("record audit: %w", err)
	}
	return nil
}
