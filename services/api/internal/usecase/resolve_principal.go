package usecase

import (
	"context"
	"errors"
	"fmt"

	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/domain"
)

// Claims are the facts of a validated token that the use case needs. The OIDC
// adapter validates signature, issuer, audience and expiry before building
// them; this layer trusts nothing else.
type Claims struct {
	Issuer        string
	Subject       string
	Email         string
	EmailVerified bool
	// ClientRoles is resource_access.agentes-agro.roles (effective roles).
	ClientRoles []string
	// SyncRole allows writing the role of the token into the users.role
	// projection. It is true only for roles just obtained from the provider (the
	// login code exchange or a token refresh). A Bearer token may have been
	// issued before a role change made through this API, so it never rewrites
	// the projection; the principal still takes the role from the token (ADR 0015, D3).
	SyncRole bool
}

// ResolvePrincipalInput is the request of the authentication step.
type ResolvePrincipalInput struct {
	Claims Claims
}

// ResolvePrincipal turns a validated identity into the Principal of a request
// (ADR 0015, D3 and D4): exactly one product role from the token, a user found
// by (issuer, subject) in an active company, and the status from the database.
// There is no just-in-time provisioning.
type ResolvePrincipal struct {
	users       UserFinder
	companies   CompanyFinder
	invitations InvitationFinder
	tx          TxRunner
	clock       Clock
	settings    Settings
}

// NewResolvePrincipal builds the use case.
func NewResolvePrincipal(users UserFinder, companies CompanyFinder, invitations InvitationFinder, tx TxRunner, clock Clock, settings Settings) *ResolvePrincipal {
	return &ResolvePrincipal{users: users, companies: companies, invitations: invitations, tx: tx, clock: clock, settings: settings}
}

// Execute resolves the principal or returns one of: domain.ErrNoAccessToProduct,
// domain.ErrAmbiguousRole, ErrUserNotProvisioned, ErrAccessDenied.
func (uc *ResolvePrincipal) Execute(ctx context.Context, in ResolvePrincipalInput) (domain.Principal, error) {
	claims := in.Claims
	access, err := domain.ReadAccess(claims.ClientRoles)
	if errors.Is(err, domain.ErrAmbiguousRole) {
		return domain.Principal{}, uc.reportAmbiguousRole(ctx, claims, err)
	}
	if err != nil {
		return domain.Principal{}, err
	}

	user, err := uc.users.FindUserByIdentity(ctx, claims.Issuer, claims.Subject)
	if errors.Is(err, ErrNotFound) {
		return domain.Principal{}, ErrUserNotProvisioned
	}
	if err != nil {
		return domain.Principal{}, fmt.Errorf("find user by identity: %w", err)
	}
	if user.IsDeleted() {
		return domain.Principal{}, ErrUserNotProvisioned
	}

	company, err := uc.companies.FindCompany(ctx, user.CompanyID)
	if errors.Is(err, ErrNotFound) {
		return domain.Principal{}, fmt.Errorf("%w: company not found", ErrAccessDenied)
	}
	if err != nil {
		return domain.Principal{}, fmt.Errorf("find company: %w", err)
	}
	if !company.AllowsAccess() {
		return domain.Principal{}, fmt.Errorf("%w: company is inactive", ErrAccessDenied)
	}

	access = access.ForCompany(company)
	updated, err := uc.reconcile(ctx, user, company, access, claims)
	if err != nil {
		return domain.Principal{}, err
	}
	return principalOf(updated, access), nil
}

// reconcile applies what a successful login changes: the first login of an
// invited user accepts the invitation, the role projection follows the token
// when it is fresh (Claims.SyncRole), and the last access is stamped (rate
// limited).
func (uc *ResolvePrincipal) reconcile(ctx context.Context, user domain.User, company domain.Company, access domain.Access, claims Claims) (domain.User, error) {
	now := uc.clock.Now()
	updated := user
	var invitation *domain.Invitation
	var accepted, roleSynced bool

	switch user.Status {
	case domain.UserInactive:
		return domain.User{}, fmt.Errorf("%w: user is inactive", ErrAccessDenied)
	case domain.UserInvited:
		if !claims.EmailVerified {
			return domain.User{}, fmt.Errorf("%w: e-mail not verified yet", ErrAccessDenied)
		}
		found, err := uc.invitations.FindOpenInvitationByUser(ctx, user.ID)
		if errors.Is(err, ErrNotFound) {
			return domain.User{}, fmt.Errorf("%w: invitation is no longer open", ErrAccessDenied)
		}
		if err != nil {
			return domain.User{}, fmt.Errorf("find invitation: %w", err)
		}
		if err := found.Accept(now); err != nil {
			return domain.User{}, fmt.Errorf("%w: %w", ErrAccessDenied, err)
		}
		if err := updated.Activate(now); err != nil {
			return domain.User{}, fmt.Errorf("activate user: %w", err)
		}
		invitation, accepted = &found, true
	case domain.UserActive:
	}

	if access.Role != updated.Role {
		if err := domain.CheckAssignment(company, access.Role, updated.FarmScope); err != nil {
			return domain.User{}, fmt.Errorf("%w: role in token does not fit the user: %w", ErrAccessDenied, err)
		}
		if claims.SyncRole {
			updated.Role = access.Role
			roleSynced = true
		}
	}

	touched := false
	if !accepted && (updated.LastAccessAt == nil || now.Sub(*updated.LastAccessAt) >= uc.settings.LastAccessInterval) {
		updated.RecordAccess(now)
		touched = true
	}
	if !accepted && !roleSynced && !touched {
		return updated, nil
	}

	err := uc.tx.InTx(ctx, func(ctx context.Context, tx Stores) error {
		if err := tx.Users.UpdateUser(ctx, updated); err != nil {
			return fmt.Errorf("update user: %w", err)
		}
		self := principalOf(updated, access)
		if accepted {
			if err := tx.Invitations.UpdateInvitation(ctx, *invitation); err != nil {
				return fmt.Errorf("update invitation: %w", err)
			}
			event := actorEvent(self, now, updated.CompanyID, domain.AuditInvitationAccepted, domain.AuditEntityInvitation, string(invitation.ID), nil)
			if err := tx.Audit.Record(ctx, event); err != nil {
				return fmt.Errorf("record audit: %w", err)
			}
		}
		if roleSynced {
			event := actorEvent(self, now, updated.CompanyID, domain.AuditUserRoleSynced, domain.AuditEntityUser, string(updated.ID),
				map[string]string{"from": string(user.Role), "to": string(updated.Role)})
			if err := tx.Audit.Record(ctx, event); err != nil {
				return fmt.Errorf("record audit: %w", err)
			}
		}
		return nil
	})
	// A lost race on a plain last-access stamp must not fail the login.
	if errors.Is(err, ErrVersionConflict) && !accepted && !roleSynced {
		return user, nil
	}
	if err != nil {
		return domain.User{}, err
	}
	return updated, nil
}

// reportAmbiguousRole audits a token with more than one product role when the
// identity is known, and always returns the ambiguity error.
func (uc *ResolvePrincipal) reportAmbiguousRole(ctx context.Context, claims Claims, ambiguity error) error {
	user, err := uc.users.FindUserByIdentity(ctx, claims.Issuer, claims.Subject)
	if errors.Is(err, ErrNotFound) {
		return ambiguity
	}
	if err != nil {
		return compensated(ambiguity, fmt.Errorf("find user to audit: %w", err))
	}
	now := uc.clock.Now()
	self := domain.Principal{UserID: user.ID, Name: user.Name}
	event := actorEvent(self, now, user.CompanyID, domain.AuditAmbiguousRole, domain.AuditEntityUser, string(user.ID), nil)
	auditErr := uc.tx.InTx(ctx, func(ctx context.Context, tx Stores) error {
		return tx.Audit.Record(ctx, event)
	})
	if auditErr != nil {
		return compensated(ambiguity, fmt.Errorf("record audit: %w", auditErr))
	}
	return ambiguity
}

func principalOf(user domain.User, access domain.Access) domain.Principal {
	return domain.Principal{
		UserID:      user.ID,
		CompanyID:   user.CompanyID,
		Name:        user.Name,
		Email:       user.Email,
		Role:        access.Role,
		Permissions: access.Permissions,
		FarmScope:   user.FarmScope,
	}
}
