package usecase

import (
	"context"
	"fmt"

	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/domain"
)

// ListUsersInput is the request of GET /users. CompanyID is honored only for
// callers that manage every company; everyone else always sees their own.
type ListUsersInput struct {
	Actor     domain.Principal
	Search    string
	Role      domain.Role
	Status    domain.UserStatus
	CompanyID domain.CompanyID
	Page      int
	PageSize  int
}

// ListUsers lists users and pending invitations visible to the actor.
type ListUsers struct {
	users    UserLister
	settings Settings
}

// NewListUsers builds the use case.
func NewListUsers(users UserLister, settings Settings) *ListUsers {
	return &ListUsers{users: users, settings: settings}
}

// Execute returns one page of users in the actor's scope.
func (uc *ListUsers) Execute(ctx context.Context, in ListUsersInput) (UserPage, error) {
	if err := requireCan(in.Actor, domain.PermUsersManage); err != nil {
		return UserPage{}, err
	}
	var problems domain.Violations
	if in.Role != "" && !in.Role.Valid() {
		problems.Add("role", "papel desconhecido")
	}
	if in.Status != "" && !in.Status.Valid() {
		problems.Add("status", "status desconhecido")
	}
	page, err := uc.settings.pageRequest(in.Page, in.PageSize)
	if mergeErr := problems.Merge(err); mergeErr != nil {
		return UserPage{}, mergeErr
	}
	if err := problems.Err(); err != nil {
		return UserPage{}, err
	}

	scope := ScopeOf(in.Actor).Narrow(in.CompanyID)
	result, err := uc.users.ListUsers(ctx, scope, UserFilter{
		Search: in.Search,
		Role:   in.Role,
		Status: in.Status,
	}, page)
	if err != nil {
		return UserPage{}, fmt.Errorf("list users: %w", err)
	}
	return result, nil
}

// UsersSummaryInput is the request of GET /users/summary.
type UsersSummaryInput struct {
	Actor     domain.Principal
	CompanyID domain.CompanyID
}

// GetUsersSummary counts users by role in the actor's scope.
type GetUsersSummary struct {
	users UserLister
}

// NewGetUsersSummary builds the use case.
func NewGetUsersSummary(users UserLister) *GetUsersSummary {
	return &GetUsersSummary{users: users}
}

// Execute returns the summary of the actor's scope.
func (uc *GetUsersSummary) Execute(ctx context.Context, in UsersSummaryInput) (UsersSummary, error) {
	if err := requireCan(in.Actor, domain.PermUsersManage); err != nil {
		return UsersSummary{}, err
	}
	summary, err := uc.users.SummarizeUsers(ctx, ScopeOf(in.Actor).Narrow(in.CompanyID))
	if err != nil {
		return UsersSummary{}, fmt.Errorf("summarize users: %w", err)
	}
	if summary.ByRole == nil {
		summary.ByRole = make(map[domain.Role]int, len(domain.Roles()))
	}
	for _, role := range domain.Roles() {
		if _, ok := summary.ByRole[role]; !ok {
			summary.ByRole[role] = 0 // the response always lists every role
		}
	}
	return summary, nil
}

// GetUserInput is the request of GET /users/{userId}.
type GetUserInput struct {
	Actor  domain.Principal
	UserID domain.UserID
}

// GetUser returns one user or pending invitation of the actor's scope.
type GetUser struct {
	users UserLister
}

// NewGetUser builds the use case.
func NewGetUser(users UserLister) *GetUser {
	return &GetUser{users: users}
}

// Execute returns the user, or ErrNotFound outside the scope.
func (uc *GetUser) Execute(ctx context.Context, in GetUserInput) (UserListing, error) {
	if err := requireCan(in.Actor, domain.PermUsersManage); err != nil {
		return UserListing{}, err
	}
	scope := ScopeOf(in.Actor)
	listing, err := uc.users.FindUserListing(ctx, scope, in.UserID)
	if err != nil {
		return UserListing{}, fmt.Errorf("find user: %w", err)
	}
	if !scope.Includes(listing.User.CompanyID) {
		return UserListing{}, fmt.Errorf("find user: %w", ErrNotFound)
	}
	return listing, nil
}
