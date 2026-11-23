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
)

// Identity links a product user to an account of an external identity
// provider. Lookups always use (Issuer, Subject), never the e-mail (ADR 0015, D4).
type Identity struct {
	UserID  UserID
	Issuer  string
	Subject string
}
