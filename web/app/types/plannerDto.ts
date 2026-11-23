/** Wire format (snake_case) of the planner endpoints, ADR 0016. Only `services/http/` uses it. */
import type { FarmRef } from './farm'
import type {
  ConsultedSourceKind,
  PlanAlertKind,
  ProductKind,
  ReviewCheckCode,
  ReviewCheckStatus,
  TodayMetricKind,
  TodayTaskStatus,
} from './planner'
import type { SprayCondition } from './spray'

export interface DoseDto {
  readonly value: number
  readonly unit: string
}

export interface PlanWindowDto {
  readonly date: string
  readonly start_time: string
  readonly end_time: string
  readonly condition: SprayCondition
}

export interface PlanEntryDto {
  readonly field_id: string
  readonly field_label: string
  readonly area_ha: number
  readonly window: PlanWindowDto
  readonly duration_minutes: number
  readonly alert: {
    readonly kind: PlanAlertKind
    readonly message: string
    readonly required_minutes: number | null
    readonly available_minutes: number | null
  } | null
}

export interface SprayPlanDto {
  readonly id: string
  readonly farm_id: string
  readonly product_kind: ProductKind
  readonly headline: string
  readonly total_area_ha: number
  readonly capacity_ha_per_hour: number
  readonly entries: readonly PlanEntryDto[]
}

export interface PlannerMessageRequestDto {
  readonly conversation_id: string | null
  readonly text: string
  readonly context: {
    readonly farm_id: string
    readonly field_ids: readonly string[]
    readonly product_kind: ProductKind | null
  }
}

export interface PlannerReplyDto {
  readonly conversation_id: string
  readonly message: { readonly id: string, readonly text: string, readonly created_at: string }
  readonly consulted: readonly { readonly kind: ConsultedSourceKind, readonly title: string, readonly detail: string }[]
  readonly plan: SprayPlanDto | null
  readonly suggestions: readonly string[]
}

export interface SprayWindowMapDto {
  readonly farm_id: string
  readonly generated_at: string
  readonly forecast_updated_at: string
  readonly day_start_hour: number
  readonly day_end_hour: number
  readonly fields: readonly {
    readonly field_id: string
    readonly field_label: string
    readonly days: readonly {
      readonly date: string
      readonly hours: readonly { readonly hour: number, readonly condition: SprayCondition }[]
    }[]
  }[]
  readonly windows: readonly {
    readonly field_id: string
    readonly field_label: string
    readonly date: string
    readonly start_time: string
    readonly end_time: string
    readonly duration_minutes: number
    readonly condition: SprayCondition
  }[]
}

export interface DayConditionsDto {
  readonly farm_id: string
  readonly date: string
  readonly ideal_ranges: {
    readonly delta_t_c: { readonly min: number, readonly max: number }
    readonly wind_kmh: { readonly min: number, readonly max: number }
  }
  readonly hours: readonly {
    readonly hour: number
    readonly delta_t_c: number
    readonly wind_kmh: number
    readonly gust_kmh: number
    readonly rain_mm: number
    readonly condition: SprayCondition
  }[]
  readonly window_summary: {
    readonly start_time: string
    readonly end_time: string
    readonly delta_t_min_c: number
    readonly delta_t_max_c: number
    readonly wind_avg_kmh: number
    readonly gust_max_kmh: number
    readonly rain_mm: number
    readonly rain_until_hour: number
    readonly next_rain_at: string | null
  } | null
}

export interface ApplicationReviewDto {
  readonly id: string
  readonly plan_id: string
  readonly field_id: string
  readonly field_label: string
  readonly variety: string | null
  readonly area_ha: number
  readonly window: PlanWindowDto
  readonly product: {
    readonly kind: ProductKind
    readonly name: string
    readonly dose: DoseDto
    readonly mix_rate_l_per_ha: number
  }
  readonly mix_volume_l: number
  readonly tank_count: number
  readonly tank_capacity_l: number
  readonly checks: readonly {
    readonly code: ReviewCheckCode
    readonly status: ReviewCheckStatus
    readonly title: string
    readonly detail: string
  }[]
  readonly operator: { readonly id: string, readonly name: string }
  readonly equipment: { readonly id: string, readonly name: string }
  readonly can_confirm: boolean
  readonly expires_at: string
}

export interface ApplicationRegistrationDto {
  readonly plan_id: string
  readonly field_id: string
  readonly review_id: string
  readonly confirmation: {
    readonly confirmed: true
    readonly acknowledged_warnings: readonly ReviewCheckCode[]
  }
}

export interface RegisteredApplicationDto {
  readonly id: string
  readonly status: 'planned'
  readonly field_id: string
  readonly field_label: string
  readonly area_ha: number
  readonly window: PlanWindowDto
  readonly registered_at: string
  readonly registered_by: { readonly id: string, readonly name: string }
}

export interface TodayTaskDto {
  readonly id: string
  readonly field_id: string
  readonly field_label: string
  readonly field_name: string
  readonly operation: string
  readonly area_ha: number
  readonly dose: DoseDto
  readonly tank_count: number
  readonly tank_capacity_l: number
  readonly status: TodayTaskStatus
}

export interface TodayBriefDto {
  readonly farm: FarmRef
  readonly as_of: string
  readonly status: {
    readonly condition: SprayCondition
    readonly headline: string
    readonly until_time: string | null
    readonly remaining_minutes: number | null
    readonly window_start: string | null
    readonly window_end: string | null
  }
  readonly metrics: readonly {
    readonly kind: TodayMetricKind
    readonly label: string
    readonly value: number
    readonly unit: string
    readonly condition: SprayCondition
  }[]
  readonly hours: readonly { readonly hour: number, readonly condition: SprayCondition, readonly is_current: boolean }[]
  readonly task: TodayTaskDto | null
}
