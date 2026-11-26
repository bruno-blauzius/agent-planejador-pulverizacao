package httpadapter_test

import (
	"net/http"
	"regexp"
	"slices"
	"strings"
	"testing"

	apispec "github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/api"
)

// The Terrain operations of the contract and the routes of the handler are the
// same set: every operation has a protected route (not 404/405) and no route
// lacks an operation.
func TestTerrainRoutesMatchTheContract(t *testing.T) {
	pathLine := regexp.MustCompile(`^  (/\S+):$`)
	methodLine := regexp.MustCompile(`^    (get|put|post|delete|patch):$`)
	var path, method string
	var operations []string
	for _, line := range strings.Split(string(apispec.Spec), "\n") {
		line = strings.TrimRight(line, "\r")
		if m := pathLine.FindStringSubmatch(line); m != nil {
			path, method = m[1], ""
		}
		if m := methodLine.FindStringSubmatch(line); m != nil {
			method = strings.ToUpper(m[1])
		}
		if strings.TrimSpace(line) == "tags: [Terrain]" && path != "" && method != "" {
			operations = append(operations, method+" "+path)
		}
	}
	want := []string{
		"GET /farms/{farmId}/terrain",
		"POST /farms/{farmId}/terrain/calculations",
		"GET /farms/{farmId}/fields/{fieldId}/terrain",
		"PUT /farms/{farmId}/fields/{fieldId}/terrain/adjustment",
		"DELETE /farms/{farmId}/fields/{fieldId}/terrain/adjustment",
	}
	slices.Sort(operations)
	slices.Sort(want)
	if !slices.Equal(operations, want) {
		t.Fatalf("Terrain operations in the contract:\n got %v\nwant %v", operations, want)
	}

	e := newTerrainEnv(t, &terrainStub{})
	for _, operation := range operations {
		verb, template, _ := strings.Cut(operation, " ")
		concrete := strings.NewReplacer("{farmId}", farmA1, "{fieldId}", terrainFieldID).Replace(template)
		if got := e.call("", verb, concrete, nil); got.status != http.StatusUnauthorized {
			t.Errorf("%s is not a registered protected route: status %d", operation, got.status)
		}
	}
}
