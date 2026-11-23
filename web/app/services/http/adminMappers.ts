/** DTO <-> model of page 03 (ADR 0013): users, companies, profile, dashboard. */
import type {
  Company,
  CompaniesSummary,
  CompanyInput,
  CompanyUpdateInput,
  InvitationInput,
  UserAccount,
  UsersSummary,
  UserUpdateInput,
} from '~/types/admin'
import type { Dashboard } from '~/types/dashboard'
import type {
  CompaniesSummaryDto,
  CompanyDto,
  CompanyInputDto,
  CompanyUpdateDto,
  DashboardDto,
  InvitationInputDto,
  ProfileDto,
  UserAccountDto,
  UsersSummaryDto,
  UserUpdateDto,
} from '~/types/dto'
import type { Profile } from '~/types/profile'
import { toFarmScope, toFarmScopeDto } from './mappers'

const dateOrNull = (value: string | null) => (value ? new Date(value) : null)

type Mutable<T> = { -readonly [K in keyof T]: T[K] }

export function toUserAccount(dto: UserAccountDto): UserAccount {
  return {
    id: dto.id,
    name: dto.name,
    email: dto.email,
    role: dto.role,
    company: dto.company,
    farmScope: toFarmScope(dto.farm_scope),
    status: dto.status,
    lastAccessAt: dateOrNull(dto.last_access_at),
    invitation: dto.invitation ? { id: dto.invitation.id, expiresAt: new Date(dto.invitation.expires_at) } : null,
  }
}

export function toUsersSummary(dto: UsersSummaryDto): UsersSummary {
  return {
    totalUsers: dto.total_users,
    companyCount: dto.company_count,
    pendingInvitations: dto.pending_invitations,
    byRole: dto.by_role,
  }
}

/** PATCH body with only the keys that change. */
export function toUserUpdateDto(input: UserUpdateInput): UserUpdateDto {
  const dto: Mutable<UserUpdateDto> = {}
  if (input.name !== undefined) dto.name = input.name.trim()
  if (input.role !== undefined) dto.role = input.role
  if (input.companyId !== undefined) dto.company_id = input.companyId
  if (input.farmScope !== undefined) dto.farm_scope = toFarmScopeDto(input.farmScope)
  if (input.status !== undefined) dto.status = input.status
  return dto
}

export function toInvitationInputDto(input: InvitationInput): InvitationInputDto {
  return {
    name: input.name.trim(),
    email: input.email.trim(),
    role: input.role,
    company_id: input.companyId,
    farm_scope: toFarmScopeDto(input.farmScope),
  }
}

export function toCompany(dto: CompanyDto): Company {
  return {
    id: dto.id,
    name: dto.name,
    document: dto.document,
    city: dto.city,
    state: dto.state,
    plan: dto.plan,
    status: dto.status,
    trialEndsAt: dateOrNull(dto.trial_ends_at),
    farmCount: dto.farm_count,
    userCount: dto.user_count,
    createdAt: new Date(dto.created_at),
  }
}

export function toCompaniesSummary(dto: CompaniesSummaryDto): CompaniesSummary {
  return {
    activeCompanies: dto.active_companies,
    trialCompanies: dto.trial_companies,
    farmCount: dto.farm_count,
    monitoredAreaHa: dto.monitored_area_ha,
    userCount: dto.user_count,
    pendingInvitations: dto.pending_invitations,
    applicationsThisMonth: dto.applications_this_month,
    applicationsPreviousMonth: dto.applications_previous_month,
  }
}

export function toCompanyInputDto(input: CompanyInput): CompanyInputDto {
  return { ...input }
}

/** PATCH body with only the keys that change. */
export function toCompanyUpdateDto(input: CompanyUpdateInput): CompanyUpdateDto {
  const dto: Mutable<CompanyUpdateDto> = {}
  if (input.name !== undefined) dto.name = input.name
  if (input.city !== undefined) dto.city = input.city
  if (input.state !== undefined) dto.state = input.state
  if (input.plan !== undefined) dto.plan = input.plan
  if (input.status !== undefined) dto.status = input.status
  return dto
}

export function toProfile(dto: ProfileDto): Profile {
  return {
    id: dto.id,
    name: dto.name,
    email: dto.email,
    role: dto.role,
    company: dto.company,
    farmScope: toFarmScope(dto.farm_scope),
    passwordChangedAt: dateOrNull(dto.password_changed_at),
  }
}

export function toDashboard(dto: DashboardDto): Dashboard {
  const { indicators } = dto
  return {
    generatedAt: new Date(dto.generated_at),
    farmId: dto.farm_id,
    farmCount: dto.farm_count,
    indicators: {
      suitableWindowsToday: {
        count: indicators.suitable_windows_today.count,
        fieldLabels: indicators.suitable_windows_today.field_labels,
        nextStartAt: dateOrNull(indicators.suitable_windows_today.next_start_at),
      },
      applicationsThisMonth: {
        count: indicators.applications_this_month.count,
        confirmedThisWeek: indicators.applications_this_month.confirmed_this_week,
      },
      sprayedArea: { areaHa: indicators.sprayed_area.area_ha, productiveAreaHa: indicators.sprayed_area.productive_area_ha },
      pendingItems: { count: indicators.pending_items.count, headline: indicators.pending_items.headline },
    },
    windowsVsApplications: {
      days: dto.windows_vs_applications.days.map(day => ({ date: day.date, suitableHours: day.suitable_hours, applications: day.applications })),
      summary: dto.windows_vs_applications.summary,
    },
    upcomingWindows: dto.upcoming_windows.map(window => ({
      fieldId: window.field_id,
      fieldLabel: window.field_label,
      farmId: window.farm_id,
      date: window.date,
      startTime: window.start_time,
      endTime: window.end_time,
      cropVariety: window.crop_variety,
      operation: window.operation,
      condition: window.condition,
    })),
    agentAlerts: dto.agent_alerts.map(alert => ({ ...alert })),
    recentActivity: dto.recent_activity.map(entry => ({
      id: entry.id,
      actor: entry.actor,
      description: entry.description,
      occurredAt: new Date(entry.occurred_at),
    })),
  }
}
