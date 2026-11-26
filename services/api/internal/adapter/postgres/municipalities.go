package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/adapter/postgres/sqlcgen"
	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/domain"
	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/usecase"
)

// IBGE municipal registry (ADR 0019). The API reads it with api_app; only the
// loader command, connected as api_migrator, writes it.

// LocateMunicipality implements usecase.MunicipalityLocator.
func (r *Repository) LocateMunicipality(ctx context.Context, point domain.GeoPoint) (*domain.Municipality, error) {
	row, err := r.q.LocateMunicipality(ctx, sqlcgen.LocateMunicipalityParams{Lng: point.Lng, Lat: point.Lat})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("locate municipality: %w", err)
	}
	return &domain.Municipality{Code: row.IbgeCode, Name: row.Name, State: row.State}, nil
}

// ReplaceMunicipalities implements usecase.MunicipalityWriter in one
// transaction: readers see the old registry or the new one, never a mix.
func (d *Database) ReplaceMunicipalities(ctx context.Context, municipalities []usecase.MunicipalityBoundary) error {
	loadedAt := time.Now().UTC()
	codes := make([]string, 0, len(municipalities))
	err := pgx.BeginTxFunc(ctx, d.pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		q := sqlcgen.New(tx)
		for _, item := range municipalities {
			err := q.UpsertMunicipality(ctx, sqlcgen.UpsertMunicipalityParams{
				IbgeCode: item.Municipality.Code, Name: item.Municipality.Name, State: item.Municipality.State,
				Boundary: item.Boundary, LoadedAt: loadedAt,
			})
			if err != nil {
				return translateWrite("upsert municipality "+item.Municipality.Code, err)
			}
			codes = append(codes, item.Municipality.Code)
		}
		return translateWrite("delete stale municipalities", q.DeleteMunicipalitiesNotIn(ctx, codes))
	})
	if err != nil {
		return fmt.Errorf("replace municipalities: %w", err)
	}
	return nil
}
