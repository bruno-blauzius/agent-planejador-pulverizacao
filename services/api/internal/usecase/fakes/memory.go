// Package fakes provides in-memory implementations of the use case ports for
// tests (CLAUDE.md: fakes, not mocks of a library). They enforce the same
// contracts the real adapters must: soft delete, company scope, uniqueness,
// optimistic versions and transaction rollback.
package fakes

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/domain"
	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/usecase"
)

// Memory is an in-memory database implementing every storage port.
type Memory struct {
	Users       map[domain.UserID]domain.User
	Companies   map[domain.CompanyID]domain.Company
	Invitations map[domain.InvitationID]domain.Invitation
	Identities  []domain.Identity
	// Farms[companyID][farmID] = farm name.
	Farms map[domain.CompanyID]map[domain.FarmID]string

	// Session effects, in call order, as "<effect>:<id>".
	SessionLog []string
	Audit      []domain.AuditEvent

	failures map[string]error
}

// NewMemory returns an empty database.
func NewMemory() *Memory {
	return &Memory{
		Users:       map[domain.UserID]domain.User{},
		Companies:   map[domain.CompanyID]domain.Company{},
		Invitations: map[domain.InvitationID]domain.Invitation{},
		Farms:       map[domain.CompanyID]map[domain.FarmID]string{},
		failures:    map[string]error{},
	}
}

// Fail makes every call of the named operation return err until Heal.
func (m *Memory) Fail(operation string, err error) { m.failures[operation] = err }

// Heal removes the injected failure of an operation.
func (m *Memory) Heal(operation string) { delete(m.failures, operation) }

func (m *Memory) check(operation string) error {
	if err := m.failures[operation]; err != nil {
		return fmt.Errorf("%s: %w", operation, err)
	}
	return nil
}

// AddFarm registers a farm of a company.
func (m *Memory) AddFarm(companyID domain.CompanyID, id domain.FarmID, name string) {
	if m.Farms[companyID] == nil {
		m.Farms[companyID] = map[domain.FarmID]string{}
	}
	m.Farms[companyID][id] = name
}

// PutUser stores a user with its identity, for test setup.
func (m *Memory) PutUser(user domain.User, identity *domain.Identity) {
	m.Users[user.ID] = user
	if identity != nil {
		m.Identities = append(m.Identities, *identity)
	}
}

// AuditActions lists the recorded actions in order.
func (m *Memory) AuditActions() []string {
	actions := make([]string, len(m.Audit))
	for i, event := range m.Audit {
		actions[i] = event.Action
	}
	return actions
}

// ---- transactions ----------------------------------------------------------

type snapshot struct {
	users       map[domain.UserID]domain.User
	companies   map[domain.CompanyID]domain.Company
	invitations map[domain.InvitationID]domain.Invitation
	identities  []domain.Identity
	sessionLog  []string
	audit       []domain.AuditEvent
}

// InTx runs fn and rolls every change back when it fails.
func (m *Memory) InTx(ctx context.Context, fn func(ctx context.Context, tx usecase.Stores) error) error {
	if err := m.check("InTx"); err != nil {
		return err
	}
	saved := snapshot{
		users:       maps.Clone(m.Users),
		companies:   maps.Clone(m.Companies),
		invitations: maps.Clone(m.Invitations),
		identities:  slices.Clone(m.Identities),
		sessionLog:  slices.Clone(m.SessionLog),
		audit:       slices.Clone(m.Audit),
	}
	stores := usecase.Stores{Users: m, Companies: m, Invitations: m, Identities: m, Sessions: m, Audit: m}
	if err := fn(ctx, stores); err != nil {
		m.Users, m.Companies, m.Invitations = saved.users, saved.companies, saved.invitations
		m.Identities, m.SessionLog, m.Audit = saved.identities, saved.sessionLog, saved.audit
		return err
	}
	if err := m.check("Commit"); err != nil {
		m.Users, m.Companies, m.Invitations = saved.users, saved.companies, saved.invitations
		m.Identities, m.SessionLog, m.Audit = saved.identities, saved.sessionLog, saved.audit
		return err
	}
	return nil
}

// ---- users -----------------------------------------------------------------

func (m *Memory) visibleUser(scope usecase.Scope, id domain.UserID) (domain.User, bool) {
	user, ok := m.Users[id]
	if !ok || user.IsDeleted() || !scope.Includes(user.CompanyID) {
		return domain.User{}, false
	}
	return user, true
}

// FindUser implements usecase.UserFinder.
func (m *Memory) FindUser(_ context.Context, scope usecase.Scope, id domain.UserID) (domain.User, error) {
	if err := m.check("FindUser"); err != nil {
		return domain.User{}, err
	}
	user, ok := m.visibleUser(scope, id)
	if !ok {
		return domain.User{}, usecase.ErrNotFound
	}
	return user, nil
}

// FindUserByIdentity implements usecase.UserFinder.
func (m *Memory) FindUserByIdentity(_ context.Context, issuer, subject string) (domain.User, error) {
	if err := m.check("FindUserByIdentity"); err != nil {
		return domain.User{}, err
	}
	for _, identity := range m.Identities {
		if identity.Issuer == issuer && identity.Subject == subject {
			if user, ok := m.Users[identity.UserID]; ok && !user.IsDeleted() {
				return user, nil
			}
		}
	}
	return domain.User{}, usecase.ErrNotFound
}

// FindUserByEmail implements usecase.UserFinder.
func (m *Memory) FindUserByEmail(_ context.Context, email domain.Email) (domain.User, error) {
	if err := m.check("FindUserByEmail"); err != nil {
		return domain.User{}, err
	}
	for _, user := range m.Users {
		if user.Email == email && !user.IsDeleted() {
			return user, nil
		}
	}
	return domain.User{}, usecase.ErrNotFound
}

// InsertUser implements usecase.UserWriter.
func (m *Memory) InsertUser(_ context.Context, user domain.User) error {
	if err := m.check("InsertUser"); err != nil {
		return err
	}
	if err := checkDeletedInactive(user); err != nil {
		return err
	}
	for _, other := range m.Users {
		if other.Email == user.Email && !other.IsDeleted() {
			return usecase.ErrEmailAlreadyRegistered
		}
	}
	m.Users[user.ID] = user
	return nil
}

// UpdateUser implements usecase.UserWriter with optimistic concurrency.
func (m *Memory) UpdateUser(_ context.Context, user domain.User) error {
	if err := m.check("UpdateUser"); err != nil {
		return err
	}
	stored, ok := m.Users[user.ID]
	if !ok {
		return usecase.ErrNotFound
	}
	if stored.Version != user.Version {
		return usecase.ErrVersionConflict
	}
	if err := checkDeletedInactive(user); err != nil {
		return err
	}
	user.Version++
	m.Users[user.ID] = user
	return nil
}

// checkDeletedInactive mirrors ck_users_deleted_inactive (migration 000005): the
// database refuses a soft deleted user that is not inactive, so the fake must too.
func checkDeletedInactive(user domain.User) error {
	if user.IsDeleted() && user.Status != domain.UserInactive {
		return fmt.Errorf("ck_users_deleted_inactive: deleted user %s has status %q", user.ID, user.Status)
	}
	return nil
}

func (m *Memory) listing(user domain.User) usecase.UserListing {
	listing := usecase.UserListing{User: user, CompanyName: m.Companies[user.CompanyID].Name}
	if user.Status == domain.UserInvited {
		for _, invitation := range m.Invitations {
			if invitation.UserID == user.ID && invitation.IsOpen() {
				found := invitation
				listing.Invitation = &found
			}
		}
	}
	return listing
}

// FindUserListing implements usecase.UserLister.
func (m *Memory) FindUserListing(_ context.Context, scope usecase.Scope, id domain.UserID) (usecase.UserListing, error) {
	if err := m.check("FindUserListing"); err != nil {
		return usecase.UserListing{}, err
	}
	user, ok := m.visibleUser(scope, id)
	if !ok {
		return usecase.UserListing{}, usecase.ErrNotFound
	}
	return m.listing(user), nil
}

// ListUsers implements usecase.UserLister.
func (m *Memory) ListUsers(_ context.Context, scope usecase.Scope, filter usecase.UserFilter, page usecase.PageRequest) (usecase.UserPage, error) {
	if err := m.check("ListUsers"); err != nil {
		return usecase.UserPage{}, err
	}
	search := strings.ToLower(filter.Search)
	var rows []usecase.UserListing
	for _, user := range m.scopedUsers(scope) {
		row := m.listing(user)
		switch {
		case filter.Role != "" && user.Role != filter.Role,
			filter.Status != "" && user.Status != filter.Status,
			search != "" && !strings.Contains(strings.ToLower(user.Name+" "+user.Email.String()+" "+row.CompanyName), search):
			continue
		}
		rows = append(rows, row)
	}
	slices.SortFunc(rows, func(a, b usecase.UserListing) int {
		return strings.Compare(string(a.User.ID), string(b.User.ID))
	})
	total := len(rows)
	start := min((page.Page-1)*page.PageSize, total)
	end := min(start+page.PageSize, total)
	return usecase.UserPage{Items: rows[start:end], Total: total, PageRequest: page}, nil
}

func (m *Memory) scopedUsers(scope usecase.Scope) []domain.User {
	var users []domain.User
	for _, user := range m.Users {
		if !user.IsDeleted() && scope.Includes(user.CompanyID) {
			users = append(users, user)
		}
	}
	return users
}

// SummarizeUsers implements usecase.UserLister.
func (m *Memory) SummarizeUsers(_ context.Context, scope usecase.Scope) (usecase.UsersSummary, error) {
	if err := m.check("SummarizeUsers"); err != nil {
		return usecase.UsersSummary{}, err
	}
	summary := usecase.UsersSummary{ByRole: map[domain.Role]int{}}
	companies := map[domain.CompanyID]struct{}{}
	for _, user := range m.scopedUsers(scope) {
		summary.TotalUsers++
		summary.ByRole[user.Role]++
		companies[user.CompanyID] = struct{}{}
		if user.Status == domain.UserInvited {
			summary.PendingInvitations++
		}
	}
	summary.CompanyCount = len(companies)
	return summary, nil
}

// ListCompanyIdentities implements usecase.CompanyMembers.
func (m *Memory) ListCompanyIdentities(_ context.Context, companyID domain.CompanyID) ([]domain.Identity, error) {
	if err := m.check("ListCompanyIdentities"); err != nil {
		return nil, err
	}
	var identities []domain.Identity
	for _, identity := range m.Identities {
		if user, ok := m.Users[identity.UserID]; ok && user.CompanyID == companyID && !user.IsDeleted() {
			identities = append(identities, identity)
		}
	}
	return identities, nil
}

// CountUsersByStatus implements usecase.CompanyMembers.
func (m *Memory) CountUsersByStatus(_ context.Context, companyID domain.CompanyID) (map[domain.UserStatus]int, error) {
	if err := m.check("CountUsersByStatus"); err != nil {
		return nil, err
	}
	counts := map[domain.UserStatus]int{}
	for _, user := range m.scopedUsers(usecase.OnlyCompany(companyID)) {
		counts[user.Status]++
	}
	return counts, nil
}

// CountActiveUsersByRole implements usecase.RoleCounter.
func (m *Memory) CountActiveUsersByRole(_ context.Context, role domain.Role) (int, error) {
	if err := m.check("CountActiveUsersByRole"); err != nil {
		return 0, err
	}
	n := 0
	for _, user := range m.Users {
		if user.Role == role && user.Status == domain.UserActive && !user.IsDeleted() {
			n++
		}
	}
	return n, nil
}

// ---- identities --------------------------------------------------------------

// LinkIdentity implements usecase.IdentityStore.
func (m *Memory) LinkIdentity(_ context.Context, identity domain.Identity) error {
	if err := m.check("LinkIdentity"); err != nil {
		return err
	}
	for _, other := range m.Identities {
		if other.Issuer == identity.Issuer && other.Subject == identity.Subject {
			return usecase.ErrIdentityAlreadyLinked
		}
	}
	m.Identities = append(m.Identities, identity)
	return nil
}

// UnlinkIdentities implements usecase.IdentityStore.
func (m *Memory) UnlinkIdentities(_ context.Context, userID domain.UserID) error {
	if err := m.check("UnlinkIdentities"); err != nil {
		return err
	}
	m.Identities = slices.DeleteFunc(m.Identities, func(identity domain.Identity) bool { return identity.UserID == userID })
	return nil
}

// FindIdentityByUser implements usecase.IdentityStore.
func (m *Memory) FindIdentityByUser(_ context.Context, userID domain.UserID) (domain.Identity, error) {
	if err := m.check("FindIdentityByUser"); err != nil {
		return domain.Identity{}, err
	}
	for _, identity := range m.Identities {
		if identity.UserID == userID {
			return identity, nil
		}
	}
	return domain.Identity{}, usecase.ErrNotFound
}

// ---- invitations ----------------------------------------------------------------

// FindInvitation implements usecase.InvitationFinder.
func (m *Memory) FindInvitation(_ context.Context, scope usecase.Scope, id domain.InvitationID) (domain.Invitation, error) {
	if err := m.check("FindInvitation"); err != nil {
		return domain.Invitation{}, err
	}
	invitation, ok := m.Invitations[id]
	if !ok || !scope.Includes(invitation.CompanyID) {
		return domain.Invitation{}, usecase.ErrNotFound
	}
	if user, ok := m.Users[invitation.UserID]; ok && user.IsDeleted() {
		return domain.Invitation{}, usecase.ErrNotFound
	}
	return invitation, nil
}

// FindOpenInvitationByUser implements usecase.InvitationFinder.
func (m *Memory) FindOpenInvitationByUser(_ context.Context, userID domain.UserID) (domain.Invitation, error) {
	if err := m.check("FindOpenInvitationByUser"); err != nil {
		return domain.Invitation{}, err
	}
	for _, invitation := range m.Invitations {
		if invitation.UserID == userID && invitation.IsOpen() {
			return invitation, nil
		}
	}
	return domain.Invitation{}, usecase.ErrNotFound
}

// InsertInvitation implements usecase.InvitationWriter.
func (m *Memory) InsertInvitation(_ context.Context, invitation domain.Invitation) error {
	if err := m.check("InsertInvitation"); err != nil {
		return err
	}
	m.Invitations[invitation.ID] = invitation
	return nil
}

// UpdateInvitation implements usecase.InvitationWriter.
func (m *Memory) UpdateInvitation(_ context.Context, invitation domain.Invitation) error {
	if err := m.check("UpdateInvitation"); err != nil {
		return err
	}
	if _, ok := m.Invitations[invitation.ID]; !ok {
		return usecase.ErrNotFound
	}
	m.Invitations[invitation.ID] = invitation
	return nil
}

// ---- companies --------------------------------------------------------------------

// FindCompany implements usecase.CompanyFinder.
func (m *Memory) FindCompany(_ context.Context, id domain.CompanyID) (domain.Company, error) {
	if err := m.check("FindCompany"); err != nil {
		return domain.Company{}, err
	}
	company, ok := m.Companies[id]
	if !ok || company.DeletedAt != nil {
		return domain.Company{}, usecase.ErrNotFound
	}
	return company, nil
}

// LockCompany implements usecase.CompanyFinder.
func (m *Memory) LockCompany(ctx context.Context, id domain.CompanyID) (domain.Company, error) {
	return m.FindCompany(ctx, id)
}

// InsertCompany implements usecase.CompanyWriter.
func (m *Memory) InsertCompany(_ context.Context, company domain.Company) error {
	if err := m.check("InsertCompany"); err != nil {
		return err
	}
	for _, other := range m.Companies {
		if other.Document == company.Document {
			return usecase.ErrDocumentAlreadyRegistered
		}
	}
	m.Companies[company.ID] = company
	return nil
}

// UpdateCompany implements usecase.CompanyWriter.
func (m *Memory) UpdateCompany(_ context.Context, company domain.Company) error {
	if err := m.check("UpdateCompany"); err != nil {
		return err
	}
	stored, ok := m.Companies[company.ID]
	if !ok {
		return usecase.ErrNotFound
	}
	if stored.Version != company.Version {
		return usecase.ErrVersionConflict
	}
	// mirrors ck_companies_deleted (migration 000012)
	if company.DeletedAt != nil && (company.Status != domain.CompanyInactive || company.IsPlatformOperator) {
		return fmt.Errorf("ck_companies_deleted: deleted company %s has status %q", company.ID, company.Status)
	}
	company.Version++
	m.Companies[company.ID] = company
	return nil
}

func (m *Memory) companyListing(company domain.Company) usecase.CompanyListing {
	userCount := len(m.scopedUsers(usecase.OnlyCompany(company.ID)))
	return usecase.CompanyListing{Company: company, FarmCount: len(m.Farms[company.ID]), UserCount: userCount}
}

// FindCompanyListing implements usecase.CompanyLister.
func (m *Memory) FindCompanyListing(ctx context.Context, id domain.CompanyID) (usecase.CompanyListing, error) {
	company, err := m.FindCompany(ctx, id)
	if err != nil {
		return usecase.CompanyListing{}, err
	}
	return m.companyListing(company), nil
}

// ListCompanies implements usecase.CompanyLister; the operator never appears.
func (m *Memory) ListCompanies(_ context.Context, filter usecase.CompanyFilter, page usecase.PageRequest) (usecase.CompanyPage, error) {
	if err := m.check("ListCompanies"); err != nil {
		return usecase.CompanyPage{}, err
	}
	search := strings.ToLower(filter.Search)
	var rows []usecase.CompanyListing
	for _, company := range m.Companies {
		switch {
		case company.IsPlatformOperator, company.DeletedAt != nil,
			filter.Status != "" && company.Status != filter.Status,
			search != "" && !strings.Contains(strings.ToLower(company.Name+" "+company.City), search):
			continue
		}
		rows = append(rows, m.companyListing(company))
	}
	slices.SortFunc(rows, func(a, b usecase.CompanyListing) int {
		return strings.Compare(string(a.Company.ID), string(b.Company.ID))
	})
	total := len(rows)
	start := min((page.Page-1)*page.PageSize, total)
	end := min(start+page.PageSize, total)
	return usecase.CompanyPage{Items: rows[start:end], Total: total, PageRequest: page}, nil
}

// SummarizeCompanies implements usecase.CompanyLister.
func (m *Memory) SummarizeCompanies(_ context.Context) (usecase.CompaniesSummary, error) {
	if err := m.check("SummarizeCompanies"); err != nil {
		return usecase.CompaniesSummary{}, err
	}
	var summary usecase.CompaniesSummary
	for _, company := range m.Companies {
		if company.IsPlatformOperator || company.DeletedAt != nil {
			continue
		}
		if company.Status != domain.CompanyInactive {
			summary.ActiveCompanies++
		}
		if company.Status == domain.CompanyTrial {
			summary.TrialCompanies++
		}
		summary.FarmCount += len(m.Farms[company.ID])
	}
	return summary, nil
}

// ---- farms ------------------------------------------------------------------------------

// CheckFarmsInCompany implements usecase.FarmDirectory.
func (m *Memory) CheckFarmsInCompany(_ context.Context, companyID domain.CompanyID, ids []domain.FarmID) error {
	if err := m.check("CheckFarmsInCompany"); err != nil {
		return err
	}
	for _, id := range ids {
		if _, ok := m.Farms[companyID][id]; !ok {
			return usecase.ErrFarmNotInCompany
		}
	}
	return nil
}

// FindFarmRef implements usecase.FarmDirectory.
func (m *Memory) FindFarmRef(_ context.Context, companyID domain.CompanyID, id domain.FarmID) (usecase.FarmRef, error) {
	name, ok := m.Farms[companyID][id]
	if !ok {
		return usecase.FarmRef{}, usecase.ErrNotFound
	}
	return usecase.FarmRef{ID: id, Name: name}, nil
}

// ---- sessions and audit --------------------------------------------------------------------

// RevokeUserSessions implements usecase.SessionRevoker.
func (m *Memory) RevokeUserSessions(_ context.Context, userID domain.UserID, _ time.Time) error {
	if err := m.check("RevokeUserSessions"); err != nil {
		return err
	}
	m.SessionLog = append(m.SessionLog, "revoke_user:"+string(userID))
	return nil
}

// RevokeCompanySessions implements usecase.SessionRevoker.
func (m *Memory) RevokeCompanySessions(_ context.Context, companyID domain.CompanyID, _ time.Time) error {
	if err := m.check("RevokeCompanySessions"); err != nil {
		return err
	}
	m.SessionLog = append(m.SessionLog, "revoke_company:"+string(companyID))
	return nil
}

// ExpireUserAccess implements usecase.SessionRevoker.
func (m *Memory) ExpireUserAccess(_ context.Context, userID domain.UserID, _ time.Time) error {
	if err := m.check("ExpireUserAccess"); err != nil {
		return err
	}
	m.SessionLog = append(m.SessionLog, "expire_access:"+string(userID))
	return nil
}

// Record implements usecase.AuditRecorder.
func (m *Memory) Record(_ context.Context, event domain.AuditEvent) error {
	if err := m.check("Record"); err != nil {
		return err
	}
	m.Audit = append(m.Audit, event)
	return nil
}

// ErrInjected is a convenient error for failure injection.
var ErrInjected = errors.New("injected failure")
