package domain

import (
	"path"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// OwnerDocumentKind is what a document of an owner proves.
type OwnerDocumentKind string

// Kinds of owner documents.
const (
	DocumentIDCard                  OwnerDocumentKind = "id_card"
	DocumentDriverLicense           OwnerDocumentKind = "driver_license"
	DocumentArticlesOfIncorporation OwnerDocumentKind = "articles_of_incorporation"
	DocumentOther                   OwnerDocumentKind = "other"
)

// Valid reports whether k is a known kind.
func (k OwnerDocumentKind) Valid() bool {
	switch k {
	case DocumentIDCard, DocumentDriverLicense, DocumentArticlesOfIncorporation, DocumentOther:
		return true
	}
	return false
}

// Media types accepted for owner documents: scans and photos of documents.
const (
	MediaPDF  = "application/pdf"
	MediaJPEG = "image/jpeg"
	MediaPNG  = "image/png"
)

// AcceptedDocumentMedia reports whether a media type may be stored.
func AcceptedDocumentMedia(mediaType string) bool {
	return mediaType == MediaPDF || mediaType == MediaJPEG || mediaType == MediaPNG
}

const maxFileNameLength = 200

// OwnerDocument is a file that proves the identity of an owner (a photo ID
// for a person, the articles of incorporation for a company). The bytes live
// in the object storage under StorageKey; the database keeps the metadata.
type OwnerDocument struct {
	ID          OwnerDocumentID
	CompanyID   CompanyID
	OwnerID     OwnerID
	Kind        OwnerDocumentKind
	FileName    string
	ContentType string
	SizeBytes   int64
	StorageKey  string
	UploadedBy  UserID
	UploadedAt  time.Time
}

// NewOwnerDocumentInput describes an uploaded file.
type NewOwnerDocumentInput struct {
	Kind        OwnerDocumentKind
	FileName    string
	ContentType string
	SizeBytes   int64
	// MaxBytes is the configured upload limit.
	MaxBytes int64
}

// NewOwnerDocument validates the upload and places the file under the owner's
// own prefix: companies/{company}/owners/{owner}/documents/{id}. The key never
// carries the file name the client sent.
func NewOwnerDocument(id OwnerDocumentID, owner Owner, uploadedBy UserID, in NewOwnerDocumentInput, now time.Time) (OwnerDocument, error) {
	var problems Violations
	if !in.Kind.Valid() {
		problems.Add("kind", "tipo de documento desconhecido")
	}
	name := cleanFileName(in.FileName)
	if name == "" {
		problems.Add("file", "envie o arquivo")
	}
	if !AcceptedDocumentMedia(in.ContentType) {
		problems.Add("file", "envie PDF, JPEG ou PNG")
	}
	switch {
	case in.SizeBytes <= 0:
		problems.Add("file", "o arquivo está vazio")
	case in.SizeBytes > in.MaxBytes:
		problems.Add("file", "arquivo maior que o limite")
	}
	if err := problems.Err(); err != nil {
		return OwnerDocument{}, err
	}
	return OwnerDocument{
		ID: id, CompanyID: owner.CompanyID, OwnerID: owner.ID, Kind: in.Kind, FileName: name,
		ContentType: in.ContentType, SizeBytes: in.SizeBytes, UploadedBy: uploadedBy, UploadedAt: now,
		StorageKey: "companies/" + string(owner.CompanyID) + "/owners/" + string(owner.ID) + "/documents/" + string(id),
	}, nil
}

// cleanFileName keeps the base name without control characters, shortened to
// the stored limit (the name is shown and sent back on download).
func cleanFileName(value string) string {
	base := path.Base(strings.ReplaceAll(strings.TrimSpace(value), `\`, "/"))
	if base == "." || base == "/" {
		return ""
	}
	clean := strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || r == '"' {
			return -1
		}
		return r
	}, base)
	for utf8.RuneCountInString(clean) > maxFileNameLength {
		_, size := utf8.DecodeRuneInString(clean)
		clean = clean[size:]
	}
	return strings.TrimSpace(clean)
}

// RequiredDocumentsMissing reports whether an owner of the person type lacks
// the document it must have: a photo ID (identity card or driver's license)
// for a person, the articles of incorporation for a company. Without it the
// owner cannot sign the application records.
func RequiredDocumentsMissing(personType PersonType, documents []OwnerDocument) bool {
	for _, document := range documents {
		switch {
		case personType == PersonIndividual && (document.Kind == DocumentIDCard || document.Kind == DocumentDriverLicense):
			return false
		case personType == PersonCompany && document.Kind == DocumentArticlesOfIncorporation:
			return false
		}
	}
	return true
}

// DisplayStatus is the status a client sees: an active farm with an owner
// whose documents are pending is shown as pending.
func DisplayStatus(stored FarmStatus, ownerDocumentsPending bool) FarmStatus {
	if stored == FarmActive && ownerDocumentsPending {
		return FarmPending
	}
	return stored
}
