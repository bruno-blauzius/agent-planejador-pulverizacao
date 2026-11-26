package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/adapter/postgres/sqlcgen"
	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/domain"
	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/usecase"
)

// Daily soil state history (DER: soil_state_history; ADR 0022, phase 4).

// dateOfDay is the calendar date of a local day, whatever its zone.
func dateOfDay(day time.Time) pgtype.Date {
	return pgtype.Date{Time: time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, time.UTC), Valid: true}
}

func optionalFloat(value *float64) pgtype.Float8 {
	if value == nil {
		return pgtype.Float8{}
	}
	return pgtype.Float8{Float64: *value, Valid: true}
}

// ListSoilHistoryCompanies implements usecase.SoilHistoryStore.
func (r *Repository) ListSoilHistoryCompanies(ctx context.Context) ([]domain.CompanyID, error) {
	rows, err := r.q.ListSoilHistoryCompanies(ctx)
	if err != nil {
		return nil, fmt.Errorf("list soil history companies: %w", err)
	}
	companies := make([]domain.CompanyID, 0, len(rows))
	for _, id := range rows {
		companies = append(companies, domain.CompanyID(id))
	}
	return companies, nil
}

// ListSoilHistoryFields implements usecase.SoilHistoryStore.
func (r *Repository) ListSoilHistoryFields(ctx context.Context, companyID domain.CompanyID, day time.Time) ([]usecase.SoilHistoryField, error) {
	fields := []usecase.SoilHistoryField{}
	if !validUUID(string(companyID)) {
		return fields, nil
	}
	rows, err := r.q.ListSoilHistoryFields(ctx, sqlcgen.ListSoilHistoryFieldsParams{LocalDay: dateOfDay(day), CompanyID: string(companyID)})
	if err != nil {
		return nil, fmt.Errorf("list soil history fields: %w", err)
	}
	for _, row := range rows {
		fields = append(fields, usecase.SoilHistoryField{
			Field:    usecase.FieldRef{CompanyID: companyID, FarmID: domain.FarmID(row.FarmID), FieldID: domain.FieldID(row.FieldID)},
			Recorded: row.Recorded,
		})
	}
	return fields, nil
}

// RecordSoilState implements usecase.SoilHistoryStore with ON CONFLICT DO NOTHING.
func (r *Repository) RecordSoilState(ctx context.Context, record usecase.SoilStateRecord) (bool, error) {
	params := sqlcgen.InsertSoilStateHistoryParams{
		CompanyID: string(record.Field.CompanyID), FarmID: string(record.Field.FarmID), FieldID: string(record.Field.FieldID),
		LocalDay: dateOfDay(record.LocalDay), State: string(record.State), Basis: string(record.Basis),
		Last48hMm: optionalFloat(record.Last48hMm), Last72hMm: optionalFloat(record.Last72hMm),
		ModelMoisturePct: optionalFloat(record.ModelMoisturePct), ThresholdsSource: record.ThresholdsSource,
		RecordedAt: record.RecordedAt,
	}
	if record.RainSource != nil {
		params.RainSource = pgtype.Text{String: string(*record.RainSource), Valid: true}
	}
	if record.Texture != nil {
		params.SoilTexture = pgtype.Text{String: string(*record.Texture), Valid: true}
	}
	var rows int64
	err := r.atomic(ctx, func(q *sqlcgen.Queries) error {
		var err error
		rows, err = q.InsertSoilStateHistory(ctx, params)
		return err
	})
	if err != nil {
		return false, translateWrite("record soil state", err)
	}
	return rows == 1, nil
}

var _ usecase.SoilHistoryStore = (*Repository)(nil)
