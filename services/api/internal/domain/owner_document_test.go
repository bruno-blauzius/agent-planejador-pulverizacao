package domain_test

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/domain"
)

func upload(kind domain.OwnerDocumentKind, name, media string, size int64) domain.NewOwnerDocumentInput {
	return domain.NewOwnerDocumentInput{Kind: kind, FileName: name, ContentType: media, SizeBytes: size, MaxBytes: 10 << 20}
}

func TestNewOwnerDocument(t *testing.T) {
	owner := domain.Owner{ID: "owner-1", CompanyID: "company-1", PersonType: domain.PersonIndividual}
	now := time.Date(2026, 11, 23, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name       string
		in         domain.NewOwnerDocumentInput
		wantName   string
		wantFields []string
	}{
		{"identity card scan", upload(domain.DocumentIDCard, "rg.pdf", domain.MediaPDF, 2048), "rg.pdf", nil},
		{"path is dropped", upload(domain.DocumentDriverLicense, `C:\fotos\cnh frente.jpg`, domain.MediaJPEG, 1), "cnh frente.jpg", nil},
		{"control characters and quotes removed", upload(domain.DocumentOther, "a\"b\x00.png", domain.MediaPNG, 1), "ab.png", nil},
		{"exactly the limit", upload(domain.DocumentOther, "x.pdf", domain.MediaPDF, 10<<20), "x.pdf", nil},
		{"unknown kind", upload("passport", "p.pdf", domain.MediaPDF, 1), "", []string{"kind"}},
		{"no name", upload(domain.DocumentIDCard, "  ", domain.MediaPDF, 1), "", []string{"file"}},
		{"word document", upload(domain.DocumentIDCard, "rg.docx", "application/msword", 1), "", []string{"file"}},
		{"empty file", upload(domain.DocumentIDCard, "rg.pdf", domain.MediaPDF, 0), "", []string{"file"}},
		{"above the limit", upload(domain.DocumentIDCard, "rg.pdf", domain.MediaPDF, 10<<20+1), "", []string{"file"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			document, err := domain.NewOwnerDocument("doc-1", owner, "user-1", tt.in, now)
			if got := fieldsOf(t, err); !slices.Equal(got, tt.wantFields) {
				t.Fatalf("invalid fields = %v, want %v", got, tt.wantFields)
			}
			if err != nil {
				return
			}
			if document.FileName != tt.wantName || document.StorageKey != "companies/company-1/owners/owner-1/documents/doc-1" ||
				document.UploadedBy != "user-1" || !document.UploadedAt.Equal(now) {
				t.Fatalf("document = %+v", document)
			}
		})
	}
}

func TestNewOwnerDocumentKeepsTheExtensionOfLongNames(t *testing.T) {
	owner := domain.Owner{ID: "owner-1", CompanyID: "company-1"}
	name := strings.Repeat("a", 300) + ".pdf"
	document, err := domain.NewOwnerDocument("doc-1", owner, "user-1", upload(domain.DocumentOther, name, domain.MediaPDF, 1), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if len([]rune(document.FileName)) != 200 || !strings.HasSuffix(document.FileName, ".pdf") {
		t.Fatalf("file name = %q", document.FileName)
	}
}

func TestRequiredDocumentsMissing(t *testing.T) {
	doc := func(kind domain.OwnerDocumentKind) domain.OwnerDocument { return domain.OwnerDocument{Kind: kind} }
	tests := []struct {
		name       string
		personType domain.PersonType
		documents  []domain.OwnerDocument
		want       bool
	}{
		{"person without documents", domain.PersonIndividual, nil, true},
		{"person with identity card", domain.PersonIndividual, []domain.OwnerDocument{doc(domain.DocumentIDCard)}, false},
		{"person with driver's license", domain.PersonIndividual, []domain.OwnerDocument{doc(domain.DocumentOther), doc(domain.DocumentDriverLicense)}, false},
		{"person with articles only", domain.PersonIndividual, []domain.OwnerDocument{doc(domain.DocumentArticlesOfIncorporation)}, true},
		{"company with articles", domain.PersonCompany, []domain.OwnerDocument{doc(domain.DocumentArticlesOfIncorporation)}, false},
		{"company with an identity card only", domain.PersonCompany, []domain.OwnerDocument{doc(domain.DocumentIDCard)}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := domain.RequiredDocumentsMissing(tt.personType, tt.documents); got != tt.want {
				t.Fatalf("missing = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDisplayStatus(t *testing.T) {
	tests := []struct {
		stored  domain.FarmStatus
		pending bool
		want    domain.FarmStatus
	}{
		{domain.FarmActive, true, domain.FarmPending},
		{domain.FarmActive, false, domain.FarmActive},
		{domain.FarmDraft, true, domain.FarmDraft},
	}
	for _, tt := range tests {
		if got := domain.DisplayStatus(tt.stored, tt.pending); got != tt.want {
			t.Errorf("DisplayStatus(%s, %v) = %s, want %s", tt.stored, tt.pending, got, tt.want)
		}
	}
}

func TestNewOwnerStartsWithDocumentsPending(t *testing.T) {
	owner, err := domain.NewOwner("owner-1", "company-1", ownerDetails(), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if !owner.DocumentsPending {
		t.Fatal("a new owner has no documents yet")
	}
}
