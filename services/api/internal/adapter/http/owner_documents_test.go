package httpadapter_test

import (
	"bytes"
	"context"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/usecase"
)

// A minimal PDF: http.DetectContentType recognizes it by the "%PDF-" signature.
const pdfContent = "%PDF-1.7\n1 0 obj << >> endobj\n%%EOF"

func (e *apiEnv) uploadDocument(t *testing.T, as, owner, kind, fileName, content string) reply {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if kind != "" {
		if err := writer.WriteField("kind", kind); err != nil {
			t.Fatal(err)
		}
	}
	if fileName != "" {
		part, err := writer.CreateFormFile("file", fileName)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := part.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/owners/"+owner+"/documents", &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	request.Header.Set("X-Test-User", as)
	recorder := httptest.NewRecorder()
	e.handler.ServeHTTP(recorder, request)
	return reply{status: recorder.Code, header: recorder.Header(), raw: recorder.Body.Bytes()}
}

func TestOwnerDocumentEndpoints(t *testing.T) {
	e := newAPIEnv(t)
	seedRegistry(e)
	helena := e.reg.Owners[ownerHelena]
	helena.DocumentsPending = true
	e.reg.Owners[ownerHelena] = helena

	created := e.uploadDocument(t, adminAID, ownerHelena, "id_card", "RG Helena.pdf", pdfContent)
	if created.status != http.StatusCreated {
		t.Fatalf("upload: %d %s", created.status, created.raw)
	}
	document := created.object(t)
	id, _ := document["id"].(string)
	if document["kind"] != "id_card" || document["file_name"] != "RG Helena.pdf" || document["uploaded_at"] == nil {
		t.Fatalf("document shape: %s", created.raw)
	}

	owner := e.call(adminAID, http.MethodGet, "/owners/"+ownerHelena, nil).object(t)
	documents, _ := owner["documents"].([]any)
	if owner["has_pending_documents"] != false || len(documents) != 1 {
		t.Fatalf("owner after upload: %v", owner)
	}

	path := "/owners/" + ownerHelena + "/documents/" + id
	download := e.call(operatorAID, http.MethodGet, path+"/content", nil)
	if download.status != http.StatusOK || string(download.raw) != pdfContent ||
		download.header.Get("Content-Type") != "application/pdf" ||
		!strings.Contains(download.header.Get("Content-Disposition"), "filename*=UTF-8''RG%20Helena.pdf") ||
		download.header.Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("download: %d %v %q", download.status, download.header, download.raw)
	}
	if other := e.call(adminBID, http.MethodGet, path+"/content", nil); other.status != http.StatusNotFound {
		t.Fatalf("other company download: %d", other.status)
	}

	if forbidden := e.call(operatorAID, http.MethodDelete, path, nil); forbidden.status != http.StatusForbidden {
		t.Fatalf("operator delete: %d", forbidden.status)
	}
	if removed := e.call(adminAID, http.MethodDelete, path, nil); removed.status != http.StatusNoContent {
		t.Fatalf("delete: %d %s", removed.status, removed.raw)
	}
	if again := e.call(adminAID, http.MethodDelete, path, nil); again.status != http.StatusNotFound {
		t.Fatalf("second delete: %d", again.status)
	}
	if owner := e.call(adminAID, http.MethodGet, "/owners/"+ownerHelena, nil).object(t); owner["has_pending_documents"] != true {
		t.Fatalf("owner after removal: %v", owner)
	}
}

func TestOwnerDocumentUploadRefusals(t *testing.T) {
	e := newAPIEnv(t)
	seedRegistry(e)
	tests := []struct {
		name           string
		kind, fileName string
		content        string
		wantStatus     int
		wantFields     []string
	}{
		{"word document", "id_card", "rg.docx", "PK\x03\x04 word", http.StatusUnsupportedMediaType, nil},
		{"pdf renamed is still checked by content", "id_card", "foto.pdf", "just text", http.StatusUnsupportedMediaType, nil},
		{"unknown kind", "passport", "rg.pdf", pdfContent, http.StatusUnprocessableEntity, []string{"kind"}},
		{"no kind", "", "rg.pdf", pdfContent, http.StatusUnprocessableEntity, []string{"kind"}},
		{"no file", "id_card", "", "", http.StatusUnprocessableEntity, []string{"file"}},
		{"larger than the limit", "id_card", "rg.pdf", pdfContent + strings.Repeat("x", 1<<20), http.StatusRequestEntityTooLarge, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := e.uploadDocument(t, adminAID, ownerHelena, tt.kind, tt.fileName, tt.content)
			if got.status != tt.wantStatus {
				t.Fatalf("status = %d, body %s", got.status, got.raw)
			}
			for _, field := range tt.wantFields {
				if !slices.Contains(got.errorFields(t), field) {
					t.Errorf("missing %s in %s", field, got.raw)
				}
			}
		})
	}
	if len(e.objects.Objects) != 0 {
		t.Fatalf("refused uploads left objects: %v", e.objects.Keys())
	}
}

func TestOwnerDocumentStorageDown(t *testing.T) {
	e := newAPIEnv(t)
	seedRegistry(e)
	e.objects.PutErr = usecase.ErrDocumentStorageUnavailable
	got := e.uploadDocument(t, adminAID, ownerHelena, "id_card", "rg.pdf", pdfContent)
	if got.status != http.StatusServiceUnavailable || got.errorCode(t) != "document_storage_unavailable" {
		t.Fatalf("storage down: %d %s", got.status, got.raw)
	}
}

func TestFarmShownPendingWhileOwnerLacksDocuments(t *testing.T) {
	e := newAPIEnv(t)
	seedRegistry(e)
	farm := e.reg.Farms[farmA1]
	farm.Status = "active"
	e.reg.Farms[farmA1] = farm
	helena := e.reg.Owners[ownerHelena]
	helena.DocumentsPending = true
	e.reg.Owners[ownerHelena] = helena

	if status := e.call(adminAID, http.MethodGet, "/farms/"+farmA1, nil).object(t)["status"]; status != "pending" {
		t.Fatalf("status = %v, want pending", status)
	}
	page := e.call(adminAID, http.MethodGet, "/farms?status=pending", nil).object(t)
	if page["total"] != float64(1) {
		t.Fatalf("pending filter: %v", page)
	}
	if upload := e.uploadDocument(t, adminAID, ownerHelena, "driver_license", "cnh.pdf", pdfContent); upload.status != http.StatusCreated {
		t.Fatalf("upload: %d %s", upload.status, upload.raw)
	}
	if status := e.call(adminAID, http.MethodGet, "/farms/"+farmA1, nil).object(t)["status"]; status != "active" {
		t.Fatalf("status after the document = %v, want active", status)
	}
}
