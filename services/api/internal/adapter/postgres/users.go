package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/adapter/postgres/sqlcgen"
	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/domain"
	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/usecase"
)

// lastSuperAdminLockKey is the transaction-level advisory lock that serializes
// every write that may remove an active super admin. The number is arbitrary
// but fixed ("AGRO" in ASCII); nothing else in the database uses it.
const lastSuperAdminLockKey int64 = 0x4147524F

// ---- reads ---------------------------------------------------------------------

// FindUser implements usecase.UserFinder.
func (r *Repository) FindUser(ctx context.Context, scope usecase.Scope, id domain.UserID) (domain.User, error) {
	company, ok := scopeCompany(scope)
	if !ok {
		return domain.User{}, fmt.Errorf("find user: %w", usecase.ErrNotFound)
	}
	row, err := r.q.GetUser(ctx, sqlcgen.GetUserParams{ID: string(id), CompanyID: company})
	if err != nil {
		return domain.User{}, translateRead("find user", err)
	}
	return r.hydrateUser(ctx, row)
}

// FindUserByIdentity implements usecase.UserFinder. It is not tenant scoped: the
// company is a property of the user found (login happens before it is known).
func (r *Repository) FindUserByIdentity(ctx context.Context, issuer, subject string) (domain.User, error) {
	row, err := r.q.GetUserByIdentity(ctx, sqlcgen.GetUserByIdentityParams{Issuer: issuer, Subject: subject})
	if err != nil {
		return domain.User{}, translateRead("find user by identity", err)
	}
	return r.hydrateUser(ctx, row)
}

// FindUserByEmail implements usecase.UserFinder.
func (r *Repository) FindUserByEmail(ctx context.Context, email domain.Email) (domain.User, error) {
	row, err := r.q.GetUserByEmail(ctx, email.String())
	if err != nil {
		return domain.User{}, translateRead("find user by e-mail", err)
	}
	return r.hydrateUser(ctx, row)
}

func (r *Repository) hydrateUser(ctx context.Context, row sqlcgen.User) (domain.User, error) {
	farms, err := r.farmIDsOf(ctx, []sqlcgen.User{row})
	if err != nil {
		return domain.User{}, err
	}
	return userFromRow(row, farms[row.ID])
}

// farmIDsOf loads the selected farms of the users in one query. Users with
// scope "all" are skipped: they have no rows by construction.
func (r *Repository) farmIDsOf(ctx context.Context, users []sqlcgen.User) (map[string][]string, error) {
	var selected []string
	for _, user := range users {
		if domain.ScopeMode(user.FarmScopeMode) == domain.ScopeSelected {
			selected = append(selected, user.ID)
		}
	}
	farms := make(map[string][]string, len(selected))
	if len(selected) == 0 {
		return farms, nil
	}
	rows, err := r.q.ListFarmIDsByUsers(ctx, selected)
	if err != nil {
		return nil, fmt.Errorf("list farm access: %w", err)
	}
	for _, row := range rows {
		farms[row.UserID] = append(farms[row.UserID], row.FarmID)
	}
	return farms, nil
}

func userFromRow(row sqlcgen.User, farmIDs []string) (domain.User, error) {
	scope, err := domain.ParseFarmScope(row.FarmScopeMode, farmIDs)
	if err != nil {
		return domain.User{}, fmt.Errorf("user %s has an inconsistent farm scope: %w", row.ID, err)
	}
	user := domain.User{
		ID:           domain.UserID(row.ID),
		CompanyID:    domain.CompanyID(row.CompanyID),
		Name:         row.Name,
		Email:        domain.Email(row.Email),
		Role:         domain.Role(row.Role),
		Status:       domain.UserStatus(row.Status),
		FarmScope:    scope,
		LastAccessAt: row.LastAccessAt,
		CreatedAt:    row.CreatedAt,
		DeletedAt:    row.DeletedAt,
		Version:      int(row.Version),
	}
	if row.PrimaryFarmID != nil {
		primary := domain.FarmID(*row.PrimaryFarmID)
		user.PrimaryFarmID = &primary
	}
	return user, nil
}

// ---- writes --------------------------------------------------------------------

// InsertUser implements usecase.UserWriter. A zero Version is stored as 1.
func (r *Repository) InsertUser(ctx context.Context, user domain.User) error {
	createdAt := user.CreatedAt
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}
	farmIDs := farmIDStrings(user.FarmScope)
	err := r.atomic(ctx, func(q *sqlcgen.Queries) error {
		err := q.InsertUser(ctx, sqlcgen.InsertUserParams{
			ID:            string(user.ID),
			CompanyID:     string(user.CompanyID),
			Name:          user.Name,
			Email:         user.Email.String(),
			Role:          string(user.Role),
			Status:        string(user.Status),
			FarmScopeMode: string(user.FarmScope.Mode()),
			PrimaryFarmID: farmIDPtr(user.PrimaryFarmID),
			LastAccessAt:  user.LastAccessAt,
			DeletedAt:     user.DeletedAt,
			CreatedAt:     createdAt,
			Version:       normalizeVersion(user.Version),
		})
		if err != nil {
			return translateWrite("insert user", err)
		}
		return insertFarmAccess(ctx, q, user, farmIDs)
	})
	if err != nil {
		return fmt.Errorf("insert user: %w", err)
	}
	return nil
}

// UpdateUser implements usecase.UserWriter. It locks the stored row, checks the
// optimistic version, refuses to remove the last active super admin, and keeps
// user_farm_access in step with the scope, all in one transaction.
func (r *Repository) UpdateUser(ctx context.Context, user domain.User) error {
	farmIDs := farmIDStrings(user.FarmScope)
	err := r.atomic(ctx, func(q *sqlcgen.Queries) error {
		stored, err := q.LockUser(ctx, string(user.ID))
		if err != nil {
			return translateRead("lock user", err)
		}
		if stored.DeletedAt != nil {
			return usecase.ErrNotFound
		}
		if stored.Version != normalizeVersion(user.Version) {
			return usecase.ErrVersionConflict
		}
		if removesActiveSuperAdmin(stored, user) {
			if err := guardLastSuperAdmin(ctx, q, user.ID); err != nil {
				return err
			}
		}
		// Stale access rows go first: the composite FK has no ON UPDATE CASCADE, so a
		// company change would otherwise be refused.
		err = q.DeleteStaleUserFarmAccess(ctx, sqlcgen.DeleteStaleUserFarmAccessParams{
			UserID: string(user.ID), CompanyID: string(user.CompanyID), FarmIds: farmIDs,
		})
		if err != nil {
			return translateWrite("clear farm access", err)
		}
		updated, err := q.UpdateUser(ctx, sqlcgen.UpdateUserParams{
			ID:            string(user.ID),
			CompanyID:     string(user.CompanyID),
			Name:          user.Name,
			Email:         user.Email.String(),
			Role:          string(user.Role),
			Status:        string(user.Status),
			FarmScopeMode: string(user.FarmScope.Mode()),
			PrimaryFarmID: farmIDPtr(user.PrimaryFarmID),
			LastAccessAt:  user.LastAccessAt,
			DeletedAt:     user.DeletedAt,
			Version:       stored.Version,
		})
		if err != nil {
			return translateWrite("update user", err)
		}
		if updated == 0 {
			return usecase.ErrVersionConflict
		}
		return insertFarmAccess(ctx, q, user, farmIDs)
	})
	if err != nil {
		return fmt.Errorf("update user: %w", err)
	}
	return nil
}

func insertFarmAccess(ctx context.Context, q *sqlcgen.Queries, user domain.User, farmIDs []string) error {
	if len(farmIDs) == 0 {
		return nil
	}
	err := q.InsertUserFarmAccess(ctx, sqlcgen.InsertUserFarmAccessParams{
		UserID: string(user.ID), CompanyID: string(user.CompanyID), FarmIds: farmIDs,
	})
	return translateWrite("grant farm access", err)
}

// removesActiveSuperAdmin is true when the write turns an active super admin
// into something else (other role, inactive or deleted).
func removesActiveSuperAdmin(stored sqlcgen.LockUserRow, next domain.User) bool {
	wasActiveSuperAdmin := domain.Role(stored.Role) == domain.RoleSuperAdmin &&
		domain.UserStatus(stored.Status) == domain.UserActive
	staysActiveSuperAdmin := next.Role == domain.RoleSuperAdmin &&
		next.Status == domain.UserActive && next.DeletedAt == nil
	return wasActiveSuperAdmin && !staysActiveSuperAdmin
}

// guardLastSuperAdmin takes the advisory lock and then counts the OTHER active
// super admins with a new statement, so a concurrent writer that committed
// first is visible (READ COMMITTED). Two simultaneous removals of the last two
// super admins therefore end with exactly one success.
func guardLastSuperAdmin(ctx context.Context, q *sqlcgen.Queries, userID domain.UserID) error {
	if err := q.LockSuperAdminGuard(ctx, lastSuperAdminLockKey); err != nil {
		return fmt.Errorf("lock super admin guard: %w", err)
	}
	others, err := q.CountOtherActiveUsersByRole(ctx, sqlcgen.CountOtherActiveUsersByRoleParams{
		Role: string(domain.RoleSuperAdmin), ID: string(userID),
	})
	if err != nil {
		return fmt.Errorf("count other super admins: %w", err)
	}
	if others == 0 {
		return usecase.ErrLastSuperAdmin
	}
	return nil
}

func farmIDStrings(scope domain.FarmScope) []string {
	ids := scope.FarmIDs()
	out := make([]string, len(ids))
	for i, id := range ids {
		out[i] = string(id)
	}
	return out
}

func farmIDPtr(id *domain.FarmID) *string {
	if id == nil {
		return nil
	}
	value := string(*id)
	return &value
}

// CountActiveUsersByRole implements usecase.RoleCounter. The atomic protection
// of the last super admin lives in UpdateUser, not here: a count alone cannot
// be made safe across the use case's later write.
func (r *Repository) CountActiveUsersByRole(ctx context.Context, role domain.Role) (int, error) {
	count, err := r.q.CountActiveUsersByRole(ctx, string(role))
	if err != nil {
		return 0, fmt.Errorf("count active users by role: %w", err)
	}
	return int(count), nil
}

// ---- listings ------------------------------------------------------------------

// ListUsers implements usecase.UserLister: filters, search (name, e-mail or
// company), paging and the open invitation of invited users.
func (r *Repository) ListUsers(ctx context.Context, scope usecase.Scope, filter usecase.UserFilter, page usecase.PageRequest) (usecase.UserPage, error) {
	result := usecase.UserPage{Items: []usecase.UserListing{}, PageRequest: page}
	scopeID, scopeOK := scopeCompany(scope)
	companyID, companyOK := optionalCompany(string(filter.CompanyID))
	if !scopeOK || !companyOK {
		return result, nil
	}
	limit, offset := pageBounds(page)
	search := likePattern(filter.Search)
	role, status := optionalText(string(filter.Role)), optionalText(string(filter.Status))

	total, err := r.q.CountListedUsers(ctx, sqlcgen.CountListedUsersParams{
		ScopeCompanyID: scopeID, CompanyID: companyID, Role: role, Status: status, Search: search,
	})
	if err != nil {
		return usecase.UserPage{}, fmt.Errorf("count users: %w", err)
	}
	result.Total = int(total)
	if total == 0 || int64(offset) >= int64(total) {
		return result, nil
	}
	rows, err := r.q.ListUsers(ctx, sqlcgen.ListUsersParams{
		ScopeCompanyID: scopeID, CompanyID: companyID, Role: role, Status: status, Search: search,
		PageLimit: limit, PageOffset: offset,
	})
	if err != nil {
		return usecase.UserPage{}, fmt.Errorf("list users: %w", err)
	}
	users := make([]sqlcgen.User, len(rows))
	for i, row := range rows {
		users[i] = row.User
	}
	farms, err := r.farmIDsOf(ctx, users)
	if err != nil {
		return usecase.UserPage{}, err
	}
	for _, row := range rows {
		user, err := userFromRow(row.User, farms[row.User.ID])
		if err != nil {
			return usecase.UserPage{}, err
		}
		result.Items = append(result.Items, usecase.UserListing{
			User:        user,
			CompanyName: row.CompanyName,
			Invitation:  invitationFromListing(row),
		})
	}
	return result, nil
}

func invitationFromListing(row sqlcgen.ListUsersRow) *domain.Invitation {
	if row.InvitationID == nil {
		return nil
	}
	invitation := domain.Invitation{
		ID:             domain.InvitationID(*row.InvitationID),
		AccountCreated: row.InvitationAccountCreated.Bool,
		SendCount:      int(row.InvitationSendCount.Int32),
		AcceptedAt:     row.InvitationAcceptedAt,
		CancelledAt:    row.InvitationCancelledAt,
	}
	if row.InvitationCompanyID != nil {
		invitation.CompanyID = domain.CompanyID(*row.InvitationCompanyID)
	}
	if row.InvitationUserID != nil {
		invitation.UserID = domain.UserID(*row.InvitationUserID)
	}
	if row.InvitationInvitedBy != nil {
		invitation.InvitedBy = domain.UserID(*row.InvitationInvitedBy)
	}
	if row.InvitationExpiresAt != nil {
		invitation.ExpiresAt = *row.InvitationExpiresAt
	}
	if row.InvitationLastSentAt != nil {
		invitation.LastSentAt = *row.InvitationLastSentAt
	}
	if row.InvitationCreatedAt != nil {
		invitation.CreatedAt = *row.InvitationCreatedAt
	}
	return &invitation
}

// FindUserListing implements usecase.UserLister.
func (r *Repository) FindUserListing(ctx context.Context, scope usecase.Scope, id domain.UserID) (usecase.UserListing, error) {
	user, err := r.FindUser(ctx, scope, id)
	if err != nil {
		return usecase.UserListing{}, err
	}
	name, err := r.q.GetCompanyName(ctx, string(user.CompanyID))
	if err != nil {
		return usecase.UserListing{}, translateRead("find company name", err)
	}
	listing := usecase.UserListing{User: user, CompanyName: name}
	if user.Status == domain.UserInvited {
		invitation, err := r.FindOpenInvitationByUser(ctx, user.ID)
		if err != nil && !errors.Is(err, usecase.ErrNotFound) {
			return usecase.UserListing{}, err
		}
		if err == nil {
			listing.Invitation = &invitation
		}
	}
	return listing, nil
}

// SummarizeUsers implements usecase.UserLister.
func (r *Repository) SummarizeUsers(ctx context.Context, scope usecase.Scope) (usecase.UsersSummary, error) {
	summary := usecase.UsersSummary{ByRole: map[domain.Role]int{}}
	company, ok := scopeCompany(scope)
	if !ok {
		return summary, nil
	}
	byRole, err := r.q.CountUsersByRole(ctx, company)
	if err != nil {
		return usecase.UsersSummary{}, fmt.Errorf("count users by role: %w", err)
	}
	for _, row := range byRole {
		summary.ByRole[domain.Role(row.Role)] = int(row.Total)
		summary.TotalUsers += int(row.Total)
	}
	companies, err := r.q.CountUserCompanies(ctx, company)
	if err != nil {
		return usecase.UsersSummary{}, fmt.Errorf("count user companies: %w", err)
	}
	pending, err := r.q.CountPendingInvitations(ctx, company)
	if err != nil {
		return usecase.UsersSummary{}, fmt.Errorf("count pending invitations: %w", err)
	}
	summary.CompanyCount, summary.PendingInvitations = int(companies), int(pending)
	return summary, nil
}

// ---- company members -------------------------------------------------------------

// ListCompanyIdentities implements usecase.CompanyMembers.
func (r *Repository) ListCompanyIdentities(ctx context.Context, companyID domain.CompanyID) ([]domain.Identity, error) {
	if !validUUID(string(companyID)) {
		return nil, nil
	}
	rows, err := r.q.ListCompanyIdentities(ctx, string(companyID))
	if err != nil {
		return nil, fmt.Errorf("list company identities: %w", err)
	}
	identities := make([]domain.Identity, len(rows))
	for i, row := range rows {
		identities[i] = domain.Identity{UserID: domain.UserID(row.UserID), Issuer: row.Issuer, Subject: row.Subject}
	}
	return identities, nil
}

// CountUsersByStatus implements usecase.CompanyMembers.
func (r *Repository) CountUsersByStatus(ctx context.Context, companyID domain.CompanyID) (map[domain.UserStatus]int, error) {
	counts := map[domain.UserStatus]int{}
	if !validUUID(string(companyID)) {
		return counts, nil
	}
	rows, err := r.q.CountUsersByStatus(ctx, string(companyID))
	if err != nil {
		return nil, fmt.Errorf("count users by status: %w", err)
	}
	for _, row := range rows {
		counts[domain.UserStatus(row.Status)] = int(row.Total)
	}
	return counts, nil
}

// ---- identities -------------------------------------------------------------------

// LinkIdentity implements usecase.IdentityStore. Linking the same (issuer,
// subject) to the same user again is a no-op; to another user it returns
// ErrIdentityAlreadyLinked.
func (r *Repository) LinkIdentity(ctx context.Context, identity domain.Identity) error {
	// A savepoint keeps the surrounding transaction usable when the user already
	// has another account linked for the issuer (unique violation).
	return r.atomic(ctx, func(q *sqlcgen.Queries) error {
		owner, err := q.LinkIdentity(ctx, sqlcgen.LinkIdentityParams{
			UserID: string(identity.UserID), Issuer: identity.Issuer, Subject: identity.Subject,
		})
		if err != nil {
			return translateWrite("link identity", err)
		}
		if owner != string(identity.UserID) {
			return fmt.Errorf("link identity: %w", usecase.ErrIdentityAlreadyLinked)
		}
		return nil
	})
}

// UnlinkIdentities implements usecase.IdentityStore.
func (r *Repository) UnlinkIdentities(ctx context.Context, userID domain.UserID) error {
	if !validUUID(string(userID)) {
		return nil
	}
	if err := r.q.UnlinkIdentities(ctx, string(userID)); err != nil {
		return fmt.Errorf("unlink identities: %w", err)
	}
	return nil
}

// FindIdentityByUser implements usecase.IdentityStore.
func (r *Repository) FindIdentityByUser(ctx context.Context, userID domain.UserID) (domain.Identity, error) {
	row, err := r.q.GetIdentityByUser(ctx, string(userID))
	if err != nil {
		return domain.Identity{}, translateRead("find identity", err)
	}
	return domain.Identity{UserID: domain.UserID(row.UserID), Issuer: row.Issuer, Subject: row.Subject}, nil
}
