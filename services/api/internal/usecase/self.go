package usecase

import (
	"context"
	"errors"
	"fmt"

	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/domain"
)

// CompanyRef identifies a company for display.
type CompanyRef struct {
	ID   domain.CompanyID
	Name string
}

// Me is the session of the authenticated user (GET /auth/me). Role and
// permissions come from the token (the Principal); the rest from the database.
type Me struct {
	ID          domain.UserID
	Name        string
	Email       domain.Email
	Role        domain.Role
	PrimaryFarm *FarmRef
	Company     CompanyRef
	FarmScope   domain.FarmScope
	Permissions []domain.Permission
}

// GetMeInput is the request of GET /auth/me.
type GetMeInput struct {
	Actor domain.Principal
}

// GetMe returns the session of the actor.
type GetMe struct {
	users     UserFinder
	companies CompanyFinder
	farms     FarmDirectory
}

// NewGetMe builds the use case.
func NewGetMe(users UserFinder, companies CompanyFinder, farms FarmDirectory) *GetMe {
	return &GetMe{users: users, companies: companies, farms: farms}
}

// Execute loads the actor's user, company and primary farm.
func (uc *GetMe) Execute(ctx context.Context, in GetMeInput) (Me, error) {
	user, company, err := loadSelf(ctx, uc.users, uc.companies, in.Actor)
	if err != nil {
		return Me{}, err
	}
	me := Me{
		ID:          user.ID,
		Name:        user.Name,
		Email:       user.Email,
		Role:        in.Actor.Role,
		Company:     CompanyRef{ID: company.ID, Name: company.Name},
		FarmScope:   user.FarmScope,
		Permissions: in.Actor.Permissions,
	}
	if user.PrimaryFarmID != nil {
		farm, err := uc.farms.FindFarmRef(ctx, user.CompanyID, *user.PrimaryFarmID)
		switch {
		case err == nil:
			me.PrimaryFarm = &farm
		case errors.Is(err, ErrNotFound):
		default:
			return Me{}, fmt.Errorf("find primary farm: %w", err)
		}
	}
	return me, nil
}

// Profile is the page of the authenticated user (GET /me/profile).
type Profile struct {
	ID        domain.UserID
	Name      string
	Email     domain.Email
	Role      domain.Role
	Company   CompanyRef
	FarmScope domain.FarmScope
}

// GetProfileInput is the request of GET /me/profile.
type GetProfileInput struct {
	Actor domain.Principal
}

// GetProfile returns the profile of the actor. Any role may read it.
type GetProfile struct {
	users     UserFinder
	companies CompanyFinder
}

// NewGetProfile builds the use case.
func NewGetProfile(users UserFinder, companies CompanyFinder) *GetProfile {
	return &GetProfile{users: users, companies: companies}
}

// Execute loads the profile.
func (uc *GetProfile) Execute(ctx context.Context, in GetProfileInput) (Profile, error) {
	user, company, err := loadSelf(ctx, uc.users, uc.companies, in.Actor)
	if err != nil {
		return Profile{}, err
	}
	return profileOf(user, company, in.Actor.Role), nil
}

// UpdateProfileInput is the request of PATCH /me/profile. Only the name can be
// changed: e-mail is read-only, and role and company belong to administrators.
type UpdateProfileInput struct {
	Actor domain.Principal
	Name  string
}

// UpdateProfile changes the display name of the actor.
type UpdateProfile struct {
	users     UserFinder
	companies CompanyFinder
	tx        TxRunner
}

// NewUpdateProfile builds the use case.
func NewUpdateProfile(users UserFinder, companies CompanyFinder, tx TxRunner) *UpdateProfile {
	return &UpdateProfile{users: users, companies: companies, tx: tx}
}

// Execute renames the actor and returns the updated profile.
func (uc *UpdateProfile) Execute(ctx context.Context, in UpdateProfileInput) (Profile, error) {
	user, company, err := loadSelf(ctx, uc.users, uc.companies, in.Actor)
	if err != nil {
		return Profile{}, err
	}
	if err := user.Rename(in.Name); err != nil {
		return Profile{}, err
	}
	err = uc.tx.InTx(ctx, func(ctx context.Context, tx Stores) error {
		if err := tx.Users.UpdateUser(ctx, user); err != nil {
			return fmt.Errorf("update user: %w", err)
		}
		return nil
	})
	if err != nil {
		return Profile{}, err
	}
	return profileOf(user, company, in.Actor.Role), nil
}

func profileOf(user domain.User, company domain.Company, role domain.Role) Profile {
	return Profile{
		ID:        user.ID,
		Name:      user.Name,
		Email:     user.Email,
		Role:      role,
		Company:   CompanyRef{ID: company.ID, Name: company.Name},
		FarmScope: user.FarmScope,
	}
}

// loadSelf reads the actor's own user and company, always inside the actor's company.
func loadSelf(ctx context.Context, users UserFinder, companies CompanyFinder, actor domain.Principal) (domain.User, domain.Company, error) {
	user, err := users.FindUser(ctx, OnlyCompany(actor.CompanyID), actor.UserID)
	if err != nil {
		return domain.User{}, domain.Company{}, fmt.Errorf("find current user: %w", err)
	}
	company, err := loadCompany(ctx, companies, user.CompanyID)
	if err != nil {
		return domain.User{}, domain.Company{}, err
	}
	return user, company, nil
}
