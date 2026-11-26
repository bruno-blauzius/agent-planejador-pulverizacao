package postgres

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"time"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/adapter/postgres/sqlcgen"
	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/domain"
	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/usecase"
)

// Elevation model storage (ADR 0021). The database only tiles and clips rasters with
// PostGIS; the relief is calculated in the domain from the window this file returns.

const (
	// maxGeoTIFFBytes bounds the memory of IngestTile, which holds the whole GeoTIFF as a
	// bytea parameter. A Copernicus GLO-30 block (3600 x 3600 float32) is ~52 MB raw and
	// smaller when compressed; 256 MiB leaves room and still refuses runaway bodies.
	maxGeoTIFFBytes = 256 << 20
	// defaultIngestStatementTimeout replaces the pool statement_timeout for the ingestion
	// transaction only: decoding and tiling a block takes seconds, not milliseconds. It is
	// the ceiling when the context has no deadline; with one (the executor's TileTimeout),
	// the statement timeout follows the time left, so the two never disagree.
	defaultIngestStatementTimeout = 10 * time.Minute
	minIngestStatementTimeout     = time.Second

	wgs84SRID = 4326
)

// ErrInvalidElevationRaster means the GeoTIFF is unreadable or is not the expected
// one-band WGS84 (EPSG:4326) raster. Retrying with the same file never helps.
var ErrInvalidElevationRaster = errors.New("invalid elevation raster")

// HasTile implements usecase.ElevationStore.
func (r *Repository) HasTile(ctx context.Context, key domain.ElevationTileKey) (bool, error) {
	exists, err := r.q.HasElevationSource(ctx, string(key))
	if err != nil {
		return false, translateElevationError("has elevation tile", err)
	}
	return exists, nil
}

// IngestTile implements usecase.ElevationStore. It reads the GeoTIFF into memory (at most
// maxGeoTIFFBytes), then in ONE transaction validates it, and stores the source row and its
// 256x256 blocks. Ingesting a stored key again is a no-op, and so is losing the race to a
// concurrent ingest of the same key (the unique key makes the loser wait for the winner and
// then insert nothing).
func (r *Repository) IngestTile(ctx context.Context, key domain.ElevationTileKey, model domain.ElevationModel, geotiff io.Reader) error {
	body, err := io.ReadAll(io.LimitReader(geotiff, maxGeoTIFFBytes+1))
	if err != nil {
		return fmt.Errorf("read geotiff: %w", readFailure(err))
	}
	if len(body) > maxGeoTIFFBytes {
		return fmt.Errorf("%w: file larger than %d bytes", ErrInvalidElevationRaster, maxGeoTIFFBytes)
	}
	err = r.atomic(ctx, func(q *sqlcgen.Queries) error {
		if err := q.SetLocalStatementTimeout(ctx, ingestStatementTimeout(ctx)); err != nil {
			return fmt.Errorf("set statement timeout: %w", err)
		}
		exists, err := q.HasElevationSource(ctx, string(key))
		if err != nil {
			return fmt.Errorf("has elevation tile: %w", err)
		}
		if exists {
			return nil
		}
		if err := checkGeoTIFF(ctx, q, body); err != nil {
			return err
		}
		ingested, err := q.IngestElevationTile(ctx, sqlcgen.IngestElevationTileParams{
			Geotiff: body, TileKey: string(key), Source: model.Source, Version: model.Version,
		})
		if err != nil {
			return translateElevationError("ingest elevation tile", translateWrite("ingest elevation tile", err))
		}
		if ingested.StoredBlocks != ingested.DeclaredBlocks {
			return fmt.Errorf("ingest elevation tile %s: stored %d blocks, declared %d", key, ingested.StoredBlocks, ingested.DeclaredBlocks)
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("ingest elevation tile %s: %w", key, err)
	}
	return nil
}

func checkGeoTIFF(ctx context.Context, q *sqlcgen.Queries, body []byte) error {
	info, err := q.InspectGeoTIFF(ctx, body)
	if err != nil {
		var pgErr *pgconn.PgError
		if ctx.Err() == nil && errors.As(err, &pgErr) && pgErr.Code != sqlStateQueryCanceled {
			return fmt.Errorf("%w: %s", ErrInvalidElevationRaster, pgErr.Message)
		}
		return translateElevationError("inspect geotiff", err)
	}
	if info.Bands != 1 || info.Srid != wgs84SRID {
		return fmt.Errorf("%w: want 1 band in EPSG:%d, got %d bands in SRID %d", ErrInvalidElevationRaster, wgs84SRID, info.Bands, info.Srid)
	}
	return nil
}

// sqlStateQueryCanceled is statement_timeout or a cancel: not the file's fault.
const sqlStateQueryCanceled = "57014"

// ingestStatementTimeout is the statement_timeout of the ingestion transaction, in the
// text form PostgreSQL reads: the time left until the deadline of ctx (the executor's
// per-block timeout), never below a second, or the default ceiling without a deadline.
func ingestStatementTimeout(ctx context.Context) string {
	timeout := defaultIngestStatementTimeout
	if deadline, ok := ctx.Deadline(); ok {
		timeout = max(time.Until(deadline), minIngestStatementTimeout)
	}
	return fmt.Sprintf("%dms", timeout.Milliseconds())
}

// readFailure classifies an error while reading the GeoTIFF body: a deadline stays a
// deadline (timeout); anything else means the download was cut in the middle, which
// is "the block is unavailable", not a bug.
func readFailure(err error) error {
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, usecase.ErrElevationUnavailable) {
		return err
	}
	return fmt.Errorf("%w: %w", usecase.ErrElevationUnavailable, err)
}

// translateElevationError wraps err and, when the database canceled the statement
// for taking too long (SQLSTATE 57014), marks it as usecase.ErrElevationTimeout so
// the executor reports a timeout instead of an unexpected error.
func translateElevationError(operation string, err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == sqlStateQueryCanceled {
		return fmt.Errorf("%s: %w: %w", operation, usecase.ErrElevationTimeout, err)
	}
	return fmt.Errorf("%s: %w", operation, err)
}

// FieldBounds implements usecase.ElevationStore: the polygon grown by radiusM with a
// geography buffer (real meters, whatever the latitude), then its box in degrees.
func (r *Repository) FieldBounds(ctx context.Context, companyID domain.CompanyID, fieldID domain.FieldID, radiusM float64) (minLon, minLat, maxLon, maxLat float64, err error) {
	if err := checkRadius(radiusM); err != nil {
		return 0, 0, 0, 0, err
	}
	if !validUUID(string(companyID)) || !validUUID(string(fieldID)) {
		return 0, 0, 0, 0, fmt.Errorf("field bounds: %w", usecase.ErrNotFound)
	}
	row, err := r.q.GetFieldBounds(ctx, sqlcgen.GetFieldBoundsParams{RadiusM: radiusM, CompanyID: string(companyID), ID: string(fieldID)})
	if err != nil {
		return 0, 0, 0, 0, translateElevationError("field bounds", translateRead("field bounds", err))
	}
	return row.MinLon, row.MinLat, row.MaxLon, row.MaxLat, nil
}

// Window implements usecase.ElevationStore. See GetElevationWindow in queries/elevation.sql
// for the clipping. It answers usecase.ErrElevationUnavailable when the blocks do not cover
// the whole buffer, domain.ErrInsufficientElevationData when no cell of the field itself
// has data, and usecase.ErrElevationTimeout when the database cancels the statement.
func (r *Repository) Window(ctx context.Context, companyID domain.CompanyID, fieldID domain.FieldID, radiusM float64) (domain.ElevationWindow, error) {
	if _, _, _, _, err := r.FieldBounds(ctx, companyID, fieldID, radiusM); err != nil {
		return domain.ElevationWindow{}, fmt.Errorf("elevation window: %w", err) // unknown field: ErrNotFound
	}
	row, err := r.q.GetElevationWindow(ctx, sqlcgen.GetElevationWindowParams{RadiusM: radiusM, CompanyID: string(companyID), ID: string(fieldID)})
	if err != nil {
		return domain.ElevationWindow{}, translateElevationError("elevation window", err)
	}
	if !row.Covered || row.Cols == 0 || row.Rows == 0 {
		return domain.ElevationWindow{}, fmt.Errorf("elevation window: %w", usecase.ErrElevationUnavailable)
	}
	cells := int(row.Cols) * int(row.Rows)
	if len(row.Elevations) != cells || len(row.Inside) != cells {
		return domain.ElevationWindow{}, fmt.Errorf("elevation window: %dx%d grid came with %d values and %d mask cells", row.Rows, row.Cols, len(row.Elevations), len(row.Inside))
	}
	window := domain.ElevationWindow{
		Rows: int(row.Rows), Cols: int(row.Cols), Values: row.Elevations, Inside: make([]bool, cells),
		OriginLon: row.OriginLon, OriginLat: row.OriginLat, ScaleXDeg: row.ScaleX, ScaleYDeg: row.ScaleY,
	}
	hasData := false
	for i, inside := range row.Inside {
		window.Inside[i] = inside != 0
		if window.Inside[i] && !math.IsNaN(row.Elevations[i]) {
			hasData = true
		}
	}
	if !hasData {
		// The blocks cover the field but none of its cells has a value (void or ocean): that
		// is too little data to calculate, not a missing block.
		return domain.ElevationWindow{}, fmt.Errorf("elevation window: no elevation data inside the field: %w", domain.ErrInsufficientElevationData)
	}
	return window, nil
}

func checkRadius(radiusM float64) error {
	if math.IsNaN(radiusM) || math.IsInf(radiusM, 0) || radiusM < 0 {
		return fmt.Errorf("elevation radius must be a finite number of meters >= 0, got %v", radiusM)
	}
	return nil
}
