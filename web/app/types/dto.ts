/**
 * Wire format of services/api (api/openapi.yaml), snake_case as in the contract.
 * Only `services/http/` uses these types; the rest of the front uses the models.
 */
import type { GeoPoint, GeoJsonPolygon, PerimeterGeometry } from './geo'
import type { FarmStatus, PerimeterSourceFormat, ProductionType } from './farm'
import type { LinkRole, OwnerDocumentKind, PersonType } from './owner'
import type { SprayCondition, SprayOutlook } from './spray'
import type { Permission, UserRole } from './auth'
import type { CompanyPlan, CompanyStatus, UserStatus } from './admin'
import type { AgentAlertKind, AgentAlertTone } from './dashboard'
import type { Crop } from './crop'

export interface ApiErrorBodyDto {
  readonly error: {
    readonly code: string
    readonly message: string
    readonly details?: readonly { readonly field: string, readonly message: string }[]
  }
}

export interface PageDto<T> {
  readonly items: readonly T[]
  readonly total: number
  readonly page: number
  readonly page_size: number
}

export interface ItemsDto<T> {
  readonly items: readonly T[]
}

export interface FarmRefDto {
  readonly id: string
  readonly name: string
}

export interface CompanyRefDto {
  readonly id: string
  readonly name: string
}

export type FarmScopeDto =
  | { readonly mode: 'all' }
  | { readonly mode: 'selected', readonly farm_ids: readonly string[] }

export interface UserDto {
  readonly id: string
  readonly name: string
  readonly email: string
  readonly role: UserRole
  readonly primary_farm: FarmRefDto | null
  readonly company: CompanyRefDto
  readonly farm_scope: FarmScopeDto
  readonly permissions: readonly Permission[]
}


export type CropDto = Crop
export type SprayOutlookDto = SprayOutlook

export interface FarmOwnerShareDto {
  readonly owner_id: string
  readonly name: string
  readonly role: LinkRole
  readonly share_percent: number | null
}

export interface FarmSummaryDto {
  readonly company_id: string
  readonly id: string
  readonly name: string
  readonly code: string | null
  readonly city: string
  readonly state: string
  readonly status: FarmStatus
  readonly production_type: ProductionType
  readonly total_area_ha: number
  readonly productive_area_ha: number
  readonly field_count: number
  readonly crops: readonly CropDto[]
  readonly owners: readonly FarmOwnerShareDto[]
  readonly spray_outlook: SprayOutlookDto | null
  readonly forecast_point: GeoPoint | null
  readonly perimeter: PerimeterGeometry | null
  readonly created_at: string
}

export interface FarmLocationDto {
  readonly postal_code: string | null
  readonly car_number: string | null
  readonly forecast_point: GeoPoint | null
  readonly perimeter: PerimeterGeometry | null
}

export interface FarmDto extends FarmSummaryDto {
  readonly cooperative: string | null
  readonly location: FarmLocationDto
  readonly updated_at: string
}

export interface FarmOwnerShareInputDto {
  readonly owner_id: string
  readonly role: LinkRole
  readonly share_percent: number | null
}

export interface FarmInputDto {
  readonly name: string
  readonly code: string | null
  readonly cooperative: string | null
  readonly production_type: ProductionType
  readonly city: string
  readonly state: string
  readonly status: Exclude<FarmStatus, 'pending'>
  readonly location: FarmLocationDto
  readonly owners: readonly FarmOwnerShareInputDto[]
}

export interface PerimeterImportDto {
  readonly source_format: PerimeterSourceFormat
  readonly perimeter: PerimeterGeometry
  readonly area_ha: number
  readonly centroid: GeoPoint
  readonly municipality?: string | null
  readonly state?: string | null
  readonly datum?: string | null
  readonly field_count?: number | null
}

export interface FieldDto {
  readonly id: string
  readonly farm_id: string
  readonly name: string
  readonly block: string | null
  readonly area_ha: number
  readonly crop: CropDto
  readonly variety: string | null
  readonly season: string | null
  readonly planting_date: string | null
  readonly row_spacing_m: number | null
  readonly plant_spacing_m: number | null
  readonly plants_per_ha: number | null
  readonly polygon: GeoJsonPolygon
  readonly spray_condition: SprayCondition | null
  readonly last_application_at: string | null
}

export interface FieldInputDto {
  readonly name: string
  readonly block: string | null
  readonly crop_code: string
  readonly variety: string | null
  readonly season: string | null
  readonly planting_date: string | null
  readonly row_spacing_m: number | null
  readonly plant_spacing_m: number | null
  readonly polygon: GeoJsonPolygon
}

export interface OwnerFarmLinkDto {
  readonly farm_id: string
  readonly farm_name: string
  readonly role: LinkRole
  readonly share_percent: number | null
}

export interface OwnerFarmLinkInputDto {
  readonly farm_id: string
  readonly role: LinkRole
  readonly share_percent: number | null
}

export interface OwnerDocumentDto {
  readonly id: string
  readonly kind: OwnerDocumentKind
  readonly file_name: string
  readonly uploaded_at: string
}

export interface OwnerSummaryDto {
  readonly id: string
  readonly person_type: PersonType
  readonly name: string
  readonly document: string
  readonly has_pending_documents: boolean
  readonly links: readonly OwnerFarmLinkDto[]
}

export interface OwnerDto extends OwnerSummaryDto {
  readonly birth_date: string | null
  readonly phone: string | null
  readonly email: string | null
  readonly window_alerts_enabled: boolean
  readonly documents: readonly OwnerDocumentDto[]
  readonly created_at: string
  readonly updated_at: string
}

export interface OwnerInputDto {
  readonly person_type: PersonType
  readonly name: string
  readonly document: string
  readonly birth_date: string | null
  readonly phone: string | null
  readonly email: string | null
  readonly window_alerts_enabled: boolean
}

// ---- Page 03 (ADR 0013) -----------------------------------------------------

export interface UserAccountDto {
  readonly id: string
  readonly name: string
  readonly email: string
  readonly role: UserRole
  readonly company: CompanyRefDto
  readonly farm_scope: FarmScopeDto
  readonly status: UserStatus
  readonly last_access_at: string | null
  readonly invitation: { readonly id: string, readonly expires_at: string } | null
}

export interface UsersSummaryDto {
  readonly total_users: number
  readonly company_count: number
  readonly pending_invitations: number
  readonly by_role: Readonly<Record<UserRole, number>>
}

export interface UserUpdateDto {
  readonly name?: string
  readonly role?: UserRole
  readonly company_id?: string
  readonly farm_scope?: FarmScopeDto
  readonly status?: 'active' | 'inactive'
}

export interface InvitationInputDto {
  readonly name: string
  readonly email: string
  readonly role: UserRole
  readonly company_id: string
  readonly farm_scope: FarmScopeDto
}

export interface CompanyDto {
  readonly id: string
  readonly name: string
  readonly document: string
  readonly city: string
  readonly state: string
  readonly plan: CompanyPlan
  readonly status: CompanyStatus
  readonly trial_ends_at: string | null
  readonly farm_count: number
  readonly user_count: number
  readonly created_at: string
}

export interface CompaniesSummaryDto {
  readonly active_companies: number
  readonly trial_companies: number
  readonly farm_count: number
  readonly monitored_area_ha: number
  readonly user_count: number
  readonly pending_invitations: number
  readonly applications_this_month: number
  readonly applications_previous_month: number
}

export interface CompanyInputDto {
  readonly name: string
  readonly document: string
  readonly city: string
  readonly state: string
  readonly plan: CompanyPlan
}

export interface CompanyUpdateDto {
  readonly name?: string
  readonly city?: string
  readonly state?: string
  readonly plan?: CompanyPlan
  readonly status?: CompanyStatus
}

export interface ProfileDto {
  readonly id: string
  readonly name: string
  readonly email: string
  readonly role: UserRole
  readonly company: CompanyRefDto
  readonly farm_scope: FarmScopeDto
  readonly password_changed_at: string | null
}

export interface DashboardDto {
  readonly generated_at: string
  readonly farm_id: string | null
  readonly farm_count: number
  readonly indicators: {
    readonly suitable_windows_today: { readonly count: number, readonly field_labels: readonly string[], readonly next_start_at: string | null }
    readonly applications_this_month: { readonly count: number, readonly confirmed_this_week: number }
    readonly sprayed_area: { readonly area_ha: number, readonly productive_area_ha: number }
    readonly pending_items: { readonly count: number, readonly headline: string | null }
  }
  readonly windows_vs_applications: {
    readonly days: readonly { readonly date: string, readonly suitable_hours: number, readonly applications: number }[]
    readonly summary: string | null
  }
  readonly upcoming_windows: readonly {
    readonly field_id: string
    readonly field_label: string
    readonly farm_id: string
    readonly date: string
    readonly start_time: string | null
    readonly end_time: string | null
    readonly crop_variety: string | null
    readonly operation: string | null
    readonly condition: SprayCondition
  }[]
  readonly agent_alerts: readonly {
    readonly id: string
    readonly kind: AgentAlertKind
    readonly tone: AgentAlertTone
    readonly title: string
    readonly description: string
  }[]
  readonly recent_activity: readonly {
    readonly id: string
    readonly actor: { readonly kind: 'user' | 'agent', readonly name: string }
    readonly description: string
    readonly occurred_at: string
  }[]
}
