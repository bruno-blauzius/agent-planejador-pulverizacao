package httpadapter

import (
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/domain"
	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/usecase"
)

func TestClassifyMapsEveryUseCaseError(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantCode   string
	}{
		{"unauthenticated", usecase.ErrUnauthenticated, 401, "unauthorized"},
		{"not found", usecase.ErrNotFound, 404, "not_found"},
		{"forbidden", usecase.ErrForbidden, 403, "forbidden"},
		{"access denied", usecase.ErrAccessDenied, 403, "forbidden"},
		{"user not provisioned", usecase.ErrUserNotProvisioned, 403, "user_not_provisioned"},
		{"no access to product", domain.ErrNoAccessToProduct, 403, "no_access_to_product"},
		{"ambiguous role", domain.ErrAmbiguousRole, 403, "ambiguous_role"},
		{"email already registered", usecase.ErrEmailAlreadyRegistered, 409, "email_already_registered"},
		{"email not verified", usecase.ErrEmailNotVerified, 409, "email_not_verified"},
		{"company inactive", usecase.ErrCompanyInactive, 409, "company_inactive"},
		{"company has active users", usecase.ErrCompanyHasActiveUsers, 409, "company_has_active_users"},
		{"invitation pending", usecase.ErrInvitationPending, 409, "invitation_pending"},
		{"invitation closed", domain.ErrInvitationClosed, 409, "invitation_closed"},
		{"last super admin", usecase.ErrLastSuperAdmin, 409, "last_super_admin"},
		{"document already registered", usecase.ErrDocumentAlreadyRegistered, 409, "conflict"},
		{"version conflict", usecase.ErrVersionConflict, 409, "conflict"},
		{"identity already linked", usecase.ErrIdentityAlreadyLinked, 409, "conflict"},
		{"invalid transition", domain.ErrInvalidTransition, 409, "conflict"},
		{"farm not in company", usecase.ErrFarmNotInCompany, 422, "validation_failed"},
		{"identity provider unavailable", usecase.ErrIdentityProviderUnavailable, 502, "identity_provider_unavailable"},
		{"validation", domain.Invalid("email", "e-mail inválido"), 422, "validation_failed"},
		{"edge error", validationError("x"), 422, "validation_failed"},
		{"unknown error", errors.New("pq: password authentication failed for user api_app"), 500, "internal_error"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Wrapped, as the use cases return them.
			got := classify(fmt.Errorf("use case: %w", tt.err))
			if got.Status != tt.wantStatus || got.Code != tt.wantCode {
				t.Errorf("classify = %d %q, want %d %q", got.Status, got.Code, tt.wantStatus, tt.wantCode)
			}
		})
	}
}

func TestClassifyKeepsEveryFieldOfAValidationError(t *testing.T) {
	var problems domain.Violations
	problems.Add("name", "informe o nome")
	problems.Add("farm_scope.mode", "modo desconhecido")

	got := classify(problems.Err())
	if len(got.Details) != 2 || got.Details[0].Field != "name" || got.Details[1].Field != "farm_scope.mode" {
		t.Errorf("details = %+v", got.Details)
	}
}

func TestClassifyPrefersTheBusinessOutcomeOverAFailedCompensation(t *testing.T) {
	joined := errors.Join(usecase.ErrCompanyInactive, errors.New("compensation failed, reconcile manually"))
	if got := classify(joined); got.Code != "company_inactive" {
		t.Errorf("code = %q, want company_inactive", got.Code)
	}
}

func TestInternalErrorsNeverLeakTheirCause(t *testing.T) {
	got := classify(errors.New("dial tcp 10.0.0.5:5432: password=hunter2"))
	if got.Status != http.StatusInternalServerError || got.Message != "erro interno" {
		t.Errorf("classify = %+v", got)
	}
}

func TestCanonicalUUID(t *testing.T) {
	tests := []struct {
		in     string
		want   string
		wantOK bool
	}{
		{"3F2B8C1E-5A47-4C9D-8E21-9B0A6D7C4F10", "3f2b8c1e-5a47-4c9d-8e21-9b0a6d7c4f10", true},
		{"3f2b8c1e-5a47-4c9d-8e21-9b0a6d7c4f10", "3f2b8c1e-5a47-4c9d-8e21-9b0a6d7c4f10", true},
		{"", "", false},
		{"3f2b8c1e5a474c9d8e219b0a6d7c4f10", "", false},
		{"{3f2b8c1e-5a47-4c9d-8e21-9b0a6d7c4f10}", "", false},
		{"3f2b8c1e-5a47-4c9d-8e21-9b0a6d7c4f1g", "", false},
		{"3f2b8c1e_5a47-4c9d-8e21-9b0a6d7c4f10", "", false},
		{"'; DROP TABLE users;--", "", false},
	}
	for _, tt := range tests {
		got, ok := canonicalUUID(tt.in)
		if got != tt.want || ok != tt.wantOK {
			t.Errorf("canonicalUUID(%q) = %q, %v; want %q, %v", tt.in, got, ok, tt.want, tt.wantOK)
		}
	}
}

func TestMatchesPattern(t *testing.T) {
	pattern := []string{"", "users", "{userId}"}
	for path, want := range map[string]bool{
		"/users/abc": true, "/users/": false, "/users": false, "/users/abc/x": false, "/other/abc": false,
	} {
		if got := matchesPattern(pattern, splitPath(path)); got != want {
			t.Errorf("matchesPattern(%q) = %v, want %v", path, got, want)
		}
	}
}
