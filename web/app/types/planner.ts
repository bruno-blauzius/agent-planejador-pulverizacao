/**
 * Spray planner models (ADR 0016, screens 16, 17 and 18). Every condition,
 * duration, alert and check is computed by the backend (spray-planner); the
 * front only formats and displays them. Dates are ISO `YYYY-MM-DD` and times
 * `HH:mm` in the farm's local time, as sent by the API.
 */
import type { FarmRef } from './farm'
import type { SprayCondition } from './spray'

export type ProductKind = 'fungicide' | 'insecticide' | 'herbicide' | 'foliar' | 'other'

/** Dose always carries its unit (glossary: Dosage). */
export interface Dose {
  readonly value: number
  readonly unit: string
}

/** What the user selected in the "Fazenda / Talhões / Produto" bar. */
export interface PlannerContext {
  readonly farmId: string | null
  readonly fieldIds: readonly string[]
  readonly productKind: ProductKind | null
}

/** Context accepted by the API: a farm and at least one field. */
export interface PlannerRequestContext {
  readonly farmId: string
  readonly fieldIds: readonly string[]
  readonly productKind: ProductKind | null
}

// ---- chat ------------------------------------------------------------------

export interface PlannerMessageInput {
  /** Null on the first message; afterwards the id returned by the previous reply. */
  readonly conversationId: string | null
  readonly text: string
  readonly context: PlannerRequestContext
}

export type ConsultedSourceKind = 'weather' | 'conditions' | 'product_rules' | 'operational_capacity'

/** "O agente consultou" item. */
export interface ConsultedSource {
  readonly kind: ConsultedSourceKind
  readonly title: string
  readonly detail: string
}

export interface PlanWindow {
  /** ISO date `YYYY-MM-DD`. */
  readonly date: string
  readonly startTime: string
  readonly endTime: string
  readonly condition: SprayCondition
}

export type PlanAlertKind = 'window_too_short' | 'rain_risk' | 'interval_not_met'

export interface PlanAlert {
  readonly kind: PlanAlertKind
  /** Ready-to-display sentence, suggestion included. */
  readonly message: string
  readonly requiredMinutes: number | null
  readonly availableMinutes: number | null
}

export interface PlanEntry {
  readonly fieldId: string
  /** "T3" */
  readonly fieldLabel: string
  /** Area to apply (may be smaller than the field's registered area). */
  readonly areaHa: number
  readonly window: PlanWindow
  readonly durationMinutes: number
  readonly alert: PlanAlert | null
}

/** The agent's proposal. It is not a registered application. */
export interface SprayPlan {
  readonly id: string
  readonly farmId: string
  readonly productKind: ProductKind
  /** "2 janelas aptas cobrem os 90 ha" */
  readonly headline: string
  readonly totalAreaHa: number
  readonly capacityHaPerHour: number
  readonly entries: readonly PlanEntry[]
}

export interface PlannerReplyMessage {
  readonly id: string
  /** Paragraphs separated by a blank line. */
  readonly text: string
  readonly createdAt: Date
}

export interface PlannerReply {
  readonly conversationId: string
  readonly message: PlannerReplyMessage
  readonly consulted: readonly ConsultedSource[]
  readonly plan: SprayPlan | null
  /** Suggested next questions. */
  readonly suggestions: readonly string[]
}

// ---- window map and conditions ---------------------------------------------

export interface HourCondition {
  readonly hour: number
  readonly condition: SprayCondition
}

export interface FieldDayConditions {
  readonly date: string
  readonly hours: readonly HourCondition[]
}

export interface FieldWindowRow {
  readonly fieldId: string
  readonly fieldLabel: string
  readonly days: readonly FieldDayConditions[]
}

/** Contiguous suitable window, already grouped by the backend. */
export interface SprayWindowSlot {
  readonly fieldId: string
  readonly fieldLabel: string
  readonly date: string
  readonly startTime: string
  readonly endTime: string
  readonly durationMinutes: number
  readonly condition: SprayCondition
}

export interface SprayWindowMap {
  readonly farmId: string
  readonly generatedAt: Date
  /** When the forecast was refreshed ("dados de hoje 05:40"). */
  readonly forecastUpdatedAt: Date
  readonly dayStartHour: number
  /** Last hour included in each day. */
  readonly dayEndHour: number
  readonly fields: readonly FieldWindowRow[]
  readonly windows: readonly SprayWindowSlot[]
}

export interface WindowMapQuery {
  readonly farmId: string
  readonly fieldIds?: readonly string[]
  readonly days?: number
}

export interface ValueRange {
  readonly min: number
  readonly max: number
}

export interface HourlyConditions {
  readonly hour: number
  readonly deltaTC: number
  readonly windKmh: number
  readonly gustKmh: number
  readonly rainMm: number
  readonly condition: SprayCondition
}

export interface WindowConditionSummary {
  readonly startTime: string
  readonly endTime: string
  readonly deltaTMinC: number
  readonly deltaTMaxC: number
  readonly windAvgKmh: number
  readonly gustMaxKmh: number
  /** Accumulated until `rainUntilHour`. */
  readonly rainMm: number
  readonly rainUntilHour: number
  readonly nextRainAt: Date | null
}

export interface DayConditions {
  readonly farmId: string
  readonly date: string
  /** Ideal ranges from the spray-planner configuration. */
  readonly idealRanges: { readonly deltaTC: ValueRange, readonly windKmh: ValueRange }
  readonly hours: readonly HourlyConditions[]
  readonly windowSummary: WindowConditionSummary | null
}

export interface DayConditionsQuery {
  readonly farmId: string
  readonly date: string
  readonly fieldId?: string
  readonly windowStart?: string
  readonly windowEnd?: string
}

// ---- human confirmation ----------------------------------------------------

export type ReviewCheckStatus = 'ok' | 'warning' | 'blocked'

export type ReviewCheckCode =
  | 'delta_t'
  | 'wind'
  | 'rain_after'
  | 'interval_since_last'
  | 'pre_harvest_interval'
  | 'operator_equipment'
  | 'window_fits'

export interface ReviewCheck {
  readonly code: ReviewCheckCode
  readonly status: ReviewCheckStatus
  readonly title: string
  readonly detail: string
}

export interface PersonRef {
  readonly id: string
  readonly name: string
}

/** Snapshot shown in the confirmation modal; its id must come back on registration. */
export interface ApplicationReview {
  readonly id: string
  readonly planId: string
  readonly fieldId: string
  readonly fieldLabel: string
  readonly variety: string | null
  readonly areaHa: number
  readonly window: PlanWindow
  readonly product: {
    readonly kind: ProductKind
    readonly name: string
    readonly dose: Dose
    readonly mixRateLPerHa: number
  }
  readonly mixVolumeL: number
  readonly tankCount: number
  readonly tankCapacityL: number
  readonly checks: readonly ReviewCheck[]
  readonly operator: PersonRef
  readonly equipment: PersonRef
  /** False when any check is `blocked`. */
  readonly canConfirm: boolean
  readonly expiresAt: Date
}

/** Explicit human confirmation: `confirmed` is literally `true` or the call is not made. */
export interface ApplicationRegistrationInput {
  readonly planId: string
  readonly fieldId: string
  readonly reviewId: string
  readonly confirmation: {
    readonly confirmed: true
    readonly acknowledgedWarnings: readonly ReviewCheckCode[]
  }
}

export interface RegisteredApplication {
  readonly id: string
  readonly status: 'planned'
  readonly fieldId: string
  readonly fieldLabel: string
  readonly areaHa: number
  readonly window: PlanWindow
  readonly registeredAt: Date
  readonly registeredBy: PersonRef
}

// ---- operator "Hoje" ---------------------------------------------------------

export interface TodayStatus {
  readonly condition: SprayCondition
  /** "Pode pulverizar" */
  readonly headline: string
  readonly untilTime: string | null
  readonly remainingMinutes: number | null
  readonly windowStart: string | null
  readonly windowEnd: string | null
}

export type TodayMetricKind = 'delta_t' | 'wind' | 'rain'

export interface TodayMetric {
  readonly kind: TodayMetricKind
  /** "Delta T", "Vento", "Chuva 4h" */
  readonly label: string
  readonly value: number
  readonly unit: string
  readonly condition: SprayCondition
}

export interface TodayHour {
  readonly hour: number
  readonly condition: SprayCondition
  readonly isCurrent: boolean
}

export type TodayTaskStatus = 'planned' | 'in_progress'

export interface TodayTask {
  readonly id: string
  readonly fieldId: string
  /** "T3" */
  readonly fieldLabel: string
  /** "Talhão 3" */
  readonly fieldName: string
  /** "Fungicida" */
  readonly operation: string
  readonly areaHa: number
  readonly dose: Dose
  readonly tankCount: number
  readonly tankCapacityL: number
  readonly status: TodayTaskStatus
}

export interface TodayBrief {
  readonly farm: FarmRef
  readonly asOf: Date
  readonly status: TodayStatus
  readonly metrics: readonly TodayMetric[]
  readonly hours: readonly TodayHour[]
  readonly task: TodayTask | null
}
