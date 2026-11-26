package usecase_test

import (
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/domain"
	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/usecase"
)

// The env clock is 2026-11-22 12:00 UTC, 09:00 in BRT: "today" is 22/11.

func (e *conditionsEnv) getTrafficability() *usecase.GetFieldTrafficability {
	return usecase.NewGetFieldTrafficability(usecase.GetFieldTrafficabilityDeps{
		Fields: e.conditions, Gauges: e.conditions, Thresholds: e.conditions, Terrain: e.terrain,
		Weather: e.weather, Clock: e.clock, TerrainRules: e.rules, Settings: e.settings,
	})
}

func (e *conditionsEnv) today() time.Time {
	return domain.LocalDay(e.clock.T, e.brt)
}

// dryWeather is a model answer with no rain at all, a given soil moisture up
// to now and `forecast` mm on every forecast day.
func (e *conditionsEnv) dryWeather(moisture, forecast float64) usecase.WeatherReport {
	var report usecase.WeatherReport
	start := e.clock.T.Add(-7 * 24 * time.Hour)
	for at := start; !at.After(e.clock.T.Add(8 * 24 * time.Hour)); at = at.Add(time.Hour) {
		rain, m := 0.0, moisture
		report.Hours = append(report.Hours, domain.HourlyWeather{Time: at, RainMm: &rain, SoilMoistureM3M3: &m})
	}
	for offset := -7; offset <= 7; offset++ {
		mm := 0.0
		if offset > 0 {
			mm = forecast
		}
		report.Days = append(report.Days, domain.DailyRain{Date: e.today().AddDate(0, 0, offset), Mm: mm})
	}
	return report
}

// rainyModel puts `mm` of rain in the model hour that ended `hoursAgo` before now.
func (e *conditionsEnv) rainyModel(report usecase.WeatherReport, hoursAgo int, mm float64) usecase.WeatherReport {
	at := e.clock.T.Add(-time.Duration(hoursAgo) * time.Hour)
	for i := range report.Hours {
		if report.Hours[i].Time.Equal(at) {
			value := mm
			report.Hours[i].RainMm = &value
		}
	}
	return report
}

// clayWithGauge links field 1 to a clay profile and a gauge with 24 mm in the
// last 48 h (waterlogged for clay: above 15 mm).
func (e *conditionsEnv) clayWithGauge(t *testing.T, latestHoursAgo int) domain.RainGauge {
	t.Helper()
	gauge := e.createGauge(t, "Pluviômetro da sede")
	clay := domain.TextureClay
	e.conditions.SeedProfile(e.f1, &clay, &gauge.ID)
	e.conditions.SeedReading(domain.RainReading{ID: "r1", CompanyID: "co-a", GaugeID: gauge.ID, RainMm: 18, ObservedAt: e.clock.T.Add(-30 * time.Hour), RecordedBy: e.adminA.ID})
	e.conditions.SeedReading(domain.RainReading{ID: "r2", CompanyID: "co-a", GaugeID: gauge.ID, RainMm: 6, ObservedAt: e.clock.T.Add(-time.Duration(latestHoursAgo) * time.Hour), RecordedBy: e.adminA.ID})
	return gauge
}

func (e *conditionsEnv) trafficability(t *testing.T, actor domain.Principal, days int) usecase.FieldTrafficabilityView {
	t.Helper()
	view, err := e.getTrafficability().Execute(ctx(), usecase.GetFieldTrafficabilityInput{Actor: actor, FarmID: "farm-a1", FieldID: e.f1, Days: days})
	assertNoError(t, err)
	return view
}

func methodOf(day domain.TrafficabilityDay, method domain.SprayMethod) domain.MethodEligibility {
	for _, eligibility := range day.Methods {
		if eligibility.Method == method {
			return eligibility
		}
	}
	return domain.MethodEligibility{}
}

func releaseOf(plan domain.TrafficabilityPlan, method domain.SprayMethod) *time.Time {
	for _, release := range plan.Releases {
		if release.Method == method {
			return release.Date
		}
	}
	return nil
}

func TestTrafficabilityFromAFreshGauge(t *testing.T) {
	e := newConditionsEnv(t)
	e.clayWithGauge(t, 1)
	e.seedReady(e.f1, 2)
	e.weather.Report = e.dryWeather(0.41, 0)

	view := e.trafficability(t, e.operatorOnly(t, "farm-a1"), 0)

	soil := view.Soil
	if soil.State != domain.SoilWaterlogged || soil.Basis != domain.BasisMeasuredRain || soil.ThresholdsSource != domain.ThresholdSourceHeuristic {
		t.Fatalf("soil = %+v", soil)
	}
	if soil.Rain == nil || soil.Rain.Source != domain.RainFromGauge || soil.Rain.Last48hMm != 24 || soil.RainSourceName == nil || *soil.RainSourceName != "Pluviômetro da sede" {
		t.Fatalf("rain = %+v (%v)", soil.Rain, soil.RainSourceName)
	}
	if soil.ModelMoisture == nil || soil.ModelMoisture.FieldCapacityM3M3 != 0.36 || soil.ModelMoisture.VolumetricM3M3 != 0.41 {
		t.Errorf("model moisture = %+v, want 0.41 with the clay reference capacity", soil.ModelMoisture)
	}
	if len(view.DataIssues) != 0 {
		t.Errorf("data issues = %v", view.DataIssues)
	}
	if !view.AsOf.Equal(e.clock.T) || view.AsOf.Location() != e.brt {
		t.Errorf("as_of = %v, want now in the farm time zone", view.AsOf)
	}

	// Today plus the default 5 days; 24 mm drying 6 mm a day: 18 (waterlogged), 12 (moist), 6 (ideal), 0 (dry).
	if len(view.Plan.Days) != 6 || !view.Plan.Days[0].Date.Equal(e.today()) {
		t.Fatalf("days = %d starting %v", len(view.Plan.Days), view.Plan.Days[0].Date)
	}
	wantStates := []domain.SoilState{domain.SoilWaterlogged, domain.SoilWaterlogged, domain.SoilMoist, domain.SoilIdeal, domain.SoilDry, domain.SoilDry}
	for i, day := range view.Plan.Days {
		if day.SoilState != wantStates[i] {
			t.Errorf("day %d state = %s, want %s", i, day.SoilState, wantStates[i])
		}
	}
	if got := methodOf(view.Plan.Days[0], domain.MethodTractorBoom); got.Verdict != domain.VerdictUnfit || !slices.Contains(got.Reasons, domain.ReasonSoilWaterlogged) {
		t.Errorf("tractor boom today = %+v", got)
	}
	if got := methodOf(view.Plan.Days[0], domain.MethodDrone); got.Verdict != domain.VerdictApt {
		t.Errorf("drone today = %+v", got)
	}
	if release := releaseOf(view.Plan, domain.MethodTractorBoom); release == nil || !release.Equal(e.today().AddDate(0, 0, 3)) {
		t.Errorf("tractor boom release = %v, want today+3", release)
	}
	if release := releaseOf(view.Plan, domain.MethodKnapsack); release == nil || !release.Equal(e.today().AddDate(0, 0, 2)) {
		t.Errorf("knapsack release = %v, want today+2", release)
	}

	// 7 measured days from the gauge (18 mm on 21/11, 6 mm on 22/11), then 5 forecast days.
	if len(view.RainDaily) != 12 {
		t.Fatalf("rain_daily = %d entries, want 12", len(view.RainDaily))
	}
	if first := view.RainDaily[0]; !first.Date.Equal(e.today().AddDate(0, 0, -6)) || first.Kind != usecase.RainMeasured || first.Source != domain.RainFromGauge {
		t.Errorf("first entry = %+v", first)
	}
	if got := view.RainDaily[5]; got.RainMm != 18 {
		t.Errorf("yesterday = %+v, want 18 mm", got)
	}
	if got := view.RainDaily[6]; got.RainMm != 6 || !got.Date.Equal(e.today()) {
		t.Errorf("today = %+v, want 6 mm", got)
	}
	if got := view.RainDaily[7]; got.Kind != usecase.RainForecast || got.Source != domain.RainFromModel || !got.Date.Equal(e.today().AddDate(0, 0, 1)) {
		t.Errorf("first forecast = %+v", got)
	}

	if len(e.weather.Cells) != 1 || e.weather.Cells[0] != (domain.WeatherCell{Latitude: -21, Longitude: -47}) {
		t.Errorf("weather cells = %+v", e.weather.Cells)
	}
}

func TestTrafficabilityFallsBackToTheModelWhenTheGaugeIsStale(t *testing.T) {
	e := newConditionsEnv(t)
	gauge := e.createGauge(t, "Sede")
	medium := domain.TextureMedium
	e.conditions.SeedProfile(e.f1, &medium, &gauge.ID)
	e.conditions.SeedReading(domain.RainReading{ID: "old", CompanyID: "co-a", GaugeID: gauge.ID, RainMm: 40, ObservedAt: e.clock.T.Add(-40 * time.Hour), RecordedBy: e.adminA.ID})
	e.seedReady(e.f1, 2)
	e.weather.Report = e.rainyModel(e.dryWeather(0.10, 0), 3, 5) // 5 mm: ideal for medium; moisture 40 % of FC: dry

	view := e.trafficability(t, e.asAdminA(), 2)

	if view.Soil.Rain == nil || view.Soil.Rain.Source != domain.RainFromModel || view.Soil.RainSourceName != nil || view.Soil.Rain.Last48hMm != 5 {
		t.Fatalf("rain = %+v", view.Soil.Rain)
	}
	if view.Soil.State != domain.SoilIdeal || view.Soil.Basis != domain.BasisModel {
		t.Errorf("soil = %s/%s, want ideal/model", view.Soil.State, view.Soil.Basis)
	}
	if len(view.Plan.Days) != 3 {
		t.Errorf("days = %d, want today + 2", len(view.Plan.Days))
	}
	if first := view.RainDaily[0]; first.Source != domain.RainFromModel || first.Kind != usecase.RainMeasured {
		t.Errorf("past days must come from the model: %+v", first)
	}
}

func TestTrafficabilityWithoutWeather(t *testing.T) {
	tests := []struct {
		name       string
		withGauge  bool
		wantState  domain.SoilState
		wantIssues []usecase.TrafficabilityDataIssue
		wantDaily  int
	}{
		{"fresh gauge keeps the measured state, projection dropped", true, domain.SoilWaterlogged,
			[]usecase.TrafficabilityDataIssue{usecase.IssueWeatherUnavailable}, 7},
		{"no gauge: unknown, never invented", false, domain.SoilUnknown,
			[]usecase.TrafficabilityDataIssue{usecase.IssueNoRainSource, usecase.IssueWeatherUnavailable}, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newConditionsEnv(t)
			if tt.withGauge {
				e.clayWithGauge(t, 1)
			} else {
				clay := domain.TextureClay
				e.conditions.SeedProfile(e.f1, &clay, nil)
			}
			e.seedReady(e.f1, 2)
			e.weather.Err = errors.Join(usecase.ErrWeatherUnavailable, errBoom)

			view := e.trafficability(t, e.asAdminA(), 5)

			if view.Soil.State != tt.wantState || !slices.Equal(view.DataIssues, tt.wantIssues) {
				t.Fatalf("state = %s, issues = %v; want %s, %v", view.Soil.State, view.DataIssues, tt.wantState, tt.wantIssues)
			}
			if view.Soil.ModelMoisture != nil {
				t.Error("no model moisture without the model")
			}
			if len(view.Plan.Days) != 1 {
				t.Errorf("days = %d, want only today", len(view.Plan.Days))
			}
			if len(view.RainDaily) != tt.wantDaily {
				t.Errorf("rain_daily = %d, want %d", len(view.RainDaily), tt.wantDaily)
			}
			for _, method := range []domain.SprayMethod{domain.MethodTractorBoom, domain.MethodKnapsack} {
				if tt.wantState == domain.SoilUnknown && methodOf(view.Plan.Days[0], method).Verdict != domain.VerdictAttention {
					t.Errorf("%s must be in attention with an unknown soil", method)
				}
			}
		})
	}
}

func TestTrafficabilityInspection(t *testing.T) {
	inspection := func(e *conditionsEnv, state domain.SoilState, hoursAgo int) {
		e.conditions.SeedInspection(domain.FieldSoilInspection{
			ID: "i1", CompanyID: "co-a", FarmID: "farm-a1", FieldID: e.f1, State: state,
			InspectedAt: e.clock.T.Add(-time.Duration(hoursAgo) * time.Hour), InspectedBy: e.operatorA.ID,
		})
	}
	tests := []struct {
		name        string
		texture     *domain.SoilTexture
		state       domain.SoilState
		hoursAgo    int
		wantState   domain.SoilState
		wantBasis   domain.SoilBasis
		wantVisible bool
	}{
		{"valid inspection replaces the measurement", ptrTo(domain.TextureClay), domain.SoilIdeal, 2, domain.SoilIdeal, domain.BasisInspection, true},
		{"expired inspection is ignored", ptrTo(domain.TextureClay), domain.SoilIdeal, 12, domain.SoilWaterlogged, domain.BasisMeasuredRain, false},
		{"without texture the inspection still stands", nil, domain.SoilMoist, 1, domain.SoilMoist, domain.BasisInspection, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newConditionsEnv(t)
			gauge := e.clayWithGauge(t, 1)
			e.conditions.SeedProfile(e.f1, tt.texture, &gauge.ID)
			inspection(e, tt.state, tt.hoursAgo)
			e.seedReady(e.f1, 2)
			e.weather.Report = e.dryWeather(0.2, 0)

			view := e.trafficability(t, e.asAdminA(), 3)

			if view.Soil.State != tt.wantState || view.Soil.Basis != tt.wantBasis {
				t.Fatalf("soil = %s/%s, want %s/%s", view.Soil.State, view.Soil.Basis, tt.wantState, tt.wantBasis)
			}
			if (view.Soil.Inspection != nil) != tt.wantVisible {
				t.Fatalf("inspection view = %+v, want visible %v", view.Soil.Inspection, tt.wantVisible)
			}
			if tt.wantVisible && (view.Soil.Inspection.InspectedByName != e.operatorA.Name ||
				!view.Soil.Inspection.ValidUntil.Equal(view.Soil.Inspection.Inspection.InspectedAt.Add(12*time.Hour))) {
				t.Errorf("inspection view = %+v", view.Soil.Inspection)
			}
			if tt.texture == nil {
				if !slices.Contains(view.DataIssues, usecase.IssueNoSoilTexture) || view.Soil.Texture != nil || view.Soil.ModelMoisture != nil {
					t.Errorf("without texture: issues %v, soil %+v", view.DataIssues, view.Soil)
				}
				for _, day := range view.Plan.Days[1:] {
					if day.SoilState != domain.SoilUnknown {
						t.Errorf("projection without texture must be unknown, got %s", day.SoilState)
					}
				}
			}
		})
	}
}

func ptrTo[T any](value T) *T { return &value }

func TestTrafficabilityWithoutTerrainOrTexture(t *testing.T) {
	e := newConditionsEnv(t)
	e.weather.Report = e.dryWeather(0.2, 0)

	view := e.trafficability(t, e.asAdminA(), 1)

	want := []usecase.TrafficabilityDataIssue{usecase.IssueNoSoilTexture, usecase.IssueTerrainNotCalculated}
	if !slices.Equal(view.DataIssues, want) {
		t.Fatalf("issues = %v, want %v", view.DataIssues, want)
	}
	if view.Soil.State != domain.SoilUnknown || view.Soil.Basis != domain.BasisNone || view.Soil.ThresholdsSource != domain.ThresholdSourceHeuristic {
		t.Errorf("soil = %+v", view.Soil)
	}
	if got := methodOf(view.Plan.Days[0], domain.MethodTractorBoom); got.Verdict != domain.VerdictAttention ||
		!slices.Contains(got.Reasons, domain.ReasonTerrainUnknown) || !slices.Contains(got.Reasons, domain.ReasonSoilUnknown) {
		t.Errorf("tractor boom = %+v", got)
	}
	if view.Soil.Rain == nil || view.Soil.Rain.Source != domain.RainFromModel {
		t.Errorf("the model still reports rain without texture: %+v", view.Soil.Rain)
	}
}

func TestTrafficabilityRefusals(t *testing.T) {
	e := newConditionsEnv(t)
	e.weather.Report = e.dryWeather(0.2, 0)
	get := e.getTrafficability()
	tests := []struct {
		name  string
		input usecase.GetFieldTrafficabilityInput
		want  error
		field string
	}{
		{"too many days", usecase.GetFieldTrafficabilityInput{Actor: e.asAdminA(), FarmID: "farm-a1", FieldID: e.f1, Days: 8}, nil, "days"},
		{"negative days", usecase.GetFieldTrafficabilityInput{Actor: e.asAdminA(), FarmID: "farm-a1", FieldID: e.f1, Days: -1}, nil, "days"},
		{"another company", usecase.GetFieldTrafficabilityInput{Actor: e.asAdminB(), FarmID: "farm-a1", FieldID: e.f1}, usecase.ErrNotFound, ""},
		{"outside the scope", usecase.GetFieldTrafficabilityInput{Actor: e.operatorOnly(t, "farm-a2"), FarmID: "farm-a1", FieldID: e.f1}, usecase.ErrNotFound, ""},
		{"field of another farm", usecase.GetFieldTrafficabilityInput{Actor: e.asAdminA(), FarmID: "farm-a2", FieldID: e.f1}, usecase.ErrNotFound, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := get.Execute(ctx(), tt.input)
			if tt.field != "" {
				assertValidationOn(t, err, tt.field)
			} else {
				assertIs(t, err, tt.want)
			}
		})
	}

	t.Run("an unexpected weather error is not hidden", func(t *testing.T) {
		e.weather.Err = errBoom
		_, err := get.Execute(ctx(), usecase.GetFieldTrafficabilityInput{Actor: e.asAdminA(), FarmID: "farm-a1", FieldID: e.f1})
		assertIs(t, err, errBoom)
	})
	t.Run("broken thresholds fail loudly", func(t *testing.T) {
		e.weather.Err = nil
		delete(e.conditions.Thresholds, domain.TextureSandy)
		_, err := get.Execute(ctx(), usecase.GetFieldTrafficabilityInput{Actor: e.asAdminA(), FarmID: "farm-a1", FieldID: e.f1})
		assertIs(t, err, domain.ErrInvalidSoilRules)
	})
}
