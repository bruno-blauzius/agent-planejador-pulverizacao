package usecase

import (
	"context"
	"fmt"
	"time"

	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/domain"
)

// Owner documents (ADR 0020) follow the registry rules: reading needs no
// capability, writing needs registry.write, and an owner outside the scope of
// a planning operator is ErrNotFound. The bytes go to the object storage
// first; when the metadata cannot be stored, the object is removed again.

// OwnerDocumentDeps are the dependencies of the owner document use cases.
type OwnerDocumentDeps struct {
	Owners    OwnerReader
	Documents OwnerDocumentReader
	Storage   DocumentStorage
	Tx        RegistryTxRunner
	Clock     Clock
	IDs       IDGenerator
	Settings  Settings
}

// UploadOwnerDocumentInput is the request of POST /owners/{ownerId}/documents.
type UploadOwnerDocumentInput struct {
	Actor       domain.Principal
	OwnerID     domain.OwnerID
	Kind        domain.OwnerDocumentKind
	FileName    string
	ContentType string
	Content     []byte
}

// UploadOwnerDocument stores a document of an owner.
type UploadOwnerDocument struct {
	deps OwnerDocumentDeps
}

// NewUploadOwnerDocument builds the use case.
func NewUploadOwnerDocument(deps OwnerDocumentDeps) *UploadOwnerDocument {
	return &UploadOwnerDocument{deps: deps}
}

// Execute validates the file, sends it to the storage and records it; the
// owner's pending flag is recomputed in the same transaction.
func (uc *UploadOwnerDocument) Execute(ctx context.Context, in UploadOwnerDocumentInput) (domain.OwnerDocument, error) {
	if err := requireCan(in.Actor, domain.PermRegistryWrite); err != nil {
		return domain.OwnerDocument{}, err
	}
	listing, err := findVisibleOwner(ctx, uc.deps.Owners, in.Actor, in.OwnerID)
	if err != nil {
		return domain.OwnerDocument{}, err
	}
	if !domain.AcceptedDocumentMedia(in.ContentType) {
		return domain.OwnerDocument{}, fmt.Errorf("owner document %q: %w", in.ContentType, ErrUnsupportedDocumentMedia)
	}
	now := uc.deps.Clock.Now()
	document, err := domain.NewOwnerDocument(domain.OwnerDocumentID(uc.deps.IDs.NewID()), listing.Owner, in.Actor.UserID,
		domain.NewOwnerDocumentInput{
			Kind: in.Kind, FileName: in.FileName, ContentType: in.ContentType,
			SizeBytes: int64(len(in.Content)), MaxBytes: uc.deps.Settings.MaxDocumentBytes,
		}, now)
	if err != nil {
		return domain.OwnerDocument{}, err
	}
	if err := uc.deps.Storage.PutDocument(ctx, document.StorageKey, document.ContentType, in.Content); err != nil {
		return domain.OwnerDocument{}, fmt.Errorf("store document: %w", err)
	}
	err = uc.deps.Tx.InRegistryTx(ctx, func(ctx context.Context, tx RegistryStores) error {
		owner, err := tx.Owners.LockOwner(ctx, in.Actor.CompanyID, in.OwnerID)
		if err != nil {
			return fmt.Errorf("lock owner: %w", err)
		}
		if err := tx.Documents.InsertOwnerDocument(ctx, document); err != nil {
			return fmt.Errorf("insert owner document: %w", err)
		}
		if err := refreshDocumentsPending(ctx, tx.Documents, owner); err != nil {
			return err
		}
		return recordDocumentEvent(ctx, tx.Audit, in.Actor, now, domain.AuditOwnerDocumentAdded, document)
	})
	if err != nil {
		return domain.OwnerDocument{}, compensated(err, uc.deps.Storage.DeleteDocument(ctx, document.StorageKey))
	}
	return document, nil
}

// OwnerDocumentInput names one document of an owner.
type OwnerDocumentInput struct {
	Actor      domain.Principal
	OwnerID    domain.OwnerID
	DocumentID domain.OwnerDocumentID
}

// DownloadedDocument is a document with its bytes.
type DownloadedDocument struct {
	Document domain.OwnerDocument
	Content  []byte
}

// DownloadOwnerDocument returns the file of a document.
type DownloadOwnerDocument struct {
	deps OwnerDocumentDeps
}

// NewDownloadOwnerDocument builds the use case.
func NewDownloadOwnerDocument(deps OwnerDocumentDeps) *DownloadOwnerDocument {
	return &DownloadOwnerDocument{deps: deps}
}

// Execute reads the metadata and then the bytes from the storage.
func (uc *DownloadOwnerDocument) Execute(ctx context.Context, in OwnerDocumentInput) (DownloadedDocument, error) {
	if _, err := findVisibleOwner(ctx, uc.deps.Owners, in.Actor, in.OwnerID); err != nil {
		return DownloadedDocument{}, err
	}
	document, err := uc.deps.Documents.FindOwnerDocument(ctx, in.Actor.CompanyID, in.OwnerID, in.DocumentID)
	if err != nil {
		return DownloadedDocument{}, fmt.Errorf("find owner document: %w", err)
	}
	content, err := uc.deps.Storage.GetDocument(ctx, document.StorageKey)
	if err != nil {
		return DownloadedDocument{}, fmt.Errorf("read document: %w", err)
	}
	return DownloadedDocument{Document: document, Content: content}, nil
}

// RemoveOwnerDocument removes a document of an owner (soft delete: the object
// stays in the storage, under the versioning and lifecycle of the bucket).
type RemoveOwnerDocument struct {
	deps OwnerDocumentDeps
}

// NewRemoveOwnerDocument builds the use case.
func NewRemoveOwnerDocument(deps OwnerDocumentDeps) *RemoveOwnerDocument {
	return &RemoveOwnerDocument{deps: deps}
}

// Execute removes the document and recomputes the owner's pending flag.
func (uc *RemoveOwnerDocument) Execute(ctx context.Context, in OwnerDocumentInput) error {
	if err := requireCan(in.Actor, domain.PermRegistryWrite); err != nil {
		return err
	}
	if _, err := findVisibleOwner(ctx, uc.deps.Owners, in.Actor, in.OwnerID); err != nil {
		return err
	}
	now := uc.deps.Clock.Now()
	return uc.deps.Tx.InRegistryTx(ctx, func(ctx context.Context, tx RegistryStores) error {
		owner, err := tx.Owners.LockOwner(ctx, in.Actor.CompanyID, in.OwnerID)
		if err != nil {
			return fmt.Errorf("lock owner: %w", err)
		}
		document, err := tx.Documents.FindOwnerDocument(ctx, in.Actor.CompanyID, in.OwnerID, in.DocumentID)
		if err != nil {
			return fmt.Errorf("find owner document: %w", err)
		}
		if err := tx.Documents.RemoveOwnerDocument(ctx, in.Actor.CompanyID, in.OwnerID, in.DocumentID, now); err != nil {
			return fmt.Errorf("remove owner document: %w", err)
		}
		if err := refreshDocumentsPending(ctx, tx.Documents, owner); err != nil {
			return err
		}
		return recordDocumentEvent(ctx, tx.Audit, in.Actor, now, domain.AuditOwnerDocumentRemoved, document)
	})
}

// refreshDocumentsPending recomputes and stores whether the owner still lacks
// a required document.
func refreshDocumentsPending(ctx context.Context, documents OwnerDocumentStore, owner domain.Owner) error {
	current, err := documents.ListOwnerDocuments(ctx, owner.CompanyID, owner.ID)
	if err != nil {
		return fmt.Errorf("list owner documents: %w", err)
	}
	pending := domain.RequiredDocumentsMissing(owner.PersonType, current)
	if err := documents.SetDocumentsPending(ctx, owner.CompanyID, owner.ID, pending); err != nil {
		return fmt.Errorf("set documents pending: %w", err)
	}
	return nil
}

func recordDocumentEvent(ctx context.Context, audit AuditRecorder, actor domain.Principal, now time.Time, action string, document domain.OwnerDocument) error {
	// The file name may carry personal data: only the kind goes to the audit trail.
	event := actorEvent(actor, now, document.CompanyID, action, domain.AuditEntityOwner, string(document.OwnerID),
		map[string]string{"document_id": string(document.ID), "kind": string(document.Kind)})
	if err := audit.Record(ctx, event); err != nil {
		return fmt.Errorf("record audit: %w", err)
	}
	return nil
}
