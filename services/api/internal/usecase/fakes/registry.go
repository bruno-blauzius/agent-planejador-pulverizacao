package fakes

import (
	"cmp"
	"context"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/domain"
	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/usecase"
)

// Registry is an in-memory store of farms, owners and their links. It keeps
// the contracts of the Postgres adapter: company isolation, visibility scope,
// unique document per company, optimistic versions and rollback.
type Registry struct {
	Farms  map[domain.FarmID]domain.Farm
	Owners map[domain.OwnerID]domain.Owner
	// Links holds every farm-owner link; Farm.Owners is rebuilt from it on reads.
	Links []domain.Ownership
	// Fields and Crops back the field registry (Crops is the catalog of active crops).
	Fields map[domain.FieldID]domain.Field
	Crops  []domain.Crop
	// Documents holds owner document metadata; removed ones stay with Removed set.
	Documents []StoredDocument
	Audit     []domain.AuditEvent

	// terrain is set by NewTerrain; registry transactions invalidate it and roll it back.
	terrain  *Terrain
	failures map[string]error
}

// NewRegistry returns an empty registry with its relief store (see Terrain),
// so registry transactions invalidate and roll back the relief like the database does.
func NewRegistry() *Registry {
	r := &Registry{
		Farms:  map[domain.FarmID]domain.Farm{},
		Owners: map[domain.OwnerID]domain.Owner{},
		Fields: map[domain.FieldID]domain.Field{},
		Crops: []domain.Crop{
			{Code: "coffee_arabica", Name: "Café arábica"},
			{Code: "soybean", Name: "Soja"},
			{Code: "corn", Name: "Milho"},
		},
		failures: map[string]error{},
	}
	NewTerrain(r)
	return r
}

// DetachTerrain removes the relief store from registry transactions, to test
// that a polygon change without it fails loudly.
func (r *Registry) DetachTerrain() { r.terrain = nil }

// Fail makes every call of the named operation return err.
func (r *Registry) Fail(operation string, err error) { r.failures[operation] = err }

func (r *Registry) check(operation string) error {
	if err := r.failures[operation]; err != nil {
		return fmt.Errorf("%s: %w", operation, err)
	}
	return nil
}

// AuditActions lists the recorded actions in order.
func (r *Registry) AuditActions() []string {
	actions := make([]string, 0, len(r.Audit))
	for _, event := range r.Audit {
		actions = append(actions, event.Action)
	}
	return actions
}

// InRegistryTx implements usecase.RegistryTxRunner with rollback on failure.
func (r *Registry) InRegistryTx(ctx context.Context, fn func(ctx context.Context, tx usecase.RegistryStores) error) error {
	farms, owners, links, audit := maps.Clone(r.Farms), maps.Clone(r.Owners), slices.Clone(r.Links), slices.Clone(r.Audit)
	fields, documents := maps.Clone(r.Fields), slices.Clone(r.Documents)
	stores := usecase.RegistryStores{Farms: r, Owners: r, Ownerships: r, Fields: r, Documents: r, Audit: r}
	var terrain terrainSnapshot
	if r.terrain != nil {
		stores.Terrain = r.terrain
		terrain = r.terrain.snapshot()
	}
	err := fn(ctx, stores)
	if err == nil {
		err = r.check("Commit")
	}
	if err != nil {
		r.Farms, r.Owners, r.Links, r.Audit, r.Fields, r.Documents = farms, owners, links, audit, fields, documents
		if r.terrain != nil {
			r.terrain.restore(terrain)
		}
		return err
	}
	return nil
}

// Record implements usecase.AuditRecorder.
func (r *Registry) Record(_ context.Context, event domain.AuditEvent) error {
	if err := r.check("Record"); err != nil {
		return err
	}
	r.Audit = append(r.Audit, event)
	return nil
}

// ---- farms --------------------------------------------------------------------------

func (r *Registry) farmOf(companyID domain.CompanyID, id domain.FarmID) (domain.Farm, bool) {
	farm, ok := r.Farms[id]
	if !ok || farm.CompanyID != companyID {
		return domain.Farm{}, false
	}
	farm.Owners = r.linksOfFarm(id)
	return farm, true
}

func (r *Registry) linksOfFarm(id domain.FarmID) []domain.Ownership {
	var links []domain.Ownership
	for _, link := range r.Links {
		if link.FarmID == id {
			links = append(links, link)
		}
	}
	return links
}

func (r *Registry) farmListing(farm domain.Farm) usecase.FarmListing {
	names := map[domain.OwnerID]string{}
	for _, link := range farm.Owners {
		names[link.OwnerID] = r.Owners[link.OwnerID].Name
	}
	listing := usecase.FarmListing{Farm: farm, OwnerNames: names}
	if farm.Perimeter != nil {
		listing.AreaHa = 100 // the geometry engine is the database's; a fixed area is enough here
		listing.Outline = farm.Perimeter
	}
	r.fieldSummary(&listing)
	listing.OwnerDocumentsPending = r.ownerDocumentsPending(farm)
	return listing
}

// ListFarms implements usecase.FarmReader (search on name, code or city).
func (r *Registry) ListFarms(_ context.Context, companyID domain.CompanyID, filter usecase.FarmFilter, page usecase.PageRequest) (usecase.FarmPage, error) {
	if err := r.check("ListFarms"); err != nil {
		return usecase.FarmPage{}, err
	}
	var matches []domain.Farm
	for id := range r.Farms {
		farm, ok := r.farmOf(companyID, id)
		if !ok || !filter.Visible.Allows(id) || !r.farmMatches(farm, filter) {
			continue
		}
		matches = append(matches, farm)
	}
	slices.SortFunc(matches, func(a, b domain.Farm) int { return cmp.Or(cmp.Compare(a.Name, b.Name), cmp.Compare(a.ID, b.ID)) })
	result := usecase.FarmPage{Items: []usecase.FarmListing{}, Total: len(matches), PageRequest: page}
	for _, farm := range pageOf(matches, page) {
		result.Items = append(result.Items, r.farmListing(farm))
	}
	return result, nil
}

func (r *Registry) farmMatches(farm domain.Farm, filter usecase.FarmFilter) bool {
	if filter.Status != "" && domain.DisplayStatus(farm.Status, r.ownerDocumentsPending(farm)) != filter.Status {
		return false
	}
	if filter.ProductionType != "" && farm.ProductionType != filter.ProductionType {
		return false
	}
	code := ""
	if farm.Code != nil {
		code = *farm.Code
	}
	return containsFold(filter.Search, farm.Name, code, farm.City)
}

// FindFarm implements usecase.FarmReader.
func (r *Registry) FindFarm(_ context.Context, companyID domain.CompanyID, id domain.FarmID) (usecase.FarmListing, error) {
	farm, ok := r.farmOf(companyID, id)
	if !ok {
		return usecase.FarmListing{}, fmt.Errorf("find farm: %w", usecase.ErrNotFound)
	}
	return r.farmListing(farm), nil
}

// LockFarm implements usecase.FarmStore.
func (r *Registry) LockFarm(ctx context.Context, companyID domain.CompanyID, id domain.FarmID) (domain.Farm, error) {
	farm, ok := r.farmOf(companyID, id)
	if !ok {
		return domain.Farm{}, fmt.Errorf("lock farm: %w", usecase.ErrNotFound)
	}
	return farm, nil
}

// InsertFarm implements usecase.FarmStore.
func (r *Registry) InsertFarm(_ context.Context, farm domain.Farm) error {
	if err := r.check("InsertFarm"); err != nil {
		return err
	}
	if _, exists := r.Farms[farm.ID]; exists {
		return fmt.Errorf("insert farm: duplicate id %s", farm.ID)
	}
	farm.Version = 1
	r.storeFarm(farm)
	return nil
}

// UpdateFarm implements usecase.FarmStore with optimistic concurrency.
func (r *Registry) UpdateFarm(_ context.Context, farm domain.Farm) error {
	if err := r.check("UpdateFarm"); err != nil {
		return err
	}
	stored, ok := r.Farms[farm.ID]
	if !ok || stored.CompanyID != farm.CompanyID {
		return fmt.Errorf("update farm: %w", usecase.ErrNotFound)
	}
	if stored.Version != farm.Version {
		return fmt.Errorf("update farm: %w", usecase.ErrVersionConflict)
	}
	farm.Version++
	r.storeFarm(farm)
	return nil
}

func (r *Registry) storeFarm(farm domain.Farm) {
	r.Links = slices.DeleteFunc(r.Links, func(link domain.Ownership) bool { return link.FarmID == farm.ID })
	r.Links = append(r.Links, farm.Owners...)
	farm.Owners = nil
	r.Farms[farm.ID] = farm
}

// ---- owners -------------------------------------------------------------------------

func (r *Registry) ownerOf(companyID domain.CompanyID, id domain.OwnerID) (domain.Owner, bool) {
	owner, ok := r.Owners[id]
	return owner, ok && owner.CompanyID == companyID
}

func (r *Registry) ownerListing(owner domain.Owner, visible domain.FarmScope) usecase.OwnerListing {
	links := []usecase.OwnerLink{}
	for _, link := range r.Links {
		if link.OwnerID != owner.ID || !visible.Allows(link.FarmID) {
			continue
		}
		links = append(links, usecase.OwnerLink{FarmID: link.FarmID, FarmName: r.Farms[link.FarmID].Name, Role: link.Role, Share: link.Share})
	}
	return usecase.OwnerListing{Owner: owner, Links: links}
}

// ListOwners implements usecase.OwnerReader (search on name or document digits).
func (r *Registry) ListOwners(_ context.Context, companyID domain.CompanyID, filter usecase.OwnerFilter, page usecase.PageRequest) (usecase.OwnerPage, error) {
	if err := r.check("ListOwners"); err != nil {
		return usecase.OwnerPage{}, err
	}
	var matches []usecase.OwnerListing
	for id := range r.Owners {
		owner, ok := r.ownerOf(companyID, id)
		if !ok || (filter.PersonType != "" && owner.PersonType != filter.PersonType) {
			continue
		}
		if !ownerMatches(owner, filter.Search) {
			continue
		}
		listing := r.ownerListing(owner, filter.Visible)
		if filter.Visible.IsSelected() && len(listing.Links) == 0 {
			continue
		}
		matches = append(matches, listing)
	}
	slices.SortFunc(matches, func(a, b usecase.OwnerListing) int {
		return cmp.Or(cmp.Compare(a.Owner.Name, b.Owner.Name), cmp.Compare(a.Owner.ID, b.Owner.ID))
	})
	return usecase.OwnerPage{Items: append([]usecase.OwnerListing{}, pageOf(matches, page)...), Total: len(matches), PageRequest: page}, nil
}

// ownerMatches searches the name, or the document digits with or without mask.
func ownerMatches(owner domain.Owner, search string) bool {
	if containsFold(search, owner.Name) {
		return true
	}
	searchDigits := digits(search)
	return searchDigits != "" && strings.Contains(string(owner.Document), searchDigits)
}

// FindOwner implements usecase.OwnerReader.
func (r *Registry) FindOwner(_ context.Context, companyID domain.CompanyID, id domain.OwnerID, visible domain.FarmScope) (usecase.OwnerListing, error) {
	owner, ok := r.ownerOf(companyID, id)
	if !ok {
		return usecase.OwnerListing{}, fmt.Errorf("find owner: %w", usecase.ErrNotFound)
	}
	listing := r.ownerListing(owner, visible)
	listing.Documents = r.activeDocuments(companyID, id)
	return listing, nil
}

// LockOwner implements usecase.OwnerStore.
func (r *Registry) LockOwner(_ context.Context, companyID domain.CompanyID, id domain.OwnerID) (domain.Owner, error) {
	owner, ok := r.ownerOf(companyID, id)
	if !ok {
		return domain.Owner{}, fmt.Errorf("lock owner: %w", usecase.ErrNotFound)
	}
	return owner, nil
}

// MissingOwners implements usecase.OwnerStore.
func (r *Registry) MissingOwners(_ context.Context, companyID domain.CompanyID, ids []domain.OwnerID) ([]domain.OwnerID, error) {
	var missing []domain.OwnerID
	for _, id := range ids {
		if _, ok := r.ownerOf(companyID, id); !ok {
			missing = append(missing, id)
		}
	}
	return missing, nil
}

// InsertOwner implements usecase.OwnerStore (unique document per company).
func (r *Registry) InsertOwner(_ context.Context, owner domain.Owner) error {
	if err := r.check("InsertOwner"); err != nil {
		return err
	}
	for _, other := range r.Owners {
		if other.CompanyID == owner.CompanyID && other.Document == owner.Document {
			return fmt.Errorf("insert owner: %w", usecase.ErrOwnerDocumentTaken)
		}
	}
	owner.Version = 1
	r.Owners[owner.ID] = owner
	return nil
}

// UpdateOwner implements usecase.OwnerStore with optimistic concurrency.
func (r *Registry) UpdateOwner(_ context.Context, owner domain.Owner) error {
	if err := r.check("UpdateOwner"); err != nil {
		return err
	}
	stored, ok := r.ownerOf(owner.CompanyID, owner.ID)
	if !ok {
		return fmt.Errorf("update owner: %w", usecase.ErrNotFound)
	}
	if stored.Version != owner.Version {
		return fmt.Errorf("update owner: %w", usecase.ErrVersionConflict)
	}
	owner.Version++
	r.Owners[owner.ID] = owner
	return nil
}

// ---- links --------------------------------------------------------------------------

// LockFarms implements usecase.OwnershipStore.
func (r *Registry) LockFarms(_ context.Context, companyID domain.CompanyID, ids []domain.FarmID) ([]domain.FarmID, error) {
	var missing []domain.FarmID
	for _, id := range ids {
		if _, ok := r.farmOf(companyID, id); !ok {
			missing = append(missing, id)
		}
	}
	return missing, nil
}

// ListFarmOwnerships implements usecase.OwnershipStore.
func (r *Registry) ListFarmOwnerships(_ context.Context, companyID domain.CompanyID, farmIDs []domain.FarmID) ([]domain.Ownership, error) {
	var links []domain.Ownership
	for _, link := range r.Links {
		if slices.Contains(farmIDs, link.FarmID) && r.Farms[link.FarmID].CompanyID == companyID {
			links = append(links, link)
		}
	}
	return links, nil
}

// ReplaceOwnerLinks implements usecase.OwnershipStore.
func (r *Registry) ReplaceOwnerLinks(_ context.Context, companyID domain.CompanyID, ownerID domain.OwnerID, links []domain.Ownership) error {
	if err := r.check("ReplaceOwnerLinks"); err != nil {
		return err
	}
	if _, ok := r.ownerOf(companyID, ownerID); !ok {
		return fmt.Errorf("replace owner links: %w", usecase.ErrNotFound)
	}
	r.Links = slices.DeleteFunc(r.Links, func(link domain.Ownership) bool { return link.OwnerID == ownerID })
	r.Links = append(r.Links, links...)
	return nil
}

// ---- perimeter ------------------------------------------------------------------------

// PerimeterTools is a parser and measurer with canned answers.
type PerimeterTools struct {
	Parsed  usecase.ParsedPerimeter
	Measure usecase.PerimeterMeasure
	Err     error
}

// ParsePerimeter implements usecase.PerimeterParser.
func (p *PerimeterTools) ParsePerimeter(usecase.PerimeterFile) (usecase.ParsedPerimeter, error) {
	return p.Parsed, p.Err
}

// MeasurePerimeter implements usecase.PerimeterMeasurer.
func (p *PerimeterTools) MeasurePerimeter(context.Context, domain.Perimeter) (usecase.PerimeterMeasure, error) {
	return p.Measure, nil
}

// MeasurePerimeter implements usecase.PerimeterMeasurer with a fixed answer
// (the geometry engine is the database's).
func (r *Registry) MeasurePerimeter(context.Context, domain.Perimeter) (usecase.PerimeterMeasure, error) {
	return usecase.PerimeterMeasure{Valid: true, AreaHa: 100}, nil
}

// ---- helpers --------------------------------------------------------------------------

func pageOf[T any](items []T, page usecase.PageRequest) []T {
	start := (page.Page - 1) * page.PageSize
	if start >= len(items) {
		return nil
	}
	return items[start:min(start+page.PageSize, len(items))]
}

// containsFold reports whether search is blank or a substring of a value,
// ignoring case and accents (like unaccent + ILIKE in the database).
func containsFold(search string, values ...string) bool {
	search = foldText(search)
	if search == "" {
		return true
	}
	for _, value := range values {
		if strings.Contains(foldText(value), search) {
			return true
		}
	}
	return false
}

var accents = strings.NewReplacer(
	"á", "a", "à", "a", "â", "a", "ã", "a", "ä", "a", "é", "e", "è", "e", "ê", "e", "ë", "e",
	"í", "i", "ì", "i", "î", "i", "ï", "i", "ó", "o", "ò", "o", "ô", "o", "õ", "o", "ö", "o",
	"ú", "u", "ù", "u", "û", "u", "ü", "u", "ç", "c", "ñ", "n",
)

func foldText(value string) string {
	return accents.Replace(strings.ToLower(strings.TrimSpace(value)))
}

func digits(value string) string {
	return strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' {
			return r
		}
		return -1
	}, value)
}
