package usecase

import (
	"context"
	"time"

	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/domain"
)

// SessionRecord is a browser session of the API (table auth_sessions, ADR 0015,
// D6). It is the neutral shape shared by the OIDC/session adapter (the consumer
// of SessionStore) and the Postgres adapter (the implementation).
//
// RefreshTokenEnc is already encrypted by the caller with the key identified
// by EncKeyID; the storage never sees the plain token or the key.
type SessionRecord struct {
	ID           string
	UserID       domain.UserID
	Issuer       string
	IdPSessionID string // Keycloak sid, empty when the token has none
	// GrantedRoles are the roles of the last renewed token
	// (resource_access.agentes-agro.roles).
	GrantedRoles []string
	// AccessExpiresAt is when the access token must be renewed; the API sets it
	// to now to force a renewal after a role or scope change.
	AccessExpiresAt time.Time
	ExpiresAt       time.Time
	RefreshTokenEnc []byte
	EncKeyID        int16
	LastSeenAt      *time.Time
	RevokedAt       *time.Time
	CreatedAt       time.Time
}

// SessionStore keeps the browser sessions. The cookie value is opaque; only its
// SHA-256 (tokenHash) is stored.
type SessionStore interface {
	// CreateSession stores a session; record.ID must be set (use IDGenerator).
	// It also stamps last_login_at on the user's identity for record.Issuer.
	CreateSession(ctx context.Context, record SessionRecord, tokenHash [32]byte) error
	// FindSessionByTokenHash returns ErrNotFound when the session does not
	// exist, was revoked or has expired.
	FindSessionByTokenHash(ctx context.Context, tokenHash [32]byte) (SessionRecord, error)
	// RenewSession stores the roles and access expiry of a renewed token. An
	// empty refreshTokenEnc keeps the stored refresh token and key id. It
	// returns ErrNotFound when the session is gone.
	RenewSession(ctx context.Context, id string, grantedRoles []string, accessExpiresAt time.Time, refreshTokenEnc []byte, encKeyID int16) error
	// TouchSession records last_seen_at, at most once per configured interval.
	TouchSession(ctx context.Context, id string, at time.Time) error
	// RevokeSession ends one session (idempotent).
	RevokeSession(ctx context.Context, id string, at time.Time) error
	// RevokeSessionsByIdPSession ends the sessions of an IdP session
	// (back-channel logout; idempotent).
	RevokeSessionsByIdPSession(ctx context.Context, issuer, idpSessionID string, at time.Time) error
}
