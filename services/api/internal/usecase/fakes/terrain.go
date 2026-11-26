package fakes

import (
	"bytes"
	"cmp"
	"context"
	"fmt"
	"io"
	"maps"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/domain"
	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/usecase"
)

// ---- relief store -------------------------------------------------------------------

type terrainRow struct {
	companyID   domain.CompanyID
	farmID      domain.FarmID
	runID       string
	status      domain.TerrainStatus
	reason      string
	requestedAt time.Time
	updatedAt   time.Time
	calculation *domain.TerrainCalculation
	version     int
}

type storedAdjustment struct {
	companyID domain.CompanyID
	farmID    domain.FarmID
	adjust    domain.TerrainAdjustment
}

// Terrain is an in-memory relief store: it implements usecase.TerrainReader,
// usecase.TerrainStore and usecase.TerrainTxRunner (and TerrainStaleMarker for
// the registry transaction). Fields and farms come from the Registry it was
// built with. It is safe for the concurrent use the background worker makes of
// it: transactions are serialized, like row locks do in the database.
type Terrain struct {
	reg *Registry
	// UserNames gives the author name of an adjustment (the id when missing).
	UserNames map[domain.UserID]string
	// Audit holds the events recorded by the relief transactions.
	Audit []domain.AuditEvent

	txMu        sync.Mutex // serializes transactions
	mu          sync.Mutex // guards the data below
	rows        map[domain.FieldID]terrainRow
	adjustments map[domain.FieldID]storedAdjustment
	runs        []usecase.TerrainRun // by request time, oldest first
	failures    map[string]error
}

// NewTerrain returns the relief store bound to the registry (NewRegistry already made it), which
// forwards MarkStale to it inside registry transactions.
func NewTerrain(reg *Registry) *Terrain {
	if reg.terrain != nil {
		return reg.terrain
	}
	t := &Terrain{
		reg:         reg,
		UserNames:   map[domain.UserID]string{},
		rows:        map[domain.FieldID]terrainRow{},
		adjustments: map[domain.FieldID]storedAdjustment{},
		failures:    map[string]error{},
	}
	reg.terrain = t
	return t
}

// Fail makes every call of the named operation return err.
func (t *Terrain) Fail(operation string, err error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.failures[operation] = err
}

func (t *Terrain) check(operation string) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if err := t.failures[operation]; err != nil {
		return fmt.Errorf("%s: %w", operation, err)
	}
	return nil
}

// AuditActions lists the recorded actions in order.
func (t *Terrain) AuditActions() []string {
	t.txMu.Lock()
	defer t.txMu.Unlock()
	actions := make([]string, 0, len(t.Audit))
	for _, event := range t.Audit {
		actions = append(actions, event.Action)
	}
	return actions
}

type terrainSnapshot struct {
	rows        map[domain.FieldID]terrainRow
	adjustments map[domain.FieldID]storedAdjustment
	runs        []usecase.TerrainRun
	audit       []domain.AuditEvent
}

func (t *Terrain) snapshot() terrainSnapshot {
	t.mu.Lock()
	defer t.mu.Unlock()
	return terrainSnapshot{maps.Clone(t.rows), maps.Clone(t.adjustments), slices.Clone(t.runs), slices.Clone(t.Audit)}
}

func (t *Terrain) restore(s terrainSnapshot) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.rows, t.adjustments, t.runs, t.Audit = s.rows, s.adjustments, s.runs, s.audit
}

// InTerrainTx implements usecase.TerrainTxRunner with rollback on failure.
func (t *Terrain) InTerrainTx(ctx context.Context, fn func(ctx context.Context, tx usecase.TerrainStores) error) error {
	t.txMu.Lock()
	defer t.txMu.Unlock()
	before := t.snapshot()
	err := fn(ctx, usecase.TerrainStores{Terrain: t, Audit: terrainAudit{t}})
	if err == nil {
		err = t.check("Commit")
	}
	if err != nil {
		t.restore(before)
		return err
	}
	return nil
}

type terrainAudit struct{ t *Terrain }

// Record implements usecase.AuditRecorder.
func (a terrainAudit) Record(_ context.Context, event domain.AuditEvent) error {
	if err := a.t.check("Record"); err != nil {
		return err
	}
	a.t.mu.Lock()
	defer a.t.mu.Unlock()
	a.t.Audit = append(a.t.Audit, event)
	return nil
}

// ---- test helpers --------------------------------------------------------------------

// Stored returns the row of a field.
func (t *Terrain) Stored(fieldID domain.FieldID) (usecase.StoredTerrain, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	row, ok := t.rows[fieldID]
	if !ok {
		return usecase.StoredTerrain{}, false
	}
	return storedOf(row), true
}

// Runs returns the runs, oldest first.
func (t *Terrain) Runs() []usecase.TerrainRun {
	t.mu.Lock()
	defer t.mu.Unlock()
	runs := make([]usecase.TerrainRun, len(t.runs))
	for i, run := range t.runs {
		runs[i] = t.countedRun(run)
	}
	return runs
}

// Seed stores a row as if the field had been through the executor.
func (t *Terrain) Seed(companyID domain.CompanyID, farmID domain.FarmID, fieldID domain.FieldID, status domain.TerrainStatus, calculation *domain.TerrainCalculation) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.rows[fieldID] = terrainRow{companyID: companyID, farmID: farmID, status: status, calculation: calculation, version: 1}
}

// ForceStatus changes the status of an existing row (to simulate an interrupted calculation).
func (t *Terrain) ForceStatus(fieldID domain.FieldID, status domain.TerrainStatus) {
	t.mu.Lock()
	defer t.mu.Unlock()
	row := t.rows[fieldID]
	row.status = status
	t.rows[fieldID] = row
}

// ---- reading -------------------------------------------------------------------------

func storedOf(row terrainRow) usecase.StoredTerrain {
	return usecase.StoredTerrain{Status: row.status, FailureReason: row.reason, RequestedAt: row.requestedAt, Calculation: row.calculation}
}

func (t *Terrain) countedRun(run usecase.TerrainRun) usecase.TerrainRun {
	for _, row := range t.rows {
		if row.runID != run.ID {
			continue
		}
		run.Total++
		if row.status == domain.TerrainReady || row.status == domain.TerrainFailed {
			run.Done++
		}
	}
	return run
}

func (t *Terrain) dataOf(listing usecase.FieldListing) usecase.FieldTerrainData {
	data := usecase.FieldTerrainData{Field: listing}
	if row, ok := t.rows[listing.Field.ID]; ok {
		stored := storedOf(row)
		data.Terrain = &stored
	}
	if adj, ok := t.adjustments[listing.Field.ID]; ok {
		data.Adjustment = &usecase.StoredTerrainAdjustment{Adjustment: adj.adjust, AdjustedByName: t.nameOf(adj.adjust.AdjustedBy)}
	}
	return data
}

func (t *Terrain) nameOf(id domain.UserID) string {
	if name, ok := t.UserNames[id]; ok {
		return name
	}
	return string(id)
}

// ReadFarmTerrain implements usecase.TerrainReader.
func (t *Terrain) ReadFarmTerrain(ctx context.Context, companyID domain.CompanyID, farmID domain.FarmID) (usecase.FarmTerrainData, error) {
	if err := t.check("ReadFarmTerrain"); err != nil {
		return usecase.FarmTerrainData{}, err
	}
	listings, err := t.reg.ListFields(ctx, companyID, farmID)
	if err != nil {
		return usecase.FarmTerrainData{}, err
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	data := usecase.FarmTerrainData{Fields: make([]usecase.FieldTerrainData, 0, len(listings))}
	for _, listing := range listings {
		data.Fields = append(data.Fields, t.dataOf(listing))
	}
	for _, run := range slices.Backward(t.runs) {
		if run.CompanyID == companyID && run.FarmID == farmID {
			counted := t.countedRun(run)
			data.LatestRun = &counted
			break
		}
	}
	return data, nil
}

// ReadFieldTerrain implements usecase.TerrainReader.
func (t *Terrain) ReadFieldTerrain(ctx context.Context, companyID domain.CompanyID, farmID domain.FarmID, fieldID domain.FieldID) (usecase.FieldTerrainData, error) {
	if err := t.check("ReadFieldTerrain"); err != nil {
		return usecase.FieldTerrainData{}, err
	}
	listing, err := t.reg.FindField(ctx, companyID, farmID, fieldID)
	if err != nil {
		return usecase.FieldTerrainData{}, err
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.dataOf(listing), nil
}

// ---- writing -------------------------------------------------------------------------

// LockFarmForTerrain implements usecase.TerrainStore.
func (t *Terrain) LockFarmForTerrain(_ context.Context, companyID domain.CompanyID, farmID domain.FarmID) error {
	if err := t.check("LockFarmForTerrain"); err != nil {
		return err
	}
	if _, ok := t.reg.farmOf(companyID, farmID); !ok {
		return fmt.Errorf("lock farm: %w", usecase.ErrNotFound)
	}
	return nil
}

// FindOpenRun implements usecase.TerrainStore.
func (t *Terrain) FindOpenRun(_ context.Context, companyID domain.CompanyID, farmID domain.FarmID) (usecase.TerrainRun, bool, error) {
	if err := t.check("FindOpenRun"); err != nil {
		return usecase.TerrainRun{}, false, err
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	for _, run := range t.runs {
		if run.CompanyID == companyID && run.FarmID == farmID && run.FinishedAt == nil {
			return t.countedRun(run), true, nil
		}
	}
	return usecase.TerrainRun{}, false, nil
}

// SelectFieldsForRun implements usecase.TerrainStore.
func (t *Terrain) SelectFieldsForRun(_ context.Context, companyID domain.CompanyID, farmID domain.FarmID, scope usecase.TerrainRunScope) ([]domain.FieldID, error) {
	if err := t.check("SelectFieldsForRun"); err != nil {
		return nil, err
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	var ids []domain.FieldID
	for _, field := range t.reg.fieldsOf(companyID, farmID) {
		row, hasRow := t.rows[field.ID]
		needs := scope == usecase.TerrainScopeAll || !hasRow ||
			row.status == domain.TerrainPending || row.status == domain.TerrainFailed
		if needs {
			ids = append(ids, field.ID)
		}
	}
	return ids, nil
}

// CreateRun implements usecase.TerrainStore.
func (t *Terrain) CreateRun(_ context.Context, run usecase.TerrainRun) error {
	if err := t.check("CreateRun"); err != nil {
		return err
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	run.Total, run.Done, run.FinishedAt = 0, 0, nil
	t.runs = append(t.runs, run)
	return nil
}

// MarkFieldsPending implements usecase.TerrainStore.
func (t *Terrain) MarkFieldsPending(_ context.Context, run usecase.TerrainRun, fieldIDs []domain.FieldID, at time.Time) error {
	if err := t.check("MarkFieldsPending"); err != nil {
		return err
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	for _, id := range fieldIDs {
		row := t.rows[id]
		row.companyID, row.farmID, row.runID = run.CompanyID, run.FarmID, run.ID
		row.status, row.reason, row.requestedAt = domain.TerrainPending, "", at
		row.version++
		t.rows[id] = row
	}
	return nil
}

// MarkStale implements usecase.TerrainStaleMarker.
func (t *Terrain) MarkStale(_ context.Context, companyID domain.CompanyID, fieldID domain.FieldID, at time.Time) error {
	if err := t.check("MarkStale"); err != nil {
		return err
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	row, ok := t.rows[fieldID]
	if !ok || row.companyID != companyID {
		return nil
	}
	row.status, row.reason, row.calculation, row.requestedAt = domain.TerrainPending, "", nil, at
	row.version++
	t.rows[fieldID] = row
	return nil
}

// ClaimNextPending implements usecase.TerrainStore.
func (t *Terrain) ClaimNextPending(_ context.Context, at time.Time) (usecase.TerrainClaim, bool, error) {
	if err := t.check("ClaimNextPending"); err != nil {
		return usecase.TerrainClaim{}, false, err
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	for _, run := range t.runs {
		if run.FinishedAt != nil {
			continue
		}
		var pending []domain.FieldID
		for id, row := range t.rows {
			if row.runID == run.ID && row.status == domain.TerrainPending {
				pending = append(pending, id)
			}
		}
		if len(pending) == 0 {
			continue
		}
		slices.SortFunc(pending, func(a, b domain.FieldID) int { return cmp.Compare(a, b) })
		row := t.rows[pending[0]]
		row.status = domain.TerrainCalculating
		row.version++
		row.updatedAt = at
		t.rows[pending[0]] = row
		return usecase.TerrainClaim{CompanyID: row.companyID, FarmID: row.farmID, FieldID: pending[0], RunID: run.ID, RowVersion: row.version}, true, nil
	}
	return usecase.TerrainClaim{}, false, nil
}

// RecoverStale implements usecase.TerrainStore.
func (t *Terrain) RecoverStale(_ context.Context, staleBefore, at time.Time) error {
	if err := t.check("RecoverStale"); err != nil {
		return err
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	for id, row := range t.rows {
		if row.status == domain.TerrainCalculating && row.updatedAt.Before(staleBefore) {
			row.status = domain.TerrainPending
			row.updatedAt = at
			row.version++
			t.rows[id] = row
		}
	}
	for i := range t.runs {
		t.finishIfDrained(i, at)
	}
	return nil
}

func (t *Terrain) finishIfDrained(index int, at time.Time) bool {
	run := t.runs[index]
	if run.FinishedAt != nil {
		return false
	}
	for _, row := range t.rows {
		if row.runID == run.ID && (row.status == domain.TerrainPending || row.status == domain.TerrainCalculating) {
			return false
		}
	}
	t.runs[index].FinishedAt = &at
	return true
}

func (t *Terrain) claimed(claim usecase.TerrainClaim) (terrainRow, error) {
	row, ok := t.rows[claim.FieldID]
	if !ok || row.companyID != claim.CompanyID || row.version != claim.RowVersion || row.status != domain.TerrainCalculating {
		return terrainRow{}, fmt.Errorf("terrain row changed since the claim: %w", usecase.ErrVersionConflict)
	}
	return row, nil
}

// SaveCalculation implements usecase.TerrainStore.
func (t *Terrain) SaveCalculation(_ context.Context, claim usecase.TerrainClaim, calculation domain.TerrainCalculation, _ time.Time) error {
	if err := t.check("SaveCalculation"); err != nil {
		return err
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	row, err := t.claimed(claim)
	if err != nil {
		return err
	}
	row.status, row.reason, row.calculation = domain.TerrainReady, "", &calculation
	row.version++
	t.rows[claim.FieldID] = row
	return nil
}

// SaveFailure implements usecase.TerrainStore.
func (t *Terrain) SaveFailure(_ context.Context, claim usecase.TerrainClaim, reason string, _ time.Time) error {
	if err := t.check("SaveFailure"); err != nil {
		return err
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	row, err := t.claimed(claim)
	if err != nil {
		return err
	}
	row.status, row.reason = domain.TerrainFailed, reason
	row.version++
	t.rows[claim.FieldID] = row
	return nil
}

// FinishRun implements usecase.TerrainStore.
func (t *Terrain) FinishRun(_ context.Context, companyID domain.CompanyID, runID string, at time.Time) (bool, error) {
	if err := t.check("FinishRun"); err != nil {
		return false, err
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	for i, run := range t.runs {
		if run.ID == runID && run.CompanyID == companyID {
			return t.finishIfDrained(i, at), nil
		}
	}
	return false, fmt.Errorf("finish run %s: %w", runID, usecase.ErrNotFound)
}

// UpsertAdjustment implements usecase.TerrainStore.
func (t *Terrain) UpsertAdjustment(_ context.Context, adjustment usecase.FieldAdjustment) error {
	if err := t.check("UpsertAdjustment"); err != nil {
		return err
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.adjustments[adjustment.FieldID] = storedAdjustment{companyID: adjustment.CompanyID, farmID: adjustment.FarmID, adjust: adjustment.Adjustment}
	return nil
}

// DeleteAdjustment implements usecase.TerrainStore.
func (t *Terrain) DeleteAdjustment(_ context.Context, companyID domain.CompanyID, fieldID domain.FieldID) error {
	if err := t.check("DeleteAdjustment"); err != nil {
		return err
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	adj, ok := t.adjustments[fieldID]
	if !ok || adj.companyID != companyID {
		return fmt.Errorf("delete adjustment: %w", usecase.ErrNotFound)
	}
	delete(t.adjustments, fieldID)
	return nil
}

// ---- wake-up ---------------------------------------------------------------------------

// Wakeups counts the calls of Wake (usecase.TerrainWakeup).
type Wakeups struct{ n atomic.Int32 }

// Wake implements usecase.TerrainWakeup.
func (w *Wakeups) Wake() { w.n.Add(1) }

// Count returns how many times Wake was called.
func (w *Wakeups) Count() int { return int(w.n.Load()) }

// ---- elevation ---------------------------------------------------------------------------

// BoundingBox is a box in degrees.
type BoundingBox struct{ MinLon, MinLat, MaxLon, MaxLat float64 }

// Elevation is an in-memory usecase.ElevationStore. A Window is only served
// when every block under the box of the field was ingested, like the database
// does, so tests exercise the download-then-reuse path. Tests fill Boxes and
// Windows per field.
type Elevation struct {
	// Boxes and Windows are set per field before the executor starts.
	Boxes   map[domain.FieldID]BoundingBox
	Windows map[domain.FieldID]domain.ElevationWindow
	// OnWindow, when set, runs inside Window (to block, signal or panic).
	OnWindow func(fieldID domain.FieldID)
	// OnIngest, when set, runs inside IngestTile with its context (to simulate a slow
	// ingestion that respects the deadline). Its error is returned by IngestTile.
	OnIngest func(ctx context.Context, key domain.ElevationTileKey) error

	mu       sync.Mutex
	tiles    map[domain.ElevationTileKey]domain.ElevationModel
	ingested []domain.ElevationTileKey
}

// NewElevation returns an empty elevation store.
func NewElevation() *Elevation {
	return &Elevation{
		Boxes:   map[domain.FieldID]BoundingBox{},
		Windows: map[domain.FieldID]domain.ElevationWindow{},
		tiles:   map[domain.ElevationTileKey]domain.ElevationModel{},
	}
}

// AddTile marks a block as already ingested.
func (e *Elevation) AddTile(key domain.ElevationTileKey) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.tiles[key] = domain.ElevationModel{}
}

// Ingested lists the blocks ingested through IngestTile, in order.
func (e *Elevation) Ingested() []domain.ElevationTileKey {
	e.mu.Lock()
	defer e.mu.Unlock()
	return slices.Clone(e.ingested)
}

// HasTile implements usecase.ElevationStore.
func (e *Elevation) HasTile(_ context.Context, key domain.ElevationTileKey) (bool, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	_, ok := e.tiles[key]
	return ok, nil
}

// IngestTile implements usecase.ElevationStore (idempotent by key).
func (e *Elevation) IngestTile(ctx context.Context, key domain.ElevationTileKey, model domain.ElevationModel, geotiff io.Reader) error {
	if _, err := io.Copy(io.Discard, geotiff); err != nil {
		return fmt.Errorf("read geotiff: %w", err)
	}
	if e.OnIngest != nil {
		if err := e.OnIngest(ctx, key); err != nil {
			return err
		}
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if _, ok := e.tiles[key]; ok {
		return nil
	}
	e.tiles[key] = model
	e.ingested = append(e.ingested, key)
	return nil
}

// FieldBounds implements usecase.ElevationStore; the box is returned as set (the radius is already in it).
func (e *Elevation) FieldBounds(_ context.Context, _ domain.CompanyID, fieldID domain.FieldID, _ float64) (float64, float64, float64, float64, error) {
	box, ok := e.Boxes[fieldID]
	if !ok {
		return 0, 0, 0, 0, fmt.Errorf("field bounds %s: %w", fieldID, usecase.ErrNotFound)
	}
	return box.MinLon, box.MinLat, box.MaxLon, box.MaxLat, nil
}

// Window implements usecase.ElevationStore.
func (e *Elevation) Window(_ context.Context, _ domain.CompanyID, fieldID domain.FieldID, _ float64) (domain.ElevationWindow, error) {
	if e.OnWindow != nil {
		e.OnWindow(fieldID)
	}
	box, ok := e.Boxes[fieldID]
	if !ok {
		return domain.ElevationWindow{}, fmt.Errorf("window %s: %w", fieldID, usecase.ErrNotFound)
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, key := range domain.TileKeysCovering(box.MinLon, box.MinLat, box.MaxLon, box.MaxLat) {
		if _, ok := e.tiles[key]; !ok {
			return domain.ElevationWindow{}, fmt.Errorf("window %s: block %s: %w", fieldID, key, usecase.ErrElevationUnavailable)
		}
	}
	window, ok := e.Windows[fieldID]
	if !ok {
		return domain.ElevationWindow{}, fmt.Errorf("window %s: %w", fieldID, usecase.ErrNotFound)
	}
	return window, nil
}

// ElevationSource is an in-memory usecase.ElevationSource.
type ElevationSource struct {
	ModelInfo domain.ElevationModel
	// Tiles has the GeoTIFF bytes of the blocks that exist; any other key is unavailable.
	Tiles map[domain.ElevationTileKey][]byte
	// Errors makes FetchTile of a key fail with the given error.
	Errors map[domain.ElevationTileKey]error

	mu      sync.Mutex
	fetched []domain.ElevationTileKey
}

// NewElevationSource returns a source with the Copernicus model and no blocks.
func NewElevationSource() *ElevationSource {
	return &ElevationSource{
		ModelInfo: domain.ElevationModel{Source: "Copernicus DEM GLO-30", ResolutionM: 30, Version: "GLO-30 COG 2022-05"},
		Tiles:     map[domain.ElevationTileKey][]byte{},
		Errors:    map[domain.ElevationTileKey]error{},
	}
}

// Model implements usecase.ElevationSource.
func (s *ElevationSource) Model() domain.ElevationModel { return s.ModelInfo }

// Fetched lists the blocks requested from the source, in order.
func (s *ElevationSource) Fetched() []domain.ElevationTileKey {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.fetched)
}

// FetchTile implements usecase.ElevationSource.
func (s *ElevationSource) FetchTile(_ context.Context, key domain.ElevationTileKey) (io.ReadCloser, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.fetched = append(s.fetched, key)
	if err := s.Errors[key]; err != nil {
		return nil, err
	}
	content, ok := s.Tiles[key]
	if !ok {
		return nil, fmt.Errorf("fetch %s: %w", key, usecase.ErrElevationUnavailable)
	}
	return io.NopCloser(bytes.NewReader(content)), nil
}

// SeedFailed stores a failed row with its reason.
func (t *Terrain) SeedFailed(companyID domain.CompanyID, farmID domain.FarmID, fieldID domain.FieldID, reason string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.rows[fieldID] = terrainRow{companyID: companyID, farmID: farmID, status: domain.TerrainFailed, reason: reason, version: 1}
}

// AttachToRun links existing rows to a run (to build states a crash could leave).
func (t *Terrain) AttachToRun(runID string, fieldIDs ...domain.FieldID) {
	t.mu.Lock()
	defer t.mu.Unlock()
	for _, id := range fieldIDs {
		row := t.rows[id]
		row.runID = runID
		t.rows[id] = row
	}
}
