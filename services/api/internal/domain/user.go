package domain

import (
	"fmt"
	"time"
)

// UserStatus is the status of a user in the product only. Deactivating a user
// never disables the account in the shared realm (ADR 0015, D5).
type UserStatus string

// User statuses.
const (
	UserInvited  UserStatus = "invited"
	UserActive   UserStatus = "active"
	UserInactive UserStatus = "inactive"
)

// Valid reports whether s is a known status.
func (s UserStatus) Valid() bool {
	return s == UserInvited || s == UserActive || s == UserInactive
}

// User is a person with access to one company of the product. Role is a
// projection of the Keycloak client role (ADR 0015, D3).
type User struct {
	ID            UserID
	CompanyID     CompanyID
	Name          string
	Email         Email
	Role          Role
	Status        UserStatus
	FarmScope     FarmScope
	PrimaryFarmID *FarmID
	LastAccessAt  *time.Time
	CreatedAt     time.Time
	DeletedAt     *time.Time
	Version       int
}

// CheckAssignment validates that a role and farm scope may belong to a user of
// the company: super admin only exists in the platform operator company, and
// only a planning operator may be limited to selected farms.
func CheckAssignment(company Company, role Role, scope FarmScope) error {
	var problems Violations
	if !role.Valid() {
		problems.Add("role", fmt.Sprintf("papel desconhecido: %q", role))
	}
	if role == RoleSuperAdmin && !company.IsPlatformOperator {
		problems.Add("role", "super admin só existe na empresa operadora da plataforma")
	}
	if scope.IsSelected() && role != RolePlanningOperator {
		problems.Add("farm_scope", "somente o operador de planejamento pode ter fazendas selecionadas")
	}
	return problems.Err()
}

// NewInvitedUser creates a user in the invited status after validating every field.
func NewInvitedUser(id UserID, company Company, name, email string, role Role, scope FarmScope, now time.Time) (User, error) {
	var problems Violations
	parsedName, err := ParseName("name", name)
	if mergeErr := problems.Merge(err); mergeErr != nil {
		return User{}, mergeErr
	}
	parsedEmail, err := ParseEmail(email)
	if mergeErr := problems.Merge(err); mergeErr != nil {
		return User{}, mergeErr
	}
	if mergeErr := problems.Merge(CheckAssignment(company, role, scope)); mergeErr != nil {
		return User{}, mergeErr
	}
	if err := problems.Err(); err != nil {
		return User{}, err
	}
	return User{
		ID:        id,
		CompanyID: company.ID,
		Name:      parsedName,
		Email:     parsedEmail,
		Role:      role,
		Status:    UserInvited,
		FarmScope: scope,
		CreatedAt: now,
	}, nil
}

// Assign changes company, role and scope together, enforcing the invariants.
// The primary farm is cleared when the user moves to another company or the
// scope no longer includes it.
func (u *User) Assign(company Company, role Role, scope FarmScope) error {
	if err := CheckAssignment(company, role, scope); err != nil {
		return err
	}
	if u.CompanyID != company.ID || (u.PrimaryFarmID != nil && !scope.Allows(*u.PrimaryFarmID)) {
		u.PrimaryFarmID = nil
	}
	u.CompanyID = company.ID
	u.Role = role
	u.FarmScope = scope
	return nil
}

// Rename changes the display name.
func (u *User) Rename(name string) error {
	parsed, err := ParseName("name", name)
	if err != nil {
		return err
	}
	u.Name = parsed
	return nil
}

// Activate accepts the invitation: invited -> active.
func (u *User) Activate(now time.Time) error {
	if u.Status != UserInvited {
		return fmt.Errorf("%w: %s -> %s", ErrInvalidTransition, u.Status, UserActive)
	}
	u.Status = UserActive
	u.LastAccessAt = &now
	return nil
}

// Deactivate blocks the user in the product: active -> inactive.
func (u *User) Deactivate() error {
	if u.Status != UserActive {
		return fmt.Errorf("%w: %s -> %s", ErrInvalidTransition, u.Status, UserInactive)
	}
	u.Status = UserInactive
	return nil
}

// Reactivate restores a deactivated user: inactive -> active.
func (u *User) Reactivate() error {
	if u.Status != UserInactive {
		return fmt.Errorf("%w: %s -> %s", ErrInvalidTransition, u.Status, UserActive)
	}
	u.Status = UserActive
	return nil
}

// RecordAccess stores the last time the user was seen.
func (u *User) RecordAccess(now time.Time) {
	u.LastAccessAt = &now
}

// MarkDeleted soft deletes the user. A deleted user is always inactive
// (ck_users_deleted_inactive), whatever status it had, such as an invited user
// whose invitation was cancelled.
func (u *User) MarkDeleted(now time.Time) {
	u.Status = UserInactive
	u.DeletedAt = &now
}

// IsDeleted reports whether the user was soft deleted.
func (u User) IsDeleted() bool { return u.DeletedAt != nil }
