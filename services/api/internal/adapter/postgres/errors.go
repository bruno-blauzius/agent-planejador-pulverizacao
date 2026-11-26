package postgres

import (
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/domain"
	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/usecase"
)

// SQLSTATE classes the adapter translates (PostgreSQL appendix A).
const (
	sqlStateInvalidText      = "22P02"
	sqlStateNotNullViolation = "23502"
	sqlStateForeignKey       = "23503"
	sqlStateUnique           = "23505"
	sqlStateCheck            = "23514"
)

// Constraints whose violation has a meaning for the use cases. Names come from
// the migrations; a renamed constraint makes the integration tests fail.
var (
	uniqueViolations = map[string]error{
		"uq_users_email_active":             usecase.ErrEmailAlreadyRegistered,
		"uq_companies_document":             usecase.ErrDocumentAlreadyRegistered,
		"uq_user_identities_issuer_subject": usecase.ErrIdentityAlreadyLinked,
		"uq_user_identities_user_issuer":    usecase.ErrIdentityAlreadyLinked,
		"uq_owners_company_document":        usecase.ErrOwnerDocumentTaken,
		"uq_fields_farm_name":               domain.Invalid("name", "já existe um talhão com esse nome na fazenda"),
		"uq_rain_gauges_farm_name":          domain.Invalid("name", "já existe um pluviômetro com esse nome na fazenda"),
		"uq_rain_readings_gauge_observed":   domain.Invalid("observed_at", "já existe uma leitura desse pluviômetro nesse horário"),
	}
	foreignKeyViolations = map[string]error{
		"fk_user_farm_access_farm": usecase.ErrFarmNotInCompany,
		"fk_users_primary_farm":    usecase.ErrFarmNotInCompany,
		// The use case checks the gauge first; this catches a race with its removal.
		"fk_rain_gauge_fields_gauge": domain.Invalid("rain_gauge_id", "pluviômetro não encontrado nesta fazenda"),
	}
	// checkViolations are CHECKs only the database can decide (geometry validity).
	checkViolations = map[string]error{
		"ck_farms_perimeter_valid": domain.Invalid("location.perimeter", "perímetro inválido (polígono com autointerseção)"),
		"ck_fields_polygon_valid":  domain.Invalid("polygon", "o polígono do talhão é inválido (autointerseção)"),
		"ck_fields_area":           domain.Invalid("polygon", "o talhão precisa ter área"),
	}
)

// IntegrityError is a constraint violation that no use case handles: it means
// a bug (the domain should have refused the value) or a race the schema caught.
type IntegrityError struct {
	// Kind is "unique", "foreign_key", "check" or "not_null".
	Kind       string
	Constraint string
	Err        error
}

// Error implements error. It carries the constraint name, never the offending value.
func (e *IntegrityError) Error() string {
	return fmt.Sprintf("integrity violation (%s) on %q", e.Kind, e.Constraint)
}

// Unwrap exposes the driver error.
func (e *IntegrityError) Unwrap() error { return e.Err }

// translateWrite maps a write error to the error the port promises.
func translateWrite(operation string, err error) error {
	if err == nil {
		return nil
	}
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return fmt.Errorf("%s: %w", operation, err)
	}
	switch pgErr.Code {
	case sqlStateUnique:
		if mapped, ok := uniqueViolations[pgErr.ConstraintName]; ok {
			return fmt.Errorf("%s: %w", operation, mapped)
		}
		return fmt.Errorf("%s: %w", operation, &IntegrityError{Kind: "unique", Constraint: pgErr.ConstraintName, Err: err})
	case sqlStateForeignKey:
		if mapped, ok := foreignKeyViolations[pgErr.ConstraintName]; ok {
			return fmt.Errorf("%s: %w", operation, mapped)
		}
		return fmt.Errorf("%s: %w", operation, &IntegrityError{Kind: "foreign_key", Constraint: pgErr.ConstraintName, Err: err})
	case sqlStateCheck:
		if mapped, ok := checkViolations[pgErr.ConstraintName]; ok {
			return fmt.Errorf("%s: %w", operation, mapped)
		}
		return fmt.Errorf("%s: %w", operation, &IntegrityError{Kind: "check", Constraint: pgErr.ConstraintName, Err: err})
	case sqlStateNotNullViolation:
		return fmt.Errorf("%s: %w", operation, &IntegrityError{Kind: "not_null", Constraint: pgErr.ColumnName, Err: err})
	default:
		return fmt.Errorf("%s: %w", operation, err)
	}
}

// translateRead maps a read error: no row, or an id that is not a UUID (an id
// from the URL that cannot exist), is "not found".
func translateRead(operation string, err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("%s: %w", operation, usecase.ErrNotFound)
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == sqlStateInvalidText {
		return fmt.Errorf("%s: %w", operation, usecase.ErrNotFound)
	}
	return fmt.Errorf("%s: %w", operation, err)
}
