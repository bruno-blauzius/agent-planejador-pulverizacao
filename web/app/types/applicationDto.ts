/** Wire format (snake_case) of the application history endpoints, ADR 0016. Only `services/http/` uses it. */
import type { ApplicationConditionsSource, ApplicationStatus } from './application'
import type { DoseDto } from './plannerDto'
import type { ProductKind } from './planner'

export interface PersonRefDto {
  readonly id: string
  readonly name: string
}

export interface ApplicationSummaryDto {
  readonly id: string
  readonly status: ApplicationStatus
  readonly farm_id: string
  readonly window: { readonly date: string, readonly start_time: string, readonly end_time: string }
  readonly field: {
    readonly id: string
    readonly label: string
    readonly name: string
    readonly variety: string | null
    readonly block: string | null
  }
  readonly product: {
    readonly kind: ProductKind
    readonly name: string
    readonly dose: DoseDto
    readonly mix_rate_l_per_ha: number
  }
  readonly area_ha: number
  readonly conditions: {
    readonly source: ApplicationConditionsSource
    readonly delta_t_c: number | null
    readonly wind_kmh: number | null
    readonly rain_mm: number | null
  }
  readonly operator: PersonRefDto
}

export interface ApplicationPageDto {
  readonly items: readonly ApplicationSummaryDto[]
  readonly total: number
  readonly page: number
  readonly page_size: number
  readonly facets: {
    readonly fields: readonly { readonly id: string, readonly label: string, readonly name: string }[]
    readonly operators: readonly PersonRefDto[]
  }
  readonly summary: {
    readonly month_count: { readonly total: number, readonly planned: number, readonly in_progress: number, readonly completed: number, readonly cancelled: number }
    readonly applied_area: { readonly area_ha: number, readonly productive_area_ha: number, readonly percent: number | null }
    readonly within_window: { readonly percent: number | null, readonly within: number, readonly total: number }
    readonly next: {
      readonly application_id: string
      readonly date: string
      readonly start_time: string
      readonly end_time: string
      readonly field_label: string
      readonly product_name: string
    } | null
  }
}

export interface ApplicationDetailDto extends ApplicationSummaryDto {
  readonly registered_at: string
  readonly recorded_conditions: {
    readonly headline: string
    readonly delta_t_c: number
    readonly wind_kmh: number
    readonly rain_mm: number
    readonly rain_hours: number
    readonly duration_minutes: number
  } | null
  readonly usage: {
    readonly lot: string | null
    readonly dose_total_l: number
    readonly mix_total_l: number
    readonly tank_count: number
    readonly tank_capacity_l: number
  }
  readonly pre_harvest: { readonly days: number, readonly released_from: string | null } | null
  readonly equipment: PersonRefDto
  readonly confirmed_by: { readonly person: PersonRefDto, readonly at: string } | null
  readonly signed: boolean
  readonly cancellation: { readonly reason: string } | null
}

export interface CurrentWeatherDto {
  readonly farm_id: string
  readonly city: string
  readonly state: string
  readonly temperature_c: number
  readonly wind_kmh: number
  readonly observed_at: string
}
