package usecase

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"runtime/debug"
	"sync"
	"time"

	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/domain"
)

// Failure reasons stored in field_terrain.failure_reason and shown to the user.
// They are short and honest; the technical detail goes to the log.
const (
	TerrainFailureTileUnavailable = "bloco do modelo de elevação indisponível"
	TerrainFailureInsufficient    = "dados de elevação insuficientes"
	TerrainFailureTimeout         = "tempo limite excedido ao obter o modelo de elevação"
	TerrainFailureUnexpected      = "erro inesperado ao calcular"
)

// TerrainWorkerDeps are the dependencies of the TerrainWorker.
type TerrainWorkerDeps struct {
	Tx        TerrainTxRunner
	Elevation ElevationStore
	Source    ElevationSource
	Clock     Clock
	Rules     domain.TerrainRules
	// Log receives the technical detail of each failure (identifiers only, no
	// personal data). Nil discards it.
	Log *slog.Logger
}

// TerrainWorkerConfig tunes the executor. Zero values take the defaults.
type TerrainWorkerConfig struct {
	// Concurrency is the number of fields calculated at the same time (default 1).
	Concurrency int
	// SweepInterval is how often idle goroutines look for pending fields even
	// without a Wake, and recover expired leases (default 30s).
	SweepInterval time.Duration
	// TileTimeout bounds the download and ingestion of ONE elevation block
	// (default 10m): a 1 degree block is tens of MB and the ingestion decodes it
	// inside the database, so it takes minutes, not seconds.
	TileTimeout time.Duration
	// FieldTimeout bounds what remains of a field once its blocks exist: the
	// bounds query, the window cut and the calculation (default 2m).
	FieldTimeout time.Duration
	// LeaseSlack is added to TileTimeout + FieldTimeout to form the lease of a
	// claimed field (default 2m): a calculating row not touched for longer than
	// that is taken as abandoned (process dead, or database down when the failure
	// was to be recorded) and goes back to pending. It must cover the time between
	// the claim and the end of the work; a field that needs several blocks gets
	// TileTimeout for each one, so it can outlast the lease only in that rare case,
	// and the cost is a discarded save and a recalculation, never a wrong result.
	LeaseSlack time.Duration
}

const (
	defaultTerrainSweepInterval = 30 * time.Second
	defaultTerrainTileTimeout   = 10 * time.Minute
	defaultTerrainFieldTimeout  = 2 * time.Minute
	defaultTerrainLeaseSlack    = 2 * time.Minute
)

// TerrainWorker is the in-process background executor of the relief
// calculation (ADR 0021, decision 6). It knows only ports: for each pending
// field it makes sure the elevation blocks exist (downloading the missing
// ones), cuts the window, calls the domain and stores the result or the
// failure. It does no business calculation itself.
type TerrainWorker struct {
	deps TerrainWorkerDeps
	cfg  TerrainWorkerConfig
	log  *slog.Logger
	wake chan struct{}

	mu      sync.Mutex
	started bool
	cancel  context.CancelFunc
	wg      sync.WaitGroup
}

// NewTerrainWorker builds the executor and rejects inconsistent terrain rules.
func NewTerrainWorker(deps TerrainWorkerDeps, cfg TerrainWorkerConfig) (*TerrainWorker, error) {
	if err := deps.Rules.Validate(); err != nil {
		return nil, fmt.Errorf("terrain worker rules: %w", err)
	}
	if cfg.Concurrency < 1 {
		cfg.Concurrency = 1
	}
	if cfg.SweepInterval <= 0 {
		cfg.SweepInterval = defaultTerrainSweepInterval
	}
	if cfg.TileTimeout <= 0 {
		cfg.TileTimeout = defaultTerrainTileTimeout
	}
	if cfg.FieldTimeout <= 0 {
		cfg.FieldTimeout = defaultTerrainFieldTimeout
	}
	if cfg.LeaseSlack <= 0 {
		cfg.LeaseSlack = defaultTerrainLeaseSlack
	}
	log := deps.Log
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	return &TerrainWorker{deps: deps, cfg: cfg, log: log, wake: make(chan struct{}, cfg.Concurrency)}, nil
}

// lease is how long a field may stay calculating before it counts as abandoned.
func (w *TerrainWorker) lease() time.Duration {
	return w.cfg.TileTimeout + w.cfg.FieldTimeout + w.cfg.LeaseSlack
}

// Wake tells the executor there is work. It never blocks and implements TerrainWakeup.
func (w *TerrainWorker) Wake() {
	select {
	case w.wake <- struct{}{}:
	default:
	}
}

// Start recovers fields whose lease expired in a previous run (they go back to
// pending) and launches the goroutines. They stop when ctx ends or on Stop.
// A worker starts once. Right after a crash the fields it left calculating are
// still inside their lease, so they are recovered by the periodic sweep once it
// expires, not by this call.
func (w *TerrainWorker) Start(ctx context.Context) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.started {
		return errors.New("terrain worker already started")
	}
	if err := w.recoverStale(ctx); err != nil {
		return fmt.Errorf("recover abandoned terrain calculations: %w", err)
	}
	runCtx, cancel := context.WithCancel(ctx)
	w.started, w.cancel = true, cancel
	for range w.cfg.Concurrency {
		w.wg.Add(1)
		go w.loop(runCtx)
	}
	w.Wake()
	return nil
}

// recoverStale gives back to pending the fields calculating for longer than the
// lease, and closes the runs left with nothing to do. Younger ones may be the
// work of another live instance and are never touched.
func (w *TerrainWorker) recoverStale(ctx context.Context) error {
	now := w.deps.Clock.Now()
	return w.deps.Tx.InTerrainTx(ctx, func(ctx context.Context, tx TerrainStores) error {
		return tx.Terrain.RecoverStale(ctx, now.Add(-w.lease()), now)
	})
}

// Stop ends the loops and waits for the fields in progress to be saved (they
// are bounded by the timeouts). Whatever stays pending is picked up by the next
// Start. It is safe to call without Start and more than once.
func (w *TerrainWorker) Stop() {
	w.mu.Lock()
	cancel := w.cancel
	w.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	w.wg.Wait()
}

func (w *TerrainWorker) loop(ctx context.Context) {
	defer w.wg.Done()
	ticker := time.NewTicker(w.cfg.SweepInterval)
	defer ticker.Stop()
	for {
		w.guarded(w.log, "draining the queue", func() { w.drain(ctx) })
		select {
		case <-ctx.Done():
			return
		case <-w.wake:
		case <-ticker.C:
			w.guarded(w.log, "recovering expired leases", func() { w.sweep(ctx) })
		}
	}
}

// sweep is the periodic recovery: the lease of a field expires with time, not
// with an event, so every tick looks for it.
func (w *TerrainWorker) sweep(ctx context.Context) {
	if err := w.recoverStale(ctx); err != nil && ctx.Err() == nil {
		w.log.Error("terrain: recovering expired leases failed", "error", err)
	}
}

// guarded runs fn and turns a panic into a log line, so a bug in an adapter
// cannot take the whole process down. It reports whether fn returned normally.
func (w *TerrainWorker) guarded(logger *slog.Logger, what string, fn func()) (completed bool) {
	defer func() {
		if recovered := recover(); recovered != nil {
			logger.Error("terrain: panic while "+what, "panic", fmt.Sprint(recovered), "stack", string(debug.Stack()))
			completed = false
		}
	}()
	fn()
	return true
}

// drain claims and processes fields until nothing is pending.
func (w *TerrainWorker) drain(ctx context.Context) {
	for ctx.Err() == nil {
		claim, ok, err := w.claimNext(ctx)
		if err != nil {
			if ctx.Err() == nil {
				w.log.Error("terrain: claim next pending field failed", "error", err)
			}
			return
		}
		if !ok {
			return
		}
		w.process(ctx, claim)
	}
}

func (w *TerrainWorker) claimNext(ctx context.Context) (claim TerrainClaim, ok bool, err error) {
	err = w.deps.Tx.InTerrainTx(ctx, func(ctx context.Context, tx TerrainStores) error {
		var claimErr error
		claim, ok, claimErr = tx.Terrain.ClaimNextPending(ctx, w.deps.Clock.Now())
		return claimErr
	})
	return claim, ok, err
}

// process calculates one claimed field and always leaves it ready, failed, or
// (on a version conflict) re-marked by an edit. It outlives the cancellation of
// ctx on purpose: a stop must not lose the field in progress. A panic anywhere
// (an adapter, a store) is logged and the field is recorded as failed when the
// store still answers; the executor goes on either way.
func (w *TerrainWorker) process(ctx context.Context, claim TerrainClaim) {
	persist := context.WithoutCancel(ctx)
	logger := w.log.With("company_id", claim.CompanyID, "farm_id", claim.FarmID, "field_id", claim.FieldID, "run_id", claim.RunID)
	if !w.guarded(logger, "processing the field", func() { w.calculateAndSave(persist, logger, claim) }) {
		w.guarded(logger, "recording the failure after a panic", func() {
			w.saveFailure(persist, logger, claim, TerrainFailureUnexpected)
		})
	}
	w.guarded(logger, "finishing the run", func() { w.finishRun(persist, logger, claim) })
}

func (w *TerrainWorker) calculateAndSave(persist context.Context, logger *slog.Logger, claim TerrainClaim) {
	calculation, err := w.calculate(persist, claim)
	if err != nil {
		logger.Warn("terrain: calculation failed", "error", err)
		w.saveFailure(persist, logger, claim, failureReason(err))
		return
	}
	saveErr := w.store(persist, logger, func(ctx context.Context, tx TerrainStores, at time.Time) error {
		return tx.Terrain.SaveCalculation(ctx, claim, calculation, at)
	})
	if saveErr != nil {
		// A database error that is not a version conflict (a value outside the
		// column range, a dropped connection) must not leave the field
		// calculating forever: record it as failed so the user can retry.
		logger.Error("terrain: saving the calculation failed, recording the failure instead", "error", saveErr)
		w.saveFailure(persist, logger, claim, TerrainFailureUnexpected)
	}
}

func (w *TerrainWorker) saveFailure(ctx context.Context, logger *slog.Logger, claim TerrainClaim, reason string) {
	err := w.store(ctx, logger, func(ctx context.Context, tx TerrainStores, at time.Time) error {
		return tx.Terrain.SaveFailure(ctx, claim, reason, at)
	})
	if err != nil {
		// The database itself is failing: nothing more can be written. The loop
		// goes on; the periodic sweep puts the field back to pending once its lease
		// expires, and it is calculated again.
		logger.Error("terrain: saving the failure failed; the field stays calculating until its lease expires", "error", err)
	}
}

// store runs a save in a transaction. A version conflict means the field was
// edited meanwhile and is pending again: the result is discarded and nil is
// returned. Any other error is returned for the caller to handle.
func (w *TerrainWorker) store(ctx context.Context, logger *slog.Logger, save func(context.Context, TerrainStores, time.Time) error) error {
	err := w.deps.Tx.InTerrainTx(ctx, func(ctx context.Context, tx TerrainStores) error {
		return save(ctx, tx, w.deps.Clock.Now())
	})
	if errors.Is(err, ErrVersionConflict) {
		logger.Info("terrain: field changed during the calculation, result discarded")
		return nil
	}
	return err
}

func (w *TerrainWorker) finishRun(ctx context.Context, logger *slog.Logger, claim TerrainClaim) {
	err := w.deps.Tx.InTerrainTx(ctx, func(ctx context.Context, tx TerrainStores) error {
		_, err := tx.Terrain.FinishRun(ctx, claim.CompanyID, claim.RunID, w.deps.Clock.Now())
		return err
	})
	if err != nil {
		logger.Error("terrain: finishing the run failed", "error", err)
	}
}

// calculate downloads the missing blocks, cuts the window and runs the domain.
// The blocks, the slowest part, get TileTimeout each; the rest (bounds, window
// and calculation) gets FieldTimeout. A panic becomes an error so one bad field
// cannot stop the executor.
func (w *TerrainWorker) calculate(ctx context.Context, claim TerrainClaim) (calculation domain.TerrainCalculation, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			w.log.Error("terrain: panic while calculating", "field_id", claim.FieldID, "panic", fmt.Sprint(recovered), "stack", string(debug.Stack()))
			err = fmt.Errorf("panic while calculating: %v", recovered)
		}
	}()
	radius := w.deps.Rules.PositionRadiusM
	boundsCtx, cancelBounds := context.WithTimeout(ctx, w.cfg.FieldTimeout)
	minLon, minLat, maxLon, maxLat, err := w.deps.Elevation.FieldBounds(boundsCtx, claim.CompanyID, claim.FieldID, radius)
	cancelBounds()
	if err != nil {
		return domain.TerrainCalculation{}, fmt.Errorf("field bounds: %w", err)
	}
	keys := domain.TileKeysCovering(minLon, minLat, maxLon, maxLat)
	if len(keys) == 0 {
		return domain.TerrainCalculation{}, fmt.Errorf("%w: field bounds are outside the supported range", ErrElevationUnavailable)
	}
	for _, key := range keys {
		if err := w.ensureTile(ctx, key); err != nil {
			return domain.TerrainCalculation{}, err
		}
	}
	windowCtx, cancelWindow := context.WithTimeout(ctx, w.cfg.FieldTimeout)
	defer cancelWindow()
	window, err := w.deps.Elevation.Window(windowCtx, claim.CompanyID, claim.FieldID, radius)
	if err != nil {
		return domain.TerrainCalculation{}, fmt.Errorf("elevation window: %w", err)
	}
	calculation, err = domain.CalculateTerrain(window, w.deps.Rules, w.deps.Source.Model(), w.deps.Clock.Now())
	if err != nil {
		return domain.TerrainCalculation{}, fmt.Errorf("calculate terrain: %w", err)
	}
	return calculation, nil
}

// ensureTile makes sure the block is stored, downloading and ingesting it when
// it is not. One TileTimeout covers both steps.
func (w *TerrainWorker) ensureTile(ctx context.Context, key domain.ElevationTileKey) error {
	tileCtx, cancel := context.WithTimeout(ctx, w.cfg.TileTimeout)
	defer cancel()
	has, err := w.deps.Elevation.HasTile(tileCtx, key)
	if err != nil {
		return fmt.Errorf("check tile %s: %w", key, err)
	}
	if has {
		return nil
	}
	body, err := w.deps.Source.FetchTile(tileCtx, key)
	if err != nil {
		return fmt.Errorf("fetch tile %s: %w", key, err)
	}
	defer func() { _ = body.Close() }()
	if err := w.deps.Elevation.IngestTile(tileCtx, key, w.deps.Source.Model(), body); err != nil {
		return fmt.Errorf("ingest tile %s: %w", key, err)
	}
	return nil
}

// failureReason maps an error to the short text shown to the user.
func failureReason(err error) string {
	switch {
	// Timeouts first: the Copernicus client wraps a deadline as "unavailable" too, and
	// a deadline is a timeout, while a plain 403/404 (no deadline) is an unavailable block.
	case errors.Is(err, context.DeadlineExceeded), errors.Is(err, ErrElevationTimeout):
		return TerrainFailureTimeout
	case errors.Is(err, ErrElevationUnavailable):
		return TerrainFailureTileUnavailable
	case errors.Is(err, domain.ErrInsufficientElevationData):
		return TerrainFailureInsufficient
	default:
		return TerrainFailureUnexpected
	}
}
