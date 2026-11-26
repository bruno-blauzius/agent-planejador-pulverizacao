package usecase

import (
	"context"
	"io"
	"time"

	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/domain"
)

// Ports of the field relief (ADR 0021). As with the registry, every method
// receives the company of the caller and the use case checks the visibility
// scope of the farm before reading; a field of another company or farm is
// ErrNotFound. Classes, compatibility with methods and the "outdated
// adjustment" flag are derived in the domain at read time from the stored
// measures and the injected domain.TerrainRules; the stores keep only data.

// ---- run scope ---------------------------------------------------------------------

// TerrainRunScope says which fields a calculation request covers.
type TerrainRunScope string

// Calculation scopes of the contract (terrain_runs.scope).
const (
	// TerrainScopeMissing covers fields without relief, pending or failed.
	TerrainScopeMissing TerrainRunScope = "missing"
	// TerrainScopeAll covers every field of the farm.
	TerrainScopeAll TerrainRunScope = "all"
)

// Valid reports whether s is a known scope.
func (s TerrainRunScope) Valid() bool { return s == TerrainScopeMissing || s == TerrainScopeAll }

// ---- reading -----------------------------------------------------------------------

// TerrainRun is a "calculate relief" request for a farm. Total and Done count
// the field_terrain rows whose run_id is this run: Done are the ones that are
// ready or failed.
type TerrainRun struct {
	ID          string
	CompanyID   domain.CompanyID
	FarmID      domain.FarmID
	Scope       TerrainRunScope
	RequestedBy domain.UserID
	RequestedAt time.Time
	// FinishedAt is nil while the run is open.
	FinishedAt *time.Time
	Total      int
	Done       int
}

// StoredTerrain is the field_terrain row of a field. Calculation is nil until
// the first successful calculation, and again after the polygon of the field
// changed (MarkStale clears it, because the measures describe another shape).
// While a recalculation is pending, calculating or failed, Calculation keeps
// the last successful result, so the screens can still show it.
type StoredTerrain struct {
	Status        domain.TerrainStatus // pending, calculating, ready or failed
	FailureReason string               // set only when Status is failed
	RequestedAt   time.Time
	Calculation   *domain.TerrainCalculation
}

// StoredTerrainAdjustment is a manual adjustment with the name of its author.
type StoredTerrainAdjustment struct {
	Adjustment     domain.TerrainAdjustment
	AdjustedByName string
}

// FieldTerrainData is a field joined with its relief. Terrain is nil when the
// field has no row (never calculated); Adjustment is nil without a manual one.
type FieldTerrainData struct {
	Field      FieldListing
	Terrain    *StoredTerrain
	Adjustment *StoredTerrainAdjustment
}

// FarmTerrainData is the relief of every field of a farm.
type FarmTerrainData struct {
	// Fields has every field of the farm (LEFT JOIN: a field without a row is
	// "not calculated"), by block and name, as FieldReader.ListFields.
	Fields []FieldTerrainData
	// LatestRun is the most recent run of the farm, finished or not; nil when
	// the farm never had one.
	LatestRun *TerrainRun
}

// TerrainReader reads the relief of one company.
type TerrainReader interface {
	// ReadFarmTerrain returns the relief of the fields of a farm of the company.
	// An unknown farm yields no fields (callers check the farm first).
	ReadFarmTerrain(ctx context.Context, companyID domain.CompanyID, farmID domain.FarmID) (FarmTerrainData, error)
	// ReadFieldTerrain returns ErrNotFound when the field is not in the farm of the company.
	ReadFieldTerrain(ctx context.Context, companyID domain.CompanyID, farmID domain.FarmID, fieldID domain.FieldID) (FieldTerrainData, error)
}

// ---- writing -----------------------------------------------------------------------

// TerrainClaim is a field taken by the background executor: the row is now
// calculating. RowVersion is the version of the row at claim time; saving a
// result with an outdated version fails with ErrVersionConflict, which happens
// when the field was edited (MarkStale) in the meantime.
type TerrainClaim struct {
	CompanyID  domain.CompanyID
	FarmID     domain.FarmID
	FieldID    domain.FieldID
	RunID      string
	RowVersion int
}

// FieldAdjustment is a manual adjustment to store.
type FieldAdjustment struct {
	CompanyID  domain.CompanyID
	FarmID     domain.FarmID
	FieldID    domain.FieldID
	Adjustment domain.TerrainAdjustment
}

// TerrainStaleMarker invalidates the relief of a field. It is the only part of
// the terrain store that the registry transaction needs: editing the polygon
// of a field marks the relief in the same transaction.
type TerrainStaleMarker interface {
	// MarkStale is called when the polygon of a field changed. Without a
	// field_terrain row it does nothing. Otherwise the row becomes pending,
	// its calculated measures are cleared, its version is bumped (so a
	// calculation in flight cannot overwrite it), and its run_id is kept: if the
	// run is still open the executor recomputes the field with the new polygon,
	// otherwise the field waits for the next request. The manual adjustment is untouched.
	MarkStale(ctx context.Context, companyID domain.CompanyID, fieldID domain.FieldID, at time.Time) error
}

// TerrainStore is the transactional side of the relief. Callers that create a
// run lock the farm first (LockFarmForTerrain), so two requests for the same
// farm cannot open two runs.
type TerrainStore interface {
	TerrainStaleMarker

	// LockFarmForTerrain holds a row lock on the farm until the transaction
	// ends (ErrNotFound when the farm is not in the company).
	LockFarmForTerrain(ctx context.Context, companyID domain.CompanyID, farmID domain.FarmID) error
	// FindOpenRun returns the unfinished run of the farm, with Total and Done.
	FindOpenRun(ctx context.Context, companyID domain.CompanyID, farmID domain.FarmID) (run TerrainRun, found bool, err error)
	// SelectFieldsForRun returns the fields a run of the scope would calculate:
	// every field of the farm for TerrainScopeAll; for TerrainScopeMissing the
	// ones without a row, pending or failed.
	SelectFieldsForRun(ctx context.Context, companyID domain.CompanyID, farmID domain.FarmID, scope TerrainRunScope) ([]domain.FieldID, error)
	// CreateRun stores a new open run (Total and Done are ignored).
	CreateRun(ctx context.Context, run TerrainRun) error
	// MarkFieldsPending creates or updates the rows of the fields as pending
	// in the run, clearing the failure reason and bumping the version. The last
	// successful calculation is kept, so a recalculation does not blank the screens.
	MarkFieldsPending(ctx context.Context, run TerrainRun, fieldIDs []domain.FieldID, at time.Time) error

	// ClaimNextPending atomically takes one pending field of an open run and
	// makes it calculating (FOR UPDATE SKIP LOCKED in SQL): two executors never
	// get the same field. It serves the oldest open run first and only moves to
	// the next run when the previous one has nothing pending. ok is false when
	// nothing is pending.
	ClaimNextPending(ctx context.Context, at time.Time) (claim TerrainClaim, ok bool, err error)
	// RecoverStale puts back to pending every calculating row whose last
	// update is older than staleBefore (a lease that expired: the process that
	// claimed it died, or could not even record the failure), bumping its
	// version so a late save from that process becomes a discarded conflict.
	// Fresher calculating rows belong to a live executor and are left alone.
	// It then finishes open runs with no pending or calculating row. at is the
	// finish time of those runs.
	RecoverStale(ctx context.Context, staleBefore, at time.Time) error
	// SaveCalculation stores the result and marks the row ready. It returns
	// ErrVersionConflict when the row changed since the claim.
	SaveCalculation(ctx context.Context, claim TerrainClaim, calculation domain.TerrainCalculation, at time.Time) error
	// SaveFailure marks the row failed with a short reason in Portuguese,
	// keeping the last successful calculation. ErrVersionConflict as above.
	SaveFailure(ctx context.Context, claim TerrainClaim, reason string, at time.Time) error
	// FinishRun sets finished_at when no row of the run is pending or
	// calculating. It reports whether the run was finished by this call;
	// calling it again, or while rows are open, is harmless.
	FinishRun(ctx context.Context, companyID domain.CompanyID, runID string, at time.Time) (finished bool, err error)

	// UpsertAdjustment creates or replaces the adjustment of the field
	// (one per field), bumping its version.
	UpsertAdjustment(ctx context.Context, adjustment FieldAdjustment) error
	// DeleteAdjustment removes the adjustment of the field (ErrNotFound when there is none).
	DeleteAdjustment(ctx context.Context, companyID domain.CompanyID, fieldID domain.FieldID) error
}

// TerrainStores are the transaction-bound stores of the relief.
type TerrainStores struct {
	Terrain TerrainStore
	Audit   AuditRecorder
}

// TerrainTxRunner runs fn in one database transaction: commit when fn returns
// nil, roll back otherwise. It is separate from RegistryTxRunner because the
// registry does not need the whole terrain store (only TerrainStaleMarker,
// carried by RegistryStores), while the relief use cases need the audit
// recorder in the same transaction as the terrain writes.
type TerrainTxRunner interface {
	InTerrainTx(ctx context.Context, fn func(ctx context.Context, tx TerrainStores) error) error
}

// TerrainWakeup tells the background executor there is work. Wake never blocks.
type TerrainWakeup interface {
	Wake()
}

// ---- elevation model ---------------------------------------------------------------

// ElevationStore keeps the blocks of the elevation model (elevation_sources and
// elevation_blocks; global reference data) and cuts windows out of them. The
// database only clips; the relief is calculated in the domain.
type ElevationStore interface {
	// HasTile reports whether the 1 degree block was already ingested.
	HasTile(ctx context.Context, key domain.ElevationTileKey) (bool, error)
	// IngestTile tiles the GeoTIFF and stores the source and its blocks in one
	// transaction. It is idempotent by key: ingesting a stored key again is a no-op.
	IngestTile(ctx context.Context, key domain.ElevationTileKey, model domain.ElevationModel, geotiff io.Reader) error
	// FieldBounds returns the bounding box of the polygon of the field already
	// grown by radiusM on every side, in degrees, which tells which blocks must
	// exist. ErrNotFound when the field is not in the company.
	FieldBounds(ctx context.Context, companyID domain.CompanyID, fieldID domain.FieldID, radiusM float64) (minLon, minLat, maxLon, maxLat float64, err error)
	// Window cuts the elevations of the field plus radiusM around it, with the
	// mask of the cells inside the polygon. It returns ErrElevationUnavailable
	// when a block is missing for any part of the window.
	Window(ctx context.Context, companyID domain.CompanyID, fieldID domain.FieldID, radiusM float64) (domain.ElevationWindow, error)
}

// ElevationSource downloads blocks of the elevation model (Copernicus GLO-30).
type ElevationSource interface {
	// Model identifies the model the blocks come from.
	Model() domain.ElevationModel
	// FetchTile returns the GeoTIFF of the block. The error wraps
	// ErrElevationUnavailable when the block does not exist or the download
	// failed. The caller closes the reader.
	FetchTile(ctx context.Context, key domain.ElevationTileKey) (io.ReadCloser, error)
}
