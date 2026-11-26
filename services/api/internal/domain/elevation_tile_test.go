package domain_test

import (
	"slices"
	"testing"

	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/domain"
)

func TestNewElevationTileKey(t *testing.T) {
	tests := []struct {
		name     string
		lat, lon int
		want     domain.ElevationTileKey
	}{
		{"south west", -22, -47, "S22W047"},
		{"north east", 5, 8, "N05E008"},
		{"equator and meridian", 0, 0, "N00E000"},
		{"three digit west", -3, -179, "S03W179"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := domain.NewElevationTileKey(tt.lat, tt.lon); got != tt.want {
				t.Fatalf("key = %s, want %s", got, tt.want)
			}
		})
	}
}

func TestTileKeysCovering(t *testing.T) {
	tests := []struct {
		name                           string
		minLon, minLat, maxLon, maxLat float64
		want                           []domain.ElevationTileKey
	}{
		{"inside one block", -46.9, -21.9, -46.8, -21.8, []domain.ElevationTileKey{"S22W047"}},
		{"crosses a longitude boundary", -47.2, -21.5, -46.8, -21.4, []domain.ElevationTileKey{"S22W048", "S22W047"}},
		{"crosses a latitude boundary", -46.9, -22.1, -46.8, -21.9, []domain.ElevationTileKey{"S23W047", "S22W047"}},
		{"crosses both", -47.1, -22.1, -46.9, -21.9, []domain.ElevationTileKey{"S23W048", "S23W047", "S22W048", "S22W047"}},
		{"point exactly on the grid", -47.0, -22.0, -47.0, -22.0, []domain.ElevationTileKey{"S22W047"}},
		{"max exactly on a boundary stays in the lower block", -47.5, -21.5, -47.0, -21.2, []domain.ElevationTileKey{"S22W048"}},
		{"min exactly on a boundary starts there", -47.0, -22.0, -46.5, -21.5, []domain.ElevationTileKey{"S22W047"}},
		{"north east hemisphere", 8.2, 5.2, 8.4, 5.4, []domain.ElevationTileKey{"N05E008"}},
		{"across the equator", 8.2, -0.2, 8.4, 0.2, []domain.ElevationTileKey{"S01E008", "N00E008"}},
		{"across the prime meridian", -0.2, 5.2, 0.2, 5.4, []domain.ElevationTileKey{"N05W001", "N05E000"}},
		{"inverted box", -46, -21, -47, -22, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := domain.TileKeysCovering(tt.minLon, tt.minLat, tt.maxLon, tt.maxLat)
			if !slices.Equal(got, tt.want) {
				t.Fatalf("keys = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestTileKeysCoveringRefusesNonsense(t *testing.T) {
	nan := func() float64 { var z float64; return z / z }()
	if got := domain.TileKeysCovering(nan, 0, 1, 1); got != nil {
		t.Fatalf("NaN box = %v, want nil", got)
	}
	if got := domain.TileKeysCovering(-180, -90, 180, 90); got != nil {
		t.Fatalf("world box = %d keys, want nil", len(got))
	}
}
