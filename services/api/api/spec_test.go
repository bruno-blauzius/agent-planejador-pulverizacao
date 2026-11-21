package apispec_test

import (
	"strings"
	"testing"

	apispec "github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/api"
)

func TestSpecIsEmbeddedAndDeclaresHealthPaths(t *testing.T) {
	if len(apispec.Spec) == 0 {
		t.Fatal("embedded spec is empty")
	}
	content := string(apispec.Spec)

	for _, want := range []string{
		"openapi: 3.1",
		"/health/live:",
		"/health/startup:",
		"/health/ready:",
	} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(content, want) {
				t.Errorf("spec does not contain %q", want)
			}
		})
	}
}
