package domain

import "time"

// Audit action names. They are stable identifiers stored in audit_events.
const (
	AuditInvitationCreated    = "invitation.created"
	AuditInvitationResent     = "invitation.resent"
	AuditInvitationCancelled  = "invitation.cancelled"
	AuditInvitationAccepted   = "invitation.accepted"
	AuditAccountLinked        = "user.account_linked"
	AuditUserRoleChanged      = "user.role_changed"
	AuditUserRoleSynced       = "user.role_synced"
	AuditUserScopeChanged     = "user.scope_changed"
	AuditUserCompanyChanged   = "user.company_changed"
	AuditUserRenamed          = "user.renamed"
	AuditUserDeactivated      = "user.deactivated"
	AuditUserReactivated      = "user.reactivated"
	AuditUserDeleted          = "user.deleted"
	AuditAmbiguousRole        = "auth.ambiguous_role"
	AuditCompanyCreated       = "company.created"
	AuditCompanyUpdated       = "company.updated"
	AuditCompanyDeactivated   = "company.deactivated"
	AuditCompanyReactivated   = "company.reactivated"
	AuditCompanyDeleted       = "company.deleted"
	AuditFarmCreated          = "farm.created"
	AuditFarmUpdated          = "farm.updated"
	AuditOwnerCreated         = "owner.created"
	AuditOwnerUpdated         = "owner.updated"
	AuditOwnerLinksReplaced   = "owner.links_replaced"
	AuditFieldCreated         = "field.created"
	AuditFieldUpdated         = "field.updated"
	AuditOwnerDocumentAdded   = "owner.document_added"
	AuditOwnerDocumentRemoved = "owner.document_removed"

	AuditTerrainCalculationRequested = "terrain.calculation_requested"
	AuditTerrainAdjusted             = "terrain.adjusted"
	AuditTerrainAdjustmentRemoved    = "terrain.adjustment_removed"

	AuditRainGaugeCreated       = "rain_gauge.created"
	AuditRainGaugeUpdated       = "rain_gauge.updated"
	AuditRainReadingRecorded    = "rain_gauge.reading_recorded"
	AuditSoilProfileUpdated     = "field.soil_profile_updated"
	AuditSoilInspectionRecorded = "field.soil_inspection_recorded"
)

// Entity types of audit events.
const (
	AuditEntityUser       = "user"
	AuditEntityInvitation = "invitation"
	AuditEntityCompany    = "company"
	AuditEntityFarm       = "farm"
	AuditEntityOwner      = "owner"
	AuditEntityField      = "field"
	AuditEntityRainGauge  = "rain_gauge"
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
