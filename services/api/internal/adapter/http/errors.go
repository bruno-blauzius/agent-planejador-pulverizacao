package httpadapter

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/domain"
	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/usecase"
)

// Stable error codes of the contract (components.schemas.Error).
const (
	codeUnauthorized         = "unauthorized"
	codeForbidden            = "forbidden"
	codeNotFound             = "not_found"
	codeValidationFailed     = "validation_failed"
	codeConflict             = "conflict"
	codePayloadTooLarge      = "payload_too_large"
	codeUnsupportedMediaType = "unsupported_media_type"
	codeInternal             = "internal_error"
	codeProviderUnavailable  = "identity_provider_unavailable"
)

// apiError is a failure already expressed in HTTP terms. Handlers return it
// for problems found at the edge (body, query, path); use case errors are
// translated by classify instead.
type apiError struct {
	Status  int
	Code    string
	Message string
	Details []fieldDetail
}

func (e *apiError) Error() string { return e.Code + ": " + e.Message }

func validationError(message string, details ...fieldDetail) *apiError {
	return &apiError{Status: http.StatusUnprocessableEntity, Code: codeValidationFailed, Message: message, Details: details}
}

// writeFailure is the single place that maps errors to the contract's Error
// body. Internal causes are logged with the request id and never returned.
func writeFailure(w http.ResponseWriter, r *http.Request, logger *slog.Logger, err error) {
	failure := classify(err)
	if failure.Status >= http.StatusInternalServerError {
		logger.ErrorContext(r.Context(), "request failed",
			"error", err,
			"status", failure.Status,
			"code", failure.Code,
			"method", r.Method,
			"path", r.URL.Path,
			"request_id", RequestIDFromContext(r.Context()),
		)
	}
	if failure.Status == http.StatusUnauthorized {
		w.Header().Set("WWW-Authenticate", `Bearer realm="agentes-agro"`)
	}
	writeJSON(w, logger, failure.Status, errorBody{Error: errorDetail{
		Code:    failure.Code,
		Message: failure.Message,
		Details: failure.Details,
	}})
}

// classify translates use case, domain and edge errors into an apiError. The
// order matters: errors joined by a failed compensation can match several
// sentinels, and the most specific business outcome wins.
func classify(err error) *apiError {
	var edge *apiError
	if errors.As(err, &edge) {
		return edge
	}
	var invalid *domain.ValidationError
	if errors.As(err, &invalid) {
		details := make([]fieldDetail, 0, len(invalid.Fields))
		for _, field := range invalid.Fields {
			details = append(details, fieldDetail{Field: field.Field, Message: field.Message})
		}
		return validationError("dados inválidos", details...)
	}

	for _, rule := range failureRules {
		if errors.Is(err, rule.target) {
			return &apiError{Status: rule.status, Code: rule.code, Message: rule.message}
		}
	}
	if errors.Is(err, usecase.ErrFarmNotInCompany) {
		return validationError("dados inválidos", fieldDetail{Field: "farm_scope.farm_ids", Message: "fazenda não pertence à empresa"})
	}
	return &apiError{Status: http.StatusInternalServerError, Code: codeInternal, Message: "erro interno"}
}

type failureRule struct {
	target  error
	status  int
	code    string
	message string
}

var failureRules = []failureRule{
	{usecase.ErrUnauthenticated, http.StatusUnauthorized, codeUnauthorized, "autenticação necessária"},

	{usecase.ErrNotFound, http.StatusNotFound, codeNotFound, "recurso não encontrado"},

	{usecase.ErrUserNotProvisioned, http.StatusForbidden, "user_not_provisioned", "sua conta ainda não foi convidada para o Agentes Agro"},
	{domain.ErrNoAccessToProduct, http.StatusForbidden, "no_access_to_product", "sua conta não tem acesso ao Agentes Agro"},
	{domain.ErrAmbiguousRole, http.StatusForbidden, "ambiguous_role", "sua conta tem mais de um papel no Agentes Agro"},
	{usecase.ErrAccessDenied, http.StatusForbidden, codeForbidden, "usuário ou empresa inativos"},
	{usecase.ErrForbidden, http.StatusForbidden, codeForbidden, "você não tem permissão para esta ação"},

	{usecase.ErrEmailAlreadyRegistered, http.StatusConflict, "email_already_registered", "e-mail já cadastrado"},
	{usecase.ErrEmailNotVerified, http.StatusConflict, "email_not_verified", "o e-mail da conta existente ainda não foi verificado"},
	{usecase.ErrCompanyInactive, http.StatusConflict, "company_inactive", "a empresa está inativa"},
	{usecase.ErrCompanyHasActiveUsers, http.StatusConflict, "company_has_active_users", "a empresa ainda tem usuários ativos ou convidados"},
	{usecase.ErrInvitationPending, http.StatusConflict, "invitation_pending", "o usuário tem um convite pendente; cancele o convite"},
	{domain.ErrInvitationClosed, http.StatusConflict, "invitation_closed", "o convite já foi aceito ou cancelado"},
	{usecase.ErrLastSuperAdmin, http.StatusConflict, "last_super_admin", "a plataforma não pode ficar sem super admin ativo"},
	{usecase.ErrDocumentAlreadyRegistered, http.StatusConflict, codeConflict, "CNPJ já cadastrado"},
	{usecase.ErrVersionConflict, http.StatusConflict, codeConflict, "o registro foi alterado por outra requisição; tente novamente"},
	{usecase.ErrIdentityAlreadyLinked, http.StatusConflict, codeConflict, "a conta já está vinculada a outro usuário"},
	{domain.ErrInvalidTransition, http.StatusConflict, codeConflict, "mudança de status não permitida no estado atual"},

	{usecase.ErrIdentityProviderUnavailable, http.StatusBadGateway, codeProviderUnavailable, "o provedor de identidade está indisponível"},
}
