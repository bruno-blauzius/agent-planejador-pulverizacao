//go:build integration

package postgres_test

import (
	"bytes"
	"errors"
	"math"
	"os"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/adapter/postgres"
	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/domain"
	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/platform"
	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/usecase"
)

// The elevation tables are global reference data and the tests share one database, so every
// test uses its own patch of the planet (far from Brazil and from the others) and its own
// tile key. The keys only have to be unique: the database does not tie them to the geometry.

var syntheticModel = domain.ElevationModel{Source: "synthetic", ResolutionM: 30, Version: "test"}

// ramp is a synthetic DEM that rises eastwards by slopePct percent: the slope is known, so the
// whole chain (ingest, tiling, clipping, mask, domain calculation) can be checked against it.
type ramp struct {
	ulx, uly   float64 // upper-left corner, degrees
	cols, rows int
	scale      float64 // pixel size, degrees
	base       float64 // elevation of the first column, meters
	slopePct   float64
}

// cellWidthM is the east-west size of a pixel at the given latitude, as the domain computes it.
func (r ramp) cellWidthM(lat float64) float64 {
	return r.scale * 111_320 * math.Cos(lat*math.Pi/180)
}

func (r ramp) stepM(lat float64) float64 { return r.slopePct / 100 * r.cellWidthM(lat) }

// elevationAt is the elevation of the pixel that holds the longitude (columns count from 1).
func (r ramp) elevationAt(lon, lat float64) float64 {
	column := math.Floor((lon-r.ulx)/r.scale) + 1
	return r.base + column*r.stepM(lat)
}

// geotiff builds the GeoTIFF with PostGIS itself (api_app only needs SELECT for that).
func (r ramp) geotiff(t *testing.T, lat float64) []byte {
	t.Helper()
	expression := "(" + strconv.FormatFloat(r.base, 'f', -1, 64) + ") + [rast.x] * " + strconv.FormatFloat(r.stepM(lat), 'f', -1, 64)
	return mapAlgebraGeoTIFF(t, r, 4326, expression)
}

func mapAlgebraGeoTIFF(t *testing.T, r ramp, srid int, expression string) []byte {
	t.Helper()
	var tiff []byte
	err := testPool.QueryRow(ctx(), `
SELECT ST_AsGDALRaster(
  ST_MapAlgebra(
    ST_AddBand(ST_MakeEmptyRaster($1::int, $2::int, $3::float8, $4::float8, $5::float8, $6::float8, 0::float8, 0::float8, $7::int),
               '32BF'::text, 0::float8, NULL::float8),
    1, '32BF'::text, $8::text),
  'GTiff')`,
		r.cols, r.rows, r.ulx, r.uly, r.scale, -r.scale, srid, expression).Scan(&tiff)
	must(t, err)
	return tiff
}

func ingest(t *testing.T, key domain.ElevationTileKey, tiff []byte) {
	t.Helper()
	must(t, testDB.IngestTile(ctx(), key, syntheticModel, bytes.NewReader(tiff)))
}

func tileCounts(t *testing.T, key domain.ElevationTileKey) (sources, blocks, declared int) {
	t.Helper()
	must(t, testPool.QueryRow(ctx(), `SELECT count(*)::int, coalesce(max(block_count), 0)::int FROM elevation_sources WHERE tile_key = $1`, string(key)).Scan(&sources, &declared))
	must(t, testPool.QueryRow(ctx(), `SELECT count(*)::int FROM elevation_blocks WHERE tile_key = $1`, string(key)).Scan(&blocks))
	return sources, blocks, declared
}

// fieldBox draws a field of w x h degrees with its south-west corner at (lon, lat).
func fieldBox(lon, lat, w, h float64) domain.Polygon {
	return domain.Polygon{{{lon, lat}, {lon, lat + h}, {lon + w, lat + h}, {lon + w, lat}, {lon, lat}}}
}

func newFieldAt(t *testing.T, polygon domain.Polygon) (domain.Company, domain.Farm, domain.Field) {
	t.Helper()
	company := newCompany(t)
	farm := farmValue(company, "Fazenda do Relevo")
	insertFarm(t, farm)
	field := fieldValue(farm, "Talhão Relevo", polygon)
	insertField(t, field)
	return company, farm, field
}

func TestElevationFullCycleWithSyntheticRaster(t *testing.T) {
	// 520 x 520 px = 3 x 3 blocks of 256 (the last ones 8 px wide). The field straddles the
	// seam between blocks both ways (x = 256 px at lon 10.0768, y = 256 px at lat 40.0792), so
	// the union of the clipped blocks is exercised.
	dem := ramp{ulx: 10.0, uly: 40.156, cols: 520, rows: 520, scale: 0.0003, base: 800, slopePct: 10}
	key := domain.NewElevationTileKey(40, 10)
	const lat = 40.0795
	ingest(t, key, dem.geotiff(t, lat))

	sources, blocks, declared := tileCounts(t, key)
	if sources != 1 || blocks != 9 || declared != 9 {
		t.Fatalf("sources=%d blocks=%d declared=%d, want 1, 9, 9", sources, blocks, declared)
	}
	has, err := testDB.HasTile(ctx(), key)
	must(t, err)
	if !has {
		t.Fatal("HasTile = false after the ingest")
	}
	if other, err := testDB.HasTile(ctx(), domain.NewElevationTileKey(-33, -70)); err != nil || other {
		t.Fatalf("HasTile of an unknown key = %v, %v", other, err)
	}

	company, _, field := newFieldAt(t, fieldBox(10.0755, 40.078, 0.003, 0.003))

	const radius = 500.0
	minLon, minLat, maxLon, maxLat, err := testDB.FieldBounds(ctx(), company.ID, field.ID, radius)
	must(t, err)
	// 500 m is 0.0059 degrees of latitude and 0.0059 / cos(40) = 0.0059 * 1.31 of longitude.
	if !(minLat < 40.078-0.0040 && minLat > 40.078-0.0050 && maxLat > 40.081+0.0040 && maxLat < 40.081+0.0050) {
		t.Fatalf("latitude bounds %v..%v do not match a 500 m margin", minLat, maxLat)
	}
	if !(minLon < 10.0755-0.0050 && minLon > 10.0755-0.0070 && maxLon > 10.0785+0.0050 && maxLon < 10.0785+0.0070) {
		t.Fatalf("longitude bounds %v..%v do not match a 500 m margin", minLon, maxLon)
	}

	window, err := testDB.Window(ctx(), company.ID, field.ID, radius)
	must(t, err)
	cells := window.Rows * window.Cols
	if window.Rows < 30 || window.Cols < 30 || len(window.Values) != cells || len(window.Inside) != cells {
		t.Fatalf("window %dx%d with %d values and %d mask cells", window.Rows, window.Cols, len(window.Values), len(window.Inside))
	}
	if math.Abs(window.ScaleXDeg-0.0003) > 1e-12 || math.Abs(window.ScaleYDeg+0.0003) > 1e-12 {
		t.Fatalf("scale = %v, %v", window.ScaleXDeg, window.ScaleYDeg)
	}
	if !math.IsNaN(window.Values[0]) {
		t.Fatalf("the corner of the box is outside the buffer and must be nodata, got %v", window.Values[0])
	}

	inside := 0
	for row := 0; row < window.Rows; row++ {
		for col := 0; col < window.Cols; col++ {
			i := row*window.Cols + col
			if !window.Inside[i] {
				continue
			}
			inside++
			// The mask and the values describe the same cell: the center of every inside cell
			// is in the polygon and its elevation is the one of that pixel of the ramp.
			lon := window.OriginLon + (float64(col)+0.5)*window.ScaleXDeg
			cellLat := window.OriginLat + (float64(row)+0.5)*window.ScaleYDeg
			if lon < 10.0755 || lon > 10.0785 || cellLat < 40.078 || cellLat > 40.081 {
				t.Fatalf("inside cell (%d,%d) at %v,%v is outside the polygon", row, col, lon, cellLat)
			}
			want := dem.elevationAt(lon, lat)
			if got := window.Values[i]; math.Abs(got-want) > 0.05 {
				t.Fatalf("elevation at (%d,%d) = %v, want %v", row, col, got, want)
			}
		}
	}
	// 0.003 x 0.003 degrees = ~10 x 10 pixels.
	if inside < 80 || inside > 120 {
		t.Fatalf("%d cells inside the field, want ~100", inside)
	}

	calc, err := domain.CalculateTerrain(window, domain.DefaultTerrainRules(), syntheticModel, time.Now().UTC())
	must(t, err)
	// Horn over a plane returns the slope of the plane. The domain sizes the cell with the mean
	// latitude of the window and the DEM with the latitude of the field center: tolerance 3 %.
	if got := calc.Measurement.SlopeMeanPct; math.Abs(got-10) > 0.3 {
		t.Fatalf("mean slope = %v %%, want 10 +/- 0.3", got)
	}
	if got := *calc.Measurement.SlopeP90Pct; math.Abs(got-10) > 0.3 {
		t.Fatalf("p90 slope = %v %%, want 10 +/- 0.3", got)
	}
	if *calc.Measurement.Aspect != domain.AspectWest {
		t.Fatalf("aspect = %s, want west (the ramp rises eastwards)", *calc.Measurement.Aspect)
	}
}

func TestIngestTileIsIdempotentAndSafeUnderRace(t *testing.T) {
	key := domain.NewElevationTileKey(41, 11)
	dem := ramp{ulx: 11.0, uly: 41.09, cols: 300, rows: 300, scale: 0.0003, base: 100, slopePct: 5}
	tiff := dem.geotiff(t, 41.04)

	ingest(t, key, tiff)
	ingest(t, key, tiff)
	if sources, blocks, declared := tileCounts(t, key); sources != 1 || blocks != 4 || declared != 4 {
		t.Fatalf("after two ingests: sources=%d blocks=%d declared=%d, want 1, 4, 4", sources, blocks, declared)
	}

	raceKey := domain.NewElevationTileKey(42, 11)
	raceTIFF := ramp{ulx: 11.0, uly: 42.09, cols: 300, rows: 300, scale: 0.0003, base: 100, slopePct: 5}.geotiff(t, 42.04)
	var wg sync.WaitGroup
	errs := make([]error, 4)
	for i := range errs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs[i] = testDB.IngestTile(ctx(), raceKey, syntheticModel, bytes.NewReader(raceTIFF))
		}()
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Fatalf("concurrent ingest %d failed: %v", i, err)
		}
	}
	if sources, blocks, declared := tileCounts(t, raceKey); sources != 1 || blocks != 4 || declared != 4 {
		t.Fatalf("after a race: sources=%d blocks=%d declared=%d, want 1, 4, 4", sources, blocks, declared)
	}
}

func TestIngestTileRefusesUnexpectedRasters(t *testing.T) {
	cases := []struct {
		name string
		key  domain.ElevationTileKey
		tiff func() []byte
	}{
		{"web mercator", domain.NewElevationTileKey(43, 12), func() []byte {
			return mapAlgebraGeoTIFF(t, ramp{ulx: 100, uly: 100, cols: 20, rows: 20, scale: 30}, 3857, "1")
		}},
		{"two bands", domain.NewElevationTileKey(44, 12), func() []byte {
			var tiff []byte
			must(t, testPool.QueryRow(ctx(), `
SELECT ST_AsGDALRaster(
  ST_AddBand(ST_AddBand(ST_MakeEmptyRaster(20, 20, 12.0, 44.1, 0.0003, -0.0003, 0, 0, 4326), '32BF'::text, 1::float8, NULL::float8),
             '32BF'::text, 2::float8, NULL::float8), 'GTiff')`).Scan(&tiff))
			return tiff
		}},
		{"not a raster", domain.NewElevationTileKey(45, 12), func() []byte { return []byte("this is not a GeoTIFF") }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := testDB.IngestTile(ctx(), tc.key, syntheticModel, bytes.NewReader(tc.tiff()))
			if !errors.Is(err, postgres.ErrInvalidElevationRaster) {
				t.Fatalf("error = %v, want ErrInvalidElevationRaster", err)
			}
			if sources, blocks, _ := tileCounts(t, tc.key); sources != 0 || blocks != 0 {
				t.Fatalf("a refused raster left %d sources and %d blocks", sources, blocks)
			}
		})
	}
}

func TestWindowIsUnavailableWithoutFullCoverage(t *testing.T) {
	t.Run("no block at all", func(t *testing.T) {
		company, _, field := newFieldAt(t, fieldBox(100.0, 10.0, 0.003, 0.003))
		_, err := testDB.Window(ctx(), company.ID, field.ID, 500)
		requireIs(t, err, usecase.ErrElevationUnavailable)
	})

	t.Run("block covers only part of the buffer", func(t *testing.T) {
		// The raster ends at lon 20.06; the field straddles that edge.
		dem := ramp{ulx: 20.0, uly: 50.03, cols: 200, rows: 200, scale: 0.0003, base: 500, slopePct: 8}
		ingest(t, domain.NewElevationTileKey(49, 20), dem.geotiff(t, 50.0))
		company, _, field := newFieldAt(t, fieldBox(20.0585, 50.0, 0.003, 0.003))
		_, err := testDB.Window(ctx(), company.ID, field.ID, 500)
		requireIs(t, err, usecase.ErrElevationUnavailable)

		// The same raster is enough for a field far from the edge.
		company, _, inner := newFieldAt(t, fieldBox(20.025, 49.99, 0.003, 0.003))
		if _, err := testDB.Window(ctx(), company.ID, inner.ID, 500); err != nil {
			t.Fatalf("field well inside the raster: %v", err)
		}
	})

	t.Run("blocks without data under the field", func(t *testing.T) {
		// Every pixel is -32768, the nodata the ingestion declares for a band without one.
		dem := ramp{ulx: 30.0, uly: 55.03, cols: 200, rows: 200, scale: 0.0003}
		ingest(t, domain.NewElevationTileKey(54, 30), mapAlgebraGeoTIFF(t, dem, 4326, "-32768"))
		company, _, field := newFieldAt(t, fieldBox(30.025, 54.99, 0.003, 0.003))
		_, err := testDB.Window(ctx(), company.ID, field.ID, 500)
		// The blocks are there and cover the field: the problem is too little data, not a missing block.
		requireIs(t, err, domain.ErrInsufficientElevationData)
		if errors.Is(err, usecase.ErrElevationUnavailable) {
			t.Fatalf("a field over nodata cells is not an unavailable block: %v", err)
		}
	})
}

func TestElevationIsolatesCompanies(t *testing.T) {
	company, _, field := newFieldAt(t, fieldBox(101.0, 11.0, 0.003, 0.003))
	other := newCompany(t)
	if _, _, _, _, err := testDB.FieldBounds(ctx(), other.ID, field.ID, 100); !errors.Is(err, usecase.ErrNotFound) {
		t.Fatalf("FieldBounds of another company: %v", err)
	}
	if _, err := testDB.Window(ctx(), other.ID, field.ID, 100); !errors.Is(err, usecase.ErrNotFound) {
		t.Fatalf("Window of another company: %v", err)
	}
	if _, _, _, _, err := testDB.FieldBounds(ctx(), company.ID, field.ID, 100); err != nil {
		t.Fatalf("FieldBounds of the owner: %v", err)
	}
	if _, _, _, _, err := testDB.FieldBounds(ctx(), company.ID, domain.FieldID(newID()), 100); !errors.Is(err, usecase.ErrNotFound) {
		t.Fatalf("FieldBounds of an unknown field: %v", err)
	}
	if _, _, _, _, err := testDB.FieldBounds(ctx(), company.ID, field.ID, -1); err == nil {
		t.Fatal("a negative radius must be refused")
	}
}

// slowPool is a pool whose statement_timeout is so short that any real query is canceled by the server.
func slowPool(t *testing.T) *postgres.Database {
	t.Helper()
	cfg, err := platform.LoadDatabaseConfig(func(key string) (string, bool) {
		switch key {
		case "API_DATABASE_URL":
			return os.Getenv("API_TEST_DATABASE_URL"), true
		case "API_DB_STATEMENT_TIMEOUT":
			return "1ms", true
		}
		return "", false
	})
	must(t, err)
	pool, err := postgres.NewPool(ctx(), cfg)
	must(t, err)
	t.Cleanup(pool.Close)
	return postgres.NewDatabase(pool, postgres.Options{SessionTouchInterval: testTouchInterval})
}

func TestWindowReportsAStatementTimeoutAsATimeout(t *testing.T) {
	dem := ramp{ulx: 60.0, uly: 60.03, cols: 200, rows: 200, scale: 0.0003, base: 500, slopePct: 8}
	ingest(t, domain.NewElevationTileKey(59, 60), dem.geotiff(t, 60.0))
	company, _, field := newFieldAt(t, fieldBox(60.025, 59.99, 0.003, 0.003))

	_, err := slowPool(t).Window(ctx(), company.ID, field.ID, 500)
	requireIs(t, err, usecase.ErrElevationTimeout)
	if errors.Is(err, usecase.ErrElevationUnavailable) {
		t.Fatalf("a canceled statement is a timeout, not an unavailable block: %v", err)
	}
}
