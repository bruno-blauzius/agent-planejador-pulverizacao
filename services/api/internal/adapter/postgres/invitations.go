package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/adapter/postgres/sqlcgen"
	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/domain"
	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/usecase"
)

func invitationFromRow(row sqlcgen.Invitation) domain.Invitation {
	return domain.Invitation{
		ID:             domain.InvitationID(row.ID),
		CompanyID:      domain.CompanyID(row.CompanyID),
		UserID:         domain.UserID(row.UserID),
		InvitedBy:      domain.UserID(row.InvitedByUserID),
		AccountCreated: row.AccountCreated,
		ExpiresAt:      row.ExpiresAt,
		LastSentAt:     row.LastSentAt,
		SendCount:      int(row.SendCount),
		AcceptedAt:     row.AcceptedAt,
		CancelledAt:    row.CancelledAt,
		CreatedAt:      row.CreatedAt,
	}
}

// FindInvitation implements usecase.InvitationFinder. Invitations of deleted
// users are hidden, like the users themselves.
func (r *Repository) FindInvitation(ctx context.Context, scope usecase.Scope, id domain.InvitationID) (domain.Invitation, error) {
	company, ok := scopeCompany(scope)
	if !ok {
		return domain.Invitation{}, fmt.Errorf("find invitation: %w", usecase.ErrNotFound)
	}
	row, err := r.q.GetInvitation(ctx, sqlcgen.GetInvitationParams{ID: string(id), CompanyID: company})
	if err != nil {
		return domain.Invitation{}, translateRead("find invitation", err)
	}
	return invitationFromRow(row), nil
}

// FindOpenInvitationByUser implements usecase.InvitationFinder.
func (r *Repository) FindOpenInvitationByUser(ctx context.Context, userID domain.UserID) (domain.Invitation, error) {
	row, err := r.q.GetOpenInvitationByUser(ctx, string(userID))
	if err != nil {
		return domain.Invitation{}, translateRead("find open invitation", err)
	}
	return invitationFromRow(row), nil
}

// InsertInvitation implements usecase.InvitationWriter.
func (r *Repository) InsertInvitation(ctx context.Context, invitation domain.Invitation) error {
	createdAt := invitation.CreatedAt
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}
	err := r.atomic(ctx, func(q *sqlcgen.Queries) error {
		return translateWrite("insert invitation", q.InsertInvitation(ctx, sqlcgen.InsertInvitationParams{
			ID:              string(invitation.ID),
			CompanyID:       string(invitation.CompanyID),
			UserID:          string(invitation.UserID),
			InvitedByUserID: string(invitation.InvitedBy),
			AccountCreated:  invitation.AccountCreated,
			ExpiresAt:       invitation.ExpiresAt,
			LastSentAt:      invitation.LastSentAt,
			SendCount:       clampInt32(int64(invitation.SendCount)),
			AcceptedAt:      invitation.AcceptedAt,
			CancelledAt:     invitation.CancelledAt,
			CreatedAt:       createdAt,
		}))
	})
	if err != nil {
		return fmt.Errorf("insert invitation: %w", err)
	}
	return nil
}

// UpdateInvitation implements usecase.InvitationWriter: resend, accept and
// cancel change only expiry, send counters and the outcome timestamps.
func (r *Repository) UpdateInvitation(ctx context.Context, invitation domain.Invitation) error {
	if !validUUID(string(invitation.ID)) {
		return fmt.Errorf("update invitation: %w", usecase.ErrNotFound)
	}
	updated, err := r.q.UpdateInvitation(ctx, sqlcgen.UpdateInvitationParams{
		ID:          string(invitation.ID),
		ExpiresAt:   invitation.ExpiresAt,
		LastSentAt:  invitation.LastSentAt,
		SendCount:   clampInt32(int64(invitation.SendCount)),
		AcceptedAt:  invitation.AcceptedAt,
		CancelledAt: invitation.CancelledAt,
	})
	if err != nil {
		return translateWrite("update invitation", err)
	}
	if updated == 0 {
		return fmt.Errorf("update invitation: %w", usecase.ErrNotFound)
	}
	return nil
}

// ---- farms -------------------------------------------------------------------------

// CheckFarmsInCompany implements usecase.FarmDirectory.
func (r *Repository) CheckFarmsInCompany(ctx context.Context, companyID domain.CompanyID, ids []domain.FarmID) error {
	unique := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		if !validUUID(string(id)) {
			return fmt.Errorf("check farms: %w", usecase.ErrFarmNotInCompany)
		}
		unique[string(id)] = struct{}{}
	}
	if len(unique) == 0 {
		return nil
	}
	if !validUUID(string(companyID)) {
		return fmt.Errorf("check farms: %w", usecase.ErrFarmNotInCompany)
	}
	farmIDs := make([]string, 0, len(unique))
	for id := range unique {
		farmIDs = append(farmIDs, id)
	}
	found, err := r.q.CountFarmsInCompany(ctx, sqlcgen.CountFarmsInCompanyParams{CompanyID: string(companyID), FarmIds: farmIDs})
	if err != nil {
		return fmt.Errorf("check farms: %w", err)
	}
	if int(found) != len(unique) {
		return fmt.Errorf("check farms: %w", usecase.ErrFarmNotInCompany)
	}
	return nil
}

// FindFarmRef implements usecase.FarmDirectory.
func (r *Repository) FindFarmRef(ctx context.Context, companyID domain.CompanyID, id domain.FarmID) (usecase.FarmRef, error) {
	if !validUUID(string(companyID)) {
		return usecase.FarmRef{}, fmt.Errorf("find farm: %w", usecase.ErrNotFound)
	}
	row, err := r.q.GetFarmRef(ctx, sqlcgen.GetFarmRefParams{CompanyID: string(companyID), ID: string(id)})
	if err != nil {
		return usecase.FarmRef{}, translateRead("find farm", err)
	}
	return usecase.FarmRef{ID: domain.FarmID(row.ID), Name: row.Name}, nil
}
