package fakes

import (
	"context"
	"fmt"
	"slices"

	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/domain"
	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/usecase"
)

// MunicipalityRegistry is an in-memory IBGE registry: a source with canned
// municipalities, a writer that keeps what it received and a locator with a
// canned answer.
type MunicipalityRegistry struct {
	Source   []usecase.MunicipalityBoundary
	FetchErr error
	Stored   []usecase.MunicipalityBoundary
	WriteErr error
	// Located is the answer of LocateMunicipality (nil: not found).
	Located *domain.Municipality
	// Points records every point located.
	Points []domain.GeoPoint
}

// FetchMunicipalities implements usecase.MunicipalitySource.
func (m *MunicipalityRegistry) FetchMunicipalities(context.Context) ([]usecase.MunicipalityBoundary, error) {
	if m.FetchErr != nil {
		return nil, fmt.Errorf("fetch: %w", m.FetchErr)
	}
	return slices.Clone(m.Source), nil
}

// ReplaceMunicipalities implements usecase.MunicipalityWriter.
func (m *MunicipalityRegistry) ReplaceMunicipalities(_ context.Context, municipalities []usecase.MunicipalityBoundary) error {
	if m.WriteErr != nil {
		return fmt.Errorf("replace: %w", m.WriteErr)
	}
	m.Stored = slices.Clone(municipalities)
	return nil
}

// LocateMunicipality implements usecase.MunicipalityLocator.
func (m *MunicipalityRegistry) LocateMunicipality(_ context.Context, point domain.GeoPoint) (*domain.Municipality, error) {
	m.Points = append(m.Points, point)
	return m.Located, nil
}

// LocateMunicipality implements usecase.MunicipalityLocator for a registry
// without the IBGE data: nothing is ever found.
func (r *Registry) LocateMunicipality(context.Context, domain.GeoPoint) (*domain.Municipality, error) {
	return nil, nil
}
