package domain

// Typed identifiers keep a user id from being passed where a company id is expected.
type (
	// UserID identifies a user of the product (the API id, not the IdP subject).
	UserID string
	// CompanyID identifies a company (the tenant).
	CompanyID string
	// InvitationID identifies an invitation.
	InvitationID string
	// FarmID identifies a farm.
	FarmID string
	// OwnerID identifies an owner (person or company linked to farms).
	OwnerID string
	// FieldID identifies a field (talhão) of a farm.
	FieldID string
	// OwnerDocumentID identifies a document file of an owner.
	OwnerDocumentID string
	// RainGaugeID identifies a rain gauge of a farm.
	RainGaugeID string
	// RainReadingID identifies a reading of a rain gauge.
	RainReadingID string
	// SoilInspectionID identifies a soil inspection of a field.
	SoilInspectionID string
)

// Identity links a product user to an account of an external identity
// provider. Lookups always use (Issuer, Subject), never the e-mail (ADR 0015, D4).
type Identity struct {
	UserID  UserID
	Issuer  string
	Subject string
}
