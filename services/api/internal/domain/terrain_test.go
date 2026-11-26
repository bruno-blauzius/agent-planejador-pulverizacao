package domain_test

import (
	"errors"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/domain"
)

const (
	gridSize  = 30
	fieldLo   = 5 // field cells are rows and columns fieldLo..fieldHi
	fieldHi   = 24
	fieldSize = fieldHi - fieldLo + 1
	scaleDeg  = 0.0003
	originLat = -21.0
)

var (
	terrainNow   = time.Date(2026, 11, 24, 10, 0, 0, 0, time.UTC)
	terrainModel = domain.ElevationModel{Source: "Copernicus DEM GLO-30", ResolutionM: 30, Version: "2023_1"}
)

// cellSizes mirrors the formula of the domain for the test grid.
func cellSizes() (dx, dy float64) {
	lat := originLat + float64(gridSize)/2*-scaleDeg
	return scaleDeg * 111_320 * math.Cos(lat*math.Pi/180), scaleDeg * 110_574
}

func cellHa() float64 {
	dx, dy := cellSizes()
	return dx * dy / 10_000
}

// gridWith builds a 30x30 window whose field is the central 20x20 block.
func gridWith(elevation func(r, c int, dx, dy float64) float64) domain.ElevationWindow {
	dx, dy := cellSizes()
	w := domain.ElevationWindow{
		Rows: gridSize, Cols: gridSize,
		Values: make([]float64, gridSize*gridSize), Inside: make([]bool, gridSize*gridSize),
		OriginLon: -47, OriginLat: originLat, ScaleXDeg: scaleDeg, ScaleYDeg: -scaleDeg,
	}
	for r := 0; r < gridSize; r++ {
		for c := 0; c < gridSize; c++ {
			w.Values[r*gridSize+c] = elevation(r, c, dx, dy)
			w.Inside[r*gridSize+c] = r >= fieldLo && r <= fieldHi && c >= fieldLo && c <= fieldHi
		}
	}
	return w
}

func calculate(t *testing.T, w domain.ElevationWindow) domain.TerrainCalculation {
	t.Helper()
	calc, err := domain.CalculateTerrain(w, domain.DefaultTerrainRules(), terrainModel, terrainNow)
	if err != nil {
		t.Fatalf("CalculateTerrain: %v", err)
	}
	return calc
}

func near(t *testing.T, name string, got, want, tol float64) {
	t.Helper()
	if math.Abs(got-want) > tol {
		t.Errorf("%s = %v, want %v ± %v", name, got, want, tol)
	}
}

func histogramTotal(h [domain.SlopeHistogramBins]float64) float64 {
	sum := 0.0
	for _, v := range h {
		sum += v
	}
	return sum
}

func flatGround(int, int, float64, float64) float64 { return 500 }

func TestCalculateTerrainFlat(t *testing.T) {
	calc := calculate(t, gridWith(flatGround))
	m := calc.Measurement
	near(t, "mean", m.SlopeMeanPct, 0, 1e-9)
	near(t, "max", m.SlopeMaxPct, 0, 1e-9)
	if m.Class != domain.SlopeFlat || *m.Aspect != domain.AspectFlat || m.Position != domain.PositionSlope {
		t.Errorf("got class=%s aspect=%s position=%s", m.Class, *m.Aspect, m.Position)
	}
	near(t, "histogram bin 0", calc.SlopeHistogramHa[0], fieldSize*fieldSize*cellHa(), 1e-9)
	near(t, "min", calc.ElevationMinM, 500, 1e-9)
	near(t, "max elevation", calc.ElevationMaxM, 500, 1e-9)
	if calc.CalculatedAt != terrainNow || calc.ElevationModel != terrainModel {
		t.Errorf("metadata not carried: %v %v", calc.CalculatedAt, calc.ElevationModel)
	}
}

func TestCalculateTerrainRamp(t *testing.T) {
	// The elevation falls 10 % towards the named direction.
	diagonal := 10 * math.Hypot(1/math.Sqrt2, 1/math.Sqrt2)
	tests := []struct {
		name      string
		elev      func(r, c int, dx, dy float64) float64
		wantSlope float64
		aspect    domain.TerrainAspect
	}{
		{"east", func(r, c int, dx, dy float64) float64 { return 800 - 0.10*dx*float64(c) }, 10, domain.AspectEast},
		{"west", func(r, c int, dx, dy float64) float64 { return 800 + 0.10*dx*float64(c) }, 10, domain.AspectWest},
		{"south", func(r, c int, dx, dy float64) float64 { return 800 - 0.10*dy*float64(r) }, 10, domain.AspectSouth},
		{"north", func(r, c int, dx, dy float64) float64 { return 800 + 0.10*dy*float64(r) }, 10, domain.AspectNorth},
		{"southeast", func(r, c int, dx, dy float64) float64 {
			return 800 - 0.10*(dx*float64(c)+dy*float64(r))/math.Sqrt2
		}, diagonal, domain.AspectSoutheast},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			calc := calculate(t, gridWith(tt.elev))
			m := calc.Measurement
			near(t, "mean", m.SlopeMeanPct, tt.wantSlope, 0.1)
			near(t, "p90", *m.SlopeP90Pct, tt.wantSlope, 0.1)
			near(t, "max", m.SlopeMaxPct, tt.wantSlope, 0.1)
			if m.Class != domain.SlopeUndulating {
				t.Errorf("class = %s, want undulating", m.Class)
			}
			if *m.Aspect != tt.aspect {
				t.Errorf("aspect = %s, want %s", *m.Aspect, tt.aspect)
			}
			if m.Position != domain.PositionSlope {
				t.Errorf("position = %s, want slope", m.Position)
			}
			if len(calc.Profile) < 2 {
				t.Fatalf("profile too short: %v", calc.Profile)
			}
			for i := 1; i < len(calc.Profile); i++ {
				if calc.Profile[i].DistanceM <= calc.Profile[i-1].DistanceM || calc.Profile[i].ElevationM >= calc.Profile[i-1].ElevationM {
					t.Fatalf("profile not monotone at %d: %v", i, calc.Profile)
				}
			}
			near(t, "histogram total", histogramTotal(calc.SlopeHistogramHa), fieldSize*fieldSize*cellHa(), 1e-9)
		})
	}
}

func TestCalculateTerrainRidgeHasNoPredominantAspect(t *testing.T) {
	// A ridge one column off center: 9 columns face west, 10 face east, so the
	// mean downhill vector is about 0.05 long. It is steep (10 %), but it has no
	// predominant direction.
	ridge := func(r, c int, dx, dy float64) float64 { return 800 - 0.10*dx*math.Abs(float64(c-14)) }
	calc := calculate(t, gridWith(ridge))
	near(t, "mean", calc.Measurement.SlopeMeanPct, 9.5, 0.6)
	if *calc.Measurement.Aspect != domain.AspectFlat {
		t.Errorf("aspect = %s, want flat for a nearly symmetric ridge", *calc.Measurement.Aspect)
	}
	// Two thirds of the cells facing east is a clear direction.
	lopsided := func(r, c int, dx, dy float64) float64 { return 800 - 0.10*dx*math.Abs(float64(c-9)) }
	if got := *calculate(t, gridWith(lopsided)).Measurement.Aspect; got != domain.AspectEast {
		t.Errorf("aspect = %s, want east for a lopsided ridge", got)
	}
}

func TestCalculateTerrainEastRampElevationAndProfile(t *testing.T) {
	dx, _ := cellSizes()
	calc := calculate(t, gridWith(func(r, c int, dx, dy float64) float64 { return 800 - 0.10*dx*float64(c) }))
	near(t, "min", calc.ElevationMinM, 800-0.10*dx*fieldHi, 1e-6)
	near(t, "max", calc.ElevationMaxM, 800-0.10*dx*fieldLo, 1e-6)
	if len(calc.Profile) > domain.DefaultTerrainRules().ProfilePoints {
		t.Errorf("profile has %d points", len(calc.Profile))
	}
	span := calc.Profile[len(calc.Profile)-1].DistanceM
	if span < 0.9*dx*(fieldSize-1) || span > dx*fieldSize {
		t.Errorf("profile span %v not close to the field width %v", span, dx*fieldSize)
	}
}

func TestCalculateTerrainVaryingSlope(t *testing.T) {
	// Column c has slope c+0.5 %, with 20 cells per column.
	calc := calculate(t, gridWith(func(r, c int, dx, dy float64) float64 {
		return 800 - dx*0.005*float64(c*c+c)
	}))
	m := calc.Measurement
	near(t, "mean", m.SlopeMeanPct, 15, 1e-6)
	near(t, "p90 (nearest rank)", *m.SlopeP90Pct, 22.5, 1e-6)
	near(t, "max", m.SlopeMaxPct, 24.5, 1e-6)
	if m.Class != domain.SlopeUndulating {
		t.Errorf("class = %s", m.Class)
	}
	for bin := 0; bin < domain.SlopeHistogramBins; bin++ {
		want := 0.0
		if bin >= fieldLo && bin <= fieldHi {
			want = fieldSize * cellHa()
		}
		near(t, "bin", calc.SlopeHistogramHa[bin], want, 1e-9)
	}
	areas := calc.ClassAreas(domain.DefaultTerrainRules())
	wantShares := []float64{0, 15, 60, 25, 0}
	if len(areas) != 5 {
		t.Fatalf("want 5 classes, got %d", len(areas))
	}
	for i, class := range domain.SlopeClasses() {
		if areas[i].Class != class {
			t.Errorf("class %d = %s, want %s", i, areas[i].Class, class)
		}
		near(t, string(class)+" share", areas[i].SharePct, wantShares[i], 1e-6)
		near(t, string(class)+" area", areas[i].AreaHa, wantShares[i]/100*histogramTotal(calc.SlopeHistogramHa), 1e-9)
	}
}

func TestCalculateTerrainPosition(t *testing.T) {
	withOffset := func(offset float64) domain.ElevationWindow {
		w := gridWith(flatGround)
		for i, in := range w.Inside {
			if in {
				w.Values[i] += offset
			}
		}
		return w
	}
	tests := []struct {
		name   string
		offset float64
		want   domain.TerrainPosition
	}{
		{"hilltop", 30, domain.PositionHilltop},
		{"lowland", -30, domain.PositionLowland},
		{"within tolerance above", 5, domain.PositionSlope},
		{"within tolerance below", -5, domain.PositionSlope},
		{"just above tolerance", 5.1, domain.PositionHilltop},
		{"just below tolerance", -5.1, domain.PositionLowland},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			calc := calculate(t, withOffset(tt.offset))
			if calc.Measurement.Position != tt.want {
				t.Errorf("position = %s, want %s", calc.Measurement.Position, tt.want)
			}
		})
	}
}

func TestCalculateTerrainMissingData(t *testing.T) {
	onlyInside := func(w *domain.ElevationWindow, cells [][2]int) {
		for i := range w.Inside {
			w.Inside[i] = false
		}
		for _, rc := range cells {
			w.Inside[rc[0]*gridSize+rc[1]] = true
		}
	}
	block := func(n int) [][2]int {
		var cells [][2]int
		for k := 0; k < n; k++ {
			cells = append(cells, [2]int{10 + k/5, 10 + k%5})
		}
		return cells
	}

	t.Run("NaN inside removes the cell and its neighbors from the statistics", func(t *testing.T) {
		w := gridWith(flatGround)
		w.Values[10*gridSize+10] = math.NaN()
		calc := calculate(t, w)
		near(t, "histogram total", histogramTotal(calc.SlopeHistogramHa), (fieldSize*fieldSize-9)*cellHa(), 1e-9)
	})
	t.Run("cells on the grid border have no slope", func(t *testing.T) {
		w := gridWith(flatGround)
		for c := 0; c < gridSize; c++ {
			w.Inside[c] = true // first row: border cells
		}
		calc := calculate(t, w)
		near(t, "histogram total", histogramTotal(calc.SlopeHistogramHa), fieldSize*fieldSize*cellHa(), 1e-9)
	})
	t.Run("NaN in the surroundings does not matter", func(t *testing.T) {
		w := gridWith(flatGround)
		w.Values[0] = math.NaN()
		calc := calculate(t, w)
		if calc.Measurement.Position != domain.PositionSlope {
			t.Errorf("position = %s", calc.Measurement.Position)
		}
	})
	t.Run("too few valid cells", func(t *testing.T) {
		w := gridWith(flatGround)
		onlyInside(&w, block(19))
		_, err := domain.CalculateTerrain(w, domain.DefaultTerrainRules(), terrainModel, terrainNow)
		if !errors.Is(err, domain.ErrInsufficientElevationData) {
			t.Errorf("err = %v, want ErrInsufficientElevationData", err)
		}
	})
	t.Run("exactly MinValidCells is accepted", func(t *testing.T) {
		w := gridWith(flatGround)
		onlyInside(&w, block(20))
		if _, err := domain.CalculateTerrain(w, domain.DefaultTerrainRules(), terrainModel, terrainNow); err != nil {
			t.Errorf("err = %v", err)
		}
	})
	t.Run("all NaN", func(t *testing.T) {
		w := gridWith(func(int, int, float64, float64) float64 { return math.NaN() })
		_, err := domain.CalculateTerrain(w, domain.DefaultTerrainRules(), terrainModel, terrainNow)
		if !errors.Is(err, domain.ErrInsufficientElevationData) {
			t.Errorf("err = %v", err)
		}
	})
}

func TestCalculateTerrainInvalidInput(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*domain.ElevationWindow, *domain.TerrainRules)
		wantErr error
	}{
		{"no rows", func(w *domain.ElevationWindow, _ *domain.TerrainRules) { w.Rows = 0 }, domain.ErrInvalidElevationWindow},
		{"size mismatch", func(w *domain.ElevationWindow, _ *domain.TerrainRules) { w.Values = w.Values[1:] }, domain.ErrInvalidElevationWindow},
		{"mask mismatch", func(w *domain.ElevationWindow, _ *domain.TerrainRules) { w.Inside = w.Inside[1:] }, domain.ErrInvalidElevationWindow},
		{"zero scale", func(w *domain.ElevationWindow, _ *domain.TerrainRules) { w.ScaleXDeg = 0 }, domain.ErrInvalidElevationWindow},
		{"bad rules", func(_ *domain.ElevationWindow, r *domain.TerrainRules) { r.MinValidCells = 0 }, domain.ErrInvalidTerrainRules},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w, rules := gridWith(flatGround), domain.DefaultTerrainRules()
			tt.mutate(&w, &rules)
			_, err := domain.CalculateTerrain(w, rules, terrainModel, terrainNow)
			if !errors.Is(err, tt.wantErr) {
				t.Errorf("err = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestClassOfLimits(t *testing.T) {
	rules := domain.DefaultTerrainRules()
	tests := []struct {
		slope float64
		want  domain.SlopeClass
	}{
		{0, domain.SlopeFlat},
		{2.999, domain.SlopeFlat},
		{3, domain.SlopeGentlyUndulating},
		{7.999, domain.SlopeGentlyUndulating},
		{8, domain.SlopeUndulating},
		{19.999, domain.SlopeUndulating},
		{20, domain.SlopeStronglyUndulating},
		{44.999, domain.SlopeStronglyUndulating},
		{45, domain.SlopeMountainous},
		{75, domain.SlopeMountainous},
		{120, domain.SlopeMountainous},
	}
	for _, tt := range tests {
		t.Run(string(tt.want), func(t *testing.T) {
			if got := rules.ClassOf(tt.slope); got != tt.want {
				t.Errorf("ClassOf(%v) = %s, want %s", tt.slope, got, tt.want)
			}
		})
	}
}

func TestClassAreasBinsAtTheLimits(t *testing.T) {
	rules := domain.DefaultTerrainRules()
	var h [domain.SlopeHistogramBins]float64
	h[2], h[3], h[7], h[8], h[19], h[20], h[44], h[45], h[100] = 1, 2, 3, 4, 5, 6, 7, 8, 9
	want := []float64{1, 5, 9, 13, 17}
	for i, a := range rules.ClassAreas(h) {
		near(t, string(a.Class), a.AreaHa, want[i], 1e-12)
		near(t, string(a.Class)+" share", a.SharePct, want[i]/45*100, 1e-9)
	}
	for _, a := range rules.ClassAreas([domain.SlopeHistogramBins]float64{}) {
		if a.AreaHa != 0 || a.SharePct != 0 {
			t.Errorf("empty histogram gave %+v", a)
		}
	}
}

func TestTerrainRulesValidate(t *testing.T) {
	if err := domain.DefaultTerrainRules().Validate(); err != nil {
		t.Fatalf("defaults invalid: %v", err)
	}
	negative := -1.0
	tests := []struct {
		name   string
		mutate func(*domain.TerrainRules)
	}{
		{"limits not increasing", func(r *domain.TerrainRules) { r.ClassUpperLimitsPct = [4]float64{3, 8, 8, 45} }},
		{"first limit zero", func(r *domain.TerrainRules) { r.ClassUpperLimitsPct[0] = 0 }},
		{"flat aspect", func(r *domain.TerrainRules) { r.FlatAspectBelowPct = 0 }},
		{"radius", func(r *domain.TerrainRules) { r.PositionRadiusM = -1 }},
		{"tolerance", func(r *domain.TerrainRules) { r.PositionToleranceM = 0 }},
		{"profile points", func(r *domain.TerrainRules) { r.ProfilePoints = 1 }},
		{"min cells", func(r *domain.TerrainRules) { r.MinValidCells = 0 }},
		{"divergence", func(r *domain.TerrainRules) { r.AdjustmentDivergencePct = 0 }},
		{"method limit", func(r *domain.TerrainRules) { r.MethodSlopeLimits[domain.MethodDrone] = &negative }},
		{"restricted share zero", func(r *domain.TerrainRules) { r.RestrictedFromSharePct = 0 }},
		{"shares equal", func(r *domain.TerrainRules) { r.RestrictedFromSharePct, r.UnsuitableFromSharePct = 50, 50 }},
		{"unsuitable above 100", func(r *domain.TerrainRules) { r.UnsuitableFromSharePct = 101 }},
		{"NaN limit", func(r *domain.TerrainRules) { r.ClassUpperLimitsPct[1] = math.NaN() }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rules := domain.DefaultTerrainRules()
			tt.mutate(&rules)
			if err := rules.Validate(); !errors.Is(err, domain.ErrInvalidTerrainRules) {
				t.Errorf("err = %v, want ErrInvalidTerrainRules", err)
			}
		})
	}
}

func adjustmentDetails() domain.TerrainAdjustmentDetails {
	return domain.TerrainAdjustmentDetails{
		Origin: domain.OriginTopographicSurvey, SlopeMeanPct: 8, SlopeMaxPct: 14,
		Position: domain.PositionSlope, Justification: "Levantamento topográfico de 2026",
	}
}

func TestNewTerrainAdjustment(t *testing.T) {
	rules := domain.DefaultTerrainRules()
	tests := []struct {
		name       string
		edit       func(*domain.TerrainAdjustmentDetails)
		wantFields []string
	}{
		{"valid", func(*domain.TerrainAdjustmentDetails) {}, nil},
		{"mean zero", func(d *domain.TerrainAdjustmentDetails) { d.SlopeMeanPct, d.SlopeMaxPct = 0, 0 }, nil},
		{"mean and max 100", func(d *domain.TerrainAdjustmentDetails) { d.SlopeMeanPct, d.SlopeMaxPct = 100, 100 }, nil},
		{"max equals mean", func(d *domain.TerrainAdjustmentDetails) { d.SlopeMaxPct = d.SlopeMeanPct }, nil},
		{"field assessment", func(d *domain.TerrainAdjustmentDetails) { d.Origin = domain.OriginFieldAssessment }, nil},
		{"justification 1000 runes", func(d *domain.TerrainAdjustmentDetails) { d.Justification = strings.Repeat("ã", 1000) }, nil},
		{"justification 1001 runes", func(d *domain.TerrainAdjustmentDetails) { d.Justification = strings.Repeat("ã", 1001) }, []string{"justification"}},
		{"justification blank", func(d *domain.TerrainAdjustmentDetails) { d.Justification = "   \n\t" }, []string{"justification"}},
		{"justification empty", func(d *domain.TerrainAdjustmentDetails) { d.Justification = "" }, []string{"justification"}},
		{"invalid origin", func(d *domain.TerrainAdjustmentDetails) { d.Origin = "guess" }, []string{"origin"}},
		{"empty origin", func(d *domain.TerrainAdjustmentDetails) { d.Origin = "" }, []string{"origin"}},
		{"negative mean", func(d *domain.TerrainAdjustmentDetails) { d.SlopeMeanPct = -0.1 }, []string{"slope_mean_pct"}},
		{"mean above 100", func(d *domain.TerrainAdjustmentDetails) { d.SlopeMeanPct, d.SlopeMaxPct = 100.1, 100 }, []string{"slope_mean_pct"}},
		{"NaN mean", func(d *domain.TerrainAdjustmentDetails) { d.SlopeMeanPct = math.NaN() }, []string{"slope_mean_pct"}},
		{"max above 100", func(d *domain.TerrainAdjustmentDetails) { d.SlopeMaxPct = 100.1 }, []string{"slope_max_pct"}},
		{"negative max", func(d *domain.TerrainAdjustmentDetails) { d.SlopeMeanPct, d.SlopeMaxPct = 0, -1 }, []string{"slope_max_pct"}},
		{"max below mean", func(d *domain.TerrainAdjustmentDetails) { d.SlopeMaxPct = 7.9 }, []string{"slope_max_pct"}},
		{"invalid position", func(d *domain.TerrainAdjustmentDetails) { d.Position = "valley" }, []string{"position"}},
		{"everything wrong", func(d *domain.TerrainAdjustmentDetails) {
			*d = domain.TerrainAdjustmentDetails{SlopeMeanPct: -1, SlopeMaxPct: 200}
		}, []string{"origin", "slope_mean_pct", "slope_max_pct", "position", "justification"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			details := adjustmentDetails()
			tt.edit(&details)
			_, err := domain.NewTerrainAdjustment(details, "user-1", rules, terrainNow)
			got := fieldsOf(t, err)
			if len(got) != len(tt.wantFields) {
				t.Fatalf("fields = %v, want %v", got, tt.wantFields)
			}
			for i := range got {
				if got[i] != tt.wantFields[i] {
					t.Fatalf("fields = %v, want %v", got, tt.wantFields)
				}
			}
		})
	}
}

func TestTerrainAdjustmentMeasurement(t *testing.T) {
	details := adjustmentDetails()
	details.Justification = "  medido em campo  "
	adj, err := domain.NewTerrainAdjustment(details, "user-1", domain.DefaultTerrainRules(), terrainNow)
	if err != nil {
		t.Fatal(err)
	}
	if adj.Justification != "medido em campo" || adj.AdjustedBy != "user-1" || !adj.AdjustedAt.Equal(terrainNow) {
		t.Errorf("adjustment = %+v", adj)
	}
	m := adj.Measurement()
	if m.SlopeMeanPct != 8 || m.SlopeMaxPct != 14 || m.Class != domain.SlopeUndulating || m.Position != domain.PositionSlope {
		t.Errorf("measurement = %+v", m)
	}
	if m.SlopeP90Pct != nil || m.Aspect != nil {
		t.Errorf("p90 and aspect must be nil: %+v", m)
	}
}

func TestEffectiveAndOutdated(t *testing.T) {
	rules := domain.DefaultTerrainRules()
	calc := &domain.TerrainCalculation{Measurement: domain.TerrainMeasurement{SlopeMeanPct: 10, Class: domain.SlopeUndulating}}
	adjust := func(mean float64) *domain.TerrainAdjustment {
		d := adjustmentDetails()
		d.SlopeMeanPct, d.SlopeMaxPct = mean, 100
		a, err := domain.NewTerrainAdjustment(d, "user-1", rules, terrainNow)
		if err != nil {
			t.Fatal(err)
		}
		return &a
	}

	t.Run("effective", func(t *testing.T) {
		tests := []struct {
			name       string
			calc       *domain.TerrainCalculation
			adj        *domain.TerrainAdjustment
			wantSource domain.TerrainSource
			wantMean   float64
			wantOK     bool
		}{
			{"none", nil, nil, "", 0, false},
			{"calculated only", calc, nil, domain.SourceCalculated, 10, true},
			{"adjustment only", nil, adjust(4), domain.SourceManual, 4, true},
			{"adjustment wins", calc, adjust(4), domain.SourceManual, 4, true},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				m, source, ok := domain.EffectiveTerrain(tt.calc, tt.adj)
				if ok != tt.wantOK || source != tt.wantSource || m.SlopeMeanPct != tt.wantMean {
					t.Errorf("got %v %s %v", m.SlopeMeanPct, source, ok)
				}
			})
		}
	})
	t.Run("outdated", func(t *testing.T) {
		tests := []struct {
			name string
			calc *domain.TerrainCalculation
			adj  *domain.TerrainAdjustment
			want bool
		}{
			{"no adjustment", calc, nil, false},
			{"no calculation", nil, adjust(20), false},
			{"identical", calc, adjust(10), false},
			{"exactly at the gap above", calc, adjust(15), false},
			{"just beyond the gap above", calc, adjust(15.1), true},
			{"exactly at the gap below", calc, adjust(5), false},
			{"just beyond the gap below", calc, adjust(4.9), true},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				if got := domain.AdjustmentOutdated(tt.calc, tt.adj, rules); got != tt.want {
					t.Errorf("got %v, want %v", got, tt.want)
				}
			})
		}
	})
}

func fitsByMethod(fits []domain.MethodFit) map[domain.SprayMethod]domain.MethodFit {
	out := map[domain.SprayMethod]domain.MethodFit{}
	for _, f := range fits {
		out[f.Method] = f
	}
	return out
}

func TestEvaluateMethodFitsLayout(t *testing.T) {
	fits := domain.EvaluateMethodFits(domain.DefaultTerrainRules(), domain.TerrainMeasurement{}, domain.SourceManual, nil)
	methods := domain.SprayMethods()
	if len(fits) != len(methods) {
		t.Fatalf("got %d fits", len(fits))
	}
	for i, f := range fits {
		if f.Method != methods[i] || f.LimitSource != domain.LimitSourceReference {
			t.Errorf("fit %d = %+v", i, f)
		}
	}
	by := fitsByMethod(fits)
	near(t, "tractor limit", *by[domain.MethodTractorBoom].SlopeLimitPct, 20, 0)
	near(t, "self propelled limit", *by[domain.MethodSelfPropelled].SlopeLimitPct, 15, 0)
	near(t, "airblast limit", *by[domain.MethodAirblast].SlopeLimitPct, 20, 0)
	for _, m := range []domain.SprayMethod{domain.MethodKnapsack, domain.MethodAircraft, domain.MethodDrone} {
		if by[m].SlopeLimitPct != nil || by[m].Status != domain.FitCompatible || by[m].AreaAboveLimitPct != nil {
			t.Errorf("%s = %+v", m, by[m])
		}
	}
}

func TestEvaluateMethodFitsFromDistribution(t *testing.T) {
	rules := domain.DefaultTerrainRules()
	// 100 ha in total; `above` hectares sit in bin 25 (above the 20 % limit).
	histogram := func(above float64) *[domain.SlopeHistogramBins]float64 {
		var h [domain.SlopeHistogramBins]float64
		h[0], h[25] = 100-above, above
		return &h
	}
	tests := []struct {
		name      string
		above     float64
		wantShare float64
		want      domain.MethodFitStatus
	}{
		{"none above", 0, 0, domain.FitCompatible},
		{"9.99 %", 9.99, 9.99, domain.FitCompatible},
		{"10 %", 10, 10, domain.FitRestricted},
		{"49.99 %", 49.99, 49.99, domain.FitRestricted},
		{"50 %", 50, 50, domain.FitUnsuitable},
		{"all above", 100, 100, domain.FitUnsuitable},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fits := fitsByMethod(domain.EvaluateMethodFits(rules, domain.TerrainMeasurement{}, domain.SourceCalculated, histogram(tt.above)))
			f := fits[domain.MethodTractorBoom]
			if f.Status != tt.want {
				t.Errorf("status = %s, want %s", f.Status, tt.want)
			}
			if f.AreaAboveLimitPct == nil {
				t.Fatal("share must be present")
			}
			near(t, "share", *f.AreaAboveLimitPct, tt.wantShare, 1e-9)
		})
	}

	t.Run("a bin starting on the limit counts whole", func(t *testing.T) {
		var h [domain.SlopeHistogramBins]float64
		h[0], h[20] = 90, 10
		f := fitsByMethod(domain.EvaluateMethodFits(rules, domain.TerrainMeasurement{}, domain.SourceCalculated, &h))[domain.MethodTractorBoom]
		near(t, "share", *f.AreaAboveLimitPct, 10, 1e-9)
		if f.Status != domain.FitRestricted {
			t.Errorf("status = %s", f.Status)
		}
	})
	t.Run("the bin below the limit does not count", func(t *testing.T) {
		var h [domain.SlopeHistogramBins]float64
		h[0], h[19] = 50, 50
		f := fitsByMethod(domain.EvaluateMethodFits(rules, domain.TerrainMeasurement{}, domain.SourceCalculated, &h))[domain.MethodTractorBoom]
		near(t, "share", *f.AreaAboveLimitPct, 0, 1e-12)
	})
	t.Run("a straddling bin is interpolated", func(t *testing.T) {
		custom := domain.DefaultTerrainRules()
		limit := 20.25
		custom.MethodSlopeLimits[domain.MethodTractorBoom] = &limit
		var h [domain.SlopeHistogramBins]float64
		h[0], h[20] = 60, 40 // 75 % of bin 20 lies above 20.25: 30 ha of 100
		f := fitsByMethod(domain.EvaluateMethodFits(custom, domain.TerrainMeasurement{}, domain.SourceCalculated, &h))[domain.MethodTractorBoom]
		near(t, "share", *f.AreaAboveLimitPct, 30, 1e-9)
		if f.Status != domain.FitRestricted {
			t.Errorf("status = %s", f.Status)
		}
	})
	t.Run("empty histogram falls back to the measurement", func(t *testing.T) {
		var h [domain.SlopeHistogramBins]float64
		m := domain.TerrainMeasurement{SlopeMeanPct: 25, SlopeMaxPct: 30}
		f := fitsByMethod(domain.EvaluateMethodFits(rules, m, domain.SourceCalculated, &h))[domain.MethodTractorBoom]
		if f.Status != domain.FitUnsuitable || f.AreaAboveLimitPct != nil {
			t.Errorf("fit = %+v", f)
		}
	})
}

func TestEvaluateMethodFitsManual(t *testing.T) {
	rules := domain.DefaultTerrainRules()
	tests := []struct {
		name       string
		mean, max  float64
		tractor    domain.MethodFitStatus
		selfProp   domain.MethodFitStatus
		restricted bool
	}{
		{"all below the limits", 10, 12, domain.FitCompatible, domain.FitCompatible, false},
		{"on the limit is fine", 15, 20, domain.FitCompatible, domain.FitRestricted, true},
		{"max above one limit", 10, 22, domain.FitRestricted, domain.FitRestricted, true},
		{"mean above the lower limit", 16, 18, domain.FitCompatible, domain.FitUnsuitable, true},
		{"mean above everything", 25, 30, domain.FitUnsuitable, domain.FitUnsuitable, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := domain.TerrainMeasurement{SlopeMeanPct: tt.mean, SlopeMaxPct: tt.max}
			fits := domain.EvaluateMethodFits(rules, m, domain.SourceManual, nil)
			by := fitsByMethod(fits)
			if by[domain.MethodTractorBoom].Status != tt.tractor || by[domain.MethodSelfPropelled].Status != tt.selfProp {
				t.Errorf("tractor=%s self=%s", by[domain.MethodTractorBoom].Status, by[domain.MethodSelfPropelled].Status)
			}
			if by[domain.MethodTractorBoom].AreaAboveLimitPct != nil {
				t.Error("manual value has no share")
			}
			if by[domain.MethodDrone].Status != domain.FitCompatible {
				t.Error("drone must stay compatible")
			}
			if got := domain.HasRestrictedMethod(fits); got != tt.restricted {
				t.Errorf("HasRestrictedMethod = %v, want %v", got, tt.restricted)
			}
		})
	}
	t.Run("a manual value ignores a histogram", func(t *testing.T) {
		var h [domain.SlopeHistogramBins]float64
		h[0] = 100
		m := domain.TerrainMeasurement{SlopeMeanPct: 25, SlopeMaxPct: 30}
		f := fitsByMethod(domain.EvaluateMethodFits(rules, m, domain.SourceManual, &h))[domain.MethodTractorBoom]
		if f.Status != domain.FitUnsuitable || f.AreaAboveLimitPct != nil {
			t.Errorf("fit = %+v", f)
		}
	})
}

func TestHasRestrictedMethodIgnoresUnlimitedMethods(t *testing.T) {
	fits := []domain.MethodFit{
		{Method: domain.MethodDrone, Status: domain.FitUnsuitable},
		{Method: domain.MethodTractorBoom, SlopeLimitPct: ptr(20.0), Status: domain.FitCompatible},
	}
	if domain.HasRestrictedMethod(fits) {
		t.Error("only limited methods count")
	}
	if domain.HasRestrictedMethod(nil) {
		t.Error("no fits, no restriction")
	}
}

func TestSummarizeFarmTerrain(t *testing.T) {
	older := terrainNow.Add(-48 * time.Hour)
	measure := func(mean float64) *domain.TerrainMeasurement { return &domain.TerrainMeasurement{SlopeMeanPct: mean} }

	t.Run("empty", func(t *testing.T) {
		s := domain.SummarizeFarmTerrain(nil)
		if s.FieldCount != 0 || s.SlopeMeanPct != nil || s.ElevationMinM != nil || s.CalculatedAt != nil {
			t.Errorf("summary = %+v", s)
		}
	})
	t.Run("fields without relief", func(t *testing.T) {
		s := domain.SummarizeFarmTerrain([]domain.FieldTerrainRow{{AreaHa: 10}, {AreaHa: 5}})
		if s.FieldCount != 2 || s.CalculatedFieldCount != 0 || s.SlopeMeanPct != nil {
			t.Errorf("summary = %+v", s)
		}
	})
	t.Run("weighted by area", func(t *testing.T) {
		rows := []domain.FieldTerrainRow{
			{AreaHa: 30, Measurement: measure(4), ElevationMinM: ptr(700.0), ElevationMaxM: ptr(720.0), CalculatedAt: &older},
			{AreaHa: 10, Measurement: measure(12), ElevationMinM: ptr(650.0), ElevationMaxM: ptr(760.0), CalculatedAt: &terrainNow, Restricted: true},
			{AreaHa: 50}, // no relief: out of the mean
		}
		s := domain.SummarizeFarmTerrain(rows)
		if s.FieldCount != 3 || s.CalculatedFieldCount != 2 || s.RestrictedFieldCount != 1 {
			t.Errorf("counts = %+v", s)
		}
		near(t, "mean", *s.SlopeMeanPct, (30*4+10*12)/40.0, 1e-9)
		near(t, "min", *s.ElevationMinM, 650, 0)
		near(t, "max", *s.ElevationMaxM, 760, 0)
		if !s.CalculatedAt.Equal(terrainNow) {
			t.Errorf("calculated at = %v", s.CalculatedAt)
		}
	})
	t.Run("zero total area falls back to the plain mean", func(t *testing.T) {
		s := domain.SummarizeFarmTerrain([]domain.FieldTerrainRow{{Measurement: measure(4)}, {Measurement: measure(8)}})
		near(t, "mean", *s.SlopeMeanPct, 6, 1e-9)
	})
	t.Run("a field without elevation keeps the elevation of the others", func(t *testing.T) {
		s := domain.SummarizeFarmTerrain([]domain.FieldTerrainRow{
			{AreaHa: 1, Measurement: measure(3)},
			{AreaHa: 1, Measurement: measure(3), ElevationMinM: ptr(500.0), ElevationMaxM: ptr(510.0)},
		})
		near(t, "min", *s.ElevationMinM, 500, 0)
	})
}
