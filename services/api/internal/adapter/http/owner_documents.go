package httpadapter

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"net/url"
	"path"
	"strings"

	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/domain"
	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/usecase"
)

// Owner documents (ADR 0020): upload as multipart (kind + file), download as
// the file itself, removal as 204.

// maxKindBytes bounds the "kind" part of the multipart body.
const maxKindBytes = 64

// OwnerDocumentsHandler serves the document routes of the Owners tag.
type OwnerDocumentsHandler struct {
	logger   *slog.Logger
	deps     OwnerDocumentsDeps
	maxBytes int64
}

// OwnerDocumentsDeps groups the use cases of OwnerDocumentsHandler.
type OwnerDocumentsDeps struct {
	Upload   *usecase.UploadOwnerDocument
	Download *usecase.DownloadOwnerDocument
	Remove   *usecase.RemoveOwnerDocument
}

// NewOwnerDocumentsHandler builds the handler; maxBytes is the upload limit
// (the same value the use case enforces, used here to stop reading early).
func NewOwnerDocumentsHandler(logger *slog.Logger, deps OwnerDocumentsDeps, maxBytes int64) *OwnerDocumentsHandler {
	return &OwnerDocumentsHandler{logger: logger, deps: deps, maxBytes: maxBytes}
}

// Register attaches the routes (protected; registry.write is checked by the use cases).
func (h *OwnerDocumentsHandler) Register(routes Routes) {
	routes.Protected(http.MethodPost, "/owners/{ownerId}/documents", serve(h.logger, h.upload))
	routes.Protected(http.MethodGet, "/owners/{ownerId}/documents/{documentId}/content", serve(h.logger, h.download))
	routes.Protected(http.MethodDelete, "/owners/{ownerId}/documents/{documentId}", serve(h.logger, h.remove))
}

func (h *OwnerDocumentsHandler) upload(w http.ResponseWriter, r *http.Request, actor domain.Principal) (response, error) {
	ownerID, err := pathID(r, "ownerId")
	if err != nil {
		return response{}, err
	}
	upload, err := h.readUpload(w, r)
	if err != nil {
		return response{}, err
	}
	// The media type comes from the bytes, never from the name or the header the client sent.
	document, err := h.deps.Upload.Execute(r.Context(), usecase.UploadOwnerDocumentInput{
		Actor: actor, OwnerID: domain.OwnerID(ownerID), Kind: domain.OwnerDocumentKind(upload.kind),
		FileName: upload.fileName, ContentType: http.DetectContentType(upload.content), Content: upload.content,
	})
	if err != nil {
		return response{}, err
	}
	return createdResponse(ownerDocumentOf(document)), nil
}

func documentRef(r *http.Request, actor domain.Principal) (usecase.OwnerDocumentInput, error) {
	ownerID, err := pathID(r, "ownerId")
	if err != nil {
		return usecase.OwnerDocumentInput{}, err
	}
	documentID, err := pathID(r, "documentId")
	if err != nil {
		return usecase.OwnerDocumentInput{}, err
	}
	return usecase.OwnerDocumentInput{Actor: actor, OwnerID: domain.OwnerID(ownerID), DocumentID: domain.OwnerDocumentID(documentID)}, nil
}

func (h *OwnerDocumentsHandler) download(_ http.ResponseWriter, r *http.Request, actor domain.Principal) (response, error) {
	ref, err := documentRef(r, actor)
	if err != nil {
		return response{}, err
	}
	file, err := h.deps.Download.Execute(r.Context(), ref)
	if err != nil {
		return response{}, err
	}
	return response{status: http.StatusOK, raw: file.Content, header: map[string]string{
		"Content-Type":           file.Document.ContentType,
		"Content-Disposition":    attachment(file.Document.FileName),
		"Cache-Control":          "private, no-store",
		"X-Content-Type-Options": "nosniff",
	}}, nil
}

func (h *OwnerDocumentsHandler) remove(_ http.ResponseWriter, r *http.Request, actor domain.Principal) (response, error) {
	ref, err := documentRef(r, actor)
	if err != nil {
		return response{}, err
	}
	if err := h.deps.Remove.Execute(r.Context(), ref); err != nil {
		return response{}, err
	}
	return noContentResponse(), nil
}

// attachment builds a Content-Disposition that survives non-ASCII names: an
// ASCII fallback plus the RFC 5987 encoded name.
func attachment(fileName string) string {
	fallback := strings.Map(func(r rune) rune {
		if r < 0x20 || r > 0x7e || r == '"' || r == '\\' {
			return '_'
		}
		return r
	}, fileName)
	return fmt.Sprintf("attachment; filename=%q; filename*=UTF-8''%s", fallback, url.PathEscape(fileName))
}

type documentUpload struct {
	kind, fileName string
	content        []byte
}

// readUpload streams the multipart body: the "kind" field and the "file" part,
// refusing a file larger than the limit (413).
func (h *OwnerDocumentsHandler) readUpload(w http.ResponseWriter, r *http.Request) (documentUpload, error) {
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "multipart/form-data" {
		return documentUpload{}, &apiError{Status: http.StatusUnsupportedMediaType, Code: codeUnsupportedMediaType, Message: "envie o arquivo como multipart/form-data"}
	}
	r.Body = http.MaxBytesReader(w, r.Body, h.maxBytes+multipartOverhead)
	reader, err := r.MultipartReader()
	if err != nil {
		return documentUpload{}, validationError("corpo multipart inválido")
	}
	var upload documentUpload
	var hasFile bool
	for {
		part, err := reader.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return documentUpload{}, h.uploadFailure(err)
		}
		switch part.FormName() {
		case "kind":
			value, err := io.ReadAll(io.LimitReader(part, maxKindBytes))
			if err != nil {
				return documentUpload{}, h.uploadFailure(err)
			}
			upload.kind = strings.TrimSpace(string(value))
		case "file":
			content, err := io.ReadAll(io.LimitReader(part, h.maxBytes+1))
			if err != nil {
				return documentUpload{}, h.uploadFailure(err)
			}
			if int64(len(content)) > h.maxBytes {
				return documentUpload{}, h.tooLarge()
			}
			upload.fileName, upload.content, hasFile = path.Base(part.FileName()), content, true
		}
	}
	var details []fieldDetail
	if upload.kind == "" {
		details = append(details, fieldDetail{Field: "kind", Message: "informe o tipo do documento"})
	}
	if !hasFile {
		details = append(details, fieldDetail{Field: "file", Message: "envie o arquivo"})
	}
	if len(details) > 0 {
		return documentUpload{}, validationError("dados inválidos", details...)
	}
	return upload, nil
}

func (h *OwnerDocumentsHandler) tooLarge() error {
	return &apiError{Status: http.StatusRequestEntityTooLarge, Code: codePayloadTooLarge,
		Message: fmt.Sprintf("arquivo maior que %d MB", h.maxBytes>>20)}
}

func (h *OwnerDocumentsHandler) uploadFailure(err error) error {
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		return h.tooLarge()
	}
	return validationError("corpo multipart inválido")
}

func ownerDocumentOf(document domain.OwnerDocument) ownerDocumentBody {
	return ownerDocumentBody{
		ID: string(document.ID), Kind: string(document.Kind), FileName: document.FileName, UploadedAt: document.UploadedAt.UTC(),
	}
}
