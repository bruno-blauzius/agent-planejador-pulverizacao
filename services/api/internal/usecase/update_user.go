package usecase

import (
	"context"
	"fmt"
	"time"

	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/domain"
)

// UpdateUserInput is the request of PATCH /users/{userId} without the status,
// which has its own use cases (DeactivateUser, ReactivateUser). Nil fields are
// left unchanged.
type UpdateUserInput struct {
	Actor     domain.Principal
	UserID    domain.UserID
	Name      *string
	Role      *domain.Role
	CompanyID *domain.CompanyID
	FarmScope *domain.FarmScope
}

// UpdateUser changes name, role, company or farm scope of a user.
//
// A role change (ADR 0015, D5.2) first moves the account between groups in the
// provider, then writes the projection, the audit and the session renewal in
// one transaction; if the database fails the move is reverted. A failure of
// the provider leaves the database untouched.
type UpdateUser struct {
	users      UserFinder
	companies  CompanyFinder
	farms      FarmDirectory
	identities IdentityStore
	access     ProductAccess
	lister     UserLister
	roles      RoleCounter
	tx         TxRunner
	clock      Clock
}

// UpdateUserDeps groups the collaborators of UpdateUser.
type UpdateUserDeps struct {
	Users      UserFinder
	Companies  CompanyFinder
	Farms      FarmDirectory
	Identities IdentityStore
	Access     ProductAccess
	Lister     UserLister
	Roles      RoleCounter
	Tx         TxRunner
	Clock      Clock
}

// NewUpdateUser builds the use case.
func NewUpdateUser(d UpdateUserDeps) *UpdateUser {
	return &UpdateUser{
		users: d.Users, companies: d.Companies, farms: d.Farms, identities: d.Identities,
		access: d.Access, lister: d.Lister, roles: d.Roles, tx: d.Tx, clock: d.Clock,
	}
}

// Execute applies the change and returns the updated row.
func (uc *UpdateUser) Execute(ctx context.Context, in UpdateUserInput) (UserListing, error) {
	if err := requireCan(in.Actor, domain.PermUsersManage); err != nil {
		return UserListing{}, err
	}
	target, err := loadManagedUser(ctx, uc.users, in.Actor, in.UserID)
	if err != nil {
		return UserListing{}, err
	}
	plan, err := uc.plan(ctx, in, target)
	if err != nil {
		return UserListing{}, err
	}
	if plan.roleChanged {
		if err := guardLastSuperAdmin(ctx, uc.roles, target); err != nil {
			return UserListing{}, err
		}
	}
	if !plan.changed() {
		return uc.reload(ctx, in.Actor, target.ID)
	}

	// An inactive user is out of the product groups (deactivation removed it), so
	// its role is only a projection; reactivation puts it in the group of the
	// role it has by then.
	moveGroup := plan.roleChanged && target.Status != domain.UserInactive
	var subject string
	if moveGroup {
		identity, linked, err := userIdentity(ctx, uc.identities, target.ID)
		if err != nil {
			return UserListing{}, err
		}
		if !linked {
			return UserListing{}, fmt.Errorf("user %s has no linked account: %w", target.ID, ErrNotFound)
		}
		subject = identity.Subject
		if err := uc.access.MoveToRoleGroup(ctx, subject, target.Role, plan.updated.Role); err != nil {
			return UserListing{}, providerFailure("move to role group", err)
		}
	}

	now := uc.clock.Now()
	err = uc.tx.InTx(ctx, func(ctx context.Context, tx Stores) error {
		if err := tx.Users.UpdateUser(ctx, plan.updated); err != nil {
			return fmt.Errorf("update user: %w", err)
		}
		if plan.accessChanged() {
			if err := tx.Sessions.ExpireUserAccess(ctx, target.ID, now); err != nil {
				return fmt.Errorf("expire sessions: %w", err)
			}
		}
		for _, event := range plan.events(in.Actor, now, target) {
			if err := tx.Audit.Record(ctx, event); err != nil {
				return fmt.Errorf("record audit: %w", err)
			}
		}
		return nil
	})
	if err != nil {
		if moveGroup {
			err = compensated(err, uc.access.MoveToRoleGroup(ctx, subject, plan.updated.Role, target.Role))
		}
		return UserListing{}, err
	}
	return uc.reload(ctx, in.Actor, target.ID)
}

func (uc *UpdateUser) reload(ctx context.Context, actor domain.Principal, id domain.UserID) (UserListing, error) {
	listing, err := uc.lister.FindUserListing(ctx, ScopeOf(actor), id)
	if err != nil {
		return UserListing{}, fmt.Errorf("reload user: %w", err)
	}
	return listing, nil
}

// updatePlan is the validated result of an UpdateUserInput.
type updatePlan struct {
	updated        domain.User
	renamed        bool
	roleChanged    bool
	scopeChanged   bool
	companyChanged bool
}

func (p updatePlan) changed() bool { return p.renamed || p.accessChanged() }

// accessChanged tells whether sessions must renew their token.
func (p updatePlan) accessChanged() bool { return p.roleChanged || p.scopeChanged || p.companyChanged }

func (p updatePlan) events(actor domain.Principal, now time.Time, before domain.User) []domain.AuditEvent {
	id := string(before.ID)
	var events []domain.AuditEvent
	add := func(action string, companyID domain.CompanyID, metadata map[string]string) {
		events = append(events, actorEvent(actor, now, companyID, action, domain.AuditEntityUser, id, metadata))
	}
	if p.renamed {
		add(domain.AuditUserRenamed, p.updated.CompanyID, nil)
	}
	if p.roleChanged {
		add(domain.AuditUserRoleChanged, p.updated.CompanyID, map[string]string{"from": string(before.Role), "to": string(p.updated.Role)})
	}
	if p.scopeChanged {
		add(domain.AuditUserScopeChanged, p.updated.CompanyID, map[string]string{
			"from": string(before.FarmScope.Mode()), "to": string(p.updated.FarmScope.Mode()),
		})
	}
	if p.companyChanged {
		add(domain.AuditUserCompanyChanged, p.updated.CompanyID, map[string]string{
			"from_company_id": string(before.CompanyID), "to_company_id": string(p.updated.CompanyID),
		})
	}
	return events
}

// plan validates the change against the actor's powers and the domain
// invariants without touching anything.
func (uc *UpdateUser) plan(ctx context.Context, in UpdateUserInput, target domain.User) (updatePlan, error) {
	role := target.Role
	if in.Role != nil {
		role = *in.Role
	}
	companyID := target.CompanyID
	if in.CompanyID != nil {
		companyID = *in.CompanyID
	}
	if err := authorizeAssignment(in.Actor, role, companyID); err != nil {
		return updatePlan{}, err
	}
	if target.ID == in.Actor.UserID && (role != target.Role || companyID != target.CompanyID) {
		return updatePlan{}, fmt.Errorf("%w: you cannot change your own role or company", ErrForbidden)
	}

	plan := updatePlan{updated: target}
	if in.Name != nil {
		if err := plan.updated.Rename(*in.Name); err != nil {
			return updatePlan{}, err
		}
		plan.renamed = plan.updated.Name != target.Name
	}

	company, err := uc.targetCompany(ctx, target, companyID)
	if err != nil {
		return updatePlan{}, err
	}
	scope := nextScope(in, target, role, companyID)
	if err := checkSelectedFarms(ctx, uc.farms, company.ID, scope); err != nil {
		return updatePlan{}, err
	}
	if err := plan.updated.Assign(company, role, scope); err != nil {
		return updatePlan{}, err
	}
	plan.roleChanged = role != target.Role
	plan.companyChanged = companyID != target.CompanyID
	plan.scopeChanged = !scope.Equal(target.FarmScope)
	return plan, nil
}

// targetCompany loads the company the user will belong to. Moving into an
// inactive company is refused; staying in one is allowed so a super admin can
// still edit its users.
func (uc *UpdateUser) targetCompany(ctx context.Context, target domain.User, companyID domain.CompanyID) (domain.Company, error) {
	company, err := companyForInput(ctx, uc.companies, companyID)
	if err != nil {
		return domain.Company{}, err
	}
	if companyID != target.CompanyID && !company.AllowsAccess() {
		return domain.Company{}, fmt.Errorf("%w: cannot move users into %q", ErrCompanyInactive, company.Name)
	}
	return company, nil
}

// nextScope decides the farm scope after the update: the one sent by the
// client, or all farms when the user moves company or stops being an operator
// (a selection of farms no longer applies), otherwise the current one.
func nextScope(in UpdateUserInput, target domain.User, role domain.Role, companyID domain.CompanyID) domain.FarmScope {
	switch {
	case in.FarmScope != nil:
		return *in.FarmScope
	case companyID != target.CompanyID:
		return domain.AllFarms()
	case role != domain.RolePlanningOperator && target.FarmScope.IsSelected():
		return domain.AllFarms()
	default:
		return target.FarmScope
	}
}
