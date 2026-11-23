package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/adapter/postgres/sqlcgen"
	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/domain"
	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/usecase"
)

func companyFromRow(row sqlcgen.Company) domain.Company {
	return domain.Company{
		ID:                 domain.CompanyID(row.ID),
		Name:               row.Name,
		Document:           domain.Document(row.Document),
		City:               row.City,
		State:              row.State,
		Plan:               domain.CompanyPlan(row.Plan),
		Status:             domain.CompanyStatus(row.Status),
		TrialEndsAt:        row.TrialEndsAt,
		IsPlatformOperator: row.IsPlatformOperator,
		CreatedAt:          row.CreatedAt,
		DeletedAt:          row.DeletedAt,
		Version:            int(row.Version),
	}
}

// FindCompany implements usecase.CompanyFinder.
func (r *Repository) FindCompany(ctx context.Context, id domain.CompanyID) (domain.Company, error) {
	row, err := r.q.GetCompany(ctx, string(id))
	if err != nil {
		return domain.Company{}, translateRead("find company", err)
	}
	return companyFromRow(row), nil
}

// LockCompany implements usecase.CompanyFinder: SELECT ... FOR UPDATE, held until
// the surrounding transaction ends. Outside a transaction the lock is released
// at once, which is why use cases call it inside InTx.
func (r *Repository) LockCompany(ctx context.Context, id domain.CompanyID) (domain.Company, error) {
	row, err := r.q.LockCompany(ctx, string(id))
	if err != nil {
		return domain.Company{}, translateRead("lock company", err)
	}
	return companyFromRow(row), nil
}

// InsertCompany implements usecase.CompanyWriter. A zero Version is stored as 1.
func (r *Repository) InsertCompany(ctx context.Context, company domain.Company) error {
	createdAt := company.CreatedAt
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}
	err := r.atomic(ctx, func(q *sqlcgen.Queries) error {
		return translateWrite("insert company", q.InsertCompany(ctx, sqlcgen.InsertCompanyParams{
			ID:                 string(company.ID),
			Name:               company.Name,
			Document:           string(company.Document),
			City:               company.City,
			State:              company.State,
			Plan:               string(company.Plan),
			Status:             string(company.Status),
			TrialEndsAt:        company.TrialEndsAt,
			IsPlatformOperator: company.IsPlatformOperator,
			CreatedAt:          createdAt,
			Version:            normalizeVersion(company.Version),
		}))
	})
	if err != nil {
		return fmt.Errorf("insert company: %w", err)
	}
	return nil
}

// UpdateCompany implements usecase.CompanyWriter with optimistic concurrency.
// The document and the operator flag are immutable and never written.
func (r *Repository) UpdateCompany(ctx context.Context, company domain.Company) error {
	if !validUUID(string(company.ID)) {
		return fmt.Errorf("update company: %w", usecase.ErrNotFound)
	}
	updated, err := r.q.UpdateCompany(ctx, sqlcgen.UpdateCompanyParams{
		ID:          string(company.ID),
		Name:        company.Name,
		City:        company.City,
		State:       company.State,
		Plan:        string(company.Plan),
		Status:      string(company.Status),
		TrialEndsAt: company.TrialEndsAt,
		DeletedAt:   company.DeletedAt,
		Version:     normalizeVersion(company.Version),
	})
	if err != nil {
		return translateWrite("update company", err)
	}
	if updated > 0 {
		return nil
	}
	exists, err := r.q.CompanyExists(ctx, string(company.ID))
	if err != nil {
		return translateRead("update company", err)
	}
	if !exists {
		return fmt.Errorf("update company: %w", usecase.ErrNotFound)
	}
	return fmt.Errorf("update company: %w", usecase.ErrVersionConflict)
}

// ---- listings -------------------------------------------------------------------

func listingFromRow(company sqlcgen.Company, farms, users int32) usecase.CompanyListing {
	return usecase.CompanyListing{Company: companyFromRow(company), FarmCount: int(farms), UserCount: int(users)}
}

// FindCompanyListing implements usecase.CompanyLister.
func (r *Repository) FindCompanyListing(ctx context.Context, id domain.CompanyID) (usecase.CompanyListing, error) {
	row, err := r.q.GetCompanyListing(ctx, string(id))
	if err != nil {
		return usecase.CompanyListing{}, translateRead("find company listing", err)
	}
	return listingFromRow(row.Company, row.FarmCount, row.UserCount), nil
}

// ListCompanies implements usecase.CompanyLister. The platform operator and
// deleted companies never appear; search matches name or city.
func (r *Repository) ListCompanies(ctx context.Context, filter usecase.CompanyFilter, page usecase.PageRequest) (usecase.CompanyPage, error) {
	result := usecase.CompanyPage{Items: []usecase.CompanyListing{}, PageRequest: page}
	limit, offset := pageBounds(page)
	search, status := likePattern(filter.Search), optionalText(string(filter.Status))

	total, err := r.q.CountListedCompanies(ctx, sqlcgen.CountListedCompaniesParams{Status: status, Search: search})
	if err != nil {
		return usecase.CompanyPage{}, fmt.Errorf("count companies: %w", err)
	}
	result.Total = int(total)
	if total == 0 || int64(offset) >= int64(total) {
		return result, nil
	}
	rows, err := r.q.ListCompanies(ctx, sqlcgen.ListCompaniesParams{
		Status: status, Search: search, PageLimit: limit, PageOffset: offset,
	})
	if err != nil {
		return usecase.CompanyPage{}, fmt.Errorf("list companies: %w", err)
	}
	for _, row := range rows {
		result.Items = append(result.Items, listingFromRow(row.Company, row.FarmCount, row.UserCount))
	}
	return result, nil
}

// SummarizeCompanies implements usecase.CompanyLister. Applications are not
// persisted yet, so both application counters are zero.
func (r *Repository) SummarizeCompanies(ctx context.Context) (usecase.CompaniesSummary, error) {
	row, err := r.q.SummarizeCompanies(ctx)
	if err != nil {
		return usecase.CompaniesSummary{}, fmt.Errorf("summarize companies: %w", err)
	}
	return usecase.CompaniesSummary{
		ActiveCompanies:    int(row.ActiveCompanies),
		TrialCompanies:     int(row.TrialCompanies),
		FarmCount:          int(row.FarmCount),
		MonitoredAreaHa:    row.MonitoredAreaHa,
		UserCount:          int(row.UserCount),
		PendingInvitations: int(row.PendingInvitations),
	}, nil
}
