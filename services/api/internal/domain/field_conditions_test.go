package domain_test

import (
	"errors"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/domain"
)

var conditionsNow = time.Date(2026, 11, 26, 10, 0, 0, 0, time.UTC)

func validationField(t *testing.T, err error) string {
	t.Helper()
	var invalid *domain.ValidationError
	if !errors.As(err, &invalid) || len(invalid.Fields) == 0 {
		t.Fatalf("got %v, want a validation error", err)
	}
	return invalid.Fields[0].Field
}

func TestNewRainGauge(t *testing.T) {
	tests := []struct {
		name      string
		details   domain.RainGaugeDetails
		wantField string
	}{
		{"valid without location", domain.RainGaugeDetails{Name: "  Sede  "}, ""},
		{"valid with location", domain.RainGaugeDetails{Name: "Sede", Location: &domain.GeoPoint{Lat: -21.2, Lng: -47.1}}, ""},
		{"blank name", domain.RainGaugeDetails{Name: "   "}, "name"},
		{"name too long", domain.RainGaugeDetails{Name: strings.Repeat("a", 101)}, "name"},
		{"name at the limit", domain.RainGaugeDetails{Name: strings.Repeat("á", 100)}, ""},
		{"location out of range", domain.RainGaugeDetails{Name: "Sede", Location: &domain.GeoPoint{Lat: -91, Lng: 0}}, "location"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gauge, err := domain.NewRainGauge("g1", "c1", "f1", tt.details, conditionsNow)
			if tt.wantField != "" {
				if got := validationField(t, err); got != tt.wantField {
					t.Errorf("field = %s, want %s", got, tt.wantField)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if gauge.Name != strings.TrimSpace(tt.details.Name) || gauge.FarmID != "f1" || !gauge.CreatedAt.Equal(conditionsNow) {
				t.Errorf("gauge = %+v", gauge)
			}
		})
	}
}

func TestRainGaugeRevise(t *testing.T) {
	gauge, err := domain.NewRainGauge("g1", "c1", "f1", domain.RainGaugeDetails{Name: "Sede", Location: &domain.GeoPoint{Lat: -21, Lng: -47}}, conditionsNow)
	if err != nil {
		t.Fatal(err)
	}
	later := conditionsNow.Add(time.Hour)
	revised, err := gauge.Revise(domain.RainGaugeDetails{Name: "Baixada"}, later)
	if err != nil {
		t.Fatal(err)
	}
	if revised.Name != "Baixada" || revised.Location != nil || !revised.UpdatedAt.Equal(later) || !revised.CreatedAt.Equal(conditionsNow) {
		t.Errorf("revised = %+v", revised)
	}
	if _, err := gauge.Revise(domain.RainGaugeDetails{Name: ""}, later); validationField(t, err) != "name" {
		t.Error("blank name must be refused")
	}
}

func TestNewRainReading(t *testing.T) {
	gauge := domain.RainGauge{ID: "g1", CompanyID: "c1", FarmID: "f1"}
	tests := []struct {
		name       string
		rainMm     float64
		observedAt time.Time
		wantField  string
	}{
		{"zero rain", 0, conditionsNow.Add(-time.Hour), ""},
		{"upper limit", 500, conditionsNow, ""},
		{"above the limit", 500.1, conditionsNow, "rain_mm"},
		{"negative", -0.1, conditionsNow, "rain_mm"},
		{"not a number", math.NaN(), conditionsNow, "rain_mm"},
		{"in the future", 10, conditionsNow.Add(time.Second), "observed_at"},
		{"missing time", 10, time.Time{}, "observed_at"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reading, err := domain.NewRainReading("r1", gauge, tt.rainMm, tt.observedAt, "u1", conditionsNow)
			if tt.wantField != "" {
				if got := validationField(t, err); got != tt.wantField {
					t.Errorf("field = %s, want %s", got, tt.wantField)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if reading.GaugeID != "g1" || reading.CompanyID != "c1" || reading.RecordedBy != "u1" || reading.RainMm != tt.rainMm {
				t.Errorf("reading = %+v", reading)
			}
		})
	}
}

func reading(hoursAgo float64, mm float64) domain.RainReading {
	return domain.RainReading{RainMm: mm, ObservedAt: conditionsNow.Add(-time.Duration(hoursAgo * float64(time.Hour)))}
}

func TestGaugeRainAccumulation(t *testing.T) {
	gauge := domain.RainGauge{ID: "g1"}
	staleness := 36 * time.Hour
	tests := []struct {
		name     string
		readings []domain.RainReading
		want     *domain.RainAccumulation
	}{
		{"no readings", nil, nil},
		{"latest reading too old", []domain.RainReading{reading(36, 10)}, nil},
		{"only a future reading", []domain.RainReading{reading(-1, 10)}, nil},
		{
			"sums per window, boundaries excluded",
			[]domain.RainReading{reading(1, 6), reading(24, 18), reading(47, 5), reading(48, 7), reading(71, 20), reading(167, 15), reading(168, 100), reading(-2, 50)},
			&domain.RainAccumulation{Last24hMm: 6, Last48hMm: 29, Last72hMm: 56, Last7dMm: 71, Source: domain.RainFromGauge, SourceID: "g1", ObservedAt: conditionsNow.Add(-time.Hour)},
		},
		{
			"a dry reading keeps the gauge fresh",
			[]domain.RainReading{reading(2, 0)},
			&domain.RainAccumulation{Source: domain.RainFromGauge, SourceID: "g1", ObservedAt: conditionsNow.Add(-2 * time.Hour)},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := domain.GaugeRainAccumulation(conditionsNow, gauge, tt.readings, staleness)
			if (got == nil) != (tt.want == nil) {
				t.Fatalf("got %+v, want %+v", got, tt.want)
			}
			if got != nil && *got != *tt.want {
				t.Errorf("got %+v, want %+v", *got, *tt.want)
			}
		})
	}
}

func TestDailyGaugeRain(t *testing.T) {
	loc := time.FixedZone("BRT", -3*3600)
	day := func(d int) time.Time { return time.Date(2026, 11, d, 0, 0, 0, 0, loc) }
	readings := []domain.RainReading{
		{RainMm: 4, ObservedAt: time.Date(2026, 11, 25, 2, 0, 0, 0, time.UTC)},   // 24/11 23:00 local
		{RainMm: 18, ObservedAt: time.Date(2026, 11, 25, 10, 0, 0, 0, time.UTC)}, // 25/11 07:00 local
		{RainMm: 2, ObservedAt: time.Date(2026, 11, 25, 20, 0, 0, 0, time.UTC)},
		{RainMm: 6, ObservedAt: time.Date(2026, 11, 26, 10, 0, 0, 0, time.UTC)},
	}
	got := domain.DailyGaugeRain(readings, []time.Time{day(24), day(25), day(26), day(27)}, loc)
	want := []float64{4, 20, 6, 0}
	for i, entry := range got {
		if entry.Mm != want[i] || !entry.Date.Equal(day(24+i)) {
			t.Errorf("day %d = %+v, want %v mm", 24+i, entry, want[i])
		}
	}
}

func TestWeatherCellOf(t *testing.T) {
	tests := []struct {
		point domain.GeoPoint
		want  domain.WeatherCell
	}{
		{domain.GeoPoint{Lat: -21.12345, Lng: -47.98765}, domain.WeatherCell{Latitude: -21.12, Longitude: -47.99}},
		{domain.GeoPoint{Lat: 0.306, Lng: 0.1}, domain.WeatherCell{Latitude: 0.31, Longitude: 0.1}},
		{domain.GeoPoint{Lat: -0.004, Lng: 179.996}, domain.WeatherCell{Latitude: 0, Longitude: 180}},
	}
	for _, tt := range tests {
		got := domain.WeatherCellOf(tt.point)
		if math.Abs(got.Latitude-tt.want.Latitude) > 1e-12 || math.Abs(got.Longitude-tt.want.Longitude) > 1e-12 {
			t.Errorf("WeatherCellOf(%+v) = %+v, want %+v", tt.point, got, tt.want)
		}
	}
}

func mm(value float64) *float64 { return &value }

func TestModelRainAccumulationAndMoisture(t *testing.T) {
	hour := func(hoursAgo int, rain, moisture *float64) domain.HourlyWeather {
		return domain.HourlyWeather{Time: conditionsNow.Add(-time.Duration(hoursAgo) * time.Hour), RainMm: rain, SoilMoistureM3M3: moisture}
	}
	hours := []domain.HourlyWeather{
		hour(-3, mm(40), mm(0.5)), // forecast: ignored
		hour(0, mm(1), nil),
		hour(1, mm(2), mm(0.31)),
		hour(30, mm(10), mm(0.2)),
		hour(60, nil, nil),
		hour(70, mm(5), nil),
		hour(100, mm(7), nil),
		hour(168, mm(99), nil),
	}
	got := domain.ModelRainAccumulation(conditionsNow, hours)
	want := domain.RainAccumulation{Last24hMm: 3, Last48hMm: 13, Last72hMm: 18, Last7dMm: 25, Source: domain.RainFromModel, ObservedAt: conditionsNow}
	if got == nil || *got != want {
		t.Fatalf("accumulation = %+v, want %+v", got, want)
	}
	if moisture := domain.ModelMoistureAt(conditionsNow, hours); moisture == nil || *moisture != 0.31 {
		t.Errorf("moisture = %v, want 0.31", moisture)
	}
	if domain.ModelRainAccumulation(conditionsNow, []domain.HourlyWeather{hour(-1, mm(3), nil), hour(2, nil, nil)}) != nil {
		t.Error("only forecast or missing rain must give no accumulation")
	}
	if domain.ModelMoistureAt(conditionsNow, nil) != nil {
		t.Error("no hours must give no moisture")
	}
}

func TestNewFieldSoilInspection(t *testing.T) {
	field := domain.Field{ID: "fd1", CompanyID: "c1", FarmID: "f1"}
	note := func(text string) *string { return &text }
	tests := []struct {
		name      string
		details   domain.SoilInspectionDetails
		wantField string
		wantNote  *string
		wantAt    time.Time
	}{
		{"defaults to now", domain.SoilInspectionDetails{State: domain.SoilMoist}, "", nil, conditionsNow},
		{"keeps the time and trims the note", domain.SoilInspectionDetails{State: domain.SoilIdeal, InspectedAt: conditionsNow.Add(-time.Hour), Note: note("  barro na baixada ")}, "", note("barro na baixada"), conditionsNow.Add(-time.Hour)},
		{"blank note is no note", domain.SoilInspectionDetails{State: domain.SoilDry, Note: note("   ")}, "", nil, conditionsNow},
		{"unknown state", domain.SoilInspectionDetails{State: domain.SoilUnknown}, "state", nil, time.Time{}},
		{"bogus state", domain.SoilInspectionDetails{State: "muddy"}, "state", nil, time.Time{}},
		{"future", domain.SoilInspectionDetails{State: domain.SoilDry, InspectedAt: conditionsNow.Add(time.Minute)}, "inspected_at", nil, time.Time{}},
		{"note too long", domain.SoilInspectionDetails{State: domain.SoilDry, Note: note(strings.Repeat("x", 501))}, "note", nil, time.Time{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			inspection, err := domain.NewFieldSoilInspection("i1", field, tt.details, "u1", conditionsNow)
			if tt.wantField != "" {
				if got := validationField(t, err); got != tt.wantField {
					t.Errorf("field = %s, want %s", got, tt.wantField)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !inspection.InspectedAt.Equal(tt.wantAt) || inspection.FieldID != "fd1" || inspection.FarmID != "f1" || inspection.InspectedBy != "u1" {
				t.Errorf("inspection = %+v", inspection)
			}
			if (inspection.Note == nil) != (tt.wantNote == nil) || (tt.wantNote != nil && *inspection.Note != *tt.wantNote) {
				t.Errorf("note = %v, want %v", inspection.Note, tt.wantNote)
			}
			if assessment := inspection.Assessment(); assessment.State != tt.details.State || !assessment.InspectedAt.Equal(tt.wantAt) {
				t.Errorf("assessment = %+v", assessment)
			}
		})
	}
}

func TestNewSoilProfile(t *testing.T) {
	clay, bogus := domain.TextureClay, domain.SoilTexture("loam")
	gauge, blank := domain.RainGaugeID("g1"), domain.RainGaugeID(" ")
	tests := []struct {
		name      string
		texture   *domain.SoilTexture
		gauge     *domain.RainGaugeID
		wantField string
	}{
		{"empty profile", nil, nil, ""},
		{"texture and gauge", &clay, &gauge, ""},
		{"unknown texture", &bogus, nil, "soil_texture"},
		{"blank gauge", nil, &blank, "rain_gauge_id"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			profile, err := domain.NewSoilProfile(tt.texture, tt.gauge)
			if tt.wantField != "" {
				if got := validationField(t, err); got != tt.wantField {
					t.Errorf("field = %s, want %s", got, tt.wantField)
				}
				return
			}
			if err != nil || profile.Texture != tt.texture || profile.RainGaugeID != tt.gauge {
				t.Errorf("profile = %+v, err = %v", profile, err)
			}
		})
	}
}

func TestDefaultThresholdsCarryTheReferenceFieldCapacity(t *testing.T) {
	want := map[domain.SoilTexture]float64{domain.TextureSandy: 0.15, domain.TextureMedium: 0.25, domain.TextureClay: 0.36}
	rules := domain.DefaultSoilTrafficabilityRules()
	for texture, capacity := range want {
		if got := rules.Thresholds[texture].FieldCapacityM3M3; got != capacity {
			t.Errorf("%s field capacity = %v, want %v", texture, got, capacity)
		}
	}
}

func TestInspectionValidity(t *testing.T) {
	calc, err := domain.NewSoilStateCalculator(domain.DefaultSoilTrafficabilityRules())
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name     string
		state    domain.SoilState
		hoursAgo float64
		want     bool
	}{
		{"fresh", domain.SoilMoist, 1, true},
		{"just before the validity", domain.SoilMoist, 11.99, true},
		{"exactly at the validity", domain.SoilMoist, 12, false},
		{"in the future", domain.SoilMoist, -1, false},
		{"unknown state", domain.SoilUnknown, 1, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			inspection := domain.SoilInspection{State: tt.state, InspectedAt: conditionsNow.Add(-time.Duration(tt.hoursAgo * float64(time.Hour)))}
			if got := calc.InspectionValid(conditionsNow, inspection); got != tt.want {
				t.Errorf("valid = %v, want %v", got, tt.want)
			}
			if until := calc.InspectionValidUntil(inspection); !until.Equal(inspection.InspectedAt.Add(12 * time.Hour)) {
				t.Errorf("valid until = %v", until)
			}
		})
	}
}
