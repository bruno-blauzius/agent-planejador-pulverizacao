package usecase

import (
	"errors"
	"fmt"
)

// Errors returned by use cases and by the ports they declare. Adapters map
// them to HTTP statuses with errors.Is. Domain errors (domain.ErrNoAccessToProduct,
// domain.ErrAmbiguousRole, domain.ErrInvitationClosed, *domain.ValidationError)
// pass through unchanged.
var (
	// ErrNotFound: the resource does not exist, was deleted or belongs to
	// another company (the API never reveals the difference).
	ErrNotFound = errors.New("not found")
	// ErrForbidden: the caller lacks the permission or may not touch the target (403 forbidden).
	ErrForbidden = errors.New("forbidden")
	// ErrAccessDenied: valid identity, but the user or company is inactive,
	// or the invitation cannot be accepted yet (403 forbidden).
	ErrAccessDenied = errors.New("access denied")
	// ErrUserNotProvisioned: a valid identity that nobody invited (403 user_not_provisioned).
	ErrUserNotProvisioned = errors.New("user not provisioned")

	// ErrEmailAlreadyRegistered: the e-mail already belongs to a user of the product (409).
	ErrEmailAlreadyRegistered = errors.New("e-mail already registered")
	// ErrEmailNotVerified: the existing realm account has an unverified e-mail and is not linked (409).
	ErrEmailNotVerified = errors.New("identity e-mail not verified")
	// ErrCompanyInactive: the operation needs an active company (409).
	ErrCompanyInactive = errors.New("company is inactive")
	// ErrCompanyHasActiveUsers: the company still has active or invited users (409).
	ErrCompanyHasActiveUsers = errors.New("company has active users")
	// ErrInvitationPending: the user has an open invitation; cancel it instead (409).
	ErrInvitationPending = errors.New("user has a pending invitation")
	// ErrDocumentAlreadyRegistered: the CNPJ is taken (409 conflict).
	ErrDocumentAlreadyRegistered = errors.New("document already registered")
	// ErrLastSuperAdmin: the operation would leave the platform without an
	// active super admin (409 last_super_admin).
	ErrLastSuperAdmin = errors.New("cannot remove the last active super admin")
	// ErrVersionConflict: optimistic concurrency failure (409 conflict).
	ErrVersionConflict = errors.New("version conflict")
	// ErrIdentityAlreadyLinked: the external identity belongs to another user.
	ErrIdentityAlreadyLinked = errors.New("identity already linked")
	// ErrFarmNotInCompany: a selected farm does not belong to the company (422).
	ErrFarmNotInCompany = errors.New("farm does not belong to the company")
	// ErrOwnerDocumentTaken: the CPF/CNPJ already belongs to an owner of the company (409 conflict).
	ErrOwnerDocumentTaken = errors.New("owner document already registered")
	// ErrUnsupportedDocumentMedia: an owner document is not PDF, JPEG or PNG (415).
	ErrUnsupportedDocumentMedia = errors.New("unsupported owner document media type")
	// ErrDocumentStorageUnavailable: the object storage of owner documents is
	// down or not configured (503 document_storage_unavailable).
	ErrDocumentStorageUnavailable = errors.New("document storage unavailable")
	// ErrElevationUnavailable: the elevation model has no data for a tile (it
	// does not exist, e.g. over the ocean) or the download failed (ADR 0021).
	// The field ends as failed with this reason; nothing is invented.
	ErrElevationUnavailable = errors.New("elevation data unavailable")
	// ErrElevationTimeout: the database canceled an elevation statement for taking
	// too long (SQLSTATE 57014, statement_timeout). The field ends as failed with the
	// timeout reason, like a context deadline.
	ErrElevationTimeout = errors.New("elevation operation timed out")
	// ErrUnsupportedPerimeterFormat: the perimeter file is not KML, KMZ, GeoJSON or a zipped Shapefile (415).
	ErrUnsupportedPerimeterFormat = errors.New("unsupported perimeter file format")

	// ErrIdentityProviderUnavailable: the identity provider is down or refused
	// the operation (502 identity_provider_unavailable). Nothing is left half-registered.
	ErrIdentityProviderUnavailable = errors.New("identity provider unavailable")
	// ErrAccountNotFound is returned by the provider port when no account matches.
	ErrAccountNotFound = errors.New("identity account not found")
	// ErrAccountInUse is returned by the provider port when an account cannot
	// be deleted because it was already used (it has signed in).
	ErrAccountInUse = errors.New("identity account already in use")
)

// providerFailure wraps an unexpected provider error so callers can match
// ErrIdentityProviderUnavailable while keeping the cause for logs.
func providerFailure(operation string, err error) error {
	return fmt.Errorf("%w: %s: %w", ErrIdentityProviderUnavailable, operation, err)
}

// compensated adds the outcome of a compensation step to the primary error.
// Both stay matchable with errors.Is; a failed compensation is never dropped.
func compensated(primary error, compensation error) error {
	if compensation == nil {
		return primary
	}
	return errors.Join(primary, fmt.Errorf("compensation failed, reconcile manually: %w", compensation))
}
