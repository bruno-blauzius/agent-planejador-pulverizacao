package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/adapter/postgres/sqlcgen"
	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/domain"
	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/usecase"
)

// CreateSession implements usecase.SessionStore. The session and the
// last_login_at of the user's identity are written together.
func (r *Repository) CreateSession(ctx context.Context, record usecase.SessionRecord, tokenHash [32]byte) error {
	if !validUUID(record.ID) {
		return errors.New("create session: session id must be a UUID")
	}
	roles := record.GrantedRoles
	if roles == nil {
		roles = []string{} // granted_roles is NOT NULL
	}
	loggedInAt := time.Now().UTC()
	if !record.CreatedAt.IsZero() {
		loggedInAt = record.CreatedAt
	}
	err := r.atomic(ctx, func(q *sqlcgen.Queries) error {
		err := q.InsertSession(ctx, sqlcgen.InsertSessionParams{
			ID:              record.ID,
			UserID:          string(record.UserID),
			TokenHash:       tokenHash[:],
			Issuer:          record.Issuer,
			IdpSessionID:    optionalText(record.IdPSessionID),
			RefreshTokenEnc: record.RefreshTokenEnc,
			EncKeyID:        record.EncKeyID,
			GrantedRoles:    roles,
			AccessExpiresAt: record.AccessExpiresAt,
			ExpiresAt:       record.ExpiresAt,
		})
		if err != nil {
			return translateWrite("insert session", err)
		}
		err = q.TouchIdentityLogin(ctx, sqlcgen.TouchIdentityLoginParams{
			UserID: string(record.UserID), Issuer: record.Issuer, LastLoginAt: &loggedInAt,
		})
		return translateWrite("stamp last login", err)
	})
	if err != nil {
		return fmt.Errorf("create session: %w", err)
	}
	return nil
}

// FindSessionByTokenHash implements usecase.SessionStore: revoked and expired
// sessions are not found.
func (r *Repository) FindSessionByTokenHash(ctx context.Context, tokenHash [32]byte) (usecase.SessionRecord, error) {
	row, err := r.q.GetSessionByTokenHash(ctx, tokenHash[:])
	if err != nil {
		return usecase.SessionRecord{}, translateRead("find session", err)
	}
	return usecase.SessionRecord{
		ID:              row.ID,
		UserID:          domain.UserID(row.UserID),
		Issuer:          row.Issuer,
		IdPSessionID:    row.IdpSessionID.String,
		GrantedRoles:    row.GrantedRoles,
		AccessExpiresAt: row.AccessExpiresAt,
		ExpiresAt:       row.ExpiresAt,
		RefreshTokenEnc: row.RefreshTokenEnc,
		EncKeyID:        row.EncKeyID,
		LastSeenAt:      row.LastSeenAt,
		RevokedAt:       row.RevokedAt,
		CreatedAt:       row.CreatedAt,
	}, nil
}

// RenewSession implements usecase.SessionStore.
func (r *Repository) RenewSession(ctx context.Context, id string, grantedRoles []string, accessExpiresAt time.Time, refreshTokenEnc []byte, encKeyID int16) error {
	if !validUUID(id) {
		return fmt.Errorf("renew session: %w", usecase.ErrNotFound)
	}
	if grantedRoles == nil {
		grantedRoles = []string{}
	}
	var refresh []byte
	if len(refreshTokenEnc) > 0 {
		refresh = refreshTokenEnc
	}
	updated, err := r.q.RenewSession(ctx, sqlcgen.RenewSessionParams{
		ID: id, GrantedRoles: grantedRoles, AccessExpiresAt: accessExpiresAt,
		RefreshTokenEnc: refresh, EncKeyID: encKeyID,
	})
	if err != nil {
		return translateWrite("renew session", err)
	}
	if updated == 0 {
		return fmt.Errorf("renew session: %w", usecase.ErrNotFound)
	}
	return nil
}

// TouchSession implements usecase.SessionStore.
func (r *Repository) TouchSession(ctx context.Context, id string, at time.Time) error {
	if !validUUID(id) {
		return nil
	}
	err := r.q.TouchSession(ctx, sqlcgen.TouchSessionParams{
		ID: id, SeenAt: at,
		MinInterval: pgtype.Interval{Microseconds: r.opts.SessionTouchInterval.Microseconds(), Valid: true},
	})
	if err != nil {
		return fmt.Errorf("touch session: %w", err)
	}
	return nil
}

// RevokeSession implements usecase.SessionStore.
func (r *Repository) RevokeSession(ctx context.Context, id string, at time.Time) error {
	if !validUUID(id) {
		return nil
	}
	if err := r.q.RevokeSession(ctx, sqlcgen.RevokeSessionParams{ID: id, RevokedAt: &at}); err != nil {
		return fmt.Errorf("revoke session: %w", err)
	}
	return nil
}

// RevokeSessionsByIdPSession implements usecase.SessionStore.
func (r *Repository) RevokeSessionsByIdPSession(ctx context.Context, issuer, idpSessionID string, at time.Time) error {
	err := r.q.RevokeSessionsByIdPSession(ctx, sqlcgen.RevokeSessionsByIdPSessionParams{
		Issuer: issuer, IdpSessionID: optionalText(idpSessionID), RevokedAt: &at,
	})
	if err != nil {
		return fmt.Errorf("revoke sessions by idp session: %w", err)
	}
	return nil
}

// RevokeUserSessions implements usecase.SessionRevoker.
func (r *Repository) RevokeUserSessions(ctx context.Context, userID domain.UserID, at time.Time) error {
	if !validUUID(string(userID)) {
		return nil
	}
	if err := r.q.RevokeUserSessions(ctx, sqlcgen.RevokeUserSessionsParams{UserID: string(userID), RevokedAt: &at}); err != nil {
		return fmt.Errorf("revoke user sessions: %w", err)
	}
	return nil
}

// RevokeCompanySessions implements usecase.SessionRevoker.
func (r *Repository) RevokeCompanySessions(ctx context.Context, companyID domain.CompanyID, at time.Time) error {
	if !validUUID(string(companyID)) {
		return nil
	}
	if err := r.q.RevokeCompanySessions(ctx, sqlcgen.RevokeCompanySessionsParams{CompanyID: string(companyID), RevokedAt: &at}); err != nil {
		return fmt.Errorf("revoke company sessions: %w", err)
	}
	return nil
}

// ExpireUserAccess implements usecase.SessionRevoker.
func (r *Repository) ExpireUserAccess(ctx context.Context, userID domain.UserID, at time.Time) error {
	if !validUUID(string(userID)) {
		return nil
	}
	if err := r.q.ExpireUserAccess(ctx, sqlcgen.ExpireUserAccessParams{UserID: string(userID), AccessExpiresAt: at}); err != nil {
		return fmt.Errorf("expire user access: %w", err)
	}
	return nil
}

// ---- audit ----------------------------------------------------------------------------

// Record implements usecase.AuditRecorder. It only inserts: the runtime role
// has no UPDATE or DELETE on audit_events.
func (r *Repository) Record(ctx context.Context, event domain.AuditEvent) error {
	metadata := event.Metadata
	if metadata == nil {
		metadata = map[string]string{}
	}
	payload, err := json.Marshal(metadata)
	if err != nil {
		return fmt.Errorf("record audit event: encode metadata: %w", err)
	}
	var occurredAt *time.Time
	if !event.OccurredAt.IsZero() {
		occurredAt = &event.OccurredAt
	}
	var actor *string
	if event.ActorUserID != nil {
		value := string(*event.ActorUserID)
		actor = &value
	}
	err = r.atomic(ctx, func(q *sqlcgen.Queries) error {
		return translateWrite("insert audit event", q.InsertAuditEvent(ctx, sqlcgen.InsertAuditEventParams{
			CompanyID:   string(event.CompanyID),
			OccurredAt:  occurredAt,
			ActorKind:   string(event.ActorKind),
			ActorUserID: actor,
			ActorName:   event.ActorName,
			Action:      event.Action,
			EntityType:  event.EntityType,
			EntityID:    event.EntityID,
			Metadata:    payload,
		}))
	})
	if err != nil {
		return fmt.Errorf("record audit event: %w", err)
	}
	return nil
}
