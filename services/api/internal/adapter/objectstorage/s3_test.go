package objectstorage_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/adapter/objectstorage"
	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/usecase"
)

// fakeS3 answers the three S3 calls of the adapter on path-style URLs
// (/{bucket}/{key}) and records what it received; it never touches the network.
type fakeS3 struct {
	t       *testing.T
	mu      sync.Mutex
	objects map[string][]byte
	types   map[string]string
	signed  bool
	fail    bool
}

func (f *fakeS3) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.signed = f.signed || strings.HasPrefix(r.Header.Get("Authorization"), "AWS4-HMAC-SHA256")
	if f.fail {
		w.WriteHeader(http.StatusInternalServerError)
		f.write(w, `<Error><Code>InternalError</Code><Message>boom</Message></Error>`)
		return
	}
	key := strings.TrimPrefix(r.URL.Path, "/agro-docs/")
	switch r.Method {
	case http.MethodPut:
		body, err := io.ReadAll(r.Body)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		f.objects[key], f.types[key] = body, r.Header.Get("Content-Type")
		w.WriteHeader(http.StatusOK)
	case http.MethodGet:
		body, ok := f.objects[key]
		if !ok {
			w.Header().Set("Content-Type", "application/xml")
			w.WriteHeader(http.StatusNotFound)
			f.write(w, `<?xml version="1.0" encoding="UTF-8"?><Error><Code>NoSuchKey</Code><Message>missing</Message></Error>`)
			return
		}
		w.Header().Set("Content-Type", f.types[key])
		f.write(w, string(body))
	case http.MethodDelete:
		delete(f.objects, key)
		w.WriteHeader(http.StatusNoContent)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (f *fakeS3) write(w io.Writer, body string) {
	if _, err := io.WriteString(w, body); err != nil {
		f.t.Errorf("fake S3 write: %v", err)
	}
}

func newStore(t *testing.T) (*objectstorage.S3Store, *fakeS3) {
	t.Helper()
	fake := &fakeS3{t: t, objects: map[string][]byte{}, types: map[string]string{}}
	server := httptest.NewServer(fake)
	t.Cleanup(server.Close)
	// Static test credentials through the default chain; no shared profile, no instance metadata.
	t.Setenv("AWS_ACCESS_KEY_ID", "test-access-key")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "test-secret-key")
	t.Setenv("AWS_SESSION_TOKEN", "")
	t.Setenv("AWS_PROFILE", "")
	t.Setenv("AWS_CONFIG_FILE", t.TempDir()+"/none")
	t.Setenv("AWS_SHARED_CREDENTIALS_FILE", t.TempDir()+"/none")
	t.Setenv("AWS_EC2_METADATA_DISABLED", "true")
	store, err := objectstorage.NewS3Store(context.Background(), objectstorage.Config{
		Bucket: "agro-docs", Region: "sa-east-1", Endpoint: server.URL, UsePathStyle: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	return store, fake
}

func TestS3StoreRoundTrip(t *testing.T) {
	store, fake := newStore(t)
	ctx := context.Background()
	key := "companies/c1/owners/o1/documents/d1"

	if err := store.PutDocument(ctx, key, "application/pdf", []byte("%PDF-1.7")); err != nil {
		t.Fatal(err)
	}
	if string(fake.objects[key]) != "%PDF-1.7" || fake.types[key] != "application/pdf" || !fake.signed {
		t.Fatalf("stored %q (%s), signed %v", fake.objects[key], fake.types[key], fake.signed)
	}
	content, err := store.GetDocument(ctx, key)
	if err != nil || string(content) != "%PDF-1.7" {
		t.Fatalf("get = %q, %v", content, err)
	}
	if err := store.DeleteDocument(ctx, key); err != nil {
		t.Fatal(err)
	}
	if _, err := store.GetDocument(ctx, key); !errors.Is(err, usecase.ErrNotFound) {
		t.Fatalf("missing object: %v", err)
	}
}

func TestS3StoreFailuresAreUnavailable(t *testing.T) {
	store, fake := newStore(t)
	fake.fail = true
	ctx := context.Background()
	if err := store.PutDocument(ctx, "k", "application/pdf", []byte("x")); !errors.Is(err, usecase.ErrDocumentStorageUnavailable) {
		t.Fatalf("put: %v", err)
	}
	if _, err := store.GetDocument(ctx, "k"); !errors.Is(err, usecase.ErrDocumentStorageUnavailable) {
		t.Fatalf("get: %v", err)
	}
	if err := store.DeleteDocument(ctx, "k"); !errors.Is(err, usecase.ErrDocumentStorageUnavailable) {
		t.Fatalf("delete: %v", err)
	}
}

func TestDisabledStorage(t *testing.T) {
	var storage usecase.DocumentStorage = objectstorage.Disabled{}
	if err := storage.PutDocument(context.Background(), "k", "application/pdf", nil); !errors.Is(err, usecase.ErrDocumentStorageUnavailable) {
		t.Fatalf("put: %v", err)
	}
}

func TestConfigFromEnv(t *testing.T) {
	env := func(values map[string]string) objectstorage.LookupFunc {
		return func(key string) (string, bool) { v, ok := values[key]; return v, ok }
	}
	tests := []struct {
		name        string
		values      map[string]string
		wantEnabled bool
		wantErr     bool
		want        objectstorage.Config
	}{
		{"no bucket disables the documents", map[string]string{}, false, false, objectstorage.Config{}},
		{"bucket and region", map[string]string{"API_DOCUMENTS_BUCKET": "b", "API_DOCUMENTS_REGION": "sa-east-1"}, true, false,
			objectstorage.Config{Bucket: "b", Region: "sa-east-1"}},
		{"emulator", map[string]string{
			"API_DOCUMENTS_BUCKET": "b", "API_DOCUMENTS_REGION": "us-east-1",
			"API_DOCUMENTS_S3_ENDPOINT": "http://localhost:9000", "API_DOCUMENTS_S3_PATH_STYLE": "true",
		}, true, false, objectstorage.Config{Bucket: "b", Region: "us-east-1", Endpoint: "http://localhost:9000", UsePathStyle: true}},
		{"bucket without region", map[string]string{"API_DOCUMENTS_BUCKET": "b"}, false, true, objectstorage.Config{}},
		{"bad path style flag", map[string]string{
			"API_DOCUMENTS_BUCKET": "b", "API_DOCUMENTS_REGION": "r", "API_DOCUMENTS_S3_PATH_STYLE": "sometimes",
		}, false, true, objectstorage.Config{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, enabled, err := objectstorage.ConfigFromEnv(env(tt.values))
			if (err != nil) != tt.wantErr || enabled != tt.wantEnabled || cfg != tt.want {
				t.Fatalf("cfg = %+v, enabled %v, err %v", cfg, enabled, err)
			}
		})
	}
}
