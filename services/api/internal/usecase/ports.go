package usecase

import (
	"context"
	"time"

	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/domain"
)

// Ports are declared where they are consumed (CLAUDE.md) and kept small. None
// of them mentions SQL: adapters decide how to store and query.
//
// Every query that returns business data receives a Scope so the adapter
// filters by company_id; use cases check it again (defense in depth).

// Scope is the set of companies a caller may touch: all of them (super admin)
// or exactly one. It is derived from the Principal, never from client input.
type Scope struct {
	all       bool
	companyID domain.CompanyID
}

// ScopeOf returns the company scope of the principal.
func ScopeOf(actor domain.Principal) Scope {
	if actor.ManagesAllCompanies() {
		return Scope{all: true}
	}
	return Scope{companyID: actor.CompanyID}
}

// OnlyCompany returns a scope limited to one company.
func OnlyCompany(id domain.CompanyID) Scope { return Scope{companyID: id} }

// All reports whether the scope spans every company.
func (s Scope) All() bool { return s.all }

// CompanyID returns the company of a single-company scope (empty when All).
func (s Scope) CompanyID() domain.CompanyID { return s.companyID }

// Includes reports whether the company is inside the scope.
func (s Scope) Includes(id domain.CompanyID) bool {
	return s.all || s.companyID == id
}

// Narrow restricts a scope that spans every company to one company. A
// single-company scope is returned unchanged: the client cannot widen it.
func (s Scope) Narrow(id domain.CompanyID) Scope {
	if s.all && id != "" {
		return Scope{companyID: id}
	}
	return s
}

// ---- users -----------------------------------------------------------------

// UserFinder reads users. Soft deleted users are never returned.
type UserFinder interface {
	// FindUser returns ErrNotFound when the user is outside the scope.
	FindUser(ctx context.Context, scope Scope, id domain.UserID) (domain.User, error)
	// FindUserByIdentity resolves the login. It is not tenant scoped: the
	// company is a property of the user found.
	FindUserByIdentity(ctx context.Context, issuer, subject string) (domain.User, error)
	// FindUserByEmail looks for a user of the product in any company (the
	// e-mail is unique across the product).
	FindUserByEmail(ctx context.Context, email domain.Email) (domain.User, error)
}

// UserWriter persists users. Adapters enforce optimistic concurrency with
// User.Version (ErrVersionConflict) and uniqueness (ErrEmailAlreadyRegistered).
type UserWriter interface {
	InsertUser(ctx context.Context, user domain.User) error
	UpdateUser(ctx context.Context, user domain.User) error
}

// UserFilter narrows a user listing.
type UserFilter struct {
	Search    string
	Role      domain.Role
	Status    domain.UserStatus
	CompanyID domain.CompanyID
}

// PageRequest is a validated page (1-based).
type PageRequest struct {
	Page     int
	PageSize int
}

// UserListing is a row of the users screen: a user with its company name and,
// for invited users, the open invitation.
type UserListing struct {
	User        domain.User
	CompanyName string
	Invitation  *domain.Invitation
}

// UserPage is one page of users.
type UserPage struct {
	Items []UserListing
	Total int
	PageRequest
}

// UsersSummary counts users and pending invitations in a scope.
type UsersSummary struct {
	TotalUsers         int
	CompanyCount       int
	PendingInvitations int
	ByRole             map[domain.Role]int
}

// UserLister serves the users screen.
type UserLister interface {
	ListUsers(ctx context.Context, scope Scope, filter UserFilter, page PageRequest) (UserPage, error)
	SummarizeUsers(ctx context.Context, scope Scope) (UsersSummary, error)
	// FindUserListing returns one row (used after a write to build the response).
	FindUserListing(ctx context.Context, scope Scope, id domain.UserID) (UserListing, error)
}

// CompanyMembers reads the people of one company, for company-wide operations.
type CompanyMembers interface {
	// ListCompanyIdentities returns the linked identities of the non-deleted users of the company.
	ListCompanyIdentities(ctx context.Context, companyID domain.CompanyID) ([]domain.Identity, error)
	// CountUsersByStatus counts the non-deleted users of the company per status.
	CountUsersByStatus(ctx context.Context, companyID domain.CompanyID) (map[domain.UserStatus]int, error)
}

// RoleCounter counts active users by role across the whole product.
type RoleCounter interface {
	CountActiveUsersByRole(ctx context.Context, role domain.Role) (int, error)
}

// IdentityStore keeps the link between a user and an external account.
type IdentityStore interface {
	// LinkIdentity returns ErrIdentityAlreadyLinked when (issuer, subject)
	// belongs to another user.
	LinkIdentity(ctx context.Context, identity domain.Identity) error
	// UnlinkIdentities removes every link of the user (on delete or cancel).
	UnlinkIdentities(ctx context.Context, userID domain.UserID) error
	// FindIdentityByUser returns ErrNotFound when the user has no link.
	FindIdentityByUser(ctx context.Context, userID domain.UserID) (domain.Identity, error)
}

// ---- invitations -------------------------------------------------------------

// InvitationFinder reads invitations.
type InvitationFinder interface {
	// FindInvitation returns ErrNotFound when the invitation is outside the scope.
	FindInvitation(ctx context.Context, scope Scope, id domain.InvitationID) (domain.Invitation, error)
	// FindOpenInvitationByUser returns ErrNotFound when the user has none.
	FindOpenInvitationByUser(ctx context.Context, userID domain.UserID) (domain.Invitation, error)
}

// InvitationWriter persists invitations.
type InvitationWriter interface {
	InsertInvitation(ctx context.Context, invitation domain.Invitation) error
	UpdateInvitation(ctx context.Context, invitation domain.Invitation) error
}

// ---- companies ---------------------------------------------------------------

// CompanyFinder reads companies. Soft deleted companies are never returned.
type CompanyFinder interface {
	FindCompany(ctx context.Context, id domain.CompanyID) (domain.Company, error)
	// LockCompany is FindCompany that serializes concurrent writers of the
	// same company (SELECT ... FOR UPDATE). Only meaningful inside a transaction.
	LockCompany(ctx context.Context, id domain.CompanyID) (domain.Company, error)
}

// CompanyWriter persists companies. InsertCompany returns
// ErrDocumentAlreadyRegistered; UpdateCompany returns ErrVersionConflict.
type CompanyWriter interface {
	InsertCompany(ctx context.Context, company domain.Company) error
	UpdateCompany(ctx context.Context, company domain.Company) error
}

// CompanyFilter narrows the company listing.
type CompanyFilter struct {
	Search string
	Status domain.CompanyStatus
}

// CompanyListing is a company with the counts shown on the screen.
type CompanyListing struct {
	Company   domain.Company
	FarmCount int
	UserCount int
}

// CompanyPage is one page of companies.
type CompanyPage struct {
	Items []CompanyListing
	Total int
	PageRequest
}

// CompaniesSummary holds the indicators of the companies screen.
type CompaniesSummary struct {
	ActiveCompanies           int
	TrialCompanies            int
	FarmCount                 int
	MonitoredAreaHa           float64
	UserCount                 int
	PendingInvitations        int
	ApplicationsThisMonth     int
	ApplicationsPreviousMonth int
}

// CompanyLister serves the companies screen. The platform operator company
// never appears in listings.
type CompanyLister interface {
	ListCompanies(ctx context.Context, filter CompanyFilter, page PageRequest) (CompanyPage, error)
	SummarizeCompanies(ctx context.Context) (CompaniesSummary, error)
	FindCompanyListing(ctx context.Context, id domain.CompanyID) (CompanyListing, error)
}

// ---- farms (schema only for now) ----------------------------------------------

// FarmRef identifies a farm for display.
type FarmRef struct {
	ID   domain.FarmID
	Name string
}

// FarmDirectory answers questions about farms without exposing the farm model.
type FarmDirectory interface {
	// CheckFarmsInCompany returns ErrFarmNotInCompany unless every farm
	// exists and belongs to the company.
	CheckFarmsInCompany(ctx context.Context, companyID domain.CompanyID, ids []domain.FarmID) error
	// FindFarmRef returns ErrNotFound when the farm is not in the company.
	FindFarmRef(ctx context.Context, companyID domain.CompanyID, id domain.FarmID) (FarmRef, error)
}

// ---- sessions and audit ----
//
// Session storage (auth_sessions) belongs to the OIDC/session adapter, which
// declares the ports it consumes itself (create, find by token hash, renew
// granted roles and access expiry, store the encrypted refresh token, revoke
// by session or by IdP session id). Use cases only need SessionRevoker below.-----------------------------------------------------

// SessionRevoker acts on the browser sessions of the API (auth_sessions).
type SessionRevoker interface {
	// RevokeUserSessions ends every session of the user.
	RevokeUserSessions(ctx context.Context, userID domain.UserID, at time.Time) error
	// RevokeCompanySessions ends every session of every user of the company.
	RevokeCompanySessions(ctx context.Context, companyID domain.CompanyID, at time.Time) error
	// ExpireUserAccess makes the next request of each session of the user
	// renew the token, so a new role or scope takes effect at once.
	ExpireUserAccess(ctx context.Context, userID domain.UserID, at time.Time) error
}

// AuditRecorder appends audit events (audit_events is append-only).
type AuditRecorder interface {
	Record(ctx context.Context, event domain.AuditEvent) error
}

// ---- transactions -----------------------------------------------------------------

// Stores are the transaction-bound writers handed to a TxRunner callback. All
// of them share one database transaction.
type Stores struct {
	Users interface {
		UserFinder
		UserWriter
	}
	Companies interface {
		CompanyFinder
		CompanyWriter
	}
	Invitations interface {
		InvitationFinder
		InvitationWriter
	}
	Identities IdentityStore
	Sessions   SessionRevoker
	Audit      AuditRecorder
}

// TxRunner runs fn in one database transaction: commit when fn returns nil,
// roll back otherwise. The identity provider is not part of it (ADR 0015, D5),
// so use cases order the two and compensate.
type TxRunner interface {
	InTx(ctx context.Context, fn func(ctx context.Context, tx Stores) error) error
}

// ---- identity provider (Keycloak) ---------------------------------------------------

// Account is a person's account in the identity provider realm.
type Account struct {
	Issuer        string
	Subject       string
	Email         domain.Email
	EmailVerified bool
}

// NewAccount asks the provider to create an account that must set its own
// password. The provider splits Name into the first and last names Keycloak requires.
type NewAccount struct {
	Email domain.Email
	Name  string
}

// AccountDirectory finds and creates accounts. FindAccountByEmail and the
// other lookups return ErrAccountNotFound; any other error means the provider
// is unavailable.
type AccountDirectory interface {
	FindAccountByEmail(ctx context.Context, email domain.Email) (Account, error)
	// CreateAccount creates the account with the required actions (verify
	// e-mail, update password). It does not send e-mail.
	CreateAccount(ctx context.Context, account NewAccount) (Account, error)
	// DeleteCreatedAccount removes an account this product created. It must
	// return ErrAccountInUse instead of deleting an account that already
	// signed in, and treat a missing account as success (idempotent).
	DeleteCreatedAccount(ctx context.Context, subject string) error
}

// ProductAccess manages membership in the product groups
// (/agentes-agro/...) and the product sessions. Every method is idempotent.
// None of them disables the account in the realm (ADR 0015, D5, rule 1).
type ProductAccess interface {
	AddToRoleGroup(ctx context.Context, subject string, role domain.Role) error
	RemoveFromProductGroups(ctx context.Context, subject string) error
	MoveToRoleGroup(ctx context.Context, subject string, from, to domain.Role) error
	// EndProductSessions ends the sessions of the client agentes-agro only.
	EndProductSessions(ctx context.Context, subject string) error
}

// IdentityProvisioner is the whole provisioning surface. Use cases depend on
// the smaller interfaces they need.
type IdentityProvisioner interface {
	AccountDirectory
	ProductAccess
}

// EmailKind selects the action e-mail the provider sends.
type EmailKind string

// Action e-mails.
const (
	// EmailSetupAccount asks the person to verify the e-mail and set a password.
	EmailSetupAccount EmailKind = "setup_account"
	// EmailVerifyEmail asks the person to verify the e-mail of an existing account.
	EmailVerifyEmail EmailKind = "verify_email"
)

// InviteSender asks the provider to send an action e-mail (execute-actions-email).
// A missing SMTP configuration or a send failure is an error here; use cases
// turn it into EmailNotSent without invalidating the invitation.
type InviteSender interface {
	SendActionEmail(ctx context.Context, subject string, kind EmailKind) error
}

// EmailDelivery tells whether the action e-mail left (header X-Invitation-Email).
type EmailDelivery string

// Delivery outcomes.
const (
	EmailSent      EmailDelivery = "sent"
	EmailNotSent   EmailDelivery = "not_sent"
	EmailNotNeeded EmailDelivery = "not_needed"
)

// ---- infrastructure -----------------------------------------------------------------

// Clock gives the current time, so tests are deterministic.
type Clock interface {
	Now() time.Time
}

// IDGenerator creates identifiers for new records.
type IDGenerator interface {
	NewID() string
}
