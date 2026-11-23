package usecase

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/domain"
)

// requireCan fails with ErrForbidden unless the actor holds the permission.
func requireCan(actor domain.Principal, permission domain.Permission) error {
	if !actor.Can(permission) {
		return fmt.Errorf("%w: requires %s", ErrForbidden, permission)
	}
	return nil
}

// authorizeTarget applies the rules about who may manage whom: only a caller
// that manages every company touches a super admin.
func authorizeTarget(actor domain.Principal, target domain.User) error {
	if target.Role == domain.RoleSuperAdmin && !actor.ManagesAllCompanies() {
		return fmt.Errorf("%w: only a super admin manages this role", ErrForbidden)
	}
	return nil
}

// authorizeAssignment checks the role and company an actor may hand out:
// administrators neither grant super admin nor move people between companies.
func authorizeAssignment(actor domain.Principal, role domain.Role, companyID domain.CompanyID) error {
	if actor.ManagesAllCompanies() {
		return nil
	}
	if role == domain.RoleSuperAdmin {
		return fmt.Errorf("%w: only a super admin grants this role", ErrForbidden)
	}
	if companyID != actor.CompanyID {
		return fmt.Errorf("%w: users of other companies are out of reach", ErrForbidden)
	}
	return nil
}

// loadManagedUser finds a user inside the actor's scope and checks the actor
// may manage it. A user of another company is ErrNotFound, never ErrForbidden.
func loadManagedUser(ctx context.Context, users UserFinder, actor domain.Principal, id domain.UserID) (domain.User, error) {
	scope := ScopeOf(actor)
	user, err := users.FindUser(ctx, scope, id)
	if err != nil {
		return domain.User{}, fmt.Errorf("find user: %w", err)
	}
	if !scope.Includes(user.CompanyID) || user.IsDeleted() {
		return domain.User{}, fmt.Errorf("find user: %w", ErrNotFound)
	}
	if err := authorizeTarget(actor, user); err != nil {
		return domain.User{}, err
	}
	return user, nil
}

// loadCompany finds a company, translating a missing one for a field of the input.
func loadCompany(ctx context.Context, companies CompanyFinder, id domain.CompanyID) (domain.Company, error) {
	company, err := companies.FindCompany(ctx, id)
	if err != nil {
		return domain.Company{}, fmt.Errorf("find company: %w", err)
	}
	if company.DeletedAt != nil {
		return domain.Company{}, fmt.Errorf("find company: %w", ErrNotFound)
	}
	return company, nil
}

// companyForInput loads the company named by client input. A missing company
// becomes a validation error on company_id instead of a 404.
func companyForInput(ctx context.Context, companies CompanyFinder, id domain.CompanyID) (domain.Company, error) {
	company, err := loadCompany(ctx, companies, id)
	if errors.Is(err, ErrNotFound) {
		return domain.Company{}, domain.Invalid("company_id", "empresa não encontrada")
	}
	return company, err
}

// actorEvent builds an audit event whose actor is the calling user.
func actorEvent(actor domain.Principal, now time.Time, companyID domain.CompanyID, action, entityType, entityID string, metadata map[string]string) domain.AuditEvent {
	id := actor.UserID
	return domain.AuditEvent{
		CompanyID:   companyID,
		OccurredAt:  now,
		ActorKind:   domain.ActorUser,
		ActorUserID: &id,
		ActorName:   actor.Name,
		Action:      action,
		EntityType:  entityType,
		EntityID:    entityID,
		Metadata:    metadata,
	}
}

// userIdentity returns the linked account of a user, if any. A user without a
// link has nothing to clean up in the provider.
func userIdentity(ctx context.Context, identities IdentityStore, userID domain.UserID) (domain.Identity, bool, error) {
	identity, err := identities.FindIdentityByUser(ctx, userID)
	if errors.Is(err, ErrNotFound) {
		return domain.Identity{}, false, nil
	}
	if err != nil {
		return domain.Identity{}, false, fmt.Errorf("find identity: %w", err)
	}
	return identity, true, nil
}

// checkSelectedFarms verifies that the farms of a selected scope belong to the
// company and reports a mismatch as a validation error on the field.
func checkSelectedFarms(ctx context.Context, farms FarmDirectory, companyID domain.CompanyID, scope domain.FarmScope) error {
	if !scope.IsSelected() {
		return nil
	}
	err := farms.CheckFarmsInCompany(ctx, companyID, scope.FarmIDs())
	if errors.Is(err, ErrFarmNotInCompany) {
		return domain.Invalid("farm_scope.farm_ids", "fazenda não pertence à empresa")
	}
	if err != nil {
		return fmt.Errorf("check farms: %w", err)
	}
	return nil
}

// guardLastSuperAdmin refuses to remove the last active super admin: the
// platform could no longer manage companies. Only an active super admin counts.
// Adapters should serialize concurrent calls (the check is not atomic with the write).
func guardLastSuperAdmin(ctx context.Context, roles RoleCounter, target domain.User) error {
	if target.Role != domain.RoleSuperAdmin || target.Status != domain.UserActive {
		return nil
	}
	count, err := roles.CountActiveUsersByRole(ctx, domain.RoleSuperAdmin)
	if err != nil {
		return fmt.Errorf("count super admins: %w", err)
	}
	if count <= 1 {
		return ErrLastSuperAdmin
	}
	return nil
}
