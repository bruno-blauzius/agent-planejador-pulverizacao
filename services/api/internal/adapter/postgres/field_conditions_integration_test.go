//go:build integration

package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/domain"
	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/usecase"
)

type conditionsFixture struct {
	company domain.Company
	user    domain.User
	farm    domain.Farm
	other   domain.Farm // second farm of the same company
	field   domain.Field
	field2  domain.Field // field of the other farm
}

func newConditionsFixture(t *testing.T) conditionsFixture {
	t.Helper()
	company := newCompany(t)
	f := conditionsFixture{company: company, user: newUser(t, company)}
	f.farm = farmValue(company, "Fazenda Chuva")
	insertFarm(t, f.farm)
	f.other = farmValue(company, "Fazenda Vizinha")
	insertFarm(t, f.other)
	f.field = fieldValue(f.farm, "Talhão 4", fieldRect(1, 1, 3, 3))
	insertField(t, f.field)
	f.field2 = fieldValue(f.other, "Talhão 9", fieldRect(1, 1, 3, 3))
	insertField(t, f.field2)
	return f
}

func inConditionsTx(t *testing.T, fn func(ctx context.Context, tx usecase.FieldConditionsStores) error) error {
	t.Helper()
	return testDB.InFieldConditionsTx(ctx(), fn)
}

func (f conditionsFixture) gauge(t *testing.T, farm domain.Farm, name string, location *domain.GeoPoint) domain.RainGauge {
	t.Helper()
	gauge, err := domain.NewRainGauge(domain.RainGaugeID(newID()), f.company.ID, farm.ID, domain.RainGaugeDetails{Name: name, Location: location}, nowMicro())
	must(t, err)
	must(t, inConditionsTx(t, func(ctx context.Context, tx usecase.FieldConditionsStores) error {
		if err := tx.Conditions.LockFarmForConditions(ctx, f.company.ID, farm.ID); err != nil {
			return err
		}
		return tx.Conditions.InsertRainGauge(ctx, gauge)
	}))
	gauge.Version = 1
	return gauge
}

func assertValidationField(t *testing.T, err error, field string) {
	t.Helper()
	var invalid *domain.ValidationError
	if !errors.As(err, &invalid) || len(invalid.Fields) == 0 || invalid.Fields[0].Field != field {
		t.Fatalf("err = %v, want a validation error on %s", err, field)
	}
}

func TestRainGauges(t *testing.T) {
	f := newConditionsFixture(t)
	location := &domain.GeoPoint{Lat: -21.0021, Lng: -47.0012}
	sede := f.gauge(t, f.farm, "Sede", location)
	f.gauge(t, f.farm, "baixada", nil)

	gauges, err := testDB.ListRainGauges(ctx(), f.company.ID, f.farm.ID)
	must(t, err)
	if len(gauges) != 2 || gauges[0].Name != "baixada" || gauges[0].Location != nil || gauges[1].Name != "Sede" {
		t.Fatalf("gauges = %+v", gauges)
	}
	if got := gauges[1].Location; got == nil || *got != *location || gauges[1].Version != 1 {
		t.Fatalf("location = %+v, version %d", got, gauges[1].Version)
	}

	duplicate, err := domain.NewRainGauge(domain.RainGaugeID(newID()), f.company.ID, f.farm.ID, domain.RainGaugeDetails{Name: "SEDE"}, nowMicro())
	must(t, err)
	err = inConditionsTx(t, func(ctx context.Context, tx usecase.FieldConditionsStores) error {
		return tx.Conditions.InsertRainGauge(ctx, duplicate)
	})
	assertValidationField(t, err, "name")
	// The same name in another farm is fine.
	f.gauge(t, f.other, "Sede", nil)

	// Update with optimistic concurrency.
	err = inConditionsTx(t, func(ctx context.Context, tx usecase.FieldConditionsStores) error {
		locked, err := tx.Conditions.LockRainGauge(ctx, f.company.ID, f.farm.ID, sede.ID)
		if err != nil {
			return err
		}
		revised, err := locked.Revise(domain.RainGaugeDetails{Name: "Sede nova"}, nowMicro())
		if err != nil {
			return err
		}
		return tx.Conditions.UpdateRainGauge(ctx, revised)
	})
	must(t, err)
	found, err := testDB.FindRainGauge(ctx(), f.company.ID, f.farm.ID, sede.ID)
	must(t, err)
	if found.Name != "Sede nova" || found.Location != nil || found.Version != 2 {
		t.Fatalf("updated = %+v", found)
	}
	err = inConditionsTx(t, func(ctx context.Context, tx usecase.FieldConditionsStores) error {
		return tx.Conditions.UpdateRainGauge(ctx, sede) // version 1 is outdated
	})
	if !errors.Is(err, usecase.ErrVersionConflict) {
		t.Fatalf("stale update = %v, want ErrVersionConflict", err)
	}

	// Tenant and farm isolation.
	other := newCompany(t)
	if _, err := testDB.FindRainGauge(ctx(), other.ID, f.farm.ID, sede.ID); !errors.Is(err, usecase.ErrNotFound) {
		t.Errorf("gauge seen by another company: %v", err)
	}
	if _, err := testDB.FindRainGauge(ctx(), f.company.ID, f.other.ID, sede.ID); !errors.Is(err, usecase.ErrNotFound) {
		t.Errorf("gauge seen through another farm: %v", err)
	}
	if list, err := testDB.ListRainGauges(ctx(), other.ID, f.farm.ID); err != nil || len(list) != 0 {
		t.Errorf("listing of another company = %v, %v", list, err)
	}
	if err := inConditionsTx(t, func(ctx context.Context, tx usecase.FieldConditionsStores) error {
		return tx.Conditions.LockFarmForConditions(ctx, other.ID, f.farm.ID)
	}); !errors.Is(err, usecase.ErrNotFound) {
		t.Errorf("lock farm of another company = %v", err)
	}
	if _, err := testDB.FindRainGauge(ctx(), f.company.ID, f.farm.ID, "not-a-uuid"); !errors.Is(err, usecase.ErrNotFound) {
		t.Errorf("malformed id = %v", err)
	}
}

func TestRainReadings(t *testing.T) {
	f := newConditionsFixture(t)
	gauge := f.gauge(t, f.farm, "Sede", nil)
	base := nowMicro().Add(-time.Hour)
	record := func(mm float64, observedAt time.Time) error {
		reading, err := domain.NewRainReading(domain.RainReadingID(newID()), gauge, mm, observedAt, f.user.ID, nowMicro())
		must(t, err)
		return inConditionsTx(t, func(ctx context.Context, tx usecase.FieldConditionsStores) error {
			return tx.Conditions.InsertRainReading(ctx, reading)
		})
	}
	must(t, record(18.25, base.Add(-30*time.Hour)))
	must(t, record(6, base))
	must(t, record(40, base.Add(-10*24*time.Hour)))
	assertValidationField(t, record(1, base), "observed_at")

	readings, err := testDB.ListRainReadings(ctx(), f.company.ID, gauge.ID, base.Add(-7*24*time.Hour), base)
	must(t, err)
	if len(readings) != 2 || readings[0].Reading.RainMm != 6 || readings[1].Reading.RainMm != 18.25 {
		t.Fatalf("readings = %+v", readings)
	}
	if got := readings[0]; got.RecordedByName != f.user.Name || got.Reading.RecordedBy != f.user.ID || !got.Reading.ObservedAt.Equal(base) {
		t.Errorf("reading = %+v", got)
	}
	// (after, until]: a reading exactly at `after` is out.
	readings, err = testDB.ListRainReadings(ctx(), f.company.ID, gauge.ID, base, base.Add(time.Hour))
	must(t, err)
	if len(readings) != 0 {
		t.Errorf("readings after base = %+v", readings)
	}
	other := newCompany(t)
	readings, err = testDB.ListRainReadings(ctx(), other.ID, gauge.ID, base.Add(-30*24*time.Hour), base)
	must(t, err)
	if len(readings) != 0 {
		t.Errorf("readings seen by another company: %+v", readings)
	}
}

func TestSoilProfileAndConditions(t *testing.T) {
	f := newConditionsFixture(t)
	gauge := f.gauge(t, f.farm, "Sede", &domain.GeoPoint{Lat: -21, Lng: -47})
	otherFarmGauge := f.gauge(t, f.other, "Vizinha", nil)

	data, err := testDB.ReadFieldConditions(ctx(), f.company.ID, f.farm.ID, f.field.ID)
	must(t, err)
	if data.Texture != nil || data.Gauge != nil || data.FarmID != f.farm.ID {
		t.Fatalf("fresh field = %+v", data)
	}
	// fieldRect(1,1,3,3) is centred at (-46.998, -20.998).
	if diff := data.Centroid.Lat + 20.998; diff > 1e-6 || diff < -1e-6 {
		t.Errorf("centroid = %+v", data.Centroid)
	}

	clay := domain.TextureClay
	save := func(field domain.Field, profile domain.SoilProfile) error {
		return inConditionsTx(t, func(ctx context.Context, tx usecase.FieldConditionsStores) error {
			if _, err := tx.Conditions.LockFieldForConditions(ctx, f.company.ID, field.FarmID, field.ID); err != nil {
				return err
			}
			return tx.Conditions.SaveSoilProfile(ctx, f.company.ID, field.FarmID, field.ID, profile, nowMicro())
		})
	}
	must(t, save(f.field, domain.SoilProfile{Texture: &clay, RainGaugeID: &gauge.ID}))
	data, err = testDB.ReadFieldConditions(ctx(), f.company.ID, f.farm.ID, f.field.ID)
	must(t, err)
	if data.Texture == nil || *data.Texture != clay || data.Gauge == nil || data.Gauge.ID != gauge.ID || data.Gauge.Location == nil {
		t.Fatalf("profile = %+v", data)
	}
	// Re-linking replaces; a gauge of another farm is refused by the composite FK.
	must(t, save(f.field, domain.SoilProfile{Texture: &clay, RainGaugeID: &gauge.ID}))
	assertValidationField(t, save(f.field, domain.SoilProfile{RainGaugeID: &otherFarmGauge.ID}), "rain_gauge_id")
	// Clearing.
	must(t, save(f.field, domain.SoilProfile{}))
	data, err = testDB.ReadFieldConditions(ctx(), f.company.ID, f.farm.ID, f.field.ID)
	must(t, err)
	if data.Texture != nil || data.Gauge != nil {
		t.Fatalf("cleared profile = %+v", data)
	}

	other := newCompany(t)
	if _, err := testDB.ReadFieldConditions(ctx(), other.ID, f.farm.ID, f.field.ID); !errors.Is(err, usecase.ErrNotFound) {
		t.Errorf("field seen by another company: %v", err)
	}
	if _, err := testDB.ReadFieldConditions(ctx(), f.company.ID, f.other.ID, f.field.ID); !errors.Is(err, usecase.ErrNotFound) {
		t.Errorf("field seen through another farm: %v", err)
	}
	if err := inConditionsTx(t, func(ctx context.Context, tx usecase.FieldConditionsStores) error {
		_, err := tx.Conditions.LockFieldForConditions(ctx, other.ID, f.farm.ID, f.field.ID)
		return err
	}); !errors.Is(err, usecase.ErrNotFound) {
		t.Errorf("lock field of another company = %v", err)
	}
}

func TestSoilInspections(t *testing.T) {
	f := newConditionsFixture(t)
	if latest, err := testDB.LatestSoilInspection(ctx(), f.company.ID, f.field.ID); err != nil || latest != nil {
		t.Fatalf("no inspection yet = %+v, %v", latest, err)
	}
	note := "barro na baixada"
	insert := func(state domain.SoilState, at time.Time, note *string) {
		t.Helper()
		inspection, err := domain.NewFieldSoilInspection(domain.SoilInspectionID(newID()), f.field,
			domain.SoilInspectionDetails{State: state, InspectedAt: at, Note: note}, f.user.ID, nowMicro())
		must(t, err)
		must(t, inConditionsTx(t, func(ctx context.Context, tx usecase.FieldConditionsStores) error {
			return tx.Conditions.InsertSoilInspection(ctx, inspection)
		}))
	}
	insert(domain.SoilMoist, nowMicro().Add(-time.Hour), &note)
	insert(domain.SoilDry, nowMicro().Add(-5*time.Hour), nil)

	latest, err := testDB.LatestSoilInspection(ctx(), f.company.ID, f.field.ID)
	must(t, err)
	if latest == nil || latest.Inspection.State != domain.SoilMoist || latest.Inspection.Note == nil || *latest.Inspection.Note != note ||
		latest.InspectedByName != f.user.Name || latest.Inspection.FarmID != f.farm.ID {
		t.Fatalf("latest = %+v", latest)
	}
	other := newCompany(t)
	if latest, err := testDB.LatestSoilInspection(ctx(), other.ID, f.field.ID); err != nil || latest != nil {
		t.Errorf("inspection seen by another company: %+v, %v", latest, err)
	}
}

func TestCurrentTrafficabilityThresholds(t *testing.T) {
	thresholds, err := testDB.CurrentTrafficabilityThresholds(ctx(), time.Date(2026, 11, 26, 0, 0, 0, 0, time.FixedZone("BRT", -3*3600)))
	must(t, err)
	rules := domain.SoilTrafficabilityRules{Thresholds: thresholds, InspectionValidity: 12 * time.Hour}
	must(t, rules.Validate())
	defaults := domain.DefaultSoilTrafficabilityRules().Thresholds
	for texture, want := range defaults {
		got := thresholds[texture]
		if got.Rain48hUpperMm != want.Rain48hUpperMm || got.MoistureUpperPctOfFC != want.MoistureUpperPctOfFC ||
			got.Rain72hFactor != want.Rain72hFactor || got.DryingRateMmPerDay != want.DryingRateMmPerDay ||
			got.FieldCapacityM3M3 != want.FieldCapacityM3M3 || got.Source != domain.ThresholdSourceHeuristic {
			t.Errorf("%s = %+v, want the seeded heuristic %+v", texture, got, want)
		}
	}
	before, err := testDB.CurrentTrafficabilityThresholds(ctx(), time.Date(2026, 11, 25, 0, 0, 0, 0, time.UTC))
	must(t, err)
	if len(before) != 0 {
		t.Errorf("thresholds before the effective date = %+v", before)
	}
}
