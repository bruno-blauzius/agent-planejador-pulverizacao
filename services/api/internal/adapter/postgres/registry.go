package postgres

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/adapter/postgres/sqlcgen"
	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/domain"
	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/usecase"
)

// Registry storage: farms, owners and the links between them (ADR 0013).
// Every query filters by the caller's company; a selected farm scope becomes
// the visible_farm_ids filter.

// visibleFarmIDs is nil (no filter) for a scope that covers every farm.
func visibleFarmIDs(scope domain.FarmScope) []string {
	if !scope.IsSelected() {
		return nil
	}
	return farmIDStrings(scope)
}

// validIDs keeps the ids that can exist (UUIDs); the others are reported as missing.
func validIDs[T ~string](ids []T) (valid []string, invalid []T) {
	for _, id := range ids {
		if validUUID(string(id)) {
			valid = append(valid, string(id))
		} else {
			invalid = append(invalid, id)
		}
	}
	return valid, invalid
}

func textPtr(value pgtype.Text) *string {
	if !value.Valid {
		return nil
	}
	return &value.String
}

func ptrText(value *string) pgtype.Text {
	if value == nil {
		return pgtype.Text{}
	}
	return pgtype.Text{String: *value, Valid: true}
}

// ---- farms ---------------------------------------------------------------------------

type farmColumns struct {
	ID, CompanyID, Name                 string
	Code, Cooperative                   pgtype.Text
	ProductionType, City, State, Status string
	PostalCode, CarNumber               pgtype.Text
	ForecastPoint, Perimeter            string
	CreatedAt, UpdatedAt                time.Time
	Version                             int32
}

func farmFromColumns(c farmColumns) (domain.Farm, error) {
	point, err := decodePoint(c.ForecastPoint)
	if err != nil {
		return domain.Farm{}, err
	}
	perimeter, err := decodePerimeter(c.Perimeter)
	if err != nil {
		return domain.Farm{}, err
	}
	return domain.Farm{
		ID: domain.FarmID(c.ID), CompanyID: domain.CompanyID(c.CompanyID), Name: c.Name,
		Code: textPtr(c.Code), Cooperative: textPtr(c.Cooperative),
		ProductionType: domain.ProductionType(c.ProductionType), City: c.City, State: c.State,
		Status: domain.FarmStatus(c.Status), PostalCode: textPtr(c.PostalCode), CARNumber: textPtr(c.CarNumber),
		ForecastPoint: point, Perimeter: perimeter,
		CreatedAt: c.CreatedAt, UpdatedAt: c.UpdatedAt, Version: int(c.Version),
	}, nil
}

func listingOf(farm domain.Farm, outline string, areaHa float64) (usecase.FarmListing, error) {
	simplified, err := decodePerimeter(outline)
	if err != nil {
		return usecase.FarmListing{}, err
	}
	return usecase.FarmListing{Farm: farm, OwnerNames: map[domain.OwnerID]string{}, AreaHa: areaHa, Outline: simplified}, nil
}

// ListFarms implements usecase.FarmReader. Search matches name, code or city.
func (r *Repository) ListFarms(ctx context.Context, companyID domain.CompanyID, filter usecase.FarmFilter, page usecase.PageRequest) (usecase.FarmPage, error) {
	result := usecase.FarmPage{Items: []usecase.FarmListing{}, PageRequest: page}
	if !validUUID(string(companyID)) {
		return result, nil
	}
	limit, offset := pageBounds(page)
	status, production := optionalText(string(filter.Status)), optionalText(string(filter.ProductionType))
	search, visible := likePattern(filter.Search), visibleFarmIDs(filter.Visible)

	total, err := r.q.CountFarms(ctx, sqlcgen.CountFarmsParams{
		CompanyID: string(companyID), Status: status, ProductionType: production, VisibleFarmIds: visible, Search: search,
	})
	if err != nil {
		return usecase.FarmPage{}, fmt.Errorf("count farms: %w", err)
	}
	result.Total = int(total)
	if total == 0 || int64(offset) >= int64(total) {
		return result, nil
	}
	rows, err := r.q.ListFarms(ctx, sqlcgen.ListFarmsParams{
		CompanyID: string(companyID), Status: status, ProductionType: production, VisibleFarmIds: visible,
		Search: search, PageLimit: limit, PageOffset: offset,
	})
	if err != nil {
		return usecase.FarmPage{}, fmt.Errorf("list farms: %w", err)
	}
	for _, row := range rows {
		farm, err := farmFromColumns(farmColumns{
			ID: row.ID, CompanyID: row.CompanyID, Name: row.Name, Code: row.Code, Cooperative: row.Cooperative,
			ProductionType: row.ProductionType, City: row.City, State: row.State, Status: row.Status,
			PostalCode: row.PostalCode, CarNumber: row.CarNumber, ForecastPoint: row.ForecastPoint,
			CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt, Version: row.Version,
		})
		if err != nil {
			return usecase.FarmPage{}, err
		}
		listing, err := listingOf(farm, row.Outline, row.AreaHa)
		if err != nil {
			return usecase.FarmPage{}, err
		}
		listing.OwnerDocumentsPending = row.DocumentsPending
		result.Items = append(result.Items, listing)
	}
	if err := r.attachFarmOwners(ctx, companyID, result.Items); err != nil {
		return usecase.FarmPage{}, err
	}
	if err := r.attachFieldSummaries(ctx, companyID, result.Items); err != nil {
		return usecase.FarmPage{}, err
	}
	return result, nil
}

// FindFarm implements usecase.FarmReader.
func (r *Repository) FindFarm(ctx context.Context, companyID domain.CompanyID, id domain.FarmID) (usecase.FarmListing, error) {
	row, err := r.q.GetFarm(ctx, sqlcgen.GetFarmParams{CompanyID: string(companyID), ID: string(id)})
	if err != nil {
		return usecase.FarmListing{}, translateRead("find farm", err)
	}
	farm, err := farmFromColumns(farmColumns{
		ID: row.ID, CompanyID: row.CompanyID, Name: row.Name, Code: row.Code, Cooperative: row.Cooperative,
		ProductionType: row.ProductionType, City: row.City, State: row.State, Status: row.Status,
		PostalCode: row.PostalCode, CarNumber: row.CarNumber, ForecastPoint: row.ForecastPoint, Perimeter: row.Perimeter,
		CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt, Version: row.Version,
	})
	if err != nil {
		return usecase.FarmListing{}, err
	}
	listing, err := listingOf(farm, row.Outline, row.AreaHa)
	if err != nil {
		return usecase.FarmListing{}, err
	}
	listing.OwnerDocumentsPending = row.DocumentsPending
	items := []usecase.FarmListing{listing}
	if err := r.attachFarmOwners(ctx, companyID, items); err != nil {
		return usecase.FarmListing{}, err
	}
	if err := r.attachFieldSummaries(ctx, companyID, items); err != nil {
		return usecase.FarmListing{}, err
	}
	return items[0], nil
}

// attachFarmOwners loads the links of every listed farm in one query.
func (r *Repository) attachFarmOwners(ctx context.Context, companyID domain.CompanyID, items []usecase.FarmListing) error {
	ids := make([]string, 0, len(items))
	for _, item := range items {
		ids = append(ids, string(item.Farm.ID))
	}
	rows, err := r.q.ListLinksByFarms(ctx, sqlcgen.ListLinksByFarmsParams{CompanyID: string(companyID), FarmIds: ids})
	if err != nil {
		return fmt.Errorf("list farm owners: %w", err)
	}
	byID := make(map[string]*usecase.FarmListing, len(items))
	for i := range items {
		byID[string(items[i].Farm.ID)] = &items[i]
	}
	for _, row := range rows {
		item, ok := byID[row.FarmID]
		if !ok {
			continue
		}
		item.Farm.Owners = append(item.Farm.Owners, ownershipOf(row.FarmID, row.OwnerID, row.Role, row.ShareHundredths))
		item.OwnerNames[domain.OwnerID(row.OwnerID)] = row.OwnerName
	}
	return nil
}

func ownershipOf(farmID, ownerID, role string, hundredths int32) domain.Ownership {
	link := domain.Ownership{FarmID: domain.FarmID(farmID), OwnerID: domain.OwnerID(ownerID), Role: domain.LinkRole(role)}
	if hundredths > 0 {
		share := domain.SharePercent(hundredths)
		link.Share = &share
	}
	return link
}

// LockFarm implements usecase.FarmStore.
func (r *Repository) LockFarm(ctx context.Context, companyID domain.CompanyID, id domain.FarmID) (domain.Farm, error) {
	row, err := r.q.LockFarm(ctx, sqlcgen.LockFarmParams{CompanyID: string(companyID), ID: string(id)})
	if err != nil {
		return domain.Farm{}, translateRead("lock farm", err)
	}
	farm, err := farmFromColumns(farmColumns{
		ID: row.ID, CompanyID: row.CompanyID, Name: row.Name, Code: row.Code, Cooperative: row.Cooperative,
		ProductionType: row.ProductionType, City: row.City, State: row.State, Status: row.Status,
		PostalCode: row.PostalCode, CarNumber: row.CarNumber, ForecastPoint: row.ForecastPoint, Perimeter: row.Perimeter,
		CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt, Version: row.Version,
	})
	if err != nil {
		return domain.Farm{}, err
	}
	links, err := r.ListFarmOwnerships(ctx, companyID, []domain.FarmID{id})
	if err != nil {
		return domain.Farm{}, err
	}
	farm.Owners = links
	return farm, nil
}

type farmGeometry struct {
	point, perimeter pgtype.Text
}

func geometryOf(farm domain.Farm) (farmGeometry, error) {
	point, err := pointGeoJSON(farm.ForecastPoint)
	if err != nil {
		return farmGeometry{}, err
	}
	perimeter, err := perimeterGeoJSON(farm.Perimeter)
	if err != nil {
		return farmGeometry{}, err
	}
	return farmGeometry{point: point, perimeter: perimeter}, nil
}

// InsertFarm implements usecase.FarmStore: the farm and its links, atomically.
func (r *Repository) InsertFarm(ctx context.Context, farm domain.Farm) error {
	geometry, err := geometryOf(farm)
	if err != nil {
		return err
	}
	err = r.atomic(ctx, func(q *sqlcgen.Queries) error {
		err := q.InsertFarm(ctx, sqlcgen.InsertFarmParams{
			ID: string(farm.ID), CompanyID: string(farm.CompanyID), Name: farm.Name,
			Code: ptrText(farm.Code), Cooperative: ptrText(farm.Cooperative),
			ProductionType: string(farm.ProductionType), City: farm.City, State: farm.State, Status: string(farm.Status),
			PostalCode: ptrText(farm.PostalCode), CarNumber: ptrText(farm.CARNumber),
			ForecastPoint: geometry.point, Perimeter: geometry.perimeter,
			CreatedAt: farm.CreatedAt, UpdatedAt: farm.UpdatedAt,
		})
		if err != nil {
			return translateWrite("insert farm", err)
		}
		return insertLinks(ctx, q, farm.CompanyID, farm.Owners)
	})
	if err != nil {
		return fmt.Errorf("insert farm: %w", err)
	}
	return nil
}

// UpdateFarm implements usecase.FarmStore with optimistic concurrency; the
// links of the farm are replaced in the same transaction.
func (r *Repository) UpdateFarm(ctx context.Context, farm domain.Farm) error {
	geometry, err := geometryOf(farm)
	if err != nil {
		return err
	}
	return r.atomic(ctx, func(q *sqlcgen.Queries) error {
		updated, err := q.UpdateFarm(ctx, sqlcgen.UpdateFarmParams{
			CompanyID: string(farm.CompanyID), ID: string(farm.ID), Version: normalizeVersion(farm.Version),
			Name: farm.Name, Code: ptrText(farm.Code), Cooperative: ptrText(farm.Cooperative),
			ProductionType: string(farm.ProductionType), City: farm.City, State: farm.State, Status: string(farm.Status),
			PostalCode: ptrText(farm.PostalCode), CarNumber: ptrText(farm.CARNumber),
			ForecastPoint: geometry.point, Perimeter: geometry.perimeter, UpdatedAt: farm.UpdatedAt,
		})
		if err != nil {
			return translateWrite("update farm", err)
		}
		if updated == 0 {
			return missingOrStale(ctx, q.FarmExists, sqlcgen.FarmExistsParams{CompanyID: string(farm.CompanyID), ID: string(farm.ID)}, "update farm")
		}
		if err := q.DeleteFarmLinks(ctx, sqlcgen.DeleteFarmLinksParams{CompanyID: string(farm.CompanyID), FarmID: string(farm.ID)}); err != nil {
			return translateWrite("delete farm links", err)
		}
		return insertLinks(ctx, q, farm.CompanyID, farm.Owners)
	})
}

// missingOrStale explains an UPDATE that touched no row: gone, or another version.
func missingOrStale[P any](ctx context.Context, exists func(context.Context, P) (bool, error), params P, operation string) error {
	found, err := exists(ctx, params)
	if err != nil {
		return translateRead(operation, err)
	}
	if !found {
		return fmt.Errorf("%s: %w", operation, usecase.ErrNotFound)
	}
	return fmt.Errorf("%s: %w", operation, usecase.ErrVersionConflict)
}

func insertLinks(ctx context.Context, q *sqlcgen.Queries, companyID domain.CompanyID, links []domain.Ownership) error {
	for _, link := range links {
		var hundredths int32
		if link.Share != nil {
			hundredths = clampInt32(int64(*link.Share))
		}
		err := q.InsertLink(ctx, sqlcgen.InsertLinkParams{
			CompanyID: string(companyID), FarmID: string(link.FarmID), OwnerID: string(link.OwnerID),
			Role: string(link.Role), ShareHundredths: hundredths,
		})
		if err != nil {
			return translateWrite("insert link", err)
		}
	}
	return nil
}

// MeasurePerimeter implements usecase.PerimeterMeasurer with PostGIS.
func (r *Repository) MeasurePerimeter(ctx context.Context, perimeter domain.Perimeter) (usecase.PerimeterMeasure, error) {
	text, err := encodePerimeter(perimeter)
	if err != nil {
		return usecase.PerimeterMeasure{}, err
	}
	row, err := r.q.MeasurePerimeter(ctx, text)
	if err != nil {
		return usecase.PerimeterMeasure{}, fmt.Errorf("measure perimeter: %w", err)
	}
	return usecase.PerimeterMeasure{
		Valid: row.Valid, AreaHa: row.AreaHa,
		Centroid:      domain.GeoPoint{Lat: row.CentroidLat, Lng: row.CentroidLng},
		InteriorPoint: domain.GeoPoint{Lat: row.InteriorLat, Lng: row.InteriorLng},
	}, nil
}

// ---- owners --------------------------------------------------------------------------

func ownerFromRow(row sqlcgen.Owner) domain.Owner {
	owner := domain.Owner{
		ID: domain.OwnerID(row.ID), CompanyID: domain.CompanyID(row.CompanyID),
		PersonType: domain.PersonType(row.PersonType), Name: row.Name, Document: domain.TaxID(row.Document),
		WindowAlertsEnabled: row.WindowAlertsEnabled, DocumentsPending: row.DocumentsPending,
		CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt, Version: int(row.Version),
	}
	if row.BirthDate.Valid {
		date := row.BirthDate.Time
		owner.BirthDate = &date
	}
	if row.Phone.Valid {
		phone := domain.Phone(row.Phone.String)
		owner.Phone = &phone
	}
	if row.Email.Valid {
		email := domain.Email(row.Email.String)
		owner.Email = &email
	}
	return owner
}

type ownerColumns struct {
	birthDate    pgtype.Date
	phone, email pgtype.Text
}

func columnsOf(owner domain.Owner) ownerColumns {
	var c ownerColumns
	if owner.BirthDate != nil {
		c.birthDate = pgtype.Date{Time: *owner.BirthDate, Valid: true}
	}
	if owner.Phone != nil {
		c.phone = pgtype.Text{String: string(*owner.Phone), Valid: true}
	}
	if owner.Email != nil {
		c.email = pgtype.Text{String: owner.Email.String(), Valid: true}
	}
	return c
}

// documentPattern matches the digits typed in the search box anywhere in the
// document; text without digits does not search documents.
func documentPattern(search string) pgtype.Text {
	var digits []rune
	for _, r := range search {
		if r >= '0' && r <= '9' {
			digits = append(digits, r)
		}
	}
	if len(digits) == 0 {
		return pgtype.Text{}
	}
	return pgtype.Text{String: "%" + string(digits) + "%", Valid: true}
}

// ListOwners implements usecase.OwnerReader.
func (r *Repository) ListOwners(ctx context.Context, companyID domain.CompanyID, filter usecase.OwnerFilter, page usecase.PageRequest) (usecase.OwnerPage, error) {
	result := usecase.OwnerPage{Items: []usecase.OwnerListing{}, PageRequest: page}
	if !validUUID(string(companyID)) {
		return result, nil
	}
	limit, offset := pageBounds(page)
	personType, search := optionalText(string(filter.PersonType)), likePattern(filter.Search)
	documentSearch, visible := documentPattern(filter.Search), visibleFarmIDs(filter.Visible)

	total, err := r.q.CountOwners(ctx, sqlcgen.CountOwnersParams{
		CompanyID: string(companyID), PersonType: personType, Search: search, DocumentSearch: documentSearch, VisibleFarmIds: visible,
	})
	if err != nil {
		return usecase.OwnerPage{}, fmt.Errorf("count owners: %w", err)
	}
	result.Total = int(total)
	if total == 0 || int64(offset) >= int64(total) {
		return result, nil
	}
	rows, err := r.q.ListOwners(ctx, sqlcgen.ListOwnersParams{
		CompanyID: string(companyID), PersonType: personType, Search: search, DocumentSearch: documentSearch,
		VisibleFarmIds: visible, PageLimit: limit, PageOffset: offset,
	})
	if err != nil {
		return usecase.OwnerPage{}, fmt.Errorf("list owners: %w", err)
	}
	for _, row := range rows {
		result.Items = append(result.Items, usecase.OwnerListing{Owner: ownerFromRow(row), Links: []usecase.OwnerLink{}})
	}
	if err := r.attachOwnerLinks(ctx, companyID, filter.Visible, result.Items); err != nil {
		return usecase.OwnerPage{}, err
	}
	return result, nil
}

// FindOwner implements usecase.OwnerReader.
func (r *Repository) FindOwner(ctx context.Context, companyID domain.CompanyID, id domain.OwnerID, visible domain.FarmScope) (usecase.OwnerListing, error) {
	row, err := r.q.GetOwner(ctx, sqlcgen.GetOwnerParams{CompanyID: string(companyID), ID: string(id)})
	if err != nil {
		return usecase.OwnerListing{}, translateRead("find owner", err)
	}
	items := []usecase.OwnerListing{{Owner: ownerFromRow(row), Links: []usecase.OwnerLink{}}}
	if err := r.attachOwnerLinks(ctx, companyID, visible, items); err != nil {
		return usecase.OwnerListing{}, err
	}
	documents, err := r.ListOwnerDocuments(ctx, companyID, id)
	if err != nil {
		return usecase.OwnerListing{}, err
	}
	items[0].Documents = documents
	return items[0], nil
}

func (r *Repository) attachOwnerLinks(ctx context.Context, companyID domain.CompanyID, visible domain.FarmScope, items []usecase.OwnerListing) error {
	ids := make([]string, 0, len(items))
	for _, item := range items {
		ids = append(ids, string(item.Owner.ID))
	}
	rows, err := r.q.ListLinksByOwners(ctx, sqlcgen.ListLinksByOwnersParams{
		CompanyID: string(companyID), OwnerIds: ids, VisibleFarmIds: visibleFarmIDs(visible),
	})
	if err != nil {
		return fmt.Errorf("list owner links: %w", err)
	}
	byID := make(map[string]*usecase.OwnerListing, len(items))
	for i := range items {
		byID[string(items[i].Owner.ID)] = &items[i]
	}
	for _, row := range rows {
		item, ok := byID[row.OwnerID]
		if !ok {
			continue
		}
		link := ownershipOf(row.FarmID, row.OwnerID, row.Role, row.ShareHundredths)
		item.Links = append(item.Links, usecase.OwnerLink{
			FarmID: link.FarmID, FarmName: row.FarmName, Role: link.Role, Share: link.Share,
		})
	}
	return nil
}

// LockOwner implements usecase.OwnerStore.
func (r *Repository) LockOwner(ctx context.Context, companyID domain.CompanyID, id domain.OwnerID) (domain.Owner, error) {
	row, err := r.q.LockOwner(ctx, sqlcgen.LockOwnerParams{CompanyID: string(companyID), ID: string(id)})
	if err != nil {
		return domain.Owner{}, translateRead("lock owner", err)
	}
	return ownerFromRow(row), nil
}

// MissingOwners implements usecase.OwnerStore.
func (r *Repository) MissingOwners(ctx context.Context, companyID domain.CompanyID, ids []domain.OwnerID) ([]domain.OwnerID, error) {
	valid, missing := validIDs(ids)
	if len(valid) == 0 {
		return missing, nil
	}
	found, err := r.q.ListExistingOwnerIDs(ctx, sqlcgen.ListExistingOwnerIDsParams{CompanyID: string(companyID), OwnerIds: valid})
	if err != nil {
		return nil, fmt.Errorf("list existing owners: %w", err)
	}
	for _, id := range valid {
		if !slices.Contains(found, id) {
			missing = append(missing, domain.OwnerID(id))
		}
	}
	return missing, nil
}

// InsertOwner implements usecase.OwnerStore.
func (r *Repository) InsertOwner(ctx context.Context, owner domain.Owner) error {
	columns := columnsOf(owner)
	err := r.atomic(ctx, func(q *sqlcgen.Queries) error {
		return translateWrite("insert owner", q.InsertOwner(ctx, sqlcgen.InsertOwnerParams{
			ID: string(owner.ID), CompanyID: string(owner.CompanyID), PersonType: string(owner.PersonType),
			Name: owner.Name, Document: string(owner.Document), BirthDate: columns.birthDate,
			Phone: columns.phone, Email: columns.email, WindowAlertsEnabled: owner.WindowAlertsEnabled,
			CreatedAt: owner.CreatedAt, UpdatedAt: owner.UpdatedAt,
		}))
	})
	if err != nil {
		return fmt.Errorf("insert owner: %w", err)
	}
	return nil
}

// UpdateOwner implements usecase.OwnerStore with optimistic concurrency.
func (r *Repository) UpdateOwner(ctx context.Context, owner domain.Owner) error {
	columns := columnsOf(owner)
	updated, err := r.q.UpdateOwner(ctx, sqlcgen.UpdateOwnerParams{
		CompanyID: string(owner.CompanyID), ID: string(owner.ID), Version: normalizeVersion(owner.Version),
		Name: owner.Name, BirthDate: columns.birthDate, Phone: columns.phone, Email: columns.email,
		WindowAlertsEnabled: owner.WindowAlertsEnabled, UpdatedAt: owner.UpdatedAt,
	})
	if err != nil {
		return translateWrite("update owner", err)
	}
	if updated > 0 {
		return nil
	}
	return missingOrStale(ctx, r.q.OwnerExists, sqlcgen.OwnerExistsParams{CompanyID: string(owner.CompanyID), ID: string(owner.ID)}, "update owner")
}

// ---- links ---------------------------------------------------------------------------

// LockFarms implements usecase.OwnershipStore.
func (r *Repository) LockFarms(ctx context.Context, companyID domain.CompanyID, ids []domain.FarmID) ([]domain.FarmID, error) {
	valid, missing := validIDs(ids)
	if len(valid) == 0 {
		return missing, nil
	}
	locked, err := r.q.LockFarmsByIDs(ctx, sqlcgen.LockFarmsByIDsParams{CompanyID: string(companyID), FarmIds: valid})
	if err != nil {
		return nil, fmt.Errorf("lock farms: %w", err)
	}
	for _, id := range valid {
		if !slices.Contains(locked, id) {
			missing = append(missing, domain.FarmID(id))
		}
	}
	return missing, nil
}

// ListFarmOwnerships implements usecase.OwnershipStore.
func (r *Repository) ListFarmOwnerships(ctx context.Context, companyID domain.CompanyID, farmIDs []domain.FarmID) ([]domain.Ownership, error) {
	valid, _ := validIDs(farmIDs)
	if len(valid) == 0 {
		return nil, nil
	}
	rows, err := r.q.ListLinksByFarms(ctx, sqlcgen.ListLinksByFarmsParams{CompanyID: string(companyID), FarmIds: valid})
	if err != nil {
		return nil, fmt.Errorf("list farm ownerships: %w", err)
	}
	links := make([]domain.Ownership, 0, len(rows))
	for _, row := range rows {
		links = append(links, ownershipOf(row.FarmID, row.OwnerID, row.Role, row.ShareHundredths))
	}
	return links, nil
}

// ReplaceOwnerLinks implements usecase.OwnershipStore.
func (r *Repository) ReplaceOwnerLinks(ctx context.Context, companyID domain.CompanyID, ownerID domain.OwnerID, links []domain.Ownership) error {
	return r.atomic(ctx, func(q *sqlcgen.Queries) error {
		if err := q.DeleteOwnerLinks(ctx, sqlcgen.DeleteOwnerLinksParams{CompanyID: string(companyID), OwnerID: string(ownerID)}); err != nil {
			return translateWrite("delete owner links", err)
		}
		return insertLinks(ctx, q, companyID, links)
	})
}
