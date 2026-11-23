package domain

import "time"

// Invitation records an invitation of a user to a company. Role, company and
// scope live on the user. The link and its expiry belong to the identity
// provider; ExpiresAt only mirrors it for display (ADR 0015, D7).
type Invitation struct {
	ID        InvitationID
	CompanyID CompanyID
	UserID    UserID
	InvitedBy UserID
	// AccountCreated is true when this invitation created the account in the
	// realm. Only then may the account be deleted on cancellation (ADR 0015, D5, rule 2).
	AccountCreated bool
	ExpiresAt      time.Time
	LastSentAt     time.Time
	SendCount      int
	AcceptedAt     *time.Time
	CancelledAt    *time.Time
	CreatedAt      time.Time
}

// NewInvitation creates an open invitation, already counted as sent once.
func NewInvitation(id InvitationID, user User, invitedBy UserID, accountCreated bool, now, expiresAt time.Time) Invitation {
	return Invitation{
		ID:             id,
		CompanyID:      user.CompanyID,
		UserID:         user.ID,
		InvitedBy:      invitedBy,
		AccountCreated: accountCreated,
		ExpiresAt:      expiresAt,
		LastSentAt:     now,
		SendCount:      1,
		CreatedAt:      now,
	}
}

// IsOpen reports whether the invitation was neither accepted nor cancelled.
func (i Invitation) IsOpen() bool {
	return i.AcceptedAt == nil && i.CancelledAt == nil
}

// Resend renews the expiry and counts one more send.
func (i *Invitation) Resend(now, expiresAt time.Time) error {
	if !i.IsOpen() {
		return ErrInvitationClosed
	}
	i.ExpiresAt = expiresAt
	i.LastSentAt = now
	i.SendCount++
	return nil
}

// Accept marks the invitation as used by the first login.
func (i *Invitation) Accept(now time.Time) error {
	if !i.IsOpen() {
		return ErrInvitationClosed
	}
	i.AcceptedAt = &now
	return nil
}

// Cancel withdraws the invitation.
func (i *Invitation) Cancel(now time.Time) error {
	if !i.IsOpen() {
		return ErrInvitationClosed
	}
	i.CancelledAt = &now
	return nil
}
