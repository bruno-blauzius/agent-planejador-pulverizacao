package usecase

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/domain"
)

// MunicipalityBoundary is a municipality of the IBGE registry with its
// boundary as a GeoJSON Polygon or MultiPolygon (WGS84).
type MunicipalityBoundary struct {
	Municipality domain.Municipality
	Boundary     string
}

// MunicipalitySource reads the IBGE registry (names and boundaries).
type MunicipalitySource interface {
	FetchMunicipalities(ctx context.Context) ([]MunicipalityBoundary, error)
}

// MunicipalityWriter replaces the stored registry: every given municipality is
// stored and those left out are removed.
type MunicipalityWriter interface {
	ReplaceMunicipalities(ctx context.Context, municipalities []MunicipalityBoundary) error
}

// ErrIncompleteRegistry means the source answered fewer municipalities than
// expected (a partial download must never replace a complete registry).
var ErrIncompleteRegistry = errors.New("municipality registry is incomplete")

// LoadMunicipalities refreshes the IBGE registry used to locate perimeters
// (ADR 0019). It runs from the command line, never from a request.
type LoadMunicipalities struct {
	source MunicipalitySource
	writer MunicipalityWriter
	// minimum is the smallest acceptable number of municipalities.
	minimum int
}

// NewLoadMunicipalities builds the use case; minimum guards against a partial
// answer of the source.
func NewLoadMunicipalities(source MunicipalitySource, writer MunicipalityWriter, minimum int) *LoadMunicipalities {
	return &LoadMunicipalities{source: source, writer: writer, minimum: minimum}
}

// Execute fetches, checks and stores the registry; it returns how many
// municipalities were stored.
func (uc *LoadMunicipalities) Execute(ctx context.Context) (int, error) {
	municipalities, err := uc.source.FetchMunicipalities(ctx)
	if err != nil {
		return 0, fmt.Errorf("fetch municipalities: %w", err)
	}
	seen := make(map[string]bool, len(municipalities))
	for _, item := range municipalities {
		code := item.Municipality.Code
		if seen[code] {
			return 0, fmt.Errorf("municipality %s: repeated code", code)
		}
		seen[code] = true
		if _, err := domain.NewMunicipality(code, item.Municipality.Name, item.Municipality.State); err != nil {
			return 0, fmt.Errorf("municipality %s: %w", code, err)
		}
		if strings.TrimSpace(item.Boundary) == "" {
			return 0, fmt.Errorf("municipality %s: no boundary", code)
		}
	}
	if len(municipalities) < uc.minimum {
		return 0, fmt.Errorf("%w: %d of at least %d", ErrIncompleteRegistry, len(municipalities), uc.minimum)
	}
	if err := uc.writer.ReplaceMunicipalities(ctx, municipalities); err != nil {
		return 0, fmt.Errorf("store municipalities: %w", err)
	}
	return len(municipalities), nil
}
