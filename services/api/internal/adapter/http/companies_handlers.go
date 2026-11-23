package httpadapter

import (
	"log/slog"
	"net/http"

	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/domain"
	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/usecase"
)

// CompaniesHandler serves the Companies tag of the contract.
type CompaniesHandler struct {
	logger  *slog.Logger
	list    *usecase.ListCompanies
	summary *usecase.GetCompaniesSummary
	get     *usecase.GetCompany
	create  *usecase.CreateCompany
	update  *usecase.UpdateCompany
	remove  *usecase.DeleteCompany
}

// CompaniesDeps groups the use cases of CompaniesHandler.
type CompaniesDeps struct {
	List    *usecase.ListCompanies
	Summary *usecase.GetCompaniesSummary
	Get     *usecase.GetCompany
	Create  *usecase.CreateCompany
	Update  *usecase.UpdateCompany
	Remove  *usecase.DeleteCompany
}

// NewCompaniesHandler builds the handler.
func NewCompaniesHandler(logger *slog.Logger, deps CompaniesDeps) *CompaniesHandler {
	return &CompaniesHandler{
		logger: logger, list: deps.List, summary: deps.Summary, get: deps.Get,
		create: deps.Create, update: deps.Update, remove: deps.Remove,
	}
}

// Register attaches the routes (all protected; permission companies.manage is checked by the use cases).
func (h *CompaniesHandler) Register(routes Routes) {
	routes.Protected(http.MethodGet, "/companies", serve(h.logger, h.listCompanies))
	routes.Protected(http.MethodPost, "/companies", serve(h.logger, h.createCompany))
	routes.Protected(http.MethodGet, "/companies/summary", serve(h.logger, h.companiesSummary))
	routes.Protected(http.MethodGet, "/companies/{companyId}", serve(h.logger, h.getCompany))
	routes.Protected(http.MethodPatch, "/companies/{companyId}", serve(h.logger, h.updateCompany))
	routes.Protected(http.MethodDelete, "/companies/{companyId}", serve(h.logger, h.deleteCompany))
}

func (h *CompaniesHandler) listCompanies(_ http.ResponseWriter, r *http.Request, actor domain.Principal) (response, error) {
	params := newQuery(r)
	in := usecase.ListCompaniesInput{
		Actor:    actor,
		Search:   params.text("search", maxSearchLength),
		Status:   domain.CompanyStatus(params.enum("status")),
		Page:     params.positiveInt("page"),
		PageSize: params.positiveInt("page_size"),
	}
	if err := params.err(); err != nil {
		return response{}, err
	}
	page, err := h.list.Execute(r.Context(), in)
	if err != nil {
		return response{}, err
	}
	return okResponse(companyPageOf(page)), nil
}

func (h *CompaniesHandler) companiesSummary(_ http.ResponseWriter, r *http.Request, actor domain.Principal) (response, error) {
	summary, err := h.summary.Execute(r.Context(), actor)
	if err != nil {
		return response{}, err
	}
	return okResponse(companiesSummaryOf(summary)), nil
}

func (h *CompaniesHandler) getCompany(_ http.ResponseWriter, r *http.Request, actor domain.Principal) (response, error) {
	id, err := pathID(r, "companyId")
	if err != nil {
		return response{}, err
	}
	listing, err := h.get.Execute(r.Context(), usecase.GetCompanyInput{Actor: actor, CompanyID: domain.CompanyID(id)})
	if err != nil {
		return response{}, err
	}
	return okResponse(companyOf(listing)), nil
}

// companyInputBody is CompanyInput. Missing fields arrive empty and the domain
// answers with the message of each one.
type companyInputBody struct {
	Name     string `json:"name"`
	Document string `json:"document"`
	City     string `json:"city"`
	State    string `json:"state"`
	Plan     string `json:"plan"`
}

func (h *CompaniesHandler) createCompany(w http.ResponseWriter, r *http.Request, actor domain.Principal) (response, error) {
	var body companyInputBody
	if err := decodeJSON(w, r, &body); err != nil {
		return response{}, err
	}
	listing, err := h.create.Execute(r.Context(), usecase.CreateCompanyInput{
		Actor: actor,
		NewCompanyInput: domain.NewCompanyInput{
			Name: body.Name, Document: body.Document, City: body.City, State: body.State,
			Plan: domain.CompanyPlan(body.Plan),
		},
	})
	if err != nil {
		return response{}, err
	}
	return createdResponse(companyOf(listing)), nil
}

// companyUpdateBody is CompanyUpdate. Absent fields stay unchanged.
type companyUpdateBody struct {
	Name   *string `json:"name"`
	City   *string `json:"city"`
	State  *string `json:"state"`
	Plan   *string `json:"plan"`
	Status *string `json:"status"`
}

func (b companyUpdateBody) parse(actor domain.Principal, id domain.CompanyID) (usecase.UpdateCompanyInput, error) {
	in := usecase.UpdateCompanyInput{Actor: actor, CompanyID: id, Name: b.Name, City: b.City, State: b.State}
	if b.Name == nil && b.City == nil && b.State == nil && b.Plan == nil && b.Status == nil {
		return in, validationError("informe ao menos um campo para alterar")
	}
	if b.Plan != nil {
		plan := domain.CompanyPlan(*b.Plan)
		in.Plan = &plan
	}
	if b.Status != nil {
		status := domain.CompanyStatus(*b.Status)
		in.Status = &status
	}
	return in, nil
}

func (h *CompaniesHandler) updateCompany(w http.ResponseWriter, r *http.Request, actor domain.Principal) (response, error) {
	id, err := pathID(r, "companyId")
	if err != nil {
		return response{}, err
	}
	var body companyUpdateBody
	if err := decodeJSON(w, r, &body); err != nil {
		return response{}, err
	}
	in, err := body.parse(actor, domain.CompanyID(id))
	if err != nil {
		return response{}, err
	}
	listing, err := h.update.Execute(r.Context(), in)
	if err != nil {
		return response{}, err
	}
	return okResponse(companyOf(listing)), nil
}

func (h *CompaniesHandler) deleteCompany(_ http.ResponseWriter, r *http.Request, actor domain.Principal) (response, error) {
	id, err := pathID(r, "companyId")
	if err != nil {
		return response{}, err
	}
	if err := h.remove.Execute(r.Context(), usecase.DeleteCompanyInput{Actor: actor, CompanyID: domain.CompanyID(id)}); err != nil {
		return response{}, err
	}
	return noContentResponse(), nil
}
