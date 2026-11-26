package usecase_test

import (
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/domain"
	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/usecase"
	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/usecase/fakes"
)

// conditionsEnv adds rain and soil to the relief world: farm-a1 has two
// fields; farm-a2 (company A) and farm-b1 (company B) have one gauge each.
type conditionsEnv struct {
	*terrainEnv
	conditions *fakes.FieldConditions
	weather    *fakes.Weather
	settings   usecase.TrafficabilitySettings
	brt        *time.Location
}

func newConditionsEnv(t *testing.T) *conditionsEnv {
	t.Helper()
	brt := time.FixedZone("BRT", -3*3600)
	e := &conditionsEnv{
		terrainEnv: newTerrainEnv(t),
		weather:    &fakes.Weather{},
		brt:        brt,
		settings:   usecase.TrafficabilitySettings{Location: brt, GaugeMaxStaleness: 36 * time.Hour, InspectionValidity: 12 * time.Hour},
	}
	assertNoError(t, e.settings.Validate())
	e.conditions = fakes.NewFieldConditions(e.reg)
	e.conditions.UserNames[e.adminA.ID] = e.adminA.Name
	e.conditions.UserNames[e.operatorA.ID] = e.operatorA.Name
	e.conditions.SeedGauge(domain.RainGauge{ID: "g-a2", CompanyID: "co-a", FarmID: "farm-a2", Name: "Boa Vista"})
	e.conditions.SeedGauge(domain.RainGauge{ID: "g-b1", CompanyID: "co-b", FarmID: "farm-b1", Name: "Sede"})
	return e
}

func (e *conditionsEnv) deps() usecase.FieldConditionsDeps {
	return usecase.FieldConditionsDeps{
		Farms: e.reg, Gauges: e.conditions, Fields: e.conditions, Tx: e.conditions,
		Clock: e.clock, IDs: e.ids, Settings: e.settings,
	}
}

func (e *conditionsEnv) createGauge(t *testing.T, name string) domain.RainGauge {
	t.Helper()
	gauge, err := usecase.NewCreateRainGauge(e.deps()).Execute(ctx(), usecase.CreateRainGaugeInput{
		Actor: e.asAdminA(), FarmID: "farm-a1", Details: domain.RainGaugeDetails{Name: name},
	})
	assertNoError(t, err)
	return gauge
}

func (e *conditionsEnv) assertConditionsAudit(t *testing.T, want ...string) {
	t.Helper()
	if got := e.conditions.AuditActions(); !slices.Equal(got, want) {
		t.Fatalf("audit = %v, want %v", got, want)
	}
}

// withoutPermission removes one capability from a principal.
func withoutPermission(actor domain.Principal, permission domain.Permission) domain.Principal {
	actor.Permissions = slices.DeleteFunc(slices.Clone(actor.Permissions), func(p domain.Permission) bool { return p == permission })
	return actor
}

func TestCreateRainGauge(t *testing.T) {
	e := newConditionsEnv(t)
	location := &domain.GeoPoint{Lat: -21.0, Lng: -47.0}
	gauge, err := usecase.NewCreateRainGauge(e.deps()).Execute(ctx(), usecase.CreateRainGaugeInput{
		Actor: e.asAdminA(), FarmID: "farm-a1", Details: domain.RainGaugeDetails{Name: " Sede ", Location: location},
	})
	assertNoError(t, err)
	if gauge.Name != "Sede" || gauge.CompanyID != "co-a" || gauge.FarmID != "farm-a1" || gauge.Version != 1 || gauge.Location == nil {
		t.Fatalf("gauge = %+v", gauge)
	}
	e.assertConditionsAudit(t, domain.AuditRainGaugeCreated)
	if event := e.conditions.Audit[0]; event.EntityType != domain.AuditEntityRainGauge || event.EntityID != string(gauge.ID) || event.Metadata["farm_id"] != "farm-a1" {
		t.Errorf("audit event = %+v", event)
	}

	tests := []struct {
		name  string
		actor domain.Principal
		farm  domain.FarmID
		input domain.RainGaugeDetails
		want  error
		field string
	}{
		{"same name in another case", e.asAdminA(), "farm-a1", domain.RainGaugeDetails{Name: "SEDE"}, nil, "name"},
		{"blank name", e.asAdminA(), "farm-a1", domain.RainGaugeDetails{Name: " "}, nil, "name"},
		{"operator lacks registry.write", e.asOperatorA(), "farm-a1", domain.RainGaugeDetails{Name: "X"}, usecase.ErrForbidden, ""},
		{"farm of another company", e.asAdminA(), "farm-b1", domain.RainGaugeDetails{Name: "X"}, usecase.ErrNotFound, ""},
		{"farm outside the scope", e.writerOutsideScope(t), "farm-a1", domain.RainGaugeDetails{Name: "X"}, usecase.ErrNotFound, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := usecase.NewCreateRainGauge(e.deps()).Execute(ctx(), usecase.CreateRainGaugeInput{Actor: tt.actor, FarmID: tt.farm, Details: tt.input})
			if tt.field != "" {
				assertValidationOn(t, err, tt.field)
			} else {
				assertIs(t, err, tt.want)
			}
		})
	}
	e.assertConditionsAudit(t, domain.AuditRainGaugeCreated)
}

func TestUpdateRainGauge(t *testing.T) {
	e := newConditionsEnv(t)
	created, err := usecase.NewCreateRainGauge(e.deps()).Execute(ctx(), usecase.CreateRainGaugeInput{
		Actor: e.asAdminA(), FarmID: "farm-a1", Details: domain.RainGaugeDetails{Name: "Sede", Location: &domain.GeoPoint{Lat: -21, Lng: -47}},
	})
	assertNoError(t, err)
	e.createGauge(t, "Baixada")
	update := usecase.NewUpdateRainGauge(e.deps())

	newName := "Sede velha"
	renamed, err := update.Execute(ctx(), usecase.UpdateRainGaugeInput{Actor: e.asAdminA(), FarmID: "farm-a1", GaugeID: created.ID, Patch: usecase.RainGaugePatch{Name: &newName}})
	assertNoError(t, err)
	if renamed.Name != newName || renamed.Location == nil || renamed.Version != 2 {
		t.Fatalf("renamed = %+v, want new name, location kept, version 2", renamed)
	}
	cleared, err := update.Execute(ctx(), usecase.UpdateRainGaugeInput{Actor: e.asAdminA(), FarmID: "farm-a1", GaugeID: created.ID, Patch: usecase.RainGaugePatch{LocationSet: true}})
	assertNoError(t, err)
	if cleared.Name != newName || cleared.Location != nil || cleared.Version != 3 {
		t.Fatalf("cleared = %+v, want name kept, location cleared", cleared)
	}

	taken := "baixada"
	_, err = update.Execute(ctx(), usecase.UpdateRainGaugeInput{Actor: e.asAdminA(), FarmID: "farm-a1", GaugeID: created.ID, Patch: usecase.RainGaugePatch{Name: &taken}})
	assertValidationOn(t, err, "name")
	_, err = update.Execute(ctx(), usecase.UpdateRainGaugeInput{Actor: e.asAdminA(), FarmID: "farm-a1", GaugeID: "g-a2", Patch: usecase.RainGaugePatch{Name: &newName}})
	assertIs(t, err, usecase.ErrNotFound) // gauge of another farm
	_, err = update.Execute(ctx(), usecase.UpdateRainGaugeInput{Actor: e.asAdminB(), FarmID: "farm-a1", GaugeID: created.ID, Patch: usecase.RainGaugePatch{Name: &newName}})
	assertIs(t, err, usecase.ErrNotFound)
	e.assertConditionsAudit(t, domain.AuditRainGaugeCreated, domain.AuditRainGaugeCreated, domain.AuditRainGaugeUpdated, domain.AuditRainGaugeUpdated)
}

func TestListRainGauges(t *testing.T) {
	e := newConditionsEnv(t)
	e.createGauge(t, "Sede")
	e.createGauge(t, "baixada")
	list := usecase.NewListRainGauges(e.reg, e.conditions)

	gauges, err := list.Execute(ctx(), usecase.ListRainGaugesInput{Actor: e.operatorOnly(t, "farm-a1"), FarmID: "farm-a1"})
	assertNoError(t, err)
	if len(gauges) != 2 || gauges[0].Name != "baixada" || gauges[1].Name != "Sede" {
		t.Fatalf("gauges = %+v", gauges)
	}
	_, err = list.Execute(ctx(), usecase.ListRainGaugesInput{Actor: e.operatorOnly(t, "farm-a2"), FarmID: "farm-a1"})
	assertIs(t, err, usecase.ErrNotFound)
	_, err = list.Execute(ctx(), usecase.ListRainGaugesInput{Actor: e.asAdminB(), FarmID: "farm-a1"})
	assertIs(t, err, usecase.ErrNotFound)
}

func TestRecordRainReading(t *testing.T) {
	e := newConditionsEnv(t)
	gauge := e.createGauge(t, "Sede")
	record := usecase.NewRecordRainReading(e.deps())
	observed := e.clock.T.Add(-2 * time.Hour)

	stored, err := record.Execute(ctx(), usecase.RecordRainReadingInput{Actor: e.asOperatorA(), FarmID: "farm-a1", GaugeID: gauge.ID, RainMm: 18.5, ObservedAt: observed})
	assertNoError(t, err)
	if stored.Reading.RainMm != 18.5 || stored.Reading.RecordedBy != e.operatorA.ID || stored.RecordedByName != e.operatorA.Name || !stored.Reading.ObservedAt.Equal(observed) {
		t.Fatalf("stored = %+v", stored)
	}

	tests := []struct {
		name  string
		input usecase.RecordRainReadingInput
		want  error
		field string
	}{
		{"same instant", usecase.RecordRainReadingInput{Actor: e.asAdminA(), FarmID: "farm-a1", GaugeID: gauge.ID, RainMm: 2, ObservedAt: observed}, nil, "observed_at"},
		{"in the future", usecase.RecordRainReadingInput{Actor: e.asAdminA(), FarmID: "farm-a1", GaugeID: gauge.ID, RainMm: 2, ObservedAt: e.clock.T.Add(time.Minute)}, nil, "observed_at"},
		{"above 500 mm", usecase.RecordRainReadingInput{Actor: e.asAdminA(), FarmID: "farm-a1", GaugeID: gauge.ID, RainMm: 501, ObservedAt: e.clock.T}, nil, "rain_mm"},
		{"without the capability", usecase.RecordRainReadingInput{Actor: withoutPermission(e.asAdminA(), domain.PermFieldConditionsRecord), FarmID: "farm-a1", GaugeID: gauge.ID, RainMm: 2, ObservedAt: e.clock.T}, usecase.ErrForbidden, ""},
		{"gauge of another company", usecase.RecordRainReadingInput{Actor: e.asAdminA(), FarmID: "farm-b1", GaugeID: "g-b1", RainMm: 2, ObservedAt: e.clock.T}, usecase.ErrNotFound, ""},
		{"gauge of another farm", usecase.RecordRainReadingInput{Actor: e.asAdminA(), FarmID: "farm-a1", GaugeID: "g-a2", RainMm: 2, ObservedAt: e.clock.T}, usecase.ErrNotFound, ""},
		{"farm outside the scope", usecase.RecordRainReadingInput{Actor: e.operatorOnly(t, "farm-a2"), FarmID: "farm-a1", GaugeID: gauge.ID, RainMm: 2, ObservedAt: e.clock.T}, usecase.ErrNotFound, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := record.Execute(ctx(), tt.input)
			if tt.field != "" {
				assertValidationOn(t, err, tt.field)
			} else {
				assertIs(t, err, tt.want)
			}
		})
	}
	if got := len(e.conditions.Readings()); got != 1 {
		t.Errorf("readings = %d, want 1 (failed attempts roll back)", got)
	}
	e.assertConditionsAudit(t, domain.AuditRainGaugeCreated, domain.AuditRainReadingRecorded)
	if event := e.conditions.Audit[1]; event.EntityID != string(gauge.ID) || event.Metadata["rain_mm"] != "18.5" {
		t.Errorf("audit event = %+v", event)
	}
}

func TestListRainReadings(t *testing.T) {
	e := newConditionsEnv(t)
	gauge := e.createGauge(t, "Sede")
	for _, hoursAgo := range []int{1, 50, 24 * 8} {
		e.conditions.SeedReading(domain.RainReading{ID: domain.RainReadingID("r" + strconv.Itoa(hoursAgo)), CompanyID: "co-a", GaugeID: gauge.ID, RainMm: 1, ObservedAt: e.clock.T.Add(-time.Duration(hoursAgo) * time.Hour), RecordedBy: e.adminA.ID})
	}
	list := usecase.NewListRainReadings(e.deps())

	readings, err := list.Execute(ctx(), usecase.ListRainReadingsInput{Actor: e.asOperatorA(), FarmID: "farm-a1", GaugeID: gauge.ID})
	assertNoError(t, err)
	if len(readings) != 2 || !readings[0].Reading.ObservedAt.After(readings[1].Reading.ObservedAt) || readings[0].RecordedByName != e.adminA.Name {
		t.Fatalf("default range must return the 2 readings of the last 7 days, newest first: %+v", readings)
	}
	readings, err = list.Execute(ctx(), usecase.ListRainReadingsInput{Actor: e.asOperatorA(), FarmID: "farm-a1", GaugeID: gauge.ID, After: e.clock.T.Add(-10 * 24 * time.Hour), Until: e.clock.T.Add(-24 * time.Hour)})
	assertNoError(t, err)
	if len(readings) != 2 {
		t.Fatalf("explicit range = %d readings, want 2", len(readings))
	}

	_, err = list.Execute(ctx(), usecase.ListRainReadingsInput{Actor: e.asOperatorA(), FarmID: "farm-a1", GaugeID: gauge.ID, After: e.clock.T.Add(-32 * 24 * time.Hour), Until: e.clock.T})
	assertValidationOn(t, err, "from")
	_, err = list.Execute(ctx(), usecase.ListRainReadingsInput{Actor: e.asOperatorA(), FarmID: "farm-a1", GaugeID: gauge.ID, After: e.clock.T, Until: e.clock.T.Add(-time.Hour)})
	assertValidationOn(t, err, "from")
	_, err = list.Execute(ctx(), usecase.ListRainReadingsInput{Actor: e.asAdminB(), FarmID: "farm-a1", GaugeID: gauge.ID})
	assertIs(t, err, usecase.ErrNotFound)
}

func TestRecordSoilInspection(t *testing.T) {
	e := newConditionsEnv(t)
	record := usecase.NewRecordSoilInspection(e.deps())
	note := "barro na baixada"

	view, err := record.Execute(ctx(), usecase.RecordSoilInspectionInput{
		Actor: e.asOperatorA(), FarmID: "farm-a1", FieldID: e.f1,
		Details: domain.SoilInspectionDetails{State: domain.SoilMoist, Note: &note},
	})
	assertNoError(t, err)
	if view.Inspection.State != domain.SoilMoist || !view.Inspection.InspectedAt.Equal(e.clock.T) || view.InspectedByName != e.operatorA.Name ||
		!view.ValidUntil.Equal(e.clock.T.Add(12*time.Hour)) || view.Inspection.FarmID != "farm-a1" {
		t.Fatalf("view = %+v", view)
	}
	e.assertConditionsAudit(t, domain.AuditSoilInspectionRecorded)
	if metadata := e.conditions.Audit[0].Metadata; metadata["state"] != "moist" || metadata["note"] != "" {
		t.Errorf("audit metadata = %v (the note must not be audited)", metadata)
	}

	tests := []struct {
		name  string
		input usecase.RecordSoilInspectionInput
		want  error
		field string
	}{
		{"unknown state", usecase.RecordSoilInspectionInput{Actor: e.asAdminA(), FarmID: "farm-a1", FieldID: e.f1, Details: domain.SoilInspectionDetails{State: domain.SoilUnknown}}, nil, "state"},
		{"field of another farm", usecase.RecordSoilInspectionInput{Actor: e.asAdminA(), FarmID: "farm-a2", FieldID: e.f1, Details: domain.SoilInspectionDetails{State: domain.SoilDry}}, usecase.ErrNotFound, ""},
		{"another company", usecase.RecordSoilInspectionInput{Actor: e.asAdminB(), FarmID: "farm-a1", FieldID: e.f1, Details: domain.SoilInspectionDetails{State: domain.SoilDry}}, usecase.ErrNotFound, ""},
		{"without the capability", usecase.RecordSoilInspectionInput{Actor: withoutPermission(e.asOperatorA(), domain.PermFieldConditionsRecord), FarmID: "farm-a1", FieldID: e.f1, Details: domain.SoilInspectionDetails{State: domain.SoilDry}}, usecase.ErrForbidden, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := record.Execute(ctx(), tt.input)
			if tt.field != "" {
				assertValidationOn(t, err, tt.field)
			} else {
				assertIs(t, err, tt.want)
			}
		})
	}
	if got := len(e.conditions.Inspections()); got != 1 {
		t.Errorf("inspections = %d, want 1", got)
	}
}

func TestSoilProfile(t *testing.T) {
	e := newConditionsEnv(t)
	gauge := e.createGauge(t, "Sede")
	put := usecase.NewUpdateSoilProfile(e.deps())
	get := usecase.NewGetSoilProfile(e.conditions)
	clay := domain.TextureClay

	view, err := put.Execute(ctx(), usecase.UpdateSoilProfileInput{Actor: e.asAdminA(), FarmID: "farm-a1", FieldID: e.f1, Texture: &clay, GaugeID: &gauge.ID})
	assertNoError(t, err)
	if view.Texture == nil || *view.Texture != clay || view.Gauge == nil || view.Gauge.ID != gauge.ID {
		t.Fatalf("view = %+v", view)
	}
	read, err := get.Execute(ctx(), usecase.GetSoilProfileInput{Actor: e.operatorOnly(t, "farm-a1"), FarmID: "farm-a1", FieldID: e.f1})
	assertNoError(t, err)
	if read.Texture == nil || read.Gauge == nil || read.Gauge.Name != "Sede" {
		t.Fatalf("read = %+v", read)
	}

	otherFarm, otherCompany := domain.RainGaugeID("g-a2"), domain.RainGaugeID("g-b1")
	bogus := domain.SoilTexture("loam")
	tests := []struct {
		name  string
		input usecase.UpdateSoilProfileInput
		want  error
		field string
	}{
		{"gauge of another farm", usecase.UpdateSoilProfileInput{Actor: e.asAdminA(), FarmID: "farm-a1", FieldID: e.f1, GaugeID: &otherFarm}, nil, "rain_gauge_id"},
		{"gauge of another company", usecase.UpdateSoilProfileInput{Actor: e.asAdminA(), FarmID: "farm-a1", FieldID: e.f1, GaugeID: &otherCompany}, nil, "rain_gauge_id"},
		{"unknown texture", usecase.UpdateSoilProfileInput{Actor: e.asAdminA(), FarmID: "farm-a1", FieldID: e.f1, Texture: &bogus}, nil, "soil_texture"},
		{"operator lacks registry.write", usecase.UpdateSoilProfileInput{Actor: e.asOperatorA(), FarmID: "farm-a1", FieldID: e.f1}, usecase.ErrForbidden, ""},
		{"field of another company", usecase.UpdateSoilProfileInput{Actor: e.asAdminB(), FarmID: "farm-a1", FieldID: e.f1}, usecase.ErrNotFound, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := put.Execute(ctx(), tt.input)
			if tt.field != "" {
				assertValidationOn(t, err, tt.field)
			} else {
				assertIs(t, err, tt.want)
			}
		})
	}

	cleared, err := put.Execute(ctx(), usecase.UpdateSoilProfileInput{Actor: e.asAdminA(), FarmID: "farm-a1", FieldID: e.f1})
	assertNoError(t, err)
	if cleared.Texture != nil || cleared.Gauge != nil {
		t.Fatalf("cleared = %+v", cleared)
	}
	e.assertConditionsAudit(t, domain.AuditRainGaugeCreated, domain.AuditSoilProfileUpdated, domain.AuditSoilProfileUpdated)
	if metadata := e.conditions.Audit[1].Metadata; metadata["soil_texture"] != "clay" || metadata["rain_gauge_id"] != string(gauge.ID) {
		t.Errorf("audit metadata = %v", metadata)
	}
	_, err = get.Execute(ctx(), usecase.GetSoilProfileInput{Actor: e.asAdminB(), FarmID: "farm-a1", FieldID: e.f1})
	assertIs(t, err, usecase.ErrNotFound)
}
