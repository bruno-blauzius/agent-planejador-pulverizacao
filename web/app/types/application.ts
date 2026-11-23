/**
 * Applications: history and planned ones (screens 19 and 20, ADR 0016). Totals,
 * conditions, status and the carência date are computed by the API; the front
 * only formats them. Dates are ISO `YYYY-MM-DD`, times `HH:mm` in the farm's zone.
 */
import type { PageRequest, Page } from './pagination'
import type { Dose, PersonRef, ProductKind } from './planner'

export type ApplicationStatus = 'planned' | 'in_progress' | 'completed' | 'cancelled'

export interface ApplicationWindow {
  readonly date: string
  readonly startTime: string
  readonly endTime: string
}

export interface ApplicationFieldRef {
  readonly id: string
  /** "T5" */
  readonly label: string
  /** "Talhão 5" */
  readonly name: string
  readonly variety: string | null
  /** "Área Centro" */
  readonly block: string | null
}

export interface ApplicationProduct {
  readonly kind: ProductKind
  /** "Inseticida" */
  readonly name: string
  readonly dose: Dose
  readonly mixRateLPerHa: number
}

/** `recorded`: measured at the application; `forecast`: forecast for the window; `live`: no numbers yet. */
export type ApplicationConditionsSource = 'recorded' | 'forecast' | 'live'

export interface ApplicationConditionsSummary {
  readonly source: ApplicationConditionsSource
  readonly deltaTC: number | null
  readonly windKmh: number | null
  /** Filled on a cancelled application when rain was the reason. */
  readonly rainMm: number | null
}

export interface ApplicationSummary {
  readonly id: string
  readonly status: ApplicationStatus
  readonly farmId: string
  readonly window: ApplicationWindow
  readonly field: ApplicationFieldRef
  readonly product: ApplicationProduct
  readonly areaHa: number
  readonly conditions: ApplicationConditionsSummary
  readonly operator: PersonRef
}

export interface ApplicationFacets {
  readonly fields: readonly { readonly id: string, readonly label: string, readonly name: string }[]
  readonly operators: readonly PersonRef[]
}

/**
 * Indicators of the top of the screen, for the current month in the user's scope.
 * Counts, area and percentages are computed by the API and do not follow the table filters.
 */
export interface ApplicationMonthSummary {
  readonly monthCount: {
    readonly total: number
    readonly planned: number
    readonly inProgress: number
    readonly completed: number
    readonly cancelled: number
  }
  readonly appliedArea: { readonly areaHa: number, readonly productiveAreaHa: number, readonly percent: number | null }
  /** Applications whose Delta T and wind were within the ideal range. */
  readonly withinWindow: { readonly percent: number | null, readonly within: number, readonly total: number }
  /** Next planned application; null without planned ones. */
  readonly next: {
    readonly applicationId: string
    readonly date: string
    readonly startTime: string
    readonly endTime: string
    readonly fieldLabel: string
    readonly productName: string
  } | null
}

/** A page of the list plus the options of the filter selects and the month indicators. */
export interface ApplicationPage extends Page<ApplicationSummary> {
  readonly facets: ApplicationFacets
  readonly summary: ApplicationMonthSummary
}

export interface RecordedConditions {
  /** "Dentro da janela" */
  readonly headline: string
  readonly deltaTC: number
  readonly windKmh: number
  readonly rainMm: number
  /** Horizon of the rain figure ("Chuva 8h"). */
  readonly rainHours: number
  readonly durationMinutes: number
}

export interface ApplicationUsage {
  readonly lot: string | null
  readonly doseTotalL: number
  readonly mixTotalL: number
  readonly tankCount: number
  readonly tankCapacityL: number
}

export interface ApplicationDetail extends ApplicationSummary {
  readonly registeredAt: Date
  readonly recordedConditions: RecordedConditions | null
  readonly usage: ApplicationUsage
  readonly preHarvest: { readonly days: number, readonly releasedFrom: string | null } | null
  readonly equipment: PersonRef
  readonly confirmedBy: { readonly person: PersonRef, readonly at: Date } | null
  /** Digitally signed record, valid for the field notebook. */
  readonly signed: boolean
  readonly cancellation: { readonly reason: string } | null
}

export interface ApplicationFilters extends PageRequest {
  /** Window from today minus N days (planned future ones included). */
  readonly periodDays?: number
  readonly farmId?: string
  readonly fieldId?: string
  readonly productKind?: ProductKind
  readonly operatorId?: string
  readonly status?: ApplicationStatus
}

/** Current weather chip ("Guaxupé · 24 °C · vento 7 km/h"). */
export interface CurrentWeather {
  readonly farmId: string
  readonly city: string
  readonly state: string
  readonly temperatureC: number
  readonly windKmh: number
  readonly observedAt: Date
}
