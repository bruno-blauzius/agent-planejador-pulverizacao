package apispec_test

import (
	"strings"
	"testing"

	apispec "github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/api"
)

func TestSpecIsEmbeddedAndDeclaresHealthPaths(t *testing.T) {
	if len(apispec.Spec) == 0 {
		t.Fatal("embedded spec is empty")
	}
	content := string(apispec.Spec)

	for _, want := range []string{
		"openapi: 3.1",
		"/health/live:",
		"/health/startup:",
		"/health/ready:",
		// Contrato de identidade (ADR 0015, D9, Fase A).
		"/auth/login:",
		"/auth/callback:",
		"/auth/logout:",
		"/auth/backchannel-logout:",
		"sessionCookie:",
		"openIdConnect:",
		"identity_provider_unavailable",
		"user_not_provisioned",
		"no_access_to_product",
		"ambiguous_role",
		// Ciclo de vida de usuários e empresas (ADR 0013 e 0015).
		"operationId: getUser",
		"operationId: deleteUser",
		"operationId: getCompany",
		"operationId: deleteCompany",
		"email_already_registered",
		"email_not_verified",
		"company_inactive",
		"company_has_active_users",
		"invitation_closed",
		"invitation_pending",
	} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(content, want) {
				t.Errorf("spec does not contain %q", want)
			}
		})
	}
}

// Toda operação de administração de usuários e empresas declara a permissão que exige.
func TestAdministrationOperationsDeclareTheirPermission(t *testing.T) {
	content := string(apispec.Spec)
	want := map[string]string{
		"listUsers": "users.manage", "getUsersSummary": "users.manage", "getUser": "users.manage",
		"updateUser": "users.manage", "deleteUser": "users.manage", "createInvitation": "users.manage",
		"resendInvitation": "users.manage", "cancelInvitation": "users.manage",
		"listCompanies": "companies.manage", "getCompaniesSummary": "companies.manage", "getCompany": "companies.manage",
		"createCompany": "companies.manage", "updateCompany": "companies.manage", "deleteCompany": "companies.manage",
	}
	for operation, permission := range want {
		t.Run(operation, func(t *testing.T) {
			_, after, found := strings.Cut(content, "operationId: "+operation+"\n")
			if !found {
				t.Fatalf("operation %s not found", operation)
			}
			// x-status and x-permission come right after operationId.
			header, _, _ := strings.Cut(after, "      security:")
			if !strings.Contains(header, "x-permission: "+permission) {
				t.Errorf("operation %s must declare x-permission: %s", operation, permission)
			}
		})
	}
}

// Operações ligadas por completo na API Go não carregam mais o marcador planned (ADR 0010);
// o login do mock (obsoleto) e o restante do contrato seguem planejados.
func TestImplementedOperationsAreMarked(t *testing.T) {
	content := string(apispec.Spec)
	implemented := []string{
		"startLogin", "finishLogin", "logout", "backchannelLogout", "getCurrentUser",
		"listUsers", "getUsersSummary", "getUser", "updateUser", "deleteUser",
		"createInvitation", "resendInvitation", "cancelInvitation",
		"listCompanies", "createCompany", "getCompaniesSummary", "getCompany", "updateCompany", "deleteCompany",
		"getMyProfile", "updateMyProfile",
	}
	stillPlanned := []string{"login", "changeMyPassword", "listFarms", "getDashboard", "sendPlannerMessage"}

	statusOf := func(operation string) string {
		_, after, found := strings.Cut(content, "operationId: "+operation+"\n")
		if !found {
			t.Fatalf("operation %s not found", operation)
		}
		line, _, _ := strings.Cut(after, "\n")
		return strings.TrimSpace(line)
	}
	for _, operation := range implemented {
		t.Run(operation, func(t *testing.T) {
			if got := statusOf(operation); got != "x-status: implemented" {
				t.Errorf("%s: %q, want x-status: implemented", operation, got)
			}
		})
	}
	for _, operation := range stillPlanned {
		t.Run(operation+" stays planned", func(t *testing.T) {
			if got := statusOf(operation); got != "x-status: planned" {
				t.Errorf("%s: %q, want x-status: planned", operation, got)
			}
		})
	}
}
