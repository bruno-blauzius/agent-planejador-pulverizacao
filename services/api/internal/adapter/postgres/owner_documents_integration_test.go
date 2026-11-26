//go:build integration

package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/adapter/postgres"
	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/domain"
	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/usecase"
)

func documentOf(t *testing.T, owner domain.Owner, uploader domain.User, kind domain.OwnerDocumentKind) domain.OwnerDocument {
	t.Helper()
	document, err := domain.NewOwnerDocument(domain.OwnerDocumentID(newID()), owner, uploader.ID, domain.NewOwnerDocumentInput{
		Kind: kind, FileName: "documento.pdf", ContentType: domain.MediaPDF, SizeBytes: 1024, MaxBytes: 10 << 20,
	}, time.Now().UTC().Truncate(time.Microsecond))
	must(t, err)
	return document
}

func TestOwnerDocumentsAndFarmPending(t *testing.T) {
	company := newCompany(t)
	uploader := newUser(t, company)
	owner := newOwner(t, company, "Helena")
	farm := farmValue(company, "Santa Rita", domain.Ownership{OwnerID: owner.ID, Role: domain.LinkOwner, Share: shareOf(100)})
	insertFarm(t, farm)
	page := usecase.PageRequest{Page: 1, PageSize: 20}
	statusTotal := func(status domain.FarmStatus) int {
		t.Helper()
		result, err := testDB.ListFarms(ctx(), company.ID, usecase.FarmFilter{Status: status, Visible: domain.AllFarms()}, page)
		must(t, err)
		return result.Total
	}

	// A new owner starts pending (column default): the active farm shows as pending.
	listing, err := testDB.FindFarm(ctx(), company.ID, farm.ID)
	must(t, err)
	if !listing.OwnerDocumentsPending || statusTotal(domain.FarmPending) != 1 || statusTotal(domain.FarmActive) != 0 {
		t.Fatalf("pending = %v, totals pending %d active %d", listing.OwnerDocumentsPending, statusTotal(domain.FarmPending), statusTotal(domain.FarmActive))
	}

	document := documentOf(t, owner, uploader, domain.DocumentIDCard)
	must(t, testDB.InRegistryTx(ctx(), func(ctx context.Context, tx usecase.RegistryStores) error {
		if err := tx.Documents.InsertOwnerDocument(ctx, document); err != nil {
			return err
		}
		return tx.Documents.SetDocumentsPending(ctx, company.ID, owner.ID, false)
	}))
	if statusTotal(domain.FarmActive) != 1 || statusTotal(domain.FarmPending) != 0 {
		t.Fatal("the farm must be active once the owner has its document")
	}
	found, err := testDB.FindOwner(ctx(), company.ID, owner.ID, domain.AllFarms())
	must(t, err)
	if found.Owner.DocumentsPending || len(found.Documents) != 1 || found.Documents[0].StorageKey != document.StorageKey {
		t.Fatalf("owner = %+v, documents %+v", found.Owner, found.Documents)
	}
	stored, err := testDB.FindOwnerDocument(ctx(), company.ID, owner.ID, document.ID)
	must(t, err)
	if stored.FileName != "documento.pdf" || stored.UploadedBy != uploader.ID || !stored.UploadedAt.Equal(document.UploadedAt) {
		t.Fatalf("stored document = %+v", stored)
	}

	other := newCompany(t)
	if _, err := testDB.FindOwnerDocument(ctx(), other.ID, owner.ID, document.ID); !errors.Is(err, usecase.ErrNotFound) {
		t.Fatalf("document visible to another company: %v", err)
	}

	must(t, testDB.RemoveOwnerDocument(ctx(), company.ID, owner.ID, document.ID, time.Now()))
	if err := testDB.RemoveOwnerDocument(ctx(), company.ID, owner.ID, document.ID, time.Now()); !errors.Is(err, usecase.ErrNotFound) {
		t.Fatalf("second removal: %v", err)
	}
	remaining, err := testDB.ListOwnerDocuments(ctx(), company.ID, owner.ID)
	must(t, err)
	if len(remaining) != 0 {
		t.Fatalf("removed document still listed: %+v", remaining)
	}
}

func TestOwnerDocumentKeyMustBeTheOwnersPrefix(t *testing.T) {
	company := newCompany(t)
	uploader := newUser(t, company)
	owner := newOwner(t, company, "Marcos")
	document := documentOf(t, owner, uploader, domain.DocumentDriverLicense)
	document.StorageKey = "companies/other/owners/x/documents/y"
	err := testDB.InsertOwnerDocument(ctx(), document)
	var integrity *postgres.IntegrityError
	if !errors.As(err, &integrity) || integrity.Constraint != "ck_owner_documents_storage_key" {
		t.Fatalf("a key outside the owner's prefix was stored: %v", err)
	}
}
