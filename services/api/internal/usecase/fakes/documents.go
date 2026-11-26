package fakes

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"time"

	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/domain"
	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/usecase"
)

// StoredDocument is the metadata row of a document, with its soft delete.
type StoredDocument struct {
	domain.OwnerDocument
	Removed bool
}

func (r *Registry) activeDocuments(companyID domain.CompanyID, ownerID domain.OwnerID) []domain.OwnerDocument {
	documents := []domain.OwnerDocument{}
	for _, stored := range r.Documents {
		if !stored.Removed && stored.CompanyID == companyID && stored.OwnerID == ownerID {
			documents = append(documents, stored.OwnerDocument)
		}
	}
	return documents
}

// ownerDocumentsPending reports whether a linked owner of the farm lacks a document.
func (r *Registry) ownerDocumentsPending(farm domain.Farm) bool {
	for _, link := range r.linksOfFarm(farm.ID) {
		if owner, ok := r.Owners[link.OwnerID]; ok && owner.DocumentsPending {
			return true
		}
	}
	return false
}

// ListOwnerDocuments implements usecase.OwnerDocumentReader.
func (r *Registry) ListOwnerDocuments(_ context.Context, companyID domain.CompanyID, ownerID domain.OwnerID) ([]domain.OwnerDocument, error) {
	if err := r.check("ListOwnerDocuments"); err != nil {
		return nil, err
	}
	return r.activeDocuments(companyID, ownerID), nil
}

// FindOwnerDocument implements usecase.OwnerDocumentReader.
func (r *Registry) FindOwnerDocument(_ context.Context, companyID domain.CompanyID, ownerID domain.OwnerID, id domain.OwnerDocumentID) (domain.OwnerDocument, error) {
	for _, document := range r.activeDocuments(companyID, ownerID) {
		if document.ID == id {
			return document, nil
		}
	}
	return domain.OwnerDocument{}, fmt.Errorf("find owner document: %w", usecase.ErrNotFound)
}

// InsertOwnerDocument implements usecase.OwnerDocumentStore.
func (r *Registry) InsertOwnerDocument(_ context.Context, document domain.OwnerDocument) error {
	if err := r.check("InsertOwnerDocument"); err != nil {
		return err
	}
	r.Documents = append(r.Documents, StoredDocument{OwnerDocument: document})
	return nil
}

// RemoveOwnerDocument implements usecase.OwnerDocumentStore (soft delete).
func (r *Registry) RemoveOwnerDocument(_ context.Context, companyID domain.CompanyID, ownerID domain.OwnerID, id domain.OwnerDocumentID, _ time.Time) error {
	for i, stored := range r.Documents {
		if !stored.Removed && stored.ID == id && stored.CompanyID == companyID && stored.OwnerID == ownerID {
			r.Documents[i].Removed = true
			return nil
		}
	}
	return fmt.Errorf("remove owner document: %w", usecase.ErrNotFound)
}

// SetDocumentsPending implements usecase.OwnerDocumentStore.
func (r *Registry) SetDocumentsPending(_ context.Context, companyID domain.CompanyID, ownerID domain.OwnerID, pending bool) error {
	owner, ok := r.ownerOf(companyID, ownerID)
	if !ok {
		return fmt.Errorf("set documents pending: %w", usecase.ErrNotFound)
	}
	owner.DocumentsPending = pending
	r.Owners[ownerID] = owner
	return nil
}

// ObjectStore is an in-memory object storage of owner documents.
type ObjectStore struct {
	Objects map[string][]byte
	Types   map[string]string
	// PutErr, GetErr and DeleteErr make the matching call fail.
	PutErr, GetErr, DeleteErr error
}

// NewObjectStore returns an empty object store.
func NewObjectStore() *ObjectStore {
	return &ObjectStore{Objects: map[string][]byte{}, Types: map[string]string{}}
}

// Keys lists the stored keys in order.
func (o *ObjectStore) Keys() []string { return slices.Sorted(maps.Keys(o.Objects)) }

// PutDocument implements usecase.DocumentStorage.
func (o *ObjectStore) PutDocument(_ context.Context, key, contentType string, content []byte) error {
	if o.PutErr != nil {
		return fmt.Errorf("put %s: %w", key, o.PutErr)
	}
	o.Objects[key], o.Types[key] = slices.Clone(content), contentType
	return nil
}

// GetDocument implements usecase.DocumentStorage.
func (o *ObjectStore) GetDocument(_ context.Context, key string) ([]byte, error) {
	if o.GetErr != nil {
		return nil, fmt.Errorf("get %s: %w", key, o.GetErr)
	}
	content, ok := o.Objects[key]
	if !ok {
		return nil, fmt.Errorf("get %s: %w", key, usecase.ErrNotFound)
	}
	return slices.Clone(content), nil
}

// DeleteDocument implements usecase.DocumentStorage.
func (o *ObjectStore) DeleteDocument(_ context.Context, key string) error {
	if o.DeleteErr != nil {
		return fmt.Errorf("delete %s: %w", key, o.DeleteErr)
	}
	delete(o.Objects, key)
	delete(o.Types, key)
	return nil
}
