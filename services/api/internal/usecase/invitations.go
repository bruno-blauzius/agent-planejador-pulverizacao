package usecase

import (
	"context"
	"errors"
	"fmt"

	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/domain"
)

// ResendInvitationInput is the request of POST /invitations/{id}/resend.
type ResendInvitationInput struct {
	Actor        domain.Principal
	InvitationID domain.InvitationID
}

// ResendInvitationResult says whether the e-mail left. The invitation is
// renewed either way.
type ResendInvitationResult struct {
	EmailDelivery EmailDelivery
	EmailFailure  error
}

// ResendInvitation renews an open invitation (ADR 0015, D7): it makes sure the
// account is in the role group, extends the expiry in one transaction, and
// then asks the provider for a fresh action e-mail when the account still
// has to set up its credentials.
type ResendInvitation struct {
	invitations InvitationFinder
	users       UserFinder
	companies   CompanyFinder
	identities  IdentityStore
	accounts    AccountDirectory
	access      ProductAccess
	mailer      InviteSender
	tx          TxRunner
	clock       Clock
	settings    Settings
}

// ResendInvitationDeps groups the collaborators of ResendInvitation.
type ResendInvitationDeps struct {
	Invitations InvitationFinder
	Users       UserFinder
	Companies   CompanyFinder
	Identities  IdentityStore
	Accounts    AccountDirectory
	Access      ProductAccess
	Mailer      InviteSender
	Tx          TxRunner
	Clock       Clock
	Settings    Settings
}

// NewResendInvitation builds the use case.
func NewResendInvitation(d ResendInvitationDeps) *ResendInvitation {
	return &ResendInvitation{
		invitations: d.Invitations, users: d.Users, companies: d.Companies, identities: d.Identities,
		accounts: d.Accounts, access: d.Access, mailer: d.Mailer, tx: d.Tx, clock: d.Clock, settings: d.Settings,
	}
}

// Execute resends the invitation.
func (uc *ResendInvitation) Execute(ctx context.Context, in ResendInvitationInput) (ResendInvitationResult, error) {
	if err := requireCan(in.Actor, domain.PermUsersManage); err != nil {
		return ResendInvitationResult{}, err
	}
	invitation, user, err := loadManagedInvitation(ctx, uc.invitations, uc.users, in.Actor, in.InvitationID)
	if err != nil {
		return ResendInvitationResult{}, err
	}
	company, err := loadCompany(ctx, uc.companies, user.CompanyID)
	if err != nil {
		return ResendInvitationResult{}, err
	}
	if !company.AllowsAccess() {
		return ResendInvitationResult{}, fmt.Errorf("%w: cannot resend for %q", ErrCompanyInactive, company.Name)
	}
	identity, linked, err := userIdentity(ctx, uc.identities, user.ID)
	if err != nil {
		return ResendInvitationResult{}, err
	}
	if !linked {
		return ResendInvitationResult{}, fmt.Errorf("invitation %s has no linked account: %w", invitation.ID, ErrNotFound)
	}

	account, err := uc.accounts.FindAccountByEmail(ctx, user.Email)
	if err != nil {
		return ResendInvitationResult{}, providerFailure("find account", err)
	}
	if err := uc.access.AddToRoleGroup(ctx, identity.Subject, user.Role); err != nil {
		return ResendInvitationResult{}, providerFailure("add to role group", err)
	}

	now := uc.clock.Now()
	if err := invitation.Resend(now, now.Add(uc.settings.InvitationTTL)); err != nil {
		return ResendInvitationResult{}, err
	}
	err = uc.tx.InTx(ctx, func(ctx context.Context, tx Stores) error {
		if err := tx.Invitations.UpdateInvitation(ctx, invitation); err != nil {
			return fmt.Errorf("update invitation: %w", err)
		}
		event := actorEvent(in.Actor, now, user.CompanyID, domain.AuditInvitationResent, domain.AuditEntityInvitation, string(invitation.ID),
			map[string]string{"user_id": string(user.ID), "send_count": fmt.Sprint(invitation.SendCount)})
		if err := tx.Audit.Record(ctx, event); err != nil {
			return fmt.Errorf("record audit: %w", err)
		}
		return nil
	})
	if err != nil {
		return ResendInvitationResult{}, err
	}

	if account.EmailVerified {
		return ResendInvitationResult{EmailDelivery: EmailNotNeeded}, nil
	}
	if err := uc.mailer.SendActionEmail(ctx, account.Subject, EmailSetupAccount); err != nil {
		return ResendInvitationResult{EmailDelivery: EmailNotSent, EmailFailure: err}, nil
	}
	return ResendInvitationResult{EmailDelivery: EmailSent}, nil
}

// CancelInvitationInput is the request of DELETE /invitations/{id}.
type CancelInvitationInput struct {
	Actor        domain.Principal
	InvitationID domain.InvitationID
}

// CancelInvitation withdraws an open invitation (ADR 0015, D7). The provider
// is cleaned first (leave the group; delete the account only if this
// invitation created it and nobody used it) because every step is idempotent
// and a retry finishes the job; then one transaction cancels the invitation,
// soft deletes the invited user, unlinks the identity and audits.
type CancelInvitation struct {
	invitations InvitationFinder
	users       UserFinder
	identities  IdentityStore
	accounts    AccountDirectory
	access      ProductAccess
	tx          TxRunner
	clock       Clock
}

// CancelInvitationDeps groups the collaborators of CancelInvitation.
type CancelInvitationDeps struct {
	Invitations InvitationFinder
	Users       UserFinder
	Identities  IdentityStore
	Accounts    AccountDirectory
	Access      ProductAccess
	Tx          TxRunner
	Clock       Clock
}

// NewCancelInvitation builds the use case.
func NewCancelInvitation(d CancelInvitationDeps) *CancelInvitation {
	return &CancelInvitation{
		invitations: d.Invitations, users: d.Users, identities: d.Identities,
		accounts: d.Accounts, access: d.Access, tx: d.Tx, clock: d.Clock,
	}
}

// Execute cancels the invitation.
func (uc *CancelInvitation) Execute(ctx context.Context, in CancelInvitationInput) error {
	if err := requireCan(in.Actor, domain.PermUsersManage); err != nil {
		return err
	}
	invitation, user, err := loadManagedInvitation(ctx, uc.invitations, uc.users, in.Actor, in.InvitationID)
	if err != nil {
		return err
	}

	identity, linked, err := userIdentity(ctx, uc.identities, user.ID)
	if err != nil {
		return err
	}
	if linked {
		if err := uc.access.RemoveFromProductGroups(ctx, identity.Subject); err != nil {
			return providerFailure("remove from product groups", err)
		}
		if invitation.AccountCreated {
			err := uc.accounts.DeleteCreatedAccount(ctx, identity.Subject)
			if err != nil && !errors.Is(err, ErrAccountInUse) {
				return providerFailure("delete created account", err)
			}
		}
	}

	now := uc.clock.Now()
	if err := invitation.Cancel(now); err != nil {
		return err
	}
	user.MarkDeleted(now)
	return uc.tx.InTx(ctx, func(ctx context.Context, tx Stores) error {
		if err := tx.Invitations.UpdateInvitation(ctx, invitation); err != nil {
			return fmt.Errorf("update invitation: %w", err)
		}
		if err := tx.Users.UpdateUser(ctx, user); err != nil {
			return fmt.Errorf("delete invited user: %w", err)
		}
		if err := tx.Identities.UnlinkIdentities(ctx, user.ID); err != nil {
			return fmt.Errorf("unlink identity: %w", err)
		}
		event := actorEvent(in.Actor, now, user.CompanyID, domain.AuditInvitationCancelled, domain.AuditEntityInvitation, string(invitation.ID),
			map[string]string{"user_id": string(user.ID)})
		if err := tx.Audit.Record(ctx, event); err != nil {
			return fmt.Errorf("record audit: %w", err)
		}
		return nil
	})
}

// loadManagedInvitation finds an open invitation inside the actor's scope,
// together with the invited user. A closed invitation is domain.ErrInvitationClosed.
func loadManagedInvitation(ctx context.Context, invitations InvitationFinder, users UserFinder, actor domain.Principal, id domain.InvitationID) (domain.Invitation, domain.User, error) {
	scope := ScopeOf(actor)
	invitation, err := invitations.FindInvitation(ctx, scope, id)
	if err != nil {
		return domain.Invitation{}, domain.User{}, fmt.Errorf("find invitation: %w", err)
	}
	if !scope.Includes(invitation.CompanyID) {
		return domain.Invitation{}, domain.User{}, fmt.Errorf("find invitation: %w", ErrNotFound)
	}
	user, err := loadManagedUser(ctx, users, actor, invitation.UserID)
	if err != nil {
		return domain.Invitation{}, domain.User{}, err
	}
	if !invitation.IsOpen() {
		return domain.Invitation{}, domain.User{}, domain.ErrInvitationClosed
	}
	return invitation, user, nil
}
