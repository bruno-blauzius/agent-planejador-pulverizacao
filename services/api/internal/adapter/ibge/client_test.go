package ibge_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/adapter/ibge"
)

// server answers the two IBGE APIs with the recorded files of testdata.
func server(t *testing.T, names, mesh string, meshStatus int) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/localidades/municipios":
			if r.URL.Query().Get("view") != "nivelado" {
				t.Errorf("names query = %s", r.URL.RawQuery)
			}
			reply(t, w, names)
		case "/api/v3/malhas/paises/BR":
			query := r.URL.Query()
			if query.Get("intrarregiao") != "municipio" || query.Get("qualidade") != "intermediaria" ||
				!strings.Contains(query.Get("formato"), "geo+json") {
				t.Errorf("mesh query = %s", r.URL.RawQuery)
			}
			w.WriteHeader(meshStatus)
			reply(t, w, mesh)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func reply(t *testing.T, w http.ResponseWriter, body string) {
	if _, err := w.Write([]byte(body)); err != nil {
		t.Errorf("test server write: %v", err)
	}
}

func fixture(t *testing.T, name string) string {
	t.Helper()
	content, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return string(content)
}

func TestFetchMunicipalities(t *testing.T) {
	srv := server(t, fixture(t, "municipios.json"), fixture(t, "malha.json"), http.StatusOK)
	client, err := ibge.NewClient(srv.Client(), srv.URL, ibge.QualityIntermediate, nil)
	if err != nil {
		t.Fatal(err)
	}
	municipalities, err := client.FetchMunicipalities(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(municipalities) != 2 {
		t.Fatalf("got %d municipalities", len(municipalities))
	}
	first := municipalities[0]
	if first.Municipality.Code != "3128709" || first.Municipality.Name != "Guaxupé" || first.Municipality.State != "MG" ||
		!strings.Contains(first.Boundary, `"Polygon"`) {
		t.Fatalf("first = %+v", first)
	}
	if !strings.Contains(municipalities[1].Boundary, "MultiPolygon") {
		t.Fatalf("second boundary = %s", municipalities[1].Boundary)
	}
}

func TestFetchMunicipalitiesFailures(t *testing.T) {
	names, mesh := fixture(t, "municipios.json"), fixture(t, "malha.json")
	tests := []struct {
		name, names, mesh string
		status            int
	}{
		{"mesh unavailable", names, `{"erro":"indisponível"}`, http.StatusServiceUnavailable},
		{"names not JSON", "<html>", mesh, http.StatusOK},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := server(t, tt.names, tt.mesh, tt.status)
			client, err := ibge.NewClient(srv.Client(), srv.URL, ibge.QualityIntermediate, nil)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := client.FetchMunicipalities(context.Background()); err == nil {
				t.Fatal("want error")
			}
		})
	}
}

// The two IBGE APIs are updated at different times: entries present in only
// one of them are left out and reported, not a reason to refuse the registry.
func TestFetchMunicipalitiesSkipsMismatches(t *testing.T) {
	names := `[{"municipio-id": 3128709, "municipio-nome": "Guaxupé", "UF-sigla": "MG"},
	           {"municipio-id": 5101837, "municipio-nome": "Boa Esperança do Norte", "UF-sigla": "MT"}]`
	mesh := `{"type":"FeatureCollection","features":[
	  {"properties":{"codarea":"3128709"},"geometry":{"type":"Polygon","coordinates":[[[0,0],[0,1],[1,1],[0,0]]]}},
	  {"properties":{"codarea":"9999999"},"geometry":{"type":"Polygon","coordinates":[[[0,0],[0,1],[1,1],[0,0]]]}}]}`
	srv := server(t, names, mesh, http.StatusOK)
	var skipped []string
	client, err := ibge.NewClient(srv.Client(), srv.URL, ibge.QualityIntermediate, func(code, _, reason string) {
		skipped = append(skipped, code+": "+reason)
	})
	if err != nil {
		t.Fatal(err)
	}
	municipalities, err := client.FetchMunicipalities(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(municipalities) != 1 || municipalities[0].Municipality.Code != "3128709" {
		t.Fatalf("municipalities = %+v", municipalities)
	}
	want := []string{"5101837: no boundary in the mesh", "9999999: boundary without a municipality name"}
	if !slices.Equal(skipped, want) {
		t.Fatalf("skipped = %v, want %v", skipped, want)
	}
}

func TestNewClientRefusesUnknownQuality(t *testing.T) {
	if _, err := ibge.NewClient(http.DefaultClient, ibge.DefaultBaseURL, "ultra", nil); err == nil {
		t.Fatal("unknown quality accepted")
	}
	if _, err := ibge.NewClient(http.DefaultClient, "not a url", ibge.QualityMinimum, nil); err == nil {
		t.Fatal("invalid base URL accepted")
	}
}
