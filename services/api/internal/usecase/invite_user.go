package usecase

import (
	"context"
	"errors"
	"fmt"

	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/domain"
)

// InviteUserInput is the request of POST /invitations.
type InviteUserInput struct {
	Actor     domain.Principal
	Name      string
	Email     string
	Role      domain.Role
	CompanyID domain.CompanyID
	FarmScope domain.FarmScope
}

// InviteUserResult is the outcome of an invitation. EmailFailure carries the
// cause when the e-mail did not leave, so the adapter can log it; the
// invitation stays valid and can be resent.
type InviteUserResult struct {
	User          UserListing
	EmailDelivery EmailDelivery
	EmailFailure  error
}

// InviteUser provisions a user in the identity provider and in the database
// (ADR 0015, D5.1 and D7). The two are not transactional, so the order is:
// find or create the account, add it to the role group, write the database in
// one transaction (user, identity, invitation, audit), and only then e-mail.
// A failure after the provider changed undoes only what this flow created.
type InviteUser struct {
	users     UserFinder
	companies CompanyFinder
	farms     FarmDirectory
	accounts  AccountDirectory
	access    ProductAccess
	mailer    InviteSender
	tx        TxRunner
	clock     Clock
	ids       IDGenerator
	settings  Settings
}

// InviteUserDeps groups the collaborators of InviteUser.
type InviteUserDeps struct {
	Users     UserFinder
	Companies CompanyFinder
	Farms     FarmDirectory
	Accounts  AccountDirectory
	Access    ProductAccess
	Mailer    InviteSender
	Tx        TxRunner
	Clock     Clock
	IDs       IDGenerator
	Settings  Settings
}

// NewInviteUser builds the use case.
func NewInviteUser(d InviteUserDeps) *InviteUser {
	return &InviteUser{
		users: d.Users, companies: d.Companies, farms: d.Farms, accounts: d.Accounts, access: d.Access,
		mailer: d.Mailer, tx: d.Tx, clock: d.Clock, ids: d.IDs, settings: d.Settings,
	}
}

// Execute invites the user.
func (uc *InviteUser) Execute(ctx context.Context, in InviteUserInput) (InviteUserResult, error) {
	if err := requireCan(in.Actor, domain.PermUsersManage); err != nil {
		return InviteUserResult{}, err
	}
	if err := authorizeAssignment(in.Actor, in.Role, in.CompanyID); err != nil {
		return InviteUserResult{}, err
	}
	company, err := companyForInput(ctx, uc.companies, in.CompanyID)
	if err != nil {
		return InviteUserResult{}, err
	}
	if !company.AllowsAccess() {
		return InviteUserResult{}, fmt.Errorf("%w: cannot invite into %q", ErrCompanyInactive, company.Name)
	}

	now := uc.clock.Now()
	user, err := domain.NewInvitedUser(domain.UserID(uc.ids.NewID()), company, in.Name, in.Email, in.Role, in.FarmScope, now)
	if err != nil {
		return InviteUserResult{}, err
	}
	if err := checkSelectedFarms(ctx, uc.farms, company.ID, user.FarmScope); err != nil {
		return InviteUserResult{}, err
	}
	if err := uc.ensureEmailIsFree(ctx, user.Email); err != nil {
		return InviteUserResult{}, err
	}

	account, created, err := uc.provisionAccount(ctx, user)
	if err != nil {
		return InviteUserResult{}, err
	}

	invitation := domain.NewInvitation(domain.InvitationID(uc.ids.NewID()), user, in.Actor.UserID, created, now, now.Add(uc.settings.InvitationTTL))
	if err := uc.persist(ctx, in.Actor, user, invitation, account, created); err != nil {
		return InviteUserResult{}, uc.undoProvisioning(ctx, err, account, created)
	}

	result := InviteUserResult{
		User:          UserListing{User: user, CompanyName: company.Name, Invitation: &invitation},
		EmailDelivery: EmailNotNeeded,
	}
	if created {
		result.EmailDelivery = EmailSent
		if err := uc.mailer.SendActionEmail(ctx, account.Subject, EmailSetupAccount); err != nil {
			result.EmailDelivery = EmailNotSent
			result.EmailFailure = err
		}
	}
	return result, nil
}

func (uc *InviteUser) ensureEmailIsFree(ctx context.Context, email domain.Email) error {
	_, err := uc.users.FindUserByEmail(ctx, email)
	switch {
	case err == nil:
		return ErrEmailAlreadyRegistered
	case errors.Is(err, ErrNotFound):
		return nil
	default:
		return fmt.Errorf("check e-mail: %w", err)
	}
}

// provisionAccount reuses the realm account of the e-mail (only when verified)
// or creates a new one, then adds it to the group of the role. It returns
// whether this call created the account.
func (uc *InviteUser) provisionAccount(ctx context.Context, user domain.User) (Account, bool, error) {
	account, err := uc.accounts.FindAccountByEmail(ctx, user.Email)
	created := false
	switch {
	case err == nil:
		if !account.EmailVerified {
			// Never link an unverified account: ask the person to verify first.
			mailErr := uc.mailer.SendActionEmail(ctx, account.Subject, EmailVerifyEmail)
			if mailErr != nil {
				return Account{}, false, errors.Join(ErrEmailNotVerified, fmt.Errorf("verification e-mail not sent: %w", mailErr))
			}
			return Account{}, false, ErrEmailNotVerified
		}
		if err := uc.ensureAccountIsUnlinked(ctx, account); err != nil {
			return Account{}, false, err
		}
	case errors.Is(err, ErrAccountNotFound):
		account, err = uc.accounts.CreateAccount(ctx, NewAccount{Email: user.Email, Name: user.Name})
		if err != nil {
			return Account{}, false, providerFailure("create account", err)
		}
		created = true
	default:
		return Account{}, false, providerFailure("find account", err)
	}

	if err := uc.access.AddToRoleGroup(ctx, account.Subject, user.Role); err != nil {
		failure := providerFailure("add to role group", err)
		if created {
			failure = compensated(failure, uc.accounts.DeleteCreatedAccount(ctx, account.Subject))
		}
		return Account{}, false, failure
	}
	return account, created, nil
}

func (uc *InviteUser) persist(ctx context.Context, actor domain.Principal, user domain.User, invitation domain.Invitation, account Account, created bool) error {
	return uc.tx.InTx(ctx, func(ctx context.Context, tx Stores) error {
		// Lock the company row first and check it again: DeleteCompany holds the same
		// lock while it counts members, so either the invitation is committed before
		// the delete counts it, or it sees the company gone and is refused here.
		company, err := tx.Companies.LockCompany(ctx, user.CompanyID)
		if err != nil {
			return fmt.Errorf("lock company: %w", err)
		}
		if !company.AllowsAccess() {
			return fmt.Errorf("%w: cannot invite into %q", ErrCompanyInactive, company.Name)
		}
		if err := tx.Users.InsertUser(ctx, user); err != nil {
			return fmt.Errorf("insert user: %w", err)
		}
		identity := domain.Identity{UserID: user.ID, Issuer: account.Issuer, Subject: account.Subject}
		if err := tx.Identities.LinkIdentity(ctx, identity); err != nil {
			return fmt.Errorf("link identity: %w", err)
		}
		if err := tx.Invitations.InsertInvitation(ctx, invitation); err != nil {
			return fmt.Errorf("insert invitation: %w", err)
		}
		events := []domain.AuditEvent{actorEvent(actor, invitation.CreatedAt, user.CompanyID, domain.AuditInvitationCreated, domain.AuditEntityInvitation, string(invitation.ID),
			map[string]string{"user_id": string(user.ID), "role": string(user.Role), "farm_scope": string(user.FarmScope.Mode())})}
		if !created {
			events = append(events, actorEvent(actor, invitation.CreatedAt, user.CompanyID, domain.AuditAccountLinked, domain.AuditEntityUser, string(user.ID),
				map[string]string{"issuer": account.Issuer}))
		}
		for _, event := range events {
			if err := tx.Audit.Record(ctx, event); err != nil {
				return fmt.Errorf("record audit: %w", err)
			}
		}
		return nil
	})
}

// undoProvisioning reverts the provider after a database failure, touching
// only what this flow did: delete an account it created, or just take a reused
// account out of the product group (rule 2 of ADR 0015, D5).
func (uc *InviteUser) undoProvisioning(ctx context.Context, cause error, account Account, created bool) error {
	if errors.Is(cause, ErrIdentityAlreadyLinked) {
		// The account belongs to another user of the product: its group
		// membership is theirs, so there is nothing of ours to undo.
		return fmt.Errorf("%w: %w", ErrEmailAlreadyRegistered, cause)
	}
	if created {
		return compensated(cause, uc.accounts.DeleteCreatedAccount(ctx, account.Subject))
	}
	return compensated(cause, uc.access.RemoveFromProductGroups(ctx, account.Subject))
}

// ensureAccountIsUnlinked refuses an account that already belongs to another
// user of the product (for instance after the e-mail changed in the realm).
func (uc *InviteUser) ensureAccountIsUnlinked(ctx context.Context, account Account) error {
	_, err := uc.users.FindUserByIdentity(ctx, account.Issuer, account.Subject)
	switch {
	case err == nil:
		return fmt.Errorf("%w: the account is linked to another user", ErrEmailAlreadyRegistered)
	case errors.Is(err, ErrNotFound):
		return nil
	default:
		return fmt.Errorf("check identity link: %w", err)
	}
}
