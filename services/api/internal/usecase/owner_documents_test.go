package usecase_test

import (
	"slices"
	"testing"

	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/domain"
	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/usecase"
	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/usecase/fakes"
)

func (r *registryEnv) documentDeps(storage *fakes.ObjectStore) usecase.OwnerDocumentDeps {
	return usecase.OwnerDocumentDeps{
		Owners: r.reg, Documents: r.reg, Storage: storage, Tx: r.reg, Clock: r.clock, IDs: r.ids, Settings: r.settings,
	}
}

func idCard(actor domain.Principal, owner domain.OwnerID) usecase.UploadOwnerDocumentInput {
	return usecase.UploadOwnerDocumentInput{
		Actor: actor, OwnerID: owner, Kind: domain.DocumentIDCard, FileName: "rg.pdf",
		ContentType: domain.MediaPDF, Content: []byte("%PDF-1.7"),
	}
}

func (r *registryEnv) pendingOwner(id domain.OwnerID) {
	owner := r.reg.Owners[id]
	owner.DocumentsPending = true
	r.reg.Owners[id] = owner
}

func TestUploadOwnerDocumentClearsPending(t *testing.T) {
	r := newRegistryEnv(t)
	r.pendingOwner("own-a1")
	storage := fakes.NewObjectStore()

	document, err := usecase.NewUploadOwnerDocument(r.documentDeps(storage)).Execute(ctx(), idCard(r.asAdminA(), "own-a1"))
	assertNoError(t, err)
	if document.OwnerID != "own-a1" || document.UploadedBy != r.adminA.ID || document.SizeBytes != 8 {
		t.Fatalf("document = %+v", document)
	}
	if keys := storage.Keys(); len(keys) != 1 || keys[0] != document.StorageKey || storage.Types[keys[0]] != domain.MediaPDF {
		t.Fatalf("storage = %v %v", keys, storage.Types)
	}
	if r.reg.Owners["own-a1"].DocumentsPending {
		t.Fatal("an identity card clears the pending flag of a person")
	}
	r.assertRegistryAudit(t, domain.AuditOwnerDocumentAdded)

	listing, err := usecase.NewGetOwner(r.reg).Execute(ctx(), usecase.GetOwnerInput{Actor: r.asAdminA(), OwnerID: "own-a1"})
	assertNoError(t, err)
	if len(listing.Documents) != 1 || listing.Documents[0].ID != document.ID {
		t.Fatalf("owner documents = %+v", listing.Documents)
	}
}

func TestUploadOwnerDocumentRefusals(t *testing.T) {
	tests := []struct {
		name    string
		actor   func(*registryEnv) domain.Principal
		edit    func(*usecase.UploadOwnerDocumentInput)
		wantErr error
		field   string
	}{
		{"operator cannot write", (*registryEnv).asOperatorA, func(*usecase.UploadOwnerDocumentInput) {}, usecase.ErrForbidden, ""},
		{"owner of another company", (*registryEnv).asAdminB, func(*usecase.UploadOwnerDocumentInput) {}, usecase.ErrNotFound, ""},
		{"word document", (*registryEnv).asAdminA,
			func(in *usecase.UploadOwnerDocumentInput) { in.ContentType = "application/msword" }, usecase.ErrUnsupportedDocumentMedia, ""},
		{"unknown kind", (*registryEnv).asAdminA, func(in *usecase.UploadOwnerDocumentInput) { in.Kind = "passport" }, nil, "kind"},
		{"above the limit", (*registryEnv).asAdminA,
			func(in *usecase.UploadOwnerDocumentInput) { in.Content = make([]byte, 1<<20+1) }, nil, "file"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := newRegistryEnv(t)
			storage := fakes.NewObjectStore()
			in := idCard(tt.actor(r), "own-a1")
			tt.edit(&in)
			_, err := usecase.NewUploadOwnerDocument(r.documentDeps(storage)).Execute(ctx(), in)
			if tt.wantErr != nil {
				assertIs(t, err, tt.wantErr)
			} else {
				assertValidationOn(t, err, tt.field)
			}
			if len(storage.Objects) != 0 || len(r.reg.Documents) != 0 || len(r.reg.Audit) != 0 {
				t.Fatal("a refused upload must store nothing")
			}
		})
	}
}

func TestUploadOwnerDocumentStorageFailures(t *testing.T) {
	t.Run("storage down: nothing recorded", func(t *testing.T) {
		r := newRegistryEnv(t)
		storage := fakes.NewObjectStore()
		storage.PutErr = usecase.ErrDocumentStorageUnavailable
		_, err := usecase.NewUploadOwnerDocument(r.documentDeps(storage)).Execute(ctx(), idCard(r.asAdminA(), "own-a1"))
		assertIs(t, err, usecase.ErrDocumentStorageUnavailable)
		if len(r.reg.Documents) != 0 {
			t.Fatal("metadata stored without the file")
		}
	})
	t.Run("metadata fails: the object is removed again", func(t *testing.T) {
		r := newRegistryEnv(t)
		r.reg.Fail("InsertOwnerDocument", errBoom)
		storage := fakes.NewObjectStore()
		_, err := usecase.NewUploadOwnerDocument(r.documentDeps(storage)).Execute(ctx(), idCard(r.asAdminA(), "own-a1"))
		assertIs(t, err, errBoom)
		if len(storage.Objects) != 0 {
			t.Fatalf("orphan objects: %v", storage.Keys())
		}
	})
	t.Run("compensation fails too: both errors reported", func(t *testing.T) {
		r := newRegistryEnv(t)
		r.reg.Fail("Record", errBoom)
		storage := fakes.NewObjectStore()
		storage.DeleteErr = usecase.ErrDocumentStorageUnavailable
		_, err := usecase.NewUploadOwnerDocument(r.documentDeps(storage)).Execute(ctx(), idCard(r.asAdminA(), "own-a1"))
		assertIs(t, err, errBoom)
		assertIs(t, err, usecase.ErrDocumentStorageUnavailable)
	})
}

func TestDownloadAndRemoveOwnerDocument(t *testing.T) {
	r := newRegistryEnv(t)
	storage := fakes.NewObjectStore()
	deps := r.documentDeps(storage)
	document, err := usecase.NewUploadOwnerDocument(deps).Execute(ctx(), idCard(r.asAdminA(), "own-a1"))
	assertNoError(t, err)
	ref := usecase.OwnerDocumentInput{Actor: r.operatorOnly(t, "farm-a1"), OwnerID: "own-a1", DocumentID: document.ID}

	downloaded, err := usecase.NewDownloadOwnerDocument(deps).Execute(ctx(), ref)
	assertNoError(t, err)
	if string(downloaded.Content) != "%PDF-1.7" || downloaded.Document.FileName != "rg.pdf" {
		t.Fatalf("downloaded = %+v", downloaded)
	}
	hidden := ref
	hidden.Actor = r.operatorOnly(t, "farm-a2")
	_, err = usecase.NewDownloadOwnerDocument(deps).Execute(ctx(), hidden)
	assertIs(t, err, usecase.ErrNotFound)

	remove := usecase.NewRemoveOwnerDocument(deps)
	assertIs(t, remove.Execute(ctx(), ref), usecase.ErrForbidden)
	ref.Actor = r.asAdminA()
	assertNoError(t, remove.Execute(ctx(), ref))
	if !r.reg.Owners["own-a1"].DocumentsPending {
		t.Fatal("removing the only identity card makes the owner pending again")
	}
	assertIs(t, remove.Execute(ctx(), ref), usecase.ErrNotFound)
	_, err = usecase.NewDownloadOwnerDocument(deps).Execute(ctx(), ref)
	assertIs(t, err, usecase.ErrNotFound)
	if len(storage.Objects) != 1 {
		t.Fatal("removal is a soft delete: the object stays in the storage")
	}
	r.assertRegistryAudit(t, domain.AuditOwnerDocumentAdded, domain.AuditOwnerDocumentRemoved)
}

func TestFarmPendingFollowsOwnerDocuments(t *testing.T) {
	r := newRegistryEnv(t)
	r.withPerimeter("farm-a1")
	farm := r.reg.Farms["farm-a1"]
	farm.Status = domain.FarmActive
	r.reg.Farms["farm-a1"] = farm
	r.pendingOwner("own-a1")
	list := usecase.NewListFarms(r.reg, r.settings)
	names := func(status domain.FarmStatus) []string {
		page, err := list.Execute(ctx(), usecase.ListFarmsInput{Actor: r.asAdminA(), Status: status})
		assertNoError(t, err)
		return farmNames(page)
	}
	if got := names(domain.FarmPending); !slices.Equal(got, []string{"Santa Rita"}) {
		t.Fatalf("pending = %v", got)
	}
	if got := names(domain.FarmActive); len(got) != 0 {
		t.Fatalf("active = %v", got)
	}

	_, err := usecase.NewUploadOwnerDocument(r.documentDeps(fakes.NewObjectStore())).Execute(ctx(), idCard(r.asAdminA(), "own-a1"))
	assertNoError(t, err)
	if got := names(domain.FarmActive); !slices.Equal(got, []string{"Santa Rita"}) {
		t.Fatalf("active after the document = %v", got)
	}
}
