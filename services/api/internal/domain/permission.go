package domain

import (
	"fmt"
	"slices"
)

// Role is the single product role of a user (ADR 0013), granted through a
// Keycloak group (ADR 0015, D3).
type Role string

// Product roles.
const (
	RoleSuperAdmin       Role = "super_admin"
	RoleAdmin            Role = "admin"
	RolePlanningOperator Role = "planning_operator"
)

// Roles lists every product role.
func Roles() []Role {
	return []Role{RoleSuperAdmin, RoleAdmin, RolePlanningOperator}
}

// Valid reports whether r is a known product role.
func (r Role) Valid() bool { return slices.Contains(Roles(), r) }

// ParseRole converts text into a Role.
func ParseRole(value string) (Role, error) {
	role := Role(value)
	if !role.Valid() {
		return "", Invalid("role", "papel desconhecido: %q", value)
	}
	return role, nil
}

// Permission is a capability verified by the API (ADR 0013, D3). The names are
// the client roles of the Keycloak client `agentes-agro`.
type Permission string

// Known permissions. The role to permissions table lives in the realm and in
// api/openapi.yaml, never here (ADR 0015, D3).
const (
	PermDashboardView        Permission = "dashboard.view"
	PermCompaniesManage      Permission = "companies.manage"
	PermUsersManage          Permission = "users.manage"
	PermUsersManageAll       Permission = "users.manage_all"
	PermRegistryWrite        Permission = "registry.write"
	PermApplicationsRegister Permission = "applications.register"
	PermApplicationsConfirm  Permission = "applications.confirm"
	PermPlannerUse           Permission = "planner.use"
	// PermFieldConditionsRecord lets a user record rain gauge readings and soil
	// inspections (ADR 0022, addendum item 5).
	PermFieldConditionsRecord Permission = "field_conditions.record"
)

// Permissions lists the known permissions in canonical order.
func Permissions() []Permission {
	return []Permission{
		PermDashboardView,
		PermCompaniesManage,
		PermUsersManage,
		PermUsersManageAll,
		PermRegistryWrite,
		PermApplicationsRegister,
		PermApplicationsConfirm,
		PermPlannerUse,
		PermFieldConditionsRecord,
	}
}

// Access is what a token grants inside the product.
type Access struct {
	Role        Role
	Permissions []Permission
}

// restrictedToPlatformOperator lists the permissions that open cross-tenant
// scope. They are honoured only for a super admin of the platform operator
// company, whatever the token claims.
var restrictedToPlatformOperator = []Permission{PermUsersManageAll, PermCompaniesManage}

// ForCompany is defense in depth over the token: a client role is only as
// trustworthy as the realm configuration, so the cross-tenant permissions
// (users.manage_all, companies.manage) are dropped unless the role is super
// admin and the user belongs to the platform operator company.
func (a Access) ForCompany(company Company) Access {
	if a.Role == RoleSuperAdmin && company.IsPlatformOperator {
		return a
	}
	kept := make([]Permission, 0, len(a.Permissions))
	for _, permission := range a.Permissions {
		if !slices.Contains(restrictedToPlatformOperator, permission) {
			kept = append(kept, permission)
		}
	}
	return Access{Role: a.Role, Permissions: kept}
}

// ReadAccess derives the role and permissions from the effective client roles
// of the token (claim resource_access.agentes-agro.roles).
//
// Exactly one product role is required: none yields ErrNoAccessToProduct and
// more than one yields ErrAmbiguousRole (never "the highest wins"). The
// permissions are the intersection of the claim with the known enum; unknown
// names are ignored.
func ReadAccess(clientRoles []string) (Access, error) {
	var roles []Role
	for _, name := range clientRoles {
		role := Role(name)
		if role.Valid() && !slices.Contains(roles, role) {
			roles = append(roles, role)
		}
	}
	switch len(roles) {
	case 0:
		return Access{}, ErrNoAccessToProduct
	case 1:
	default:
		return Access{}, fmt.Errorf("%w: %v", ErrAmbiguousRole, roles)
	}

	var granted []Permission
	for _, permission := range Permissions() {
		if slices.Contains(clientRoles, string(permission)) {
			granted = append(granted, permission)
		}
	}
	return Access{Role: roles[0], Permissions: granted}, nil
}
