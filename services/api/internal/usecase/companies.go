package usecase

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/domain"
)

// ListCompaniesInput is the request of GET /companies.
type ListCompaniesInput struct {
	Actor    domain.Principal
	Search   string
	Status   domain.CompanyStatus
	Page     int
	PageSize int
}

// ListCompanies lists the client companies (super admin only).
type ListCompanies struct {
	companies CompanyLister
	settings  Settings
}

// NewListCompanies builds the use case.
func NewListCompanies(companies CompanyLister, settings Settings) *ListCompanies {
	return &ListCompanies{companies: companies, settings: settings}
}

// Execute returns one page of companies.
func (uc *ListCompanies) Execute(ctx context.Context, in ListCompaniesInput) (CompanyPage, error) {
	if err := requireCan(in.Actor, domain.PermCompaniesManage); err != nil {
		return CompanyPage{}, err
	}
	var problems domain.Violations
	if in.Status != "" && !in.Status.Valid() {
		problems.Add("status", "status desconhecido")
	}
	page, err := uc.settings.pageRequest(in.Page, in.PageSize)
	if mergeErr := problems.Merge(err); mergeErr != nil {
		return CompanyPage{}, mergeErr
	}
	if err := problems.Err(); err != nil {
		return CompanyPage{}, err
	}
	result, err := uc.companies.ListCompanies(ctx, CompanyFilter{Search: in.Search, Status: in.Status}, page)
	if err != nil {
		return CompanyPage{}, fmt.Errorf("list companies: %w", err)
	}
	return result, nil
}

// GetCompaniesSummary returns the indicators of the companies screen.
type GetCompaniesSummary struct {
	companies CompanyLister
}

// NewGetCompaniesSummary builds the use case.
func NewGetCompaniesSummary(companies CompanyLister) *GetCompaniesSummary {
	return &GetCompaniesSummary{companies: companies}
}

// Execute returns the summary (super admin only).
func (uc *GetCompaniesSummary) Execute(ctx context.Context, actor domain.Principal) (CompaniesSummary, error) {
	if err := requireCan(actor, domain.PermCompaniesManage); err != nil {
		return CompaniesSummary{}, err
	}
	summary, err := uc.companies.SummarizeCompanies(ctx)
	if err != nil {
		return CompaniesSummary{}, fmt.Errorf("summarize companies: %w", err)
	}
	return summary, nil
}

// GetCompanyInput is the request of GET /companies/{companyId}.
type GetCompanyInput struct {
	Actor     domain.Principal
	CompanyID domain.CompanyID
}

// GetCompany returns one client company.
type GetCompany struct {
	companies CompanyLister
}

// NewGetCompany builds the use case.
func NewGetCompany(companies CompanyLister) *GetCompany {
	return &GetCompany{companies: companies}
}

// Execute returns the company. The platform operator company is ErrNotFound.
func (uc *GetCompany) Execute(ctx context.Context, in GetCompanyInput) (CompanyListing, error) {
	if err := requireCan(in.Actor, domain.PermCompaniesManage); err != nil {
		return CompanyListing{}, err
	}
	return findClientCompany(ctx, uc.companies, in.CompanyID)
}

// findClientCompany loads a client company; the operator company is invisible
// to the companies endpoints.
func findClientCompany(ctx context.Context, companies CompanyLister, id domain.CompanyID) (CompanyListing, error) {
	listing, err := companies.FindCompanyListing(ctx, id)
	if err != nil {
		return CompanyListing{}, fmt.Errorf("find company: %w", err)
	}
	if listing.Company.IsPlatformOperator || listing.Company.DeletedAt != nil {
		return CompanyListing{}, fmt.Errorf("find company: %w", ErrNotFound)
	}
	return listing, nil
}

// CreateCompanyInput is the request of POST /companies.
type CreateCompanyInput struct {
	Actor domain.Principal
	domain.NewCompanyInput
}

// CreateCompany registers a client company.
type CreateCompany struct {
	lister   CompanyLister
	tx       TxRunner
	clock    Clock
	ids      IDGenerator
	settings Settings
}

// NewCreateCompany builds the use case.
func NewCreateCompany(lister CompanyLister, tx TxRunner, clock Clock, ids IDGenerator, settings Settings) *CreateCompany {
	return &CreateCompany{lister: lister, tx: tx, clock: clock, ids: ids, settings: settings}
}

// Execute validates and stores the company. A duplicate CNPJ is ErrDocumentAlreadyRegistered.
func (uc *CreateCompany) Execute(ctx context.Context, in CreateCompanyInput) (CompanyListing, error) {
	if err := requireCan(in.Actor, domain.PermCompaniesManage); err != nil {
		return CompanyListing{}, err
	}
	now := uc.clock.Now()
	company, err := domain.NewCompany(domain.CompanyID(uc.ids.NewID()), in.NewCompanyInput, now, uc.settings.TrialDuration)
	if err != nil {
		return CompanyListing{}, err
	}
	err = uc.tx.InTx(ctx, func(ctx context.Context, tx Stores) error {
		if err := tx.Companies.InsertCompany(ctx, company); err != nil {
			return fmt.Errorf("insert company: %w", err)
		}
		event := actorEvent(in.Actor, now, company.ID, domain.AuditCompanyCreated, domain.AuditEntityCompany, string(company.ID),
			map[string]string{"plan": string(company.Plan)})
		if err := tx.Audit.Record(ctx, event); err != nil {
			return fmt.Errorf("record audit: %w", err)
		}
		return nil
	})
	if err != nil {
		return CompanyListing{}, err
	}
	return CompanyListing{Company: company}, nil
}

// UpdateCompanyInput is the request of PATCH /companies/{companyId}. Nil
// fields are left unchanged.
type UpdateCompanyInput struct {
	Actor     domain.Principal
	CompanyID domain.CompanyID
	Name      *string
	City      *string
	State     *string
	Plan      *domain.CompanyPlan
	Status    *domain.CompanyStatus
}

// UpdateCompany changes a client company. Deactivating it cuts every user's
// access at once: the access check reads the company on each request, the
// transaction revokes the API sessions, and then the provider ends the product
// sessions of each user. Accounts and group membership are untouched, so
// reactivating the company restores access. Repeating the call on an inactive
// company finishes a provider cleanup that failed before.
type UpdateCompany struct {
	finder   CompanyFinder
	lister   CompanyLister
	members  CompanyMembers
	access   ProductAccess
	tx       TxRunner
	clock    Clock
	settings Settings
}

// UpdateCompanyDeps groups the collaborators of UpdateCompany.
type UpdateCompanyDeps struct {
	Finder   CompanyFinder
	Lister   CompanyLister
	Members  CompanyMembers
	Access   ProductAccess
	Tx       TxRunner
	Clock    Clock
	Settings Settings
}

// NewUpdateCompany builds the use case.
func NewUpdateCompany(d UpdateCompanyDeps) *UpdateCompany {
	return &UpdateCompany{
		finder: d.Finder, lister: d.Lister, members: d.Members, access: d.Access,
		tx: d.Tx, clock: d.Clock, settings: d.Settings,
	}
}

// Execute applies the change and returns the company with its counts.
func (uc *UpdateCompany) Execute(ctx context.Context, in UpdateCompanyInput) (CompanyListing, error) {
	if err := requireCan(in.Actor, domain.PermCompaniesManage); err != nil {
		return CompanyListing{}, err
	}
	if _, err := findClientCompany(ctx, uc.lister, in.CompanyID); err != nil {
		return CompanyListing{}, err
	}

	now := uc.clock.Now()
	var before, after domain.Company
	err := uc.tx.InTx(ctx, func(ctx context.Context, tx Stores) error {
		locked, err := tx.Companies.LockCompany(ctx, in.CompanyID)
		if err != nil {
			return fmt.Errorf("lock company: %w", err)
		}
		before, after = locked, locked
		if err := uc.apply(&after, in, now); err != nil {
			return err
		}
		if after == before {
			return nil
		}
		if err := tx.Companies.UpdateCompany(ctx, after); err != nil {
			return fmt.Errorf("update company: %w", err)
		}
		deactivated := before.AllowsAccess() && !after.AllowsAccess()
		if deactivated {
			if err := tx.Sessions.RevokeCompanySessions(ctx, after.ID, now); err != nil {
				return fmt.Errorf("revoke company sessions: %w", err)
			}
		}
		return recordCompanyEvents(ctx, tx.Audit, in.Actor, now, before, after)
	})
	if err != nil {
		return CompanyListing{}, err
	}

	if after.Status == domain.CompanyInactive {
		if err := uc.endProductSessions(ctx, after.ID); err != nil {
			return CompanyListing{}, err
		}
	}
	return findClientCompany(ctx, uc.lister, in.CompanyID)
}

func (uc *UpdateCompany) apply(company *domain.Company, in UpdateCompanyInput, now time.Time) error {
	if err := company.UpdateDetails(in.Name, in.City, in.State); err != nil {
		return err
	}
	if in.Plan != nil {
		if err := company.ChangePlan(*in.Plan, now, uc.settings.TrialDuration); err != nil {
			return err
		}
	}
	if in.Status != nil {
		if err := company.ChangeStatus(*in.Status, now, uc.settings.TrialDuration); err != nil {
			return err
		}
	}
	return nil
}

// endProductSessions ends the product sessions of every user of the company.
// It tries every user and reports one failure, so a single bad account does
// not leave the others signed in.
func (uc *UpdateCompany) endProductSessions(ctx context.Context, companyID domain.CompanyID) error {
	identities, err := uc.members.ListCompanyIdentities(ctx, companyID)
	if err != nil {
		return fmt.Errorf("list company identities: %w", err)
	}
	var failures []error
	for _, identity := range identities {
		if err := uc.access.EndProductSessions(ctx, identity.Subject); err != nil {
			failures = append(failures, providerFailure("end product sessions of "+string(identity.UserID), err))
		}
	}
	return errors.Join(failures...)
}

func recordCompanyEvents(ctx context.Context, audit AuditRecorder, actor domain.Principal, now time.Time, before, after domain.Company) error {
	id := string(after.ID)
	var actions []string
	switch {
	case before.AllowsAccess() && !after.AllowsAccess():
		actions = append(actions, domain.AuditCompanyDeactivated)
	case !before.AllowsAccess() && after.AllowsAccess():
		actions = append(actions, domain.AuditCompanyReactivated)
	}
	if before.Name != after.Name || before.City != after.City || before.State != after.State || before.Plan != after.Plan {
		actions = append(actions, domain.AuditCompanyUpdated)
	}
	for _, action := range actions {
		event := actorEvent(actor, now, after.ID, action, domain.AuditEntityCompany, id,
			map[string]string{"status": string(after.Status), "plan": string(after.Plan)})
		if err := audit.Record(ctx, event); err != nil {
			return fmt.Errorf("record audit: %w", err)
		}
	}
	return nil
}

// DeleteCompanyInput is the request of DELETE /companies/{companyId}.
type DeleteCompanyInput struct {
	Actor     domain.Principal
	CompanyID domain.CompanyID
}

// DeleteCompany soft deletes a client company that has no active or invited
// users. Inactive users stay for the audit trail. The count and the delete
// share one transaction on a locked company row, so an invitation cannot slip
// in between. Nothing in the provider needs cleaning: no active user means no
// group membership or product session of this company.
type DeleteCompany struct {
	lister  CompanyLister
	members CompanyMembers
	tx      TxRunner
	clock   Clock
}

// NewDeleteCompany builds the use case.
func NewDeleteCompany(lister CompanyLister, members CompanyMembers, tx TxRunner, clock Clock) *DeleteCompany {
	return &DeleteCompany{lister: lister, members: members, tx: tx, clock: clock}
}

// Execute deletes the company or returns ErrCompanyHasActiveUsers.
func (uc *DeleteCompany) Execute(ctx context.Context, in DeleteCompanyInput) error {
	if err := requireCan(in.Actor, domain.PermCompaniesManage); err != nil {
		return err
	}
	if _, err := findClientCompany(ctx, uc.lister, in.CompanyID); err != nil {
		return err
	}
	now := uc.clock.Now()
	return uc.tx.InTx(ctx, func(ctx context.Context, tx Stores) error {
		company, err := tx.Companies.LockCompany(ctx, in.CompanyID)
		if err != nil {
			return fmt.Errorf("lock company: %w", err)
		}
		counts, err := uc.members.CountUsersByStatus(ctx, company.ID)
		if err != nil {
			return fmt.Errorf("count users: %w", err)
		}
		if counts[domain.UserActive] > 0 || counts[domain.UserInvited] > 0 {
			return fmt.Errorf("%w: %d active, %d invited", ErrCompanyHasActiveUsers, counts[domain.UserActive], counts[domain.UserInvited])
		}
		company.MarkDeleted(now)
		if err := tx.Companies.UpdateCompany(ctx, company); err != nil {
			return fmt.Errorf("delete company: %w", err)
		}
		event := actorEvent(in.Actor, now, company.ID, domain.AuditCompanyDeleted, domain.AuditEntityCompany, string(company.ID), nil)
		if err := tx.Audit.Record(ctx, event); err != nil {
			return fmt.Errorf("record audit: %w", err)
		}
		return nil
	})
}
