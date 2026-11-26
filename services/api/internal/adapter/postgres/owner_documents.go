package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/adapter/postgres/sqlcgen"
	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/domain"
	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/usecase"
)

// Owner document metadata (ADR 0020); the bytes live in the object storage.

func ownerDocumentOf(row sqlcgen.ListOwnerDocumentsRow) domain.OwnerDocument {
	return domain.OwnerDocument{
		ID: domain.OwnerDocumentID(row.ID), CompanyID: domain.CompanyID(row.CompanyID), OwnerID: domain.OwnerID(row.OwnerID),
		Kind: domain.OwnerDocumentKind(row.Kind), FileName: row.FileName, ContentType: row.ContentType,
		SizeBytes: row.SizeBytes, StorageKey: row.StorageKey, UploadedBy: domain.UserID(row.UploadedByUserID),
		UploadedAt: row.UploadedAt,
	}
}

// ListOwnerDocuments implements usecase.OwnerDocumentReader.
func (r *Repository) ListOwnerDocuments(ctx context.Context, companyID domain.CompanyID, ownerID domain.OwnerID) ([]domain.OwnerDocument, error) {
	documents := []domain.OwnerDocument{}
	if !validUUID(string(companyID)) || !validUUID(string(ownerID)) {
		return documents, nil
	}
	rows, err := r.q.ListOwnerDocuments(ctx, sqlcgen.ListOwnerDocumentsParams{CompanyID: string(companyID), OwnerID: string(ownerID)})
	if err != nil {
		return nil, fmt.Errorf("list owner documents: %w", err)
	}
	for _, row := range rows {
		documents = append(documents, ownerDocumentOf(row))
	}
	return documents, nil
}

// FindOwnerDocument implements usecase.OwnerDocumentReader.
func (r *Repository) FindOwnerDocument(ctx context.Context, companyID domain.CompanyID, ownerID domain.OwnerID, id domain.OwnerDocumentID) (domain.OwnerDocument, error) {
	row, err := r.q.GetOwnerDocument(ctx, sqlcgen.GetOwnerDocumentParams{CompanyID: string(companyID), OwnerID: string(ownerID), ID: string(id)})
	if err != nil {
		return domain.OwnerDocument{}, translateRead("find owner document", err)
	}
	return ownerDocumentOf(sqlcgen.ListOwnerDocumentsRow(row)), nil
}

// InsertOwnerDocument implements usecase.OwnerDocumentStore.
func (r *Repository) InsertOwnerDocument(ctx context.Context, document domain.OwnerDocument) error {
	err := r.q.InsertOwnerDocument(ctx, sqlcgen.InsertOwnerDocumentParams{
		ID: string(document.ID), CompanyID: string(document.CompanyID), OwnerID: string(document.OwnerID),
		Kind: string(document.Kind), FileName: document.FileName, ContentType: document.ContentType,
		SizeBytes: document.SizeBytes, StorageKey: document.StorageKey,
		UploadedByUserID: string(document.UploadedBy), UploadedAt: document.UploadedAt,
	})
	return translateWrite("insert owner document", err)
}

// RemoveOwnerDocument implements usecase.OwnerDocumentStore (soft delete).
func (r *Repository) RemoveOwnerDocument(ctx context.Context, companyID domain.CompanyID, ownerID domain.OwnerID, id domain.OwnerDocumentID, now time.Time) error {
	removed, err := r.q.RemoveOwnerDocument(ctx, sqlcgen.RemoveOwnerDocumentParams{
		DeletedAt: &now, CompanyID: string(companyID), OwnerID: string(ownerID), ID: string(id),
	})
	if err != nil {
		return translateRead("remove owner document", err)
	}
	if removed == 0 {
		return fmt.Errorf("remove owner document: %w", usecase.ErrNotFound)
	}
	return nil
}

// SetDocumentsPending implements usecase.OwnerDocumentStore.
func (r *Repository) SetDocumentsPending(ctx context.Context, companyID domain.CompanyID, ownerID domain.OwnerID, pending bool) error {
	err := r.q.SetOwnerDocumentsPending(ctx, sqlcgen.SetOwnerDocumentsPendingParams{
		DocumentsPending: pending, CompanyID: string(companyID), ID: string(ownerID),
	})
	return translateWrite("set documents pending", err)
}
