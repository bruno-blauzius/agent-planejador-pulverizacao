package domain

import "time"

// Audit action names. They are stable identifiers stored in audit_events.
const (
	AuditInvitationCreated   = "invitation.created"
	AuditInvitationResent    = "invitation.resent"
	AuditInvitationCancelled = "invitation.cancelled"
	AuditInvitationAccepted  = "invitation.accepted"
	AuditAccountLinked       = "user.account_linked"
	AuditUserRoleChanged     = "user.role_changed"
	AuditUserRoleSynced      = "user.role_synced"
	AuditUserScopeChanged    = "user.scope_changed"
	AuditUserCompanyChanged  = "user.company_changed"
	AuditUserRenamed         = "user.renamed"
	AuditUserDeactivated     = "user.deactivated"
	AuditUserReactivated     = "user.reactivated"
	AuditUserDeleted         = "user.deleted"
	AuditAmbiguousRole       = "auth.ambiguous_role"
	AuditCompanyCreated      = "company.created"
	AuditCompanyUpdated      = "company.updated"
	AuditCompanyDeactivated  = "company.deactivated"
	AuditCompanyReactivated  = "company.reactivated"
	AuditCompanyDeleted      = "company.deleted"
)

// Entity types of audit events.
const (
	AuditEntityUser       = "user"
	AuditEntityInvitation = "invitation"
	AuditEntityCompany    = "company"
)

// ActorKind says who performed an audited action.
type ActorKind string

// Actor kinds.
const (
	ActorUser   ActorKind = "user"
	ActorSystem ActorKind = "system"
)

// AuditEvent is an append-only record of a sensitive action. Metadata must
// never carry passwords, tokens or documents (ADR 0015, security).
type AuditEvent struct {
	CompanyID   CompanyID
	OccurredAt  time.Time
	ActorKind   ActorKind
	ActorUserID *UserID
	ActorName   string
	Action      string
	EntityType  string
	EntityID    string
	Metadata    map[string]string
}
