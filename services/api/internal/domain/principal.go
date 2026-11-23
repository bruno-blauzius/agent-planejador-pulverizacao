package domain

import "slices"

// Principal is who is calling: the authenticated user as the API resolved it
// for one request. Role and permissions come from the token; company, scope
// and status come from the API database (ADR 0015, D3).
type Principal struct {
	UserID      UserID
	CompanyID   CompanyID
	Name        string
	Email       Email
	Role        Role
	Permissions []Permission
	FarmScope   FarmScope
}

// Can reports whether the principal holds the permission.
func (p Principal) Can(permission Permission) bool {
	return slices.Contains(p.Permissions, permission)
}

// ManagesAllCompanies reports whether the principal may act on every company
// (super admin) instead of only their own.
func (p Principal) ManagesAllCompanies() bool { return p.Can(PermUsersManageAll) }
