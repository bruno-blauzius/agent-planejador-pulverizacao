package usecase

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/domain"
)

// ListOwnersInput is the request of GET /owners.
type ListOwnersInput struct {
	Actor      domain.Principal
	Search     string
	PersonType domain.PersonType
	Page       int
	PageSize   int
}

// ListOwners lists the owners the caller sees: every owner of the company, or,
// for a selected farm scope, the owners linked to those farms.
type ListOwners struct {
	owners   OwnerReader
	settings Settings
}

// NewListOwners builds the use case.
func NewListOwners(owners OwnerReader, settings Settings) *ListOwners {
	return &ListOwners{owners: owners, settings: settings}
}

// Execute returns one page of owners.
func (uc *ListOwners) Execute(ctx context.Context, in ListOwnersInput) (OwnerPage, error) {
	var problems domain.Violations
	if in.PersonType != "" && !in.PersonType.Valid() {
		problems.Add("person_type", "tipo de pessoa desconhecido")
	}
	page, err := uc.settings.pageRequest(in.Page, in.PageSize)
	if mergeErr := problems.Merge(err); mergeErr != nil {
		return OwnerPage{}, mergeErr
	}
	if err := problems.Err(); err != nil {
		return OwnerPage{}, err
	}
	filter := OwnerFilter{Search: in.Search, PersonType: in.PersonType, Visible: in.Actor.FarmScope}
	result, err := uc.owners.ListOwners(ctx, in.Actor.CompanyID, filter, page)
	if err != nil {
		return OwnerPage{}, fmt.Errorf("list owners: %w", err)
	}
	return result, nil
}

// GetOwnerInput is the request of GET /owners/{ownerId} and GET /owners/{ownerId}/links.
type GetOwnerInput struct {
	Actor   domain.Principal
	OwnerID domain.OwnerID
}

// GetOwner returns one owner with its links.
type GetOwner struct {
	owners OwnerReader
}

// NewGetOwner builds the use case.
func NewGetOwner(owners OwnerReader) *GetOwner { return &GetOwner{owners: owners} }

// Execute returns the owner or ErrNotFound.
func (uc *GetOwner) Execute(ctx context.Context, in GetOwnerInput) (OwnerListing, error) {
	return findVisibleOwner(ctx, uc.owners, in.Actor, in.OwnerID)
}

// findVisibleOwner hides, from a selected farm scope, the owners without any
// link to the visible farms.
func findVisibleOwner(ctx context.Context, owners OwnerReader, actor domain.Principal, id domain.OwnerID) (OwnerListing, error) {
	listing, err := owners.FindOwner(ctx, actor.CompanyID, id, actor.FarmScope)
	if err != nil {
		return OwnerListing{}, fmt.Errorf("find owner: %w", err)
	}
	if actor.FarmScope.IsSelected() && len(listing.Links) == 0 {
		return OwnerListing{}, fmt.Errorf("find owner %s: %w", id, ErrNotFound)
	}
	return listing, nil
}

// SaveOwnerInput is the request of POST /owners (OwnerID empty) and PUT /owners/{ownerId}.
type SaveOwnerInput struct {
	Actor   domain.Principal
	OwnerID domain.OwnerID
	Details domain.OwnerDetails
}

// CreateOwner registers an owner.
type CreateOwner struct {
	owners OwnerReader
	tx     RegistryTxRunner
	clock  Clock
	ids    IDGenerator
}

// NewCreateOwner builds the use case.
func NewCreateOwner(owners OwnerReader, tx RegistryTxRunner, clock Clock, ids IDGenerator) *CreateOwner {
	return &CreateOwner{owners: owners, tx: tx, clock: clock, ids: ids}
}

// Execute validates and stores the owner. A CPF/CNPJ already used in the
// company is ErrOwnerDocumentTaken.
func (uc *CreateOwner) Execute(ctx context.Context, in SaveOwnerInput) (OwnerListing, error) {
	if err := requireCan(in.Actor, domain.PermRegistryWrite); err != nil {
		return OwnerListing{}, err
	}
	now := uc.clock.Now()
	owner, err := domain.NewOwner(domain.OwnerID(uc.ids.NewID()), in.Actor.CompanyID, in.Details, now)
	if err != nil {
		return OwnerListing{}, err
	}
	err = uc.tx.InRegistryTx(ctx, func(ctx context.Context, tx RegistryStores) error {
		if err := tx.Owners.InsertOwner(ctx, owner); err != nil {
			return fmt.Errorf("insert owner: %w", err)
		}
		return recordOwnerEvent(ctx, tx.Audit, in.Actor, now, domain.AuditOwnerCreated, owner, nil)
	})
	if err != nil {
		return OwnerListing{}, err
	}
	// A new owner has no links yet: it is returned whatever the caller's farm scope.
	return OwnerListing{Owner: owner, Links: []OwnerLink{}}, nil
}

// UpdateOwner replaces the editable data of an owner (not its document).
type UpdateOwner struct {
	owners OwnerReader
	tx     RegistryTxRunner
	clock  Clock
}

// NewUpdateOwner builds the use case.
func NewUpdateOwner(owners OwnerReader, tx RegistryTxRunner, clock Clock) *UpdateOwner {
	return &UpdateOwner{owners: owners, tx: tx, clock: clock}
}

// Execute revises the owner on a locked row.
func (uc *UpdateOwner) Execute(ctx context.Context, in SaveOwnerInput) (OwnerListing, error) {
	if err := requireCan(in.Actor, domain.PermRegistryWrite); err != nil {
		return OwnerListing{}, err
	}
	if _, err := findVisibleOwner(ctx, uc.owners, in.Actor, in.OwnerID); err != nil {
		return OwnerListing{}, err
	}
	now := uc.clock.Now()
	err := uc.tx.InRegistryTx(ctx, func(ctx context.Context, tx RegistryStores) error {
		owner, err := tx.Owners.LockOwner(ctx, in.Actor.CompanyID, in.OwnerID)
		if err != nil {
			return fmt.Errorf("lock owner: %w", err)
		}
		if err := owner.Revise(in.Details, now); err != nil {
			return err
		}
		if err := tx.Owners.UpdateOwner(ctx, owner); err != nil {
			return fmt.Errorf("update owner: %w", err)
		}
		return recordOwnerEvent(ctx, tx.Audit, in.Actor, now, domain.AuditOwnerUpdated, owner, nil)
	})
	if err != nil {
		return OwnerListing{}, err
	}
	return findVisibleOwner(ctx, uc.owners, in.Actor, in.OwnerID)
}

// ReplaceOwnerLinksInput is the request of PUT /owners/{ownerId}/links.
type ReplaceOwnerLinksInput struct {
	Actor   domain.Principal
	OwnerID domain.OwnerID
	Links   []domain.OwnershipInput
}

// ReplaceOwnerLinks replaces every link of an owner. A farm whose shares would
// go above 100% is refused; a farm may stay below 100% (the farm screen asks
// to complete it).
type ReplaceOwnerLinks struct {
	owners OwnerReader
	tx     RegistryTxRunner
	clock  Clock
}

// NewReplaceOwnerLinks builds the use case.
func NewReplaceOwnerLinks(owners OwnerReader, tx RegistryTxRunner, clock Clock) *ReplaceOwnerLinks {
	return &ReplaceOwnerLinks{owners: owners, tx: tx, clock: clock}
}

// Execute validates the links and stores them in one transaction, with every
// farm involved locked, so concurrent changes cannot exceed 100% together.
func (uc *ReplaceOwnerLinks) Execute(ctx context.Context, in ReplaceOwnerLinksInput) ([]OwnerLink, error) {
	if err := requireCan(in.Actor, domain.PermRegistryWrite); err != nil {
		return nil, err
	}
	current, err := findVisibleOwner(ctx, uc.owners, in.Actor, in.OwnerID)
	if err != nil {
		return nil, err
	}
	links, err := ownerLinks(in.OwnerID, in.Links)
	if err != nil {
		return nil, err
	}
	now := uc.clock.Now()
	err = uc.tx.InRegistryTx(ctx, func(ctx context.Context, tx RegistryStores) error {
		owner, err := tx.Owners.LockOwner(ctx, in.Actor.CompanyID, in.OwnerID)
		if err != nil {
			return fmt.Errorf("lock owner: %w", err)
		}
		if err := checkLinkedFarms(ctx, tx.Ownerships, in.Actor, links, linkedFarms(current.Links)); err != nil {
			return err
		}
		if err := tx.Ownerships.ReplaceOwnerLinks(ctx, in.Actor.CompanyID, owner.ID, links); err != nil {
			return fmt.Errorf("replace owner links: %w", err)
		}
		return recordOwnerEvent(ctx, tx.Audit, in.Actor, now, domain.AuditOwnerLinksReplaced, owner,
			map[string]string{"links": fmt.Sprint(len(links))})
	})
	if err != nil {
		return nil, err
	}
	listing, err := findVisibleOwner(ctx, uc.owners, in.Actor, in.OwnerID)
	if err != nil {
		return nil, err
	}
	return listing.Links, nil
}

// ownerLinks validates the items of the request, reporting each on its index.
func ownerLinks(ownerID domain.OwnerID, inputs []domain.OwnershipInput) ([]domain.Ownership, error) {
	var problems domain.Violations
	links := make([]domain.Ownership, 0, len(inputs))
	seen := make(map[domain.FarmID]struct{}, len(inputs))
	for i, input := range inputs {
		field := fmt.Sprintf("links[%d]", i)
		if input.FarmID == "" {
			problems.Add(field+".farm_id", "informe a fazenda")
			continue
		}
		if _, dup := seen[input.FarmID]; dup {
			problems.Add(field+".farm_id", "fazenda repetida")
			continue
		}
		seen[input.FarmID] = struct{}{}
		input.OwnerID = ownerID
		link, err := domain.NewOwnership(field, input)
		if err != nil {
			if other := problems.Merge(err); other != nil {
				return nil, other
			}
			continue
		}
		links = append(links, link)
	}
	if err := problems.Err(); err != nil {
		return nil, err
	}
	return links, nil
}

// checkLinkedFarms locks the farms the owner leaves and joins, then checks
// that each new link exists, is visible and keeps its farm at or below 100%.
func checkLinkedFarms(ctx context.Context, store OwnershipStore, actor domain.Principal, links []domain.Ownership, leaving []domain.FarmID) error {
	joining := make([]domain.FarmID, 0, len(links))
	for _, link := range links {
		joining = append(joining, link.FarmID)
	}
	involved := slices.Compact(slices.Sorted(slices.Values(append(slices.Clone(joining), leaving...))))
	missing, err := store.LockFarms(ctx, actor.CompanyID, involved)
	if err != nil {
		return fmt.Errorf("lock farms: %w", err)
	}
	existing, err := store.ListFarmOwnerships(ctx, actor.CompanyID, joining)
	if err != nil {
		return fmt.Errorf("list farm ownerships: %w", err)
	}

	var problems domain.Violations
	for i, link := range links {
		field := fmt.Sprintf("links[%d]", i)
		if slices.Contains(missing, link.FarmID) || !actor.FarmScope.Allows(link.FarmID) {
			problems.Add(field+".farm_id", "fazenda não encontrada")
			continue
		}
		if link.Share == nil {
			continue
		}
		if !domain.FitsInFarm(othersOf(existing, link), *link.Share) {
			problems.Add(field+".share_percent", "a participação total da fazenda passaria de 100%")
		}
	}
	return problems.Err()
}

// othersOf returns the links of the same farm held by other owners.
func othersOf(existing []domain.Ownership, link domain.Ownership) []domain.Ownership {
	var others []domain.Ownership
	for _, candidate := range existing {
		if candidate.FarmID == link.FarmID && candidate.OwnerID != link.OwnerID {
			others = append(others, candidate)
		}
	}
	return others
}

func linkedFarms(links []OwnerLink) []domain.FarmID {
	ids := make([]domain.FarmID, 0, len(links))
	for _, link := range links {
		ids = append(ids, link.FarmID)
	}
	return ids
}

// recordOwnerEvent audits an owner change. The document never goes to the
// metadata (ADR 0015, security).
func recordOwnerEvent(ctx context.Context, audit AuditRecorder, actor domain.Principal, now time.Time, action string, owner domain.Owner, metadata map[string]string) error {
	if metadata == nil {
		metadata = map[string]string{}
	}
	metadata["person_type"] = string(owner.PersonType)
	event := actorEvent(actor, now, owner.CompanyID, action, domain.AuditEntityOwner, string(owner.ID), metadata)
	if err := audit.Record(ctx, event); err != nil {
		return fmt.Errorf("record audit: %w", err)
	}
	return nil
}
