package httpadapter

import (
	"time"

	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/domain"
	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/usecase"
)

// Wire shapes of the contract (components.schemas). They are plain structs
// with explicit json tags, so the API never leaks a domain field by accident.

type companyRefBody struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type farmRefBody struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// farmScopeBody is FarmScope: {"mode":"all"} or {"mode":"selected","farm_ids":[...]}.
type farmScopeBody struct {
	Mode    string   `json:"mode"`
	FarmIDs []string `json:"farm_ids,omitempty"`
}

type invitationRefBody struct {
	ID        string    `json:"id"`
	ExpiresAt time.Time `json:"expires_at"`
}

type userAccountBody struct {
	ID           string             `json:"id"`
	Name         string             `json:"name"`
	Email        string             `json:"email"`
	Role         string             `json:"role"`
	Company      companyRefBody     `json:"company"`
	FarmScope    farmScopeBody      `json:"farm_scope"`
	Status       string             `json:"status"`
	LastAccessAt *time.Time         `json:"last_access_at"`
	Invitation   *invitationRefBody `json:"invitation"`
}

type pageBody[T any] struct {
	Items    []T `json:"items"`
	Total    int `json:"total"`
	Page     int `json:"page"`
	PageSize int `json:"page_size"`
}

type usersSummaryBody struct {
	TotalUsers         int            `json:"total_users"`
	CompanyCount       int            `json:"company_count"`
	PendingInvitations int            `json:"pending_invitations"`
	ByRole             map[string]int `json:"by_role"`
}

type companyBody struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	Document    string     `json:"document"`
	City        string     `json:"city"`
	State       string     `json:"state"`
	Plan        string     `json:"plan"`
	Status      string     `json:"status"`
	TrialEndsAt *time.Time `json:"trial_ends_at"`
	FarmCount   int        `json:"farm_count"`
	UserCount   int        `json:"user_count"`
	CreatedAt   time.Time  `json:"created_at"`
}

type companiesSummaryBody struct {
	ActiveCompanies           int     `json:"active_companies"`
	TrialCompanies            int     `json:"trial_companies"`
	FarmCount                 int     `json:"farm_count"`
	MonitoredAreaHa           float64 `json:"monitored_area_ha"`
	UserCount                 int     `json:"user_count"`
	PendingInvitations        int     `json:"pending_invitations"`
	ApplicationsThisMonth     int     `json:"applications_this_month"`
	ApplicationsPreviousMonth int     `json:"applications_previous_month"`
}

type currentUserBody struct {
	ID          string         `json:"id"`
	Name        string         `json:"name"`
	Email       string         `json:"email"`
	Role        string         `json:"role"`
	PrimaryFarm *farmRefBody   `json:"primary_farm"`
	Company     companyRefBody `json:"company"`
	FarmScope   farmScopeBody  `json:"farm_scope"`
	Permissions []string       `json:"permissions"`
}

type profileBody struct {
	ID        string         `json:"id"`
	Name      string         `json:"name"`
	Email     string         `json:"email"`
	Role      string         `json:"role"`
	Company   companyRefBody `json:"company"`
	FarmScope farmScopeBody  `json:"farm_scope"`
	// PasswordChangedAt is obsolete: the password belongs to Keycloak, so the
	// real API always answers null (contract, Profile).
	PasswordChangedAt *time.Time `json:"password_changed_at"`
}

func farmScopeOf(scope domain.FarmScope) farmScopeBody {
	body := farmScopeBody{Mode: string(scope.Mode())}
	for _, id := range scope.FarmIDs() {
		body.FarmIDs = append(body.FarmIDs, string(id))
	}
	return body
}

func utc(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	converted := value.UTC()
	return &converted
}

func userAccountOf(listing usecase.UserListing) userAccountBody {
	user := listing.User
	body := userAccountBody{
		ID:           string(user.ID),
		Name:         user.Name,
		Email:        user.Email.String(),
		Role:         string(user.Role),
		Company:      companyRefBody{ID: string(user.CompanyID), Name: listing.CompanyName},
		FarmScope:    farmScopeOf(user.FarmScope),
		Status:       string(user.Status),
		LastAccessAt: utc(user.LastAccessAt),
	}
	if listing.Invitation != nil {
		body.Invitation = &invitationRefBody{ID: string(listing.Invitation.ID), ExpiresAt: listing.Invitation.ExpiresAt.UTC()}
	}
	return body
}

func userPageOf(page usecase.UserPage) pageBody[userAccountBody] {
	items := make([]userAccountBody, 0, len(page.Items))
	for _, listing := range page.Items {
		items = append(items, userAccountOf(listing))
	}
	return pageBody[userAccountBody]{Items: items, Total: page.Total, Page: page.Page, PageSize: page.PageSize}
}

func usersSummaryOf(summary usecase.UsersSummary) usersSummaryBody {
	byRole := make(map[string]int, len(domain.Roles()))
	for _, role := range domain.Roles() {
		byRole[string(role)] = summary.ByRole[role]
	}
	return usersSummaryBody{
		TotalUsers:         summary.TotalUsers,
		CompanyCount:       summary.CompanyCount,
		PendingInvitations: summary.PendingInvitations,
		ByRole:             byRole,
	}
}

func companyOf(listing usecase.CompanyListing) companyBody {
	company := listing.Company
	return companyBody{
		ID:          string(company.ID),
		Name:        company.Name,
		Document:    string(company.Document),
		City:        company.City,
		State:       company.State,
		Plan:        string(company.Plan),
		Status:      string(company.Status),
		TrialEndsAt: utc(company.TrialEndsAt),
		FarmCount:   listing.FarmCount,
		UserCount:   listing.UserCount,
		CreatedAt:   company.CreatedAt.UTC(),
	}
}

func companyPageOf(page usecase.CompanyPage) pageBody[companyBody] {
	items := make([]companyBody, 0, len(page.Items))
	for _, listing := range page.Items {
		items = append(items, companyOf(listing))
	}
	return pageBody[companyBody]{Items: items, Total: page.Total, Page: page.Page, PageSize: page.PageSize}
}

func companiesSummaryOf(summary usecase.CompaniesSummary) companiesSummaryBody {
	return companiesSummaryBody(summary)
}

func currentUserOf(me usecase.Me) currentUserBody {
	body := currentUserBody{
		ID:          string(me.ID),
		Name:        me.Name,
		Email:       me.Email.String(),
		Role:        string(me.Role),
		Company:     companyRefBody{ID: string(me.Company.ID), Name: me.Company.Name},
		FarmScope:   farmScopeOf(me.FarmScope),
		Permissions: make([]string, 0, len(me.Permissions)),
	}
	for _, permission := range me.Permissions {
		body.Permissions = append(body.Permissions, string(permission))
	}
	if me.PrimaryFarm != nil {
		body.PrimaryFarm = &farmRefBody{ID: string(me.PrimaryFarm.ID), Name: me.PrimaryFarm.Name}
	}
	return body
}

func profileOf(profile usecase.Profile) profileBody {
	return profileBody{
		ID:        string(profile.ID),
		Name:      profile.Name,
		Email:     profile.Email.String(),
		Role:      string(profile.Role),
		Company:   companyRefBody{ID: string(profile.Company.ID), Name: profile.Company.Name},
		FarmScope: farmScopeOf(profile.FarmScope),
	}
}

// ---- inputs --------------------------------------------------------------------------

// farmScopeInput is the request form of FarmScope.
type farmScopeInput struct {
	Mode    string   `json:"mode"`
	FarmIDs []string `json:"farm_ids"`
}

// parse validates the shape (ids are UUIDs) and builds the domain scope.
func (in farmScopeInput) parse() (domain.FarmScope, error) {
	ids := make([]string, len(in.FarmIDs))
	for i, id := range in.FarmIDs {
		canonical, ok := canonicalUUID(id)
		if !ok {
			return domain.FarmScope{}, domain.Invalid("farm_scope.farm_ids", "identificador de fazenda inválido")
		}
		ids[i] = canonical
	}
	return domain.ParseFarmScope(in.Mode, ids)
}
