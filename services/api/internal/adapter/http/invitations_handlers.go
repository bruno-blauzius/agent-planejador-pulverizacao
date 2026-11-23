package httpadapter

import (
	"log/slog"
	"net/http"

	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/domain"
	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/usecase"
)

// invitationEmailHeader tells whether the action e-mail left (contract, headers.InvitationEmail).
const invitationEmailHeader = "X-Invitation-Email"

// InvitationsHandler serves the invitation operations of the Users tag.
type InvitationsHandler struct {
	logger *slog.Logger
	invite *usecase.InviteUser
	resend *usecase.ResendInvitation
	cancel *usecase.CancelInvitation
}

// InvitationsDeps groups the use cases of InvitationsHandler.
type InvitationsDeps struct {
	Invite *usecase.InviteUser
	Resend *usecase.ResendInvitation
	Cancel *usecase.CancelInvitation
}

// NewInvitationsHandler builds the handler.
func NewInvitationsHandler(logger *slog.Logger, deps InvitationsDeps) *InvitationsHandler {
	return &InvitationsHandler{logger: logger, invite: deps.Invite, resend: deps.Resend, cancel: deps.Cancel}
}

// Register attaches the routes (all protected; permission users.manage is checked by the use cases).
func (h *InvitationsHandler) Register(routes Routes) {
	routes.Protected(http.MethodPost, "/invitations", serve(h.logger, h.createInvitation))
	routes.Protected(http.MethodPost, "/invitations/{invitationId}/resend", serve(h.logger, h.resendInvitation))
	routes.Protected(http.MethodDelete, "/invitations/{invitationId}", serve(h.logger, h.cancelInvitation))
}

// invitationBody is InvitationInput; every field is required.
type invitationBody struct {
	Name      string          `json:"name"`
	Email     string          `json:"email"`
	Role      string          `json:"role"`
	CompanyID string          `json:"company_id"`
	FarmScope *farmScopeInput `json:"farm_scope"`
}

func (b invitationBody) parse(actor domain.Principal) (usecase.InviteUserInput, error) {
	var problems domain.Violations
	in := usecase.InviteUserInput{Actor: actor, Name: b.Name, Email: b.Email}

	role, err := domain.ParseRole(b.Role)
	if mergeErr := problems.Merge(err); mergeErr != nil {
		return in, mergeErr
	}
	in.Role = role

	companyID, valid := canonicalUUID(b.CompanyID)
	if !valid {
		problems.Add("company_id", "informe a empresa (identificador inválido)")
	}
	in.CompanyID = domain.CompanyID(companyID)

	if b.FarmScope == nil {
		problems.Add("farm_scope", "informe o escopo de fazendas")
	} else {
		scope, err := b.FarmScope.parse()
		if mergeErr := problems.Merge(err); mergeErr != nil {
			return in, mergeErr
		}
		in.FarmScope = scope
	}
	return in, problems.Err()
}

func (h *InvitationsHandler) createInvitation(w http.ResponseWriter, r *http.Request, actor domain.Principal) (response, error) {
	var body invitationBody
	if err := decodeJSON(w, r, &body); err != nil {
		return response{}, err
	}
	in, err := body.parse(actor)
	if err != nil {
		return response{}, err
	}
	result, err := h.invite.Execute(r.Context(), in)
	if err != nil {
		return response{}, err
	}
	h.logEmailFailure(r, "invitation e-mail was not sent", result.EmailFailure)
	reply := createdResponse(userAccountOf(result.User))
	reply.header = map[string]string{invitationEmailHeader: string(result.EmailDelivery)}
	return reply, nil
}

func (h *InvitationsHandler) resendInvitation(_ http.ResponseWriter, r *http.Request, actor domain.Principal) (response, error) {
	id, err := pathID(r, "invitationId")
	if err != nil {
		return response{}, err
	}
	result, err := h.resend.Execute(r.Context(), usecase.ResendInvitationInput{Actor: actor, InvitationID: domain.InvitationID(id)})
	if err != nil {
		return response{}, err
	}
	h.logEmailFailure(r, "invitation e-mail was not resent", result.EmailFailure)
	reply := noContentResponse()
	reply.header = map[string]string{invitationEmailHeader: string(result.EmailDelivery)}
	return reply, nil
}

func (h *InvitationsHandler) cancelInvitation(_ http.ResponseWriter, r *http.Request, actor domain.Principal) (response, error) {
	id, err := pathID(r, "invitationId")
	if err != nil {
		return response{}, err
	}
	if err := h.cancel.Execute(r.Context(), usecase.CancelInvitationInput{Actor: actor, InvitationID: domain.InvitationID(id)}); err != nil {
		return response{}, err
	}
	return noContentResponse(), nil
}

// logEmailFailure records why the e-mail did not leave. The invitation stays
// valid, so this is a warning for operators, not an error for the client.
func (h *InvitationsHandler) logEmailFailure(r *http.Request, message string, cause error) {
	if cause == nil {
		return
	}
	h.logger.WarnContext(r.Context(), message, "error", cause, "request_id", RequestIDFromContext(r.Context()))
}
