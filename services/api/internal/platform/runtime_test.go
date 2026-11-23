package platform_test

import (
	"regexp"
	"testing"
	"time"

	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/platform"
)

var uuidV4 = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

func TestUUIDGeneratorProducesDistinctVersion4Ids(t *testing.T) {
	generator := platform.UUIDGenerator{}
	seen := map[string]struct{}{}
	for range 1000 {
		id := generator.NewID()
		if !uuidV4.MatchString(id) {
			t.Fatalf("id %q is not a version 4 UUID", id)
		}
		if _, dup := seen[id]; dup {
			t.Fatalf("id %q repeated", id)
		}
		seen[id] = struct{}{}
	}
}

func TestSystemClockIsUTCAndCurrent(t *testing.T) {
	before := time.Now().UTC().Add(-time.Second)
	now := platform.SystemClock{}.Now()
	if now.Location() != time.UTC {
		t.Errorf("clock location = %v, want UTC", now.Location())
	}
	if now.Before(before) || now.After(time.Now().UTC().Add(time.Second)) {
		t.Errorf("clock returned %v, not close to the current time", now)
	}
}
