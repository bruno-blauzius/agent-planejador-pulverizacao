package copernicus_test

import (
	"testing"

	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/adapter/copernicus"
	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/platform"
)

// platform cannot import this adapter, so it repeats its defaults; this test
// keeps the two copies from drifting apart.
func TestPlatformElevationDefaultsMatchTheClient(t *testing.T) {
	cfg, err := platform.LoadTerrainConfig(func(string) (string, bool) { return "", false })
	if err != nil {
		t.Fatalf("defaults: %v", err)
	}
	got := cfg.Elevation
	if got.BaseURL != copernicus.DefaultBaseURL || got.RequestTimeout != copernicus.DefaultRequestTimeout ||
		got.MaxBytes != copernicus.DefaultMaxBytes || got.Retries != copernicus.DefaultRetries ||
		got.ModelVersion != copernicus.DefaultModelVersion {
		t.Errorf("platform defaults %+v differ from the copernicus client defaults", got)
	}
}
