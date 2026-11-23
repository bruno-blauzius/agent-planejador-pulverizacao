package usecase

import (
	"context"
	"fmt"

	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/domain"
)

// UserStatusInput is the request shared by DeactivateUser, ReactivateUser and DeleteUser.
type UserStatusInput struct {
	Actor  domain.Principal
	UserID domain.UserID
}

// statusDeps are the collaborators shared by the three status use cases.
type statusDeps struct {
	users      UserFinder
	companies  CompanyFinder
	identities IdentityStore
	access     ProductAccess
	lister     UserLister
	roles      RoleCounter
	tx         TxRunner
	clock      Clock
}

// UserStatusDeps groups the collaborators of the status use cases.
type UserStatusDeps struct {
	Users      UserFinder
	Companies  CompanyFinder
	Identities IdentityStore
	Access     ProductAccess
	Lister     UserLister
	Roles      RoleCounter
	Tx         TxRunner
	Clock      Clock
}

func newStatusDeps(d UserStatusDeps) statusDeps {
	return statusDeps{
		users: d.Users, companies: d.Companies, identities: d.Identities,
		access: d.Access, lister: d.Lister, roles: d.Roles, tx: d.Tx, clock: d.Clock,
	}
}

// loadStatusTarget finds the target and applies the rules shared by the three
// operations: nobody acts on themselves, and a pending invitation is not a
// user to deactivate or delete.
func (d statusDeps) loadStatusTarget(ctx context.Context, in UserStatusInput, action string) (domain.User, error) {
	if err := requireCan(in.Actor, domain.PermUsersManage); err != nil {
		return domain.User{}, err
	}
	target, err := loadManagedUser(ctx, d.users, in.Actor, in.UserID)
	if err != nil {
		return domain.User{}, err
	}
	if target.ID == in.Actor.UserID {
		return domain.User{}, fmt.Errorf("%w: you cannot %s your own account", ErrForbidden, action)
	}
	if action != "reactivate" {
		if err := guardLastSuperAdmin(ctx, d.roles, target); err != nil {
			return domain.User{}, err
		}
	}
	return target, nil
}

// cutAccess blocks an active user in the database at once: status inactive,
// sessions revoked, audit. It is the first step of deactivation and deletion
// because the database, not the token, is what denies access (ADR 0015, D3).
func (d statusDeps) cutAccess(ctx context.Context, actor domain.Principal, target domain.User) error {
	if target.Status != domain.UserActive {
		return nil
	}
	now := d.clock.Now()
	updated := target
	if err := updated.Deactivate(); err != nil {
		return err
	}
	return d.tx.InTx(ctx, func(ctx context.Context, tx Stores) error {
		if err := tx.Users.UpdateUser(ctx, updated); err != nil {
			return fmt.Errorf("deactivate user: %w", err)
		}
		if err := tx.Sessions.RevokeUserSessions(ctx, target.ID, now); err != nil {
			return fmt.Errorf("revoke sessions: %w", err)
		}
		event := actorEvent(actor, now, target.CompanyID, domain.AuditUserDeactivated, domain.AuditEntityUser, string(target.ID), nil)
		if err := tx.Audit.Record(ctx, event); err != nil {
			return fmt.Errorf("record audit: %w", err)
		}
		return nil
	})
}

// releaseProductAccess takes the account out of the product groups and ends
// its product sessions. It never disables or deletes the realm account, and
// every step is idempotent so a retry after a failure finishes the cleanup.
func (d statusDeps) releaseProductAccess(ctx context.Context, userID domain.UserID) error {
	identity, linked, err := userIdentity(ctx, d.identities, userID)
	if err != nil || !linked {
		return err
	}
	if err := d.access.RemoveFromProductGroups(ctx, identity.Subject); err != nil {
		return providerFailure("remove from product groups", err)
	}
	if err := d.access.EndProductSessions(ctx, identity.Subject); err != nil {
		return providerFailure("end product sessions", err)
	}
	return nil
}

// DeactivateUser blocks a user in the product (ADR 0015, D5.3, rule 1). The
// database goes first, so access is cut immediately even if the provider is
// down; then the account leaves the product group and its product sessions
// end. The realm account stays enabled for the other products. Repeating the
// call on an inactive user finishes a cleanup that failed before.
type DeactivateUser struct{ statusDeps }

// NewDeactivateUser builds the use case.
func NewDeactivateUser(d UserStatusDeps) *DeactivateUser {
	return &DeactivateUser{newStatusDeps(d)}
}

// Execute deactivates the user and returns the updated row. When the provider
// fails it returns ErrIdentityProviderUnavailable, but the user is already blocked.
func (uc *DeactivateUser) Execute(ctx context.Context, in UserStatusInput) (UserListing, error) {
	target, err := uc.loadStatusTarget(ctx, in, "deactivate")
	if err != nil {
		return UserListing{}, err
	}
	if target.Status == domain.UserInvited {
		return UserListing{}, domain.Invalid("status", "convite pendente; reenvie ou cancele o convite")
	}
	if err := uc.cutAccess(ctx, in.Actor, target); err != nil {
		return UserListing{}, err
	}
	if err := uc.releaseProductAccess(ctx, target.ID); err != nil {
		return UserListing{}, err
	}
	return reloadListing(ctx, uc.lister, in.Actor, target.ID)
}

// ReactivateUser lets an inactive user back in. The provider goes first
// (rejoin the role group): if the database then fails the join is undone, and
// an account in the group without an active user is still denied by the database.
type ReactivateUser struct{ statusDeps }

// NewReactivateUser builds the use case.
func NewReactivateUser(d UserStatusDeps) *ReactivateUser {
	return &ReactivateUser{newStatusDeps(d)}
}

// Execute reactivates the user and returns the updated row. An already active
// user is returned as is.
func (uc *ReactivateUser) Execute(ctx context.Context, in UserStatusInput) (UserListing, error) {
	target, err := uc.loadStatusTarget(ctx, in, "reactivate")
	if err != nil {
		return UserListing{}, err
	}
	switch target.Status {
	case domain.UserInvited:
		return UserListing{}, domain.Invalid("status", "convite pendente; reenvie ou cancele o convite")
	case domain.UserActive:
		return reloadListing(ctx, uc.lister, in.Actor, target.ID)
	}
	company, err := loadCompany(ctx, uc.companies, target.CompanyID)
	if err != nil {
		return UserListing{}, err
	}
	if !company.AllowsAccess() {
		return UserListing{}, fmt.Errorf("%w: reactivate the company first", ErrCompanyInactive)
	}

	identity, linked, err := userIdentity(ctx, uc.identities, target.ID)
	if err != nil {
		return UserListing{}, err
	}
	if !linked {
		return UserListing{}, fmt.Errorf("user %s has no linked account: %w", target.ID, ErrNotFound)
	}
	if err := uc.access.AddToRoleGroup(ctx, identity.Subject, target.Role); err != nil {
		return UserListing{}, providerFailure("add to role group", err)
	}

	now := uc.clock.Now()
	updated := target
	if err := updated.Reactivate(); err != nil {
		return UserListing{}, err
	}
	err = uc.tx.InTx(ctx, func(ctx context.Context, tx Stores) error {
		if err := tx.Users.UpdateUser(ctx, updated); err != nil {
			return fmt.Errorf("reactivate user: %w", err)
		}
		event := actorEvent(in.Actor, now, target.CompanyID, domain.AuditUserReactivated, domain.AuditEntityUser, string(target.ID), nil)
		if err := tx.Audit.Record(ctx, event); err != nil {
			return fmt.Errorf("record audit: %w", err)
		}
		return nil
	})
	if err != nil {
		return UserListing{}, compensated(err, uc.access.RemoveFromProductGroups(ctx, identity.Subject))
	}
	return reloadListing(ctx, uc.lister, in.Actor, target.ID)
}

// DeleteUser soft deletes a user of the product (ADR 0015, D5). Order: cut the
// access in the database, release the product access in the provider, and only
// then mark the user deleted and free the identity link and the e-mail. If the
// provider fails the user is left inactive and repeating the call finishes it.
// The realm account is never deleted or disabled.
type DeleteUser struct{ statusDeps }

// NewDeleteUser builds the use case.
func NewDeleteUser(d UserStatusDeps) *DeleteUser {
	return &DeleteUser{newStatusDeps(d)}
}

// Execute deletes the user. A user with an open invitation is ErrInvitationPending.
func (uc *DeleteUser) Execute(ctx context.Context, in UserStatusInput) error {
	target, err := uc.loadStatusTarget(ctx, in, "delete")
	if err != nil {
		return err
	}
	if target.Status == domain.UserInvited {
		return fmt.Errorf("%w: cancel the invitation instead", ErrInvitationPending)
	}
	if err := uc.cutAccess(ctx, in.Actor, target); err != nil {
		return err
	}
	if err := uc.releaseProductAccess(ctx, target.ID); err != nil {
		return err
	}

	// Reload: cutAccess wrote the user, so the version we hold is stale.
	deleted, err := loadManagedUser(ctx, uc.users, in.Actor, target.ID)
	if err != nil {
		return err
	}
	now := uc.clock.Now()
	deleted.MarkDeleted(now)
	return uc.tx.InTx(ctx, func(ctx context.Context, tx Stores) error {
		if err := tx.Users.UpdateUser(ctx, deleted); err != nil {
			return fmt.Errorf("delete user: %w", err)
		}
		if err := tx.Identities.UnlinkIdentities(ctx, target.ID); err != nil {
			return fmt.Errorf("unlink identity: %w", err)
		}
		event := actorEvent(in.Actor, now, target.CompanyID, domain.AuditUserDeleted, domain.AuditEntityUser, string(target.ID), nil)
		if err := tx.Audit.Record(ctx, event); err != nil {
			return fmt.Errorf("record audit: %w", err)
		}
		return nil
	})
}

func reloadListing(ctx context.Context, lister UserLister, actor domain.Principal, id domain.UserID) (UserListing, error) {
	listing, err := lister.FindUserListing(ctx, ScopeOf(actor), id)
	if err != nil {
		return UserListing{}, fmt.Errorf("reload user: %w", err)
	}
	return listing, nil
}
