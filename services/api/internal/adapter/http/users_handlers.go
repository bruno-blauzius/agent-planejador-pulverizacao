package httpadapter

import (
	"log/slog"
	"net/http"

	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/domain"
	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/usecase"
)

// UsersHandler serves the Users tag of the contract. It only translates HTTP
// to use case calls and back; authorization and every rule live in the use cases.
type UsersHandler struct {
	logger     *slog.Logger
	list       *usecase.ListUsers
	summary    *usecase.GetUsersSummary
	get        *usecase.GetUser
	update     *usecase.UpdateUser
	deactivate *usecase.DeactivateUser
	reactivate *usecase.ReactivateUser
	remove     *usecase.DeleteUser
}

// UsersDeps groups the use cases of UsersHandler.
type UsersDeps struct {
	List       *usecase.ListUsers
	Summary    *usecase.GetUsersSummary
	Get        *usecase.GetUser
	Update     *usecase.UpdateUser
	Deactivate *usecase.DeactivateUser
	Reactivate *usecase.ReactivateUser
	Remove     *usecase.DeleteUser
}

// NewUsersHandler builds the handler.
func NewUsersHandler(logger *slog.Logger, deps UsersDeps) *UsersHandler {
	return &UsersHandler{
		logger: logger, list: deps.List, summary: deps.Summary, get: deps.Get, update: deps.Update,
		deactivate: deps.Deactivate, reactivate: deps.Reactivate, remove: deps.Remove,
	}
}

// Register attaches the routes (all protected; permission users.manage is checked by the use cases).
func (h *UsersHandler) Register(routes Routes) {
	routes.Protected(http.MethodGet, "/users", serve(h.logger, h.listUsers))
	routes.Protected(http.MethodGet, "/users/summary", serve(h.logger, h.usersSummary))
	routes.Protected(http.MethodGet, "/users/{userId}", serve(h.logger, h.getUser))
	routes.Protected(http.MethodPatch, "/users/{userId}", serve(h.logger, h.updateUser))
	routes.Protected(http.MethodDelete, "/users/{userId}", serve(h.logger, h.deleteUser))
}

func (h *UsersHandler) listUsers(_ http.ResponseWriter, r *http.Request, actor domain.Principal) (response, error) {
	params := newQuery(r)
	in := usecase.ListUsersInput{
		Actor:     actor,
		Search:    params.text("search", maxSearchLength),
		Role:      domain.Role(params.enum("role")),
		Status:    domain.UserStatus(params.enum("status")),
		CompanyID: domain.CompanyID(params.uuid("company_id")),
		Page:      params.positiveInt("page"),
		PageSize:  params.positiveInt("page_size"),
	}
	if err := params.err(); err != nil {
		return response{}, err
	}
	page, err := h.list.Execute(r.Context(), in)
	if err != nil {
		return response{}, err
	}
	return okResponse(userPageOf(page)), nil
}

func (h *UsersHandler) usersSummary(_ http.ResponseWriter, r *http.Request, actor domain.Principal) (response, error) {
	params := newQuery(r)
	companyID := domain.CompanyID(params.uuid("company_id"))
	if err := params.err(); err != nil {
		return response{}, err
	}
	summary, err := h.summary.Execute(r.Context(), usecase.UsersSummaryInput{Actor: actor, CompanyID: companyID})
	if err != nil {
		return response{}, err
	}
	return okResponse(usersSummaryOf(summary)), nil
}

func (h *UsersHandler) getUser(_ http.ResponseWriter, r *http.Request, actor domain.Principal) (response, error) {
	id, err := pathID(r, "userId")
	if err != nil {
		return response{}, err
	}
	listing, err := h.get.Execute(r.Context(), usecase.GetUserInput{Actor: actor, UserID: domain.UserID(id)})
	if err != nil {
		return response{}, err
	}
	return okResponse(userAccountOf(listing)), nil
}

// userUpdateBody is UserUpdate. Absent fields stay unchanged.
type userUpdateBody struct {
	Name      *string         `json:"name"`
	Role      *string         `json:"role"`
	CompanyID *string         `json:"company_id"`
	FarmScope *farmScopeInput `json:"farm_scope"`
	Status    *string         `json:"status"`
}

// userStatusChange is the status asked by a PATCH, mapped to a use case.
type userStatusChange string

const (
	statusUnchanged  userStatusChange = ""
	statusActivate   userStatusChange = "active"
	statusDeactivate userStatusChange = "inactive"
)

// parse checks the shape of the body. Domain rules (who may assign what, valid
// combinations of role and scope) are the use case's.
func (b userUpdateBody) parse(actor domain.Principal, id domain.UserID) (usecase.UpdateUserInput, userStatusChange, error) {
	var problems domain.Violations
	in := usecase.UpdateUserInput{Actor: actor, UserID: id, Name: b.Name}

	if b.Name == nil && b.Role == nil && b.CompanyID == nil && b.FarmScope == nil && b.Status == nil {
		return in, statusUnchanged, validationError("informe ao menos um campo para alterar")
	}
	if b.Role != nil {
		role, err := domain.ParseRole(*b.Role)
		if mergeErr := problems.Merge(err); mergeErr != nil {
			return in, statusUnchanged, mergeErr
		}
		in.Role = &role
	}
	if b.CompanyID != nil {
		canonical, valid := canonicalUUID(*b.CompanyID)
		if !valid {
			problems.Add("company_id", "identificador inválido")
		}
		companyID := domain.CompanyID(canonical)
		in.CompanyID = &companyID
	}
	if b.FarmScope != nil {
		scope, err := b.FarmScope.parse()
		if mergeErr := problems.Merge(err); mergeErr != nil {
			return in, statusUnchanged, mergeErr
		}
		in.FarmScope = &scope
	}
	status := statusUnchanged
	if b.Status != nil {
		switch userStatusChange(*b.Status) {
		case statusActivate, statusDeactivate:
			status = userStatusChange(*b.Status)
		default:
			problems.Add("status", "use active ou inactive")
		}
	}
	return in, status, problems.Err()
}

func (b userUpdateBody) changesProfile() bool {
	return b.Name != nil || b.Role != nil || b.CompanyID != nil || b.FarmScope != nil
}

// updateUser applies the profile fields first and the status second, each in
// its own use case. If the second step fails the first one stays applied; the
// client sees the error and the next GET shows the real state.
func (h *UsersHandler) updateUser(w http.ResponseWriter, r *http.Request, actor domain.Principal) (response, error) {
	id, err := pathID(r, "userId")
	if err != nil {
		return response{}, err
	}
	var body userUpdateBody
	if err := decodeJSON(w, r, &body); err != nil {
		return response{}, err
	}
	in, status, err := body.parse(actor, domain.UserID(id))
	if err != nil {
		return response{}, err
	}

	var listing usecase.UserListing
	if body.changesProfile() {
		listing, err = h.update.Execute(r.Context(), in)
		if err != nil {
			return response{}, err
		}
	}
	statusInput := usecase.UserStatusInput{Actor: actor, UserID: domain.UserID(id)}
	switch status {
	case statusDeactivate:
		listing, err = h.deactivate.Execute(r.Context(), statusInput)
	case statusActivate:
		listing, err = h.reactivate.Execute(r.Context(), statusInput)
	case statusUnchanged:
	}
	if err != nil {
		return response{}, err
	}
	return okResponse(userAccountOf(listing)), nil
}

func (h *UsersHandler) deleteUser(_ http.ResponseWriter, r *http.Request, actor domain.Principal) (response, error) {
	id, err := pathID(r, "userId")
	if err != nil {
		return response{}, err
	}
	if err := h.remove.Execute(r.Context(), usecase.UserStatusInput{Actor: actor, UserID: domain.UserID(id)}); err != nil {
		return response{}, err
	}
	return noContentResponse(), nil
}
