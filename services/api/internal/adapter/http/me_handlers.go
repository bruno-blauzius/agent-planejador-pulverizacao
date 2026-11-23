package httpadapter

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/domain"
	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/usecase"
)

// MeHandler serves the session of the caller (GET /auth/me) and the Profile tag.
type MeHandler struct {
	logger        *slog.Logger
	me            *usecase.GetMe
	profile       *usecase.GetProfile
	updateProfile *usecase.UpdateProfile
}

// MeDeps groups the use cases of MeHandler.
type MeDeps struct {
	Me            *usecase.GetMe
	Profile       *usecase.GetProfile
	UpdateProfile *usecase.UpdateProfile
}

// NewMeHandler builds the handler.
func NewMeHandler(logger *slog.Logger, deps MeDeps) *MeHandler {
	return &MeHandler{logger: logger, me: deps.Me, profile: deps.Profile, updateProfile: deps.UpdateProfile}
}

// Register attaches the routes. Any authenticated role may call them.
func (h *MeHandler) Register(routes Routes) {
	routes.Protected(http.MethodGet, "/auth/me", serve(h.logger, h.currentUser))
	routes.Protected(http.MethodGet, "/me/profile", serve(h.logger, h.getProfile))
	routes.Protected(http.MethodPatch, "/me/profile", serve(h.logger, h.patchProfile))
}

func (h *MeHandler) currentUser(_ http.ResponseWriter, r *http.Request, actor domain.Principal) (response, error) {
	me, err := h.me.Execute(r.Context(), usecase.GetMeInput{Actor: actor})
	if err != nil {
		return response{}, err
	}
	return okResponse(currentUserOf(me)), nil
}

func (h *MeHandler) getProfile(_ http.ResponseWriter, r *http.Request, actor domain.Principal) (response, error) {
	profile, err := h.profile.Execute(r.Context(), usecase.GetProfileInput{Actor: actor})
	if err != nil {
		return response{}, err
	}
	return okResponse(profileOf(profile)), nil
}

// profileUpdateBody is ProfileUpdate. The contract says e-mail, role and
// company are managed by an administrator and are ignored when sent, so they
// are accepted here and dropped (unlike unknown fields, which are rejected).
type profileUpdateBody struct {
	Name      *string         `json:"name"`
	Email     json.RawMessage `json:"email"`
	Role      json.RawMessage `json:"role"`
	Company   json.RawMessage `json:"company"`
	CompanyID json.RawMessage `json:"company_id"`
	FarmScope json.RawMessage `json:"farm_scope"`
}

func (h *MeHandler) patchProfile(w http.ResponseWriter, r *http.Request, actor domain.Principal) (response, error) {
	var body profileUpdateBody
	if err := decodeJSON(w, r, &body); err != nil {
		return response{}, err
	}
	if body.Name == nil {
		return response{}, validationError("dados inválidos", fieldDetail{Field: "name", Message: "informe o nome"})
	}
	profile, err := h.updateProfile.Execute(r.Context(), usecase.UpdateProfileInput{Actor: actor, Name: *body.Name})
	if err != nil {
		return response{}, err
	}
	return okResponse(profileOf(profile)), nil
}
