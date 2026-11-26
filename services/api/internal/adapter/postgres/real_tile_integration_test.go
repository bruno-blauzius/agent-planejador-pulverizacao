//go:build integration && realtile

// Medição OPT-IN do fluxo ponta a ponta do relevo do talhão com um bloco REAL do Copernicus
// DEM GLO-30 (ADR 0021). Baixa ~42 MB do bucket público (um GET, sem enviar dado nenhum do
// projeto), ingere no Postgres descartável e mede tempo, memória, disco e plausibilidade, e
// confere a declividade do Go contra o ST_Slope do PostGIS.
//
// NÃO roda em `go test`, em `make api-test-integration` nem em `scripts/integration-test.sh`:
// exige as build tags `integration realtile` E a variável REAL_TILE=1, e usa a mesma
// infraestrutura de banco descartável (papel api_app, sem tocar no agro-postgres).
//
// Como rodar (da pasta services/api, com o Docker ligado e a imagem agentes-agro/postgres:dev):
//
//	bash scripts/real-tile-test.sh
//
// O script sobe o banco descartável, define REAL_TILE=1 e REAL_TILE_DB_CONTAINER (para o
// `docker stats`) e roda:
//
//	go test -count=1 -tags 'integration realtile' -run TestRealCopernicusTile -v -timeout 45m ./internal/adapter/postgres/...
package postgres_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/adapter/copernicus"
	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/domain"
	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/usecase"
)

const (
	realTileKey = domain.ElevationTileKey("S22W047") // lat -22..-21, lon -47..-46
	// Perto de Guaxupé-MG; o teste confere a plausibilidade do resultado e o ponto pode ser trocado.
	realFieldLon, realFieldLat = -46.71, -21.30
	realFieldSideM             = 640.0 // ~41 ha
	oceanLon, oceanLat         = -29.5, -29.5
)

// windowSQL is the GetElevationWindow query of queries/elevation.sql with positional parameters
// ($1 radius, $2 company, $3 field), to run it under EXPLAIN.
const windowSQL = `
WITH f AS (
    SELECT x.polygon AS poly,
           ST_Buffer(x.polygon::geography, $1::float8)::geometry AS buf
    FROM fields x
    WHERE x.company_id = $2 AND x.id = $3 AND x.deleted_at IS NULL
), hits AS (
    SELECT b.rast, f.buf
    FROM f
    JOIN elevation_blocks b ON st_convexhull(b.rast) && f.buf AND ST_Intersects(st_convexhull(b.rast), f.buf)
), cover AS (
    SELECT coalesce(ST_Covers(ST_Union(ST_Expand(st_convexhull(rast), 1e-9)), (SELECT buf FROM f)), false) AS covered
    FROM hits
), win AS (
    SELECT ST_Union(ST_Clip(rast, 1, buf, true)) AS rast FROM hits
), mask AS (
    SELECT ST_Clip(ST_AddBand(ST_MakeEmptyRaster(w.rast), '8BUI'::text, 1, 0), 1, f.poly, 0::float8, false) AS rast
    FROM win w, f
    WHERE w.rast IS NOT NULL
)
SELECT (SELECT covered FROM cover)::bool AS covered,
       coalesce(ST_Width(w.rast), 0)::int AS cols,
       coalesce(ST_Height(w.rast), 0)::int AS rows,
       coalesce(ARRAY(SELECT coalesce(v, 'NaN'::float8) FROM unnest(ST_DumpValues(w.rast, 1)) WITH ORDINALITY AS u(v, i) ORDER BY i), '{}')::float8[] AS elevations,
       coalesce(ARRAY(SELECT coalesce(v, 0::float8) FROM unnest(ST_DumpValues(m.rast, 1)) WITH ORDINALITY AS u(v, i) ORDER BY i), '{}')::float8[] AS inside
FROM win w
LEFT JOIN mask m ON true`

// slopeSQL is the independent check: the same window (field + $3 m), reprojected to UTM 23S
// SIRGAS 2000 (EPSG:31983) on a metric grid of $5 m, slope by ST_Slope in percent, then the
// cells whose center is inside the field ($4 is the resampling algorithm).
const slopeSQL = `
WITH f AS (
    SELECT x.polygon AS poly, ST_Buffer(x.polygon::geography, $3::float8)::geometry AS buf
    FROM fields x WHERE x.company_id = $1 AND x.id = $2
), hits AS (
    SELECT b.rast, f.buf FROM f
    JOIN elevation_blocks b ON st_convexhull(b.rast) && f.buf AND ST_Intersects(st_convexhull(b.rast), f.buf)
), win AS (
    SELECT ST_Union(ST_Clip(rast, 1, buf, true)) AS rast FROM hits
), proj AS (
    SELECT ST_Transform(rast, 31983, $5::float8, -$5::float8, $4::text) AS rast FROM win
), slope AS (
    SELECT ST_Slope(rast, 1, '32BF', 'PERCENT') AS rast FROM proj
), clipped AS (
    SELECT ST_Clip(s.rast, 1, ST_Transform(f.poly, 31983), true) AS rast FROM slope s, f
), elev AS (
    SELECT ST_Clip(p.rast, 1, ST_Transform(f.poly, 31983), true) AS rast FROM proj p, f
)
SELECT (ST_SummaryStats(c.rast, 1, true)).count::int,
       (ST_SummaryStats(c.rast, 1, true)).mean::float8,
       (ST_SummaryStats(c.rast, 1, true)).max::float8,
       ST_Quantile(c.rast, 1, true, 0.9)::float8,
       (ST_SummaryStats(e.rast, 1, true)).min::float8,
       (ST_SummaryStats(e.rast, 1, true)).max::float8,
       ST_ScaleX(c.rast)::float8
FROM clipped c, elev e`

// ---- measurement plumbing ----------------------------------------------------------

type timedValue struct {
	at    time.Time
	value float64
}

// recorder keeps the timestamps the wrappers see.
type recorder struct {
	mu            sync.Mutex
	fetchStarts   map[domain.ElevationTileKey]time.Time
	headersAt     map[domain.ElevationTileKey]time.Time
	eofAt         map[domain.ElevationTileKey]time.Time
	bytesRead     map[domain.ElevationTileKey]int64
	ingestStarts  map[domain.ElevationTileKey]time.Time
	ingestEnds    map[domain.ElevationTileKey]time.Time
	hasTile       map[domain.ElevationTileKey][]time.Duration
	hasTileFirst  map[domain.ElevationTileKey]time.Time
	windows       []time.Duration
	bounds        []time.Duration
	fetches       []domain.ElevationTileKey
	fetchFailures []string
}

func newRecorder() *recorder {
	return &recorder{
		fetchStarts: map[domain.ElevationTileKey]time.Time{}, headersAt: map[domain.ElevationTileKey]time.Time{},
		eofAt: map[domain.ElevationTileKey]time.Time{}, bytesRead: map[domain.ElevationTileKey]int64{},
		ingestStarts: map[domain.ElevationTileKey]time.Time{}, ingestEnds: map[domain.ElevationTileKey]time.Time{},
		hasTile: map[domain.ElevationTileKey][]time.Duration{}, hasTileFirst: map[domain.ElevationTileKey]time.Time{},
	}
}

func (r *recorder) with(fn func()) {
	r.mu.Lock()
	defer r.mu.Unlock()
	fn()
}

type timedSource struct {
	usecase.ElevationSource
	rec *recorder
}

func (s *timedSource) FetchTile(ctx context.Context, key domain.ElevationTileKey) (io.ReadCloser, error) {
	start := time.Now()
	s.rec.with(func() { s.rec.fetchStarts[key] = start; s.rec.fetches = append(s.rec.fetches, key) })
	body, err := s.ElevationSource.FetchTile(ctx, key)
	if err != nil {
		s.rec.with(func() {
			s.rec.fetchFailures = append(s.rec.fetchFailures, fmt.Sprintf("%s after %s: %v", key, time.Since(start).Round(time.Millisecond), err))
		})
		return nil, err
	}
	s.rec.with(func() { s.rec.headersAt[key] = time.Now() })
	return &countingBody{ReadCloser: body, key: key, rec: s.rec}, nil
}

type countingBody struct {
	io.ReadCloser
	key domain.ElevationTileKey
	rec *recorder
}

func (b *countingBody) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	b.rec.with(func() { b.rec.bytesRead[b.key] += int64(n) })
	if errors.Is(err, io.EOF) {
		b.rec.with(func() { b.rec.eofAt[b.key] = time.Now() })
	}
	return n, err
}

type timedElevation struct {
	usecase.ElevationStore
	rec *recorder
}

func (e *timedElevation) HasTile(ctx context.Context, key domain.ElevationTileKey) (bool, error) {
	start := time.Now()
	has, err := e.ElevationStore.HasTile(ctx, key)
	e.rec.with(func() {
		if _, seen := e.rec.hasTileFirst[key]; !seen || len(e.rec.hasTile[key]) == 0 {
			e.rec.hasTileFirst[key] = start
		}
		e.rec.hasTile[key] = append(e.rec.hasTile[key], time.Since(start))
	})
	return has, err
}

func (e *timedElevation) IngestTile(ctx context.Context, key domain.ElevationTileKey, model domain.ElevationModel, body io.Reader) error {
	e.rec.with(func() { e.rec.ingestStarts[key] = time.Now() })
	err := e.ElevationStore.IngestTile(ctx, key, model, body)
	e.rec.with(func() { e.rec.ingestEnds[key] = time.Now() })
	return err
}

func (e *timedElevation) FieldBounds(ctx context.Context, c domain.CompanyID, f domain.FieldID, radius float64) (float64, float64, float64, float64, error) {
	start := time.Now()
	a, b, c2, d, err := e.ElevationStore.FieldBounds(ctx, c, f, radius)
	e.rec.with(func() { e.rec.bounds = append(e.rec.bounds, time.Since(start)) })
	return a, b, c2, d, err
}

func (e *timedElevation) Window(ctx context.Context, c domain.CompanyID, f domain.FieldID, radius float64) (domain.ElevationWindow, error) {
	start := time.Now()
	w, err := e.ElevationStore.Window(ctx, c, f, radius)
	e.rec.with(func() { e.rec.windows = append(e.rec.windows, time.Since(start)) })
	return w, err
}

type sampler struct {
	mu        sync.Mutex
	heapAlloc []timedValue
	heapInuse []timedValue
	sys       []timedValue
	pgMem     []timedValue // MiB
	pgCPU     []timedValue // percent
	stop      chan struct{}
	wg        sync.WaitGroup
}

func startSampler(container string) *sampler {
	s := &sampler{stop: make(chan struct{})}
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-s.stop:
				return
			case <-ticker.C:
				var m runtime.MemStats
				runtime.ReadMemStats(&m)
				now := time.Now()
				s.mu.Lock()
				s.heapAlloc = append(s.heapAlloc, timedValue{now, float64(m.HeapAlloc)})
				s.heapInuse = append(s.heapInuse, timedValue{now, float64(m.HeapInuse)})
				s.sys = append(s.sys, timedValue{now, float64(m.Sys)})
				s.mu.Unlock()
			}
		}
	}()
	if container != "" {
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			for {
				select {
				case <-s.stop:
					return
				default:
				}
				// `docker stats --no-stream` itself takes ~1-2 s, so the loop samples about once a second.
				out, err := exec.CommandContext(context.Background(), "docker", "stats", "--no-stream", "--format", "{{.MemUsage}}|{{.CPUPerc}}", container).Output()
				if err == nil {
					if mem, cpu, ok := parseDockerStats(string(out)); ok {
						now := time.Now()
						s.mu.Lock()
						s.pgMem = append(s.pgMem, timedValue{now, mem})
						s.pgCPU = append(s.pgCPU, timedValue{now, cpu})
						s.mu.Unlock()
					}
				}
				select {
				case <-s.stop:
					return
				case <-time.After(200 * time.Millisecond):
				}
			}
		}()
	}
	return s
}

func (s *sampler) close() {
	close(s.stop)
	s.wg.Wait()
}

func peakIn(series []timedValue, from, to time.Time) (peak float64, n int) {
	for _, v := range series {
		if v.at.Before(from) || v.at.After(to) {
			continue
		}
		n++
		peak = math.Max(peak, v.value)
	}
	return peak, n
}

func parseDockerStats(out string) (memMiB, cpuPct float64, ok bool) {
	parts := strings.Split(strings.TrimSpace(out), "|")
	if len(parts) != 2 {
		return 0, 0, false
	}
	usage := strings.TrimSpace(strings.Split(parts[0], "/")[0])
	units := []struct {
		suffix string
		toMiB  float64
	}{{"GiB", 1024}, {"MiB", 1}, {"KiB", 1.0 / 1024}, {"GB", 1000 * 1000 * 1000 / 1048576.0}, {"MB", 1000 * 1000 / 1048576.0}, {"kB", 1000 / 1048576.0}, {"B", 1.0 / 1048576}}
	for _, u := range units {
		if strings.HasSuffix(usage, u.suffix) {
			v, err := strconv.ParseFloat(strings.TrimSuffix(usage, u.suffix), 64)
			if err != nil {
				return 0, 0, false
			}
			memMiB = v * u.toMiB
			cpu, err := strconv.ParseFloat(strings.TrimSuffix(strings.TrimSpace(parts[1]), "%"), 64)
			if err != nil {
				return 0, 0, false
			}
			return memMiB, cpu, true
		}
	}
	return 0, 0, false
}

type idGenerator struct{}

func (idGenerator) NewID() string { return newID() }

func mib(bytes float64) float64 { return bytes / (1 << 20) }

func percentile(values []time.Duration, p float64) time.Duration {
	sorted := append([]time.Duration(nil), values...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	return sorted[int(math.Ceil(p*float64(len(sorted))))-1]
}

// ---- fixtures ------------------------------------------------------------------------

func squareAround(lon, lat, sideM float64) domain.Polygon {
	const metersPerDegreeLat = 110574.0
	metersPerDegreeLon := 111320.0 * math.Cos(lat*math.Pi/180)
	w, h := sideM/metersPerDegreeLon, sideM/metersPerDegreeLat
	return fieldBox(lon-w/2, lat-h/2, w, h)
}

type realFixture struct {
	company domain.Company
	user    domain.User
	farm    domain.Farm
	field   domain.Field
}

func newRealField(t *testing.T, company domain.Company, user domain.User, lon, lat float64, name string) realFixture {
	t.Helper()
	farm := farmValue(company, "Fazenda "+name)
	perimeter := domain.Perimeter{squareAround(lon, lat, 2500)}
	farm.Perimeter = &perimeter
	insertFarm(t, farm)
	field := fieldValue(farm, name, squareAround(lon, lat, realFieldSideM))
	field.Block = nil
	insertField(t, field)
	return realFixture{company: company, user: user, farm: farm, field: field}
}

func (f realFixture) principal() domain.Principal {
	return domain.Principal{
		UserID: f.user.ID, CompanyID: f.company.ID, Name: f.user.Name, Email: f.user.Email, Role: domain.RoleAdmin,
		Permissions: []domain.Permission{domain.PermRegistryWrite}, FarmScope: domain.AllFarms(),
	}
}

func waitTerrain(t *testing.T, f realFixture, want domain.TerrainStatus, limit time.Duration) (*usecase.StoredTerrain, time.Duration) {
	t.Helper()
	start := time.Now()
	lastLog := start
	for {
		data, err := testDB.ReadFieldTerrain(ctx(), f.company.ID, f.farm.ID, f.field.ID)
		must(t, err)
		if st := data.Terrain; st != nil && (st.Status == domain.TerrainReady || st.Status == domain.TerrainFailed) {
			if st.Status != want {
				t.Fatalf("field %s ended %s (%q), want %s", f.field.Name, st.Status, st.FailureReason, want)
			}
			return st, time.Since(start)
		}
		if time.Since(start) > limit {
			t.Fatalf("field %s not %s after %s", f.field.Name, want, limit)
		}
		if time.Since(lastLog) >= 5*time.Second {
			lastLog = time.Now()
			status := "none"
			if data.Terrain != nil {
				status = string(data.Terrain.Status)
			}
			var m runtime.MemStats
			runtime.ReadMemStats(&m)
			t.Logf("  progress: %s %s after %s, go heap %.0f MiB", f.field.Name, status, time.Since(start).Round(time.Second), mib(float64(m.HeapAlloc)))
		}
		time.Sleep(200 * time.Millisecond)
	}
}

func asJSON(v any) string {
	out, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Sprintf("<%v>", err)
	}
	return string(out)
}

// ---- the test ------------------------------------------------------------------------

func TestRealCopernicusTile(t *testing.T) {
	if os.Getenv("REAL_TILE") != "1" {
		t.Skip("opt-in: set REAL_TILE=1 (see the header of this file)")
	}
	drainTerrainQueue(t)

	rec := newRecorder()
	rules := domain.DefaultTerrainRules()
	client := copernicus.NewClient(copernicus.Config{}, nil)
	smp := startSampler(os.Getenv("REAL_TILE_DB_CONTAINER"))
	defer smp.close()

	worker, err := usecase.NewTerrainWorker(usecase.TerrainWorkerDeps{
		Tx: testDB, Elevation: &timedElevation{ElevationStore: testDB, rec: rec}, Source: &timedSource{ElevationSource: client, rec: rec},
		Clock: microClock{}, Rules: rules,
	}, usecase.TerrainWorkerConfig{}) // defaults: what production runs with
	must(t, err)
	t.Cleanup(worker.Stop)
	must(t, worker.Start(ctx()))

	calculate := usecase.NewCalculateFarmTerrain(usecase.CalculateFarmTerrainDeps{
		Terrain: testDB, Tx: testDB, Wakeup: worker, Clock: microClock{}, IDs: idGenerator{}, Rules: rules,
	})

	company := newCompany(t)
	user := newUser(t, company)
	primary := newRealField(t, company, user, realFieldLon, realFieldLat, "Talhao Real Guaxupe")

	var areaHa float64
	must(t, testPool.QueryRow(ctx(), `SELECT ST_Area(polygon::geography)/10000 FROM fields WHERE id = $1`, string(primary.field.ID)).Scan(&areaHa))
	var version, gdal string
	must(t, testPool.QueryRow(ctx(), `SELECT PostGIS_Lib_Version(), PostGIS_GDAL_Version()`).Scan(&version, &gdal))
	t.Logf("SETUP: field %.2f ha at (%.4f, %.4f); PostGIS %s, %s; rules: radius %.0f m, min cells %d", areaHa, realFieldLon, realFieldLat, version, gdal, rules.PositionRadiusM, rules.MinValidCells)
	var baseMem float64
	var baseHeap runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&baseHeap)
	time.Sleep(2500 * time.Millisecond)
	smp.mu.Lock()
	if n := len(smp.pgMem); n > 0 {
		baseMem = smp.pgMem[n-1].value
	}
	smp.mu.Unlock()
	t.Logf("BASELINE: go heap %.1f MiB, go sys %.1f MiB, postgres container %.1f MiB", mib(float64(baseHeap.HeapAlloc)), mib(float64(baseHeap.Sys)), baseMem)

	// ---- (1) first calculation: real download, ingestion, window, calculation -------------
	started := time.Now()
	view, err := calculate.Execute(ctx(), usecase.CalculateFarmTerrainInput{Actor: primary.principal(), FarmID: primary.farm.ID})
	must(t, err)
	t.Logf("CalculateFarmTerrain accepted in %s (progress %+v)", time.Since(started).Round(time.Millisecond), view.Progress)
	stored, _ := waitTerrain(t, primary, domain.TerrainReady, 15*time.Minute)
	totalToReady := time.Since(started)
	if stored.Calculation == nil {
		t.Fatal("ready without calculation")
	}

	rec.mu.Lock()
	fetchStart, headers, eof := rec.fetchStarts[realTileKey], rec.headersAt[realTileKey], rec.eofAt[realTileKey]
	ingestStart, ingestEnd, size := rec.ingestStarts[realTileKey], rec.ingestEnds[realTileKey], rec.bytesRead[realTileKey]
	hasFirst, hasDur := rec.hasTileFirst[realTileKey], append([]time.Duration(nil), rec.hasTile[realTileKey]...)
	boundsDur, windowDur := append([]time.Duration(nil), rec.bounds...), append([]time.Duration(nil), rec.windows...)
	fetches := append([]domain.ElevationTileKey(nil), rec.fetches...)
	rec.mu.Unlock()
	download := eof.Sub(fetchStart)
	dbIngest := ingestEnd.Sub(eof)
	t.Logf("MEASURE (a) download: FetchTile headers in %s; body complete after %s total; %d bytes (%.2f MiB) => %.2f MiB/s",
		headers.Sub(fetchStart).Round(time.Millisecond), download.Round(time.Millisecond), size, mib(float64(size)), mib(float64(size))/download.Seconds())
	t.Logf("MEASURE (b) ingestion in database (IngestTile after the last byte: inspect+decode+ST_Tile+inserts): %s; IngestTile call total %s; ensureTile total (HasTile start -> ingest end) %s",
		dbIngest.Round(time.Millisecond), ingestEnd.Sub(ingestStart).Round(time.Millisecond), ingestEnd.Sub(hasFirst).Round(time.Millisecond))
	t.Logf("  HasTile calls: %v; FieldBounds calls: %v; Window calls: %v", hasDur, boundsDur, windowDur)
	smp.mu.Lock()
	for _, w := range []struct {
		name     string
		from, to time.Time
	}{{"download", fetchStart, eof}, {"ingestion", eof, ingestEnd}, {"whole ensureTile", fetchStart, ingestEnd}} {
		heap, _ := peakIn(smp.heapAlloc, w.from, w.to)
		inuse, _ := peakIn(smp.heapInuse, w.from, w.to)
		sys, n := peakIn(smp.sys, w.from, w.to)
		pg, pgn := peakIn(smp.pgMem, w.from, w.to)
		cpu, _ := peakIn(smp.pgCPU, w.from, w.to)
		t.Logf("MEASURE (c) peak during %-17s: go HeapAlloc %.0f MiB, HeapInuse %.0f MiB, Sys %.0f MiB (%d samples); postgres container %.0f MiB, CPU peak %.0f%% (%d samples)",
			w.name, mib(heap), mib(inuse), mib(sys), n, pg, cpu, pgn)
	}
	smp.mu.Unlock()
	t.Logf("MEASURE (f) total from CalculateFarmTerrain to ready: %s (field status ready)", totalToReady.Round(time.Millisecond))
	t.Logf("fetches from the source so far: %v", fetches)

	// ---- (d) disk ----------------------------------------------------------------------
	var blocks, declared int
	var total, heap, indexes, toast int64
	var avgCol, sumCol float64
	must(t, testPool.QueryRow(ctx(), `SELECT count(*)::int, coalesce(avg(pg_column_size(rast)),0)::float8, coalesce(sum(pg_column_size(rast)),0)::float8 FROM elevation_blocks WHERE tile_key = $1`, string(realTileKey)).Scan(&blocks, &avgCol, &sumCol))
	must(t, testPool.QueryRow(ctx(), `SELECT block_count FROM elevation_sources WHERE tile_key = $1`, string(realTileKey)).Scan(&declared))
	must(t, testPool.QueryRow(ctx(), `
		SELECT pg_total_relation_size('elevation_blocks'), pg_relation_size('elevation_blocks'), pg_indexes_size('elevation_blocks'),
		       coalesce((SELECT pg_total_relation_size(reltoastrelid) FROM pg_class WHERE oid = 'elevation_blocks'::regclass), 0)`).Scan(&total, &heap, &indexes, &toast))
	t.Logf("MEASURE (d) disk: elevation_blocks rows=%d, elevation_sources.block_count=%d; pg_total_relation_size=%.2f MiB (heap %.2f MiB, indexes %.2f MiB, toast+idx %.2f MiB); avg stored raster per block %.1f KiB (sum %.2f MiB); raw float32 would be %.2f MiB",
		blocks, declared, mib(float64(total)), mib(float64(heap)), mib(float64(indexes)), mib(float64(toast)), avgCol/1024, mib(sumCol), 3600.0*3600*4/(1<<20))
	var idx []string
	rows, err := testPool.Query(ctx(), `SELECT indexdef FROM pg_indexes WHERE tablename = 'elevation_blocks'`)
	must(t, err)
	for rows.Next() {
		var def string
		must(t, rows.Scan(&def))
		idx = append(idx, def)
	}
	must(t, rows.Err())
	rows.Close()
	t.Logf("  indexes: %v", idx)

	// ---- (e) Window timings and plan ----------------------------------------------------
	var timings []time.Duration
	var window domain.ElevationWindow
	for range 10 {
		s := time.Now()
		window, err = testDB.Window(ctx(), primary.company.ID, primary.field.ID, rules.PositionRadiusM)
		must(t, err)
		timings = append(timings, time.Since(s))
	}
	t.Logf("MEASURE (e) Window (10 runs): min %s, median %s, p90 %s, max %s; grid %dx%d = %d cells, inside=%d",
		percentile(timings, 0.0001), percentile(timings, 0.5), percentile(timings, 0.9), percentile(timings, 1), window.Cols, window.Rows, window.Cols*window.Rows, countInside(window))
	const calcRuns = 500
	calcStart := time.Now()
	for range calcRuns {
		if _, err := domain.CalculateTerrain(window, rules, client.Model(), time.Now()); err != nil {
			t.Fatalf("CalculateTerrain: %v", err)
		}
	}
	t.Logf("MEASURE (e) domain.CalculateTerrain on that window: %s per call (mean of %d runs)", (time.Since(calcStart) / calcRuns).Round(time.Microsecond), calcRuns)
	var analyzed *time.Time
	var liveRows int64
	must(t, testPool.QueryRow(ctx(), `SELECT last_autoanalyze, n_live_tup FROM pg_stat_user_tables WHERE relname = 'elevation_blocks'`).Scan(&analyzed, &liveRows))
	t.Logf("  elevation_blocks stats: n_live_tup=%d, last_autoanalyze=%v", liveRows, analyzed)
	explain(t, "EXPLAIN as the planner chooses now", false, primary, rules.PositionRadiusM)
	explain(t, "EXPLAIN with enable_seqscan=off (is the GiST usable?)", true, primary, rules.PositionRadiusM)

	// ---- (3) the result ------------------------------------------------------------------
	calc := *stored.Calculation
	t.Logf("RESULT stored TerrainCalculation:\n%s", asJSON(struct {
		Measurement      domain.TerrainMeasurement
		ElevationMinM    float64
		ElevationMaxM    float64
		ElevationModel   domain.ElevationModel
		CalculatedAt     time.Time
		Profile          []domain.ProfilePoint
		SlopeHistogramHa map[string]float64 // non-zero bins only
	}{calc.Measurement, calc.ElevationMinM, calc.ElevationMaxM, calc.ElevationModel, calc.CalculatedAt, calc.Profile, nonZeroBins(calc)}))
	t.Logf("  p90=%.3f%% mean=%.3f%% max=%.3f%%", *calc.Measurement.SlopeP90Pct, calc.Measurement.SlopeMeanPct, calc.Measurement.SlopeMaxPct)
	histTotal := 0.0
	for _, ha := range calc.SlopeHistogramHa {
		histTotal += ha
	}
	t.Logf("  histogram total %.3f ha (polygon %.3f ha); area per class: %s", histTotal, areaHa, asJSON(calc.ClassAreas(rules)))
	t.Logf("  rules limits: classes %v, method limits tractor_boom=%v self_propelled=%v airblast=%v", rules.ClassUpperLimitsPct,
		*rules.MethodSlopeLimits[domain.MethodTractorBoom], *rules.MethodSlopeLimits[domain.MethodSelfPropelled], *rules.MethodSlopeLimits[domain.MethodAirblast])
	t.Logf("  MethodFits: %s", asJSON(domain.EvaluateMethodFits(rules, calc.Measurement, domain.SourceCalculated, &calc.SlopeHistogramHa)))

	comparePostGIS(t, "primary field", primary, calc)

	// ---- (g) second field in the same block, then a recalculation of the first --------------
	other := newRealField(t, company, user, -46.55, -21.62, "Talhao Real Segundo")
	fetchedBefore := len(fetches)
	_, err = calculate.Execute(ctx(), usecase.CalculateFarmTerrainInput{Actor: other.principal(), FarmID: other.farm.ID})
	must(t, err)
	secondStored, secondTook := waitTerrain(t, other, domain.TerrainReady, 5*time.Minute)
	comparePostGIS(t, "second field", other, *secondStored.Calculation)
	t.Logf("MEASURE (g) second field (same block): ready in %s; mean slope %.2f%%, elevation %.0f-%.0f m", secondTook.Round(time.Millisecond),
		secondStored.Calculation.Measurement.SlopeMeanPct, secondStored.Calculation.ElevationMinM, secondStored.Calculation.ElevationMaxM)
	recalcStarted := time.Now()
	_, err = calculate.Execute(ctx(), usecase.CalculateFarmTerrainInput{Actor: primary.principal(), FarmID: primary.farm.ID, Scope: usecase.TerrainScopeAll})
	must(t, err)
	// the previous result is "ready" already: wait for the run to finish instead
	deadline := time.Now().Add(5 * time.Minute)
	for {
		farmView, err := testDB.ReadFarmTerrain(ctx(), primary.company.ID, primary.farm.ID)
		must(t, err)
		if farmView.LatestRun != nil && farmView.LatestRun.FinishedAt != nil && farmView.LatestRun.RequestedAt.After(calc.CalculatedAt) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("recalculation did not finish")
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Logf("MEASURE (g) recalculation (scope all) of the first field: finished in %s", time.Since(recalcStarted).Round(time.Millisecond))
	rec.mu.Lock()
	allFetches := append([]domain.ElevationTileKey(nil), rec.fetches...)
	rec.mu.Unlock()
	t.Logf("MEASURE (g) downloads after the first run: %d new (total fetches %v); want 0 new", len(allFetches)-fetchedBefore, allFetches)
	if len(allFetches) != 1 {
		t.Errorf("the block was requested %d times, want exactly once: %v", len(allFetches), allFetches)
	}

	// ---- (5) a block that does not exist: ocean ---------------------------------------------
	oceanKey := domain.NewElevationTileKey(-30, -30)
	s := time.Now()
	body, fetchErr := client.FetchTile(ctx(), oceanKey)
	if body != nil {
		_ = body.Close()
	}
	t.Logf("FAILURE direct FetchTile(%s): err=%v; errors.Is ErrElevationUnavailable=%v; took %s", oceanKey, fetchErr, errors.Is(fetchErr, usecase.ErrElevationUnavailable), time.Since(s).Round(time.Millisecond))
	if !errors.Is(fetchErr, usecase.ErrElevationUnavailable) {
		t.Errorf("FetchTile of a missing block = %v, want ErrElevationUnavailable", fetchErr)
	}
	ocean := newRealField(t, company, user, oceanLon, oceanLat, "Talhao Oceano")
	_, err = calculate.Execute(ctx(), usecase.CalculateFarmTerrainInput{Actor: ocean.principal(), FarmID: ocean.farm.ID})
	must(t, err)
	failed, failedTook := waitTerrain(t, ocean, domain.TerrainFailed, 5*time.Minute)
	t.Logf("FAILURE field in the ocean: status=%s reason=%q after %s", failed.Status, failed.FailureReason, failedTook.Round(time.Millisecond))
	if failed.FailureReason != usecase.TerrainFailureTileUnavailable {
		t.Errorf("failure reason = %q, want %q", failed.FailureReason, usecase.TerrainFailureTileUnavailable)
	}
	rec.mu.Lock()
	t.Logf("FAILURE source log: %v", rec.fetchFailures)
	rec.mu.Unlock()

	worker.Stop()
	drainTerrainQueue(t)
}

func countInside(w domain.ElevationWindow) int {
	n := 0
	for _, in := range w.Inside {
		if in {
			n++
		}
	}
	return n
}

func nonZeroBins(c domain.TerrainCalculation) map[string]float64 {
	out := map[string]float64{}
	for i, ha := range c.SlopeHistogramHa {
		if ha > 0 {
			out[fmt.Sprintf("bin%03d", i)] = math.Round(ha*1000) / 1000
		}
	}
	return out
}

func explain(t *testing.T, title string, noSeqScan bool, f realFixture, radius float64) {
	t.Helper()
	tx, err := testPool.Begin(ctx())
	must(t, err)
	defer func() { _ = tx.Rollback(ctx()) }()
	if noSeqScan {
		if _, err := tx.Exec(ctx(), `SET LOCAL enable_seqscan = off`); err != nil {
			t.Fatalf("set enable_seqscan: %v", err)
		}
	}
	rows, err := tx.Query(ctx(), `EXPLAIN (ANALYZE, BUFFERS) `+windowSQL, radius, string(f.company.ID), string(f.field.ID))
	must(t, err)
	defer rows.Close()
	var lines []string
	for rows.Next() {
		var line string
		must(t, rows.Scan(&line))
		lines = append(lines, line)
	}
	must(t, rows.Err())
	t.Logf("%s:\n%s", title, strings.Join(lines, "\n"))
}

// comparePostGIS recomputes the mean and p90 slope of the field with PostGIS (reprojection to a
// metric grid plus ST_Slope) and prints it against the Go calculation.
func comparePostGIS(t *testing.T, label string, f realFixture, calc domain.TerrainCalculation) {
	t.Helper()
	cellHa := func(scale float64) float64 { return scale * scale / 10000 }
	histTotal := 0.0
	for _, ha := range calc.SlopeHistogramHa {
		histTotal += ha
	}
	for _, v := range []struct {
		algorithm string
		scale     float64
	}{{"Bilinear", 30}, {"NearestNeighbor", 30}, {"Bilinear", 28.8}} {
		var count int
		var mean, maxSlope, p90, minElev, maxElev, scaleX float64
		s := time.Now()
		err := testPool.QueryRow(ctx(), slopeSQL, string(f.company.ID), string(f.field.ID), 100.0, v.algorithm, v.scale).Scan(&count, &mean, &maxSlope, &p90, &minElev, &maxElev, &scaleX)
		if err != nil {
			t.Errorf("ST_Slope check (%s, %s, %.1f m): %v", label, v.algorithm, v.scale, err)
			continue
		}
		t.Logf("POSTGIS [%s] ST_Slope (%s, %.1f m grid, EPSG:31983; scale_x=%.2f; %s): cells=%d mean=%.3f%% p90=%.3f%% max=%.3f%%; elevation min=%.1f max=%.1f",
			label, v.algorithm, v.scale, scaleX, time.Since(s).Round(time.Millisecond), count, mean, p90, maxSlope, minElev, maxElev)
		t.Logf("  vs Go [%s]: mean %.3f%% (diff %+.3f pp, %+.1f%% rel); p90 %.3f%% (diff %+.3f pp, %+.1f%% rel); max %.3f%% vs %.3f%%; elevation min/max %.1f/%.1f; go slope cells ~%d (histogram %.2f ha / cell %.4f ha)",
			label, calc.Measurement.SlopeMeanPct, calc.Measurement.SlopeMeanPct-mean, (calc.Measurement.SlopeMeanPct-mean)/mean*100,
			*calc.Measurement.SlopeP90Pct, *calc.Measurement.SlopeP90Pct-p90, (*calc.Measurement.SlopeP90Pct-p90)/p90*100,
			calc.Measurement.SlopeMaxPct, maxSlope, calc.ElevationMinM, calc.ElevationMaxM, int(math.Round(histTotal/cellHa(30))), histTotal, cellHa(30))
	}
}
