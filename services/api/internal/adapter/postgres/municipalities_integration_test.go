//go:build integration

package postgres_test

import (
	"os"
	"testing"

	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/adapter/postgres"
	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/domain"
	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/platform"
	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/usecase"
)

// migratorDB connects as api_migrator, the only role that writes the IBGE registry.
func migratorDB(t *testing.T) *postgres.Database {
	t.Helper()
	url := os.Getenv("API_TEST_MIGRATOR_DATABASE_URL")
	if url == "" {
		t.Fatal("API_TEST_MIGRATOR_DATABASE_URL is not set: run `make api-test-integration`")
	}
	cfg, err := platform.LoadDatabaseConfig(func(key string) (string, bool) {
		if key == "API_DATABASE_URL" {
			return url, true
		}
		return "", false
	})
	must(t, err)
	pool, err := postgres.NewPool(ctx(), cfg)
	must(t, err)
	t.Cleanup(pool.Close)
	return postgres.NewDatabase(pool, postgres.Options{})
}

func municipality(code, name, state, geometry string) usecase.MunicipalityBoundary {
	return usecase.MunicipalityBoundary{Municipality: domain.Municipality{Code: code, Name: name, State: state}, Boundary: geometry}
}

func TestMunicipalityRegistry(t *testing.T) {
	guaxupe := municipality("3128709", "Guaxupé", "MG",
		`{"type":"Polygon","coordinates":[[[-47.1,-21.4],[-47.1,-20.9],[-46.6,-20.9],[-46.6,-21.4],[-47.1,-21.4]]]}`)
	// A bowtie: the loader repairs it (ST_MakeValid) instead of failing the whole registry.
	saoPaulo := municipality("3550308", "São Paulo", "SP",
		`{"type":"MultiPolygon","coordinates":[[[[-46.8,-23.7],[-46.4,-23.4],[-46.4,-23.7],[-46.8,-23.4],[-46.8,-23.7]]]]}`)
	writer := migratorDB(t)
	must(t, writer.ReplaceMunicipalities(ctx(), []usecase.MunicipalityBoundary{guaxupe, saoPaulo}))

	tests := []struct {
		name  string
		point domain.GeoPoint
		want  string
	}{
		{"inside Guaxupé", domain.GeoPoint{Lat: -20.995, Lng: -46.995}, "Guaxupé"},
		{"inside the repaired polygon", domain.GeoPoint{Lat: -23.55, Lng: -46.7}, "São Paulo"},
		{"in the ocean", domain.GeoPoint{Lat: -25, Lng: -40}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := testDB.LocateMunicipality(ctx(), tt.point)
			must(t, err)
			switch {
			case tt.want == "" && got != nil:
				t.Fatalf("want none, got %+v", got)
			case tt.want != "" && (got == nil || got.Name != tt.want):
				t.Fatalf("want %s, got %+v", tt.want, got)
			}
		})
	}

	// A new load replaces the registry: municipalities left out disappear.
	must(t, writer.ReplaceMunicipalities(ctx(), []usecase.MunicipalityBoundary{guaxupe}))
	gone, err := testDB.LocateMunicipality(ctx(), domain.GeoPoint{Lat: -23.55, Lng: -46.7})
	must(t, err)
	if gone != nil {
		t.Fatalf("removed municipality still found: %+v", gone)
	}

	// The perimeter import asks for a point inside the perimeter.
	measure, err := testDB.MeasurePerimeter(ctx(), *square100ha())
	must(t, err)
	place, err := testDB.LocateMunicipality(ctx(), measure.InteriorPoint)
	must(t, err)
	if place == nil || place.State != "MG" {
		t.Fatalf("municipality of the perimeter = %+v", place)
	}
}
