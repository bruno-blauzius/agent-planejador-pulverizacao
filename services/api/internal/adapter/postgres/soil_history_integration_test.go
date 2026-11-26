//go:build integration

package postgres_test

import (
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/adapter/postgres"
	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/domain"
	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/usecase"
)

func historyRecord(f conditionsFixture, field domain.Field, day time.Time, state domain.SoilState) usecase.SoilStateRecord {
	source, last48h, last72h, pct := domain.RainFromGauge, 24.0, 55.25, 113.9
	clay := domain.TextureClay
	return usecase.SoilStateRecord{
		Field:    usecase.FieldRef{CompanyID: f.company.ID, FarmID: field.FarmID, FieldID: field.ID},
		LocalDay: day, State: state, Basis: domain.BasisMeasuredRain,
		RainSource: &source, Last48hMm: &last48h, Last72hMm: &last72h, ModelMoisturePct: &pct,
		Texture: &clay, ThresholdsSource: domain.ThresholdSourceHeuristic, RecordedAt: nowMicro(),
	}
}

func recordedFields(t *testing.T, companyID domain.CompanyID, day time.Time) map[domain.FieldID]bool {
	t.Helper()
	fields, err := testDB.ListSoilHistoryFields(ctx(), companyID, day)
	must(t, err)
	recorded := map[domain.FieldID]bool{}
	for _, field := range fields {
		if field.Field.CompanyID != companyID {
			t.Fatalf("field of another company listed: %+v", field)
		}
		recorded[field.Field.FieldID] = field.Recorded
	}
	return recorded
}

func TestSoilStateHistory(t *testing.T) {
	f := newConditionsFixture(t)
	brt := time.FixedZone("BRT", -3*3600)
	day := time.Date(2026, 11, 26, 0, 0, 0, 0, brt)

	before := recordedFields(t, f.company.ID, day)
	if len(before) != 2 || before[f.field.ID] || before[f.field2.ID] {
		t.Fatalf("fresh company = %v, want both fields not recorded", before)
	}

	inserted, err := testDB.RecordSoilState(ctx(), historyRecord(f, f.field, day, domain.SoilWaterlogged))
	must(t, err)
	if !inserted {
		t.Fatal("first record of the day must be inserted")
	}
	// Same field and day: nothing happens and the first record stays.
	inserted, err = testDB.RecordSoilState(ctx(), historyRecord(f, f.field, day, domain.SoilDry))
	must(t, err)
	if inserted {
		t.Fatal("second record of the same day must be ignored")
	}
	var state string
	must(t, testPool.QueryRow(ctx(), `SELECT state FROM soil_state_history WHERE company_id = $1 AND field_id = $2 AND local_day = $3`,
		string(f.company.ID), string(f.field.ID), "2026-11-26").Scan(&state))
	if state != string(domain.SoilWaterlogged) {
		t.Fatalf("state = %s, the first record must be kept", state)
	}

	// Unknown without any source: every optional column is null.
	unknown := usecase.SoilStateRecord{
		Field:    usecase.FieldRef{CompanyID: f.company.ID, FarmID: f.field2.FarmID, FieldID: f.field2.ID},
		LocalDay: day, State: domain.SoilUnknown, Basis: domain.BasisNone,
		ThresholdsSource: domain.ThresholdSourceHeuristic, RecordedAt: nowMicro(),
	}
	inserted, err = testDB.RecordSoilState(ctx(), unknown)
	must(t, err)
	if !inserted {
		t.Fatal("unknown record must be inserted")
	}

	after := recordedFields(t, f.company.ID, day)
	if !after[f.field.ID] || !after[f.field2.ID] {
		t.Errorf("after recording = %v", after)
	}
	if next := recordedFields(t, f.company.ID, day.AddDate(0, 0, 1)); next[f.field.ID] {
		t.Error("the next day is not recorded yet")
	}

	// Tenant isolation: another company sees none of these fields and cannot record them.
	other := newCompany(t)
	if fields := recordedFields(t, other.ID, day); len(fields) != 0 {
		t.Errorf("another company lists %v", fields)
	}
	foreign := historyRecord(f, f.field, day.AddDate(0, 0, 2), domain.SoilDry)
	foreign.Field.CompanyID = other.ID
	_, err = testDB.RecordSoilState(ctx(), foreign)
	var integrity *postgres.IntegrityError
	if !errors.As(err, &integrity) || integrity.Kind != "foreign_key" {
		t.Fatalf("record for a field of another company = %v, want a foreign key violation", err)
	}

	companies, err := testDB.ListSoilHistoryCompanies(ctx())
	must(t, err)
	if !slices.Contains(companies, f.company.ID) {
		t.Error("active company must be walked")
	}
	inactive := newCompany(t, func(c *domain.Company) { c.Status = domain.CompanyInactive })
	companies, err = testDB.ListSoilHistoryCompanies(ctx())
	must(t, err)
	if slices.Contains(companies, inactive.ID) {
		t.Error("inactive company must not be walked")
	}
}
