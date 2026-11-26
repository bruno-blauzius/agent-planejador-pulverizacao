package usecase

import (
	"context"
	"time"

	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/domain"
)

// Ports of the registry (farms and owners). Every method receives the company
// of the caller: farms and owners always belong to the caller's own company,
// whatever the role (FarmInput carries no company). Soft deleted rows are never
// returned. A visibility scope (domain.FarmScope) narrows reads to the farms
// a planning operator may see.

// ---- farms -------------------------------------------------------------------------

// FarmFilter narrows the farm listing.
type FarmFilter struct {
	Search         string
	Status         domain.FarmStatus
	ProductionType domain.ProductionType
	// Visible limits the listing to the farms of a selected scope.
	Visible domain.FarmScope
}

// FarmListing is a farm with what the store computes from it.
type FarmListing struct {
	Farm domain.Farm
	// OwnerNames has the name of every owner in Farm.Owners.
	OwnerNames map[domain.OwnerID]string
	// AreaHa is the area of the perimeter in hectares (0 without perimeter).
	AreaHa float64
	// Outline is a simplified perimeter for thumbnails and the map mode (nil without perimeter).
	Outline *domain.Perimeter
	// FieldCount, ProductiveAreaHa and Crops summarize the fields of the farm.
	FieldCount       int
	ProductiveAreaHa float64
	Crops            []domain.Crop
	// OwnerDocumentsPending is true when a linked owner lacks a required
	// document (domain.DisplayStatus shows an active farm as pending).
	OwnerDocumentsPending bool
}

// FarmPage is one page of farms.
type FarmPage struct {
	Items []FarmListing
	Total int
	PageRequest
}

// FarmReader reads farms of one company.
type FarmReader interface {
	ListFarms(ctx context.Context, companyID domain.CompanyID, filter FarmFilter, page PageRequest) (FarmPage, error)
	// FindFarm returns ErrNotFound when the farm is not in the company.
	FindFarm(ctx context.Context, companyID domain.CompanyID, id domain.FarmID) (FarmListing, error)
}

// FarmStore is the transactional side of farms.
type FarmStore interface {
	// LockFarm reads the farm with its owners and holds a row lock until the
	// transaction ends (ErrNotFound when the farm is not in the company).
	LockFarm(ctx context.Context, companyID domain.CompanyID, id domain.FarmID) (domain.Farm, error)
	// InsertFarm stores the farm and its owner links. A forecast point left
	// empty is filled with the centroid of the perimeter.
	InsertFarm(ctx context.Context, farm domain.Farm) error
	// UpdateFarm replaces the farm and its owner links with optimistic
	// concurrency (ErrVersionConflict).
	UpdateFarm(ctx context.Context, farm domain.Farm) error
	// FarmCoverage measures the fields of the farm against a perimeter (nil:
	// no perimeter, only the count and area of the fields).
	FarmCoverage(ctx context.Context, companyID domain.CompanyID, farmID domain.FarmID, perimeter *domain.Perimeter) (domain.FarmCoverage, error)
}

// ---- crops and fields --------------------------------------------------------------

// CropReader reads the global crop catalog.
type CropReader interface {
	// ListCrops returns the active crops, by name.
	ListCrops(ctx context.Context) ([]domain.Crop, error)
	// FindCrop returns ErrNotFound for an unknown or inactive crop.
	FindCrop(ctx context.Context, code string) (domain.Crop, error)
}

// FieldListing is a field with the area computed by the geometry engine.
type FieldListing struct {
	Field  domain.Field
	AreaHa float64
}

// FieldReader reads the fields of a farm of one company.
type FieldReader interface {
	// ListFields returns the fields of the farm, by block and name.
	ListFields(ctx context.Context, companyID domain.CompanyID, farmID domain.FarmID) ([]FieldListing, error)
	// FindField returns ErrNotFound when the field is not in the farm.
	FindField(ctx context.Context, companyID domain.CompanyID, farmID domain.FarmID, id domain.FieldID) (FieldListing, error)
}

// FieldStore is the transactional side of fields. Callers lock the farm first.
type FieldStore interface {
	// LockField reads the field of the farm and holds a row lock (ErrNotFound when absent).
	LockField(ctx context.Context, companyID domain.CompanyID, farmID domain.FarmID, id domain.FieldID) (domain.Field, error)
	// FieldFootprint measures a polygon inside the farm: area, part outside the
	// perimeter and overlaps with the other fields (exclude is the field being edited).
	FieldFootprint(ctx context.Context, companyID domain.CompanyID, farmID domain.FarmID, exclude domain.FieldID, polygon domain.Polygon) (domain.FieldFootprint, error)
	// InsertField returns a validation error on "name" when the farm already has the name.
	InsertField(ctx context.Context, field domain.Field) error
	// UpdateField uses optimistic concurrency (ErrVersionConflict).
	UpdateField(ctx context.Context, field domain.Field) error
}

// ---- owners ------------------------------------------------------------------------

// OwnerFilter narrows the owner listing.
type OwnerFilter struct {
	Search     string
	PersonType domain.PersonType
	// Visible limits the listing to owners linked to the farms of a selected
	// scope, and their links to those farms.
	Visible domain.FarmScope
}

// OwnerLink is a link seen from the owner, with the farm name.
type OwnerLink struct {
	FarmID   domain.FarmID
	FarmName string
	Role     domain.LinkRole
	Share    *domain.SharePercent
}

// OwnerListing is an owner with its links. Documents is filled on the
// detail (FindOwner) only; listings leave it empty.
type OwnerListing struct {
	Owner     domain.Owner
	Links     []OwnerLink
	Documents []domain.OwnerDocument
}

// OwnerPage is one page of owners.
type OwnerPage struct {
	Items []OwnerListing
	Total int
	PageRequest
}

// OwnerReader reads owners of one company.
type OwnerReader interface {
	ListOwners(ctx context.Context, companyID domain.CompanyID, filter OwnerFilter, page PageRequest) (OwnerPage, error)
	// FindOwner returns ErrNotFound when the owner is not in the company. Links
	// outside the visible scope are left out.
	FindOwner(ctx context.Context, companyID domain.CompanyID, id domain.OwnerID, visible domain.FarmScope) (OwnerListing, error)
}

// OwnerStore is the transactional side of owners.
type OwnerStore interface {
	// LockOwner reads the owner and holds a row lock until the transaction ends.
	LockOwner(ctx context.Context, companyID domain.CompanyID, id domain.OwnerID) (domain.Owner, error)
	// MissingOwners returns the ids that are not owners of the company.
	MissingOwners(ctx context.Context, companyID domain.CompanyID, ids []domain.OwnerID) ([]domain.OwnerID, error)
	// InsertOwner returns ErrOwnerDocumentTaken for a CPF/CNPJ already in the company.
	InsertOwner(ctx context.Context, owner domain.Owner) error
	// UpdateOwner uses optimistic concurrency (ErrVersionConflict).
	UpdateOwner(ctx context.Context, owner domain.Owner) error
}

// OwnershipStore keeps the links between farms and owners.
type OwnershipStore interface {
	// LockFarms locks the farms of the company (in id order, to avoid
	// deadlocks) and returns the ids that do not exist in it.
	LockFarms(ctx context.Context, companyID domain.CompanyID, ids []domain.FarmID) (missing []domain.FarmID, err error)
	// ListFarmOwnerships returns every link of the given farms.
	ListFarmOwnerships(ctx context.Context, companyID domain.CompanyID, farmIDs []domain.FarmID) ([]domain.Ownership, error)
	// ReplaceOwnerLinks replaces every link of the owner.
	ReplaceOwnerLinks(ctx context.Context, companyID domain.CompanyID, ownerID domain.OwnerID, links []domain.Ownership) error
}

// ---- owner documents ---------------------------------------------------------------

// DocumentStorage keeps the bytes of owner documents (object storage, ADR 0020).
// Failures of the storage are ErrDocumentStorageUnavailable.
type DocumentStorage interface {
	PutDocument(ctx context.Context, key, contentType string, content []byte) error
	// GetDocument returns ErrNotFound when the object does not exist.
	GetDocument(ctx context.Context, key string) ([]byte, error)
	// DeleteDocument removes an object; it undoes an upload whose metadata could not be stored.
	DeleteDocument(ctx context.Context, key string) error
}

// OwnerDocumentReader reads the document metadata of an owner.
type OwnerDocumentReader interface {
	ListOwnerDocuments(ctx context.Context, companyID domain.CompanyID, ownerID domain.OwnerID) ([]domain.OwnerDocument, error)
	// FindOwnerDocument returns ErrNotFound when the document is not the owner's or was removed.
	FindOwnerDocument(ctx context.Context, companyID domain.CompanyID, ownerID domain.OwnerID, id domain.OwnerDocumentID) (domain.OwnerDocument, error)
}

// OwnerDocumentStore is the transactional side of owner documents.
type OwnerDocumentStore interface {
	OwnerDocumentReader
	InsertOwnerDocument(ctx context.Context, document domain.OwnerDocument) error
	// RemoveOwnerDocument soft deletes the document (ErrNotFound when absent).
	RemoveOwnerDocument(ctx context.Context, companyID domain.CompanyID, ownerID domain.OwnerID, id domain.OwnerDocumentID, now time.Time) error
	// SetDocumentsPending stores the result of domain.RequiredDocumentsMissing.
	SetDocumentsPending(ctx context.Context, companyID domain.CompanyID, ownerID domain.OwnerID, pending bool) error
}

// ---- transactions ------------------------------------------------------------------

// RegistryStores are the transaction-bound stores of the registry.
type RegistryStores struct {
	Farms      FarmStore
	Owners     OwnerStore
	Ownerships OwnershipStore
	Fields     FieldStore
	Documents  OwnerDocumentStore
	// Terrain invalidates the relief of a field whose polygon changed, in the
	// same transaction as the edit (ADR 0021, decision 6).
	Terrain TerrainStaleMarker
	Audit   AuditRecorder
}

// RegistryTxRunner runs fn in one database transaction: commit when fn
// returns nil, roll back otherwise.
type RegistryTxRunner interface {
	InRegistryTx(ctx context.Context, fn func(ctx context.Context, tx RegistryStores) error) error
}

// ---- perimeter import --------------------------------------------------------------

// PerimeterFormat is the kind of file a perimeter came from.
type PerimeterFormat string

// Perimeter file formats of the contract.
const (
	PerimeterKML       PerimeterFormat = "kml"
	PerimeterKMZ       PerimeterFormat = "kmz"
	PerimeterGeoJSON   PerimeterFormat = "geojson"
	PerimeterShapefile PerimeterFormat = "shapefile"
)

// PerimeterFile is an uploaded file.
type PerimeterFile struct {
	Name    string
	Content []byte
}

// ParsedPerimeter is what a parser extracts from a file.
type ParsedPerimeter struct {
	Format    PerimeterFormat
	Perimeter domain.Perimeter
	// Datum is the reference system declared by the file, when it says.
	Datum string
	// InnerPolygons counts the polygons found besides the perimeter (possible fields).
	InnerPolygons int
}

// PerimeterParser reads a perimeter from a file. It returns
// ErrUnsupportedPerimeterFormat for a format it cannot read and a
// *domain.ValidationError on "file" for unreadable content.
type PerimeterParser interface {
	ParsePerimeter(file PerimeterFile) (ParsedPerimeter, error)
}

// PerimeterMeasure is what the geometry engine computes from a perimeter.
type PerimeterMeasure struct {
	Valid    bool
	AreaHa   float64
	Centroid domain.GeoPoint
	// InteriorPoint is a point surely inside the perimeter (the centroid of a
	// concave shape may fall outside it).
	InteriorPoint domain.GeoPoint
}

// PerimeterMeasurer computes area and centroid (the database owns the geometry engine).
type PerimeterMeasurer interface {
	MeasurePerimeter(ctx context.Context, perimeter domain.Perimeter) (PerimeterMeasure, error)
}

// MunicipalityLocator finds the municipality that contains a point.
type MunicipalityLocator interface {
	// LocateMunicipality returns nil when no loaded municipality contains the
	// point (or the IBGE registry was never loaded).
	LocateMunicipality(ctx context.Context, point domain.GeoPoint) (*domain.Municipality, error)
}
