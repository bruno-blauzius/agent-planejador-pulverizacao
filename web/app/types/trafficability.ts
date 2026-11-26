/**
 * Chuva, estado do solo e trafegabilidade do talhão (ADR 0022; `openapi.yaml`,
 * tag `Trafficability`). Front models in camelCase; the HTTP mapper converts
 * the snake_case DTOs. Every state, verdict and release here is computed by the
 * API (`SoilStateCalculator`, `PlanTrafficability`): the front only presents
 * them and never derives a soil state from rain.
 *
 * Calendar days (`date`) are `YYYY-MM-DD` strings in `America/Sao_Paulo`, kept
 * as text on purpose: turning them into `Date` would shift the day in other
 * time zones. Instants (`asOf`, `observedAt`…) are `Date`.
 */
import type { GeoPoint } from './geo'
import type { PersonRef } from './planner'
import type { SprayMethod } from './terrain'

/** `sandy` = arenoso, `medium` = médio, `clay` = argiloso. */
export type SoilTexture = 'sandy' | 'medium' | 'clay'

/** From best to worst for traffic; `unknown` without a rain source or without texture. */
export const SOIL_STATES = ['dry', 'ideal', 'moist', 'waterlogged', 'unknown'] as const
export type SoilState = (typeof SOIL_STATES)[number]

/** What decided today's state: a valid inspection, measured rain, the weather model, or nothing. */
export type SoilBasis = 'inspection' | 'measured_rain' | 'model' | 'none'

/** `gauge` = pluviômetro lançado na tela, `station` = estação da fazenda, `model` = célula do modelo (não é o talhão). */
export type RainSource = 'gauge' | 'station' | 'model'

export const ELIGIBILITY_VERDICTS = ['apt', 'attention', 'unfit'] as const
export type EligibilityVerdict = (typeof ELIGIBILITY_VERDICTS)[number]

export const ELIGIBILITY_REASONS = [
  'slope_restricted',
  'slope_unsuitable',
  'terrain_unknown',
  'soil_moist',
  'soil_waterlogged',
  'soil_unknown',
  'slope_restricted_on_wet_soil',
] as const
export type EligibilityReason = (typeof ELIGIBILITY_REASONS)[number]

export const TRAFFICABILITY_DATA_ISSUES = ['no_rain_source', 'no_soil_texture', 'weather_unavailable', 'terrain_not_calculated'] as const
export type TrafficabilityDataIssue = (typeof TRAFFICABILITY_DATA_ISSUES)[number]

/** `heuristic`: initial limits not yet calibrated; `calibrated`: reviewed by the agronomist. */
export type ThresholdsSource = 'heuristic' | 'calibrated'

export type DailyRainKind = 'measured' | 'forecast'

/** `YYYY-MM-DD`, calendar day in `America/Sao_Paulo`. */
export type CalendarDay = string

/** Projection days after today the API uses when `days` is omitted (contract default). */
export const DEFAULT_TRAFFICABILITY_DAYS = 5

export interface RainAccumulation {
  readonly last24hMm: number
  readonly last48hMm: number
  readonly last72hMm: number
  readonly last7dMm: number
  readonly source: RainSource
  /** Name of the gauge or station; null for the model. */
  readonly sourceName: string | null
  readonly observedAt: Date
}

/** Weather model moisture in the 0–27 cm layer. Trend only, never the field itself. */
export interface ModelSoilMoisture {
  readonly volumetricM3m3: number
  readonly fieldCapacityM3m3: number
  /** Moisture as a percentage of field capacity (may exceed 100). */
  readonly fieldCapacityPct: number
}

/** A still-valid inspection made by a person; it overrides the calculation. */
export interface SoilInspectionView {
  readonly state: SoilState
  readonly inspectedAt: Date
  readonly validUntil: Date
  readonly inspectedBy: PersonRef
}

export interface SoilStatus {
  readonly state: SoilState
  readonly basis: SoilBasis
  readonly texture: SoilTexture | null
  readonly thresholdsSource: ThresholdsSource
  readonly rain: RainAccumulation | null
  readonly modelMoisture: ModelSoilMoisture | null
  /** Null when there is none or it already expired. */
  readonly inspection: SoilInspectionView | null
}

export interface DailyRainEntry {
  readonly date: CalendarDay
  readonly rainMm: number
  readonly kind: DailyRainKind
  readonly source: RainSource
}

export interface MethodEligibility {
  readonly method: SprayMethod
  readonly verdict: EligibilityVerdict
  /** Codes, translated by the UI; empty when apt. */
  readonly reasons: readonly EligibilityReason[]
  /** Share of the area above the method slope limit, 0–100, when known. */
  readonly areaAboveLimitPct: number | null
}

export interface TrafficabilityDay {
  readonly date: CalendarDay
  readonly soilState: SoilState
  /** One item per method, in `SprayMethod` order. */
  readonly methods: readonly MethodEligibility[]
}

/** First day (today inclusive) the soil meets a ground method; null when not within the horizon. */
export interface MethodRelease {
  readonly method: SprayMethod
  readonly requiredState: SoilState
  readonly date: CalendarDay | null
}

export interface FieldTrafficability {
  readonly fieldId: string
  readonly farmId: string
  readonly asOf: Date
  readonly soil: SoilStatus
  /** Last 7 days (measured) and the next `days` days (forecast), by date. */
  readonly rainDaily: readonly DailyRainEntry[]
  /** Today and the next `days` days. */
  readonly days: readonly TrafficabilityDay[]
  /** Ground methods only; aerial methods do not depend on the soil. */
  readonly releases: readonly MethodRelease[]
  readonly dataIssues: readonly TrafficabilityDataIssue[]
}

// ---- field conditions recorded by people (ADR 0022, phase 2) --------------------------

/** States a person can record in an inspection (`unknown` is refused with 422). */
export const INSPECTABLE_SOIL_STATES = ['dry', 'ideal', 'moist', 'waterlogged'] as const
export type InspectableSoilState = (typeof INSPECTABLE_SOIL_STATES)[number]

export const SOIL_TEXTURES = ['sandy', 'medium', 'clay'] as const

/** Rain gauge of a farm; its readings serve every field linked to it by the soil profile. */
export interface RainGauge {
  readonly id: string
  readonly farmId: string
  readonly name: string
  readonly location: GeoPoint | null
  readonly createdAt: Date
  readonly updatedAt: Date
  readonly version: number
}

/** Body of `POST /farms/{farmId}/rain-gauges` (`registry.write`). Name 1–100, unique in the farm. */
export interface RainGaugeInput {
  readonly name: string
  readonly location?: GeoPoint | null
}

/** Body of `PATCH .../rain-gauges/{gaugeId}`: a missing key keeps the value; `location: null` removes it. */
export interface RainGaugePatch {
  readonly name?: string
  readonly location?: GeoPoint | null
}

/** Rain accumulated since the previous reading of the gauge. */
export interface RainReading {
  readonly id: string
  readonly rainGaugeId: string
  readonly rainMm: number
  readonly observedAt: Date
  readonly recordedBy: PersonRef
  readonly recordedAt: Date
}

/** Body of `POST .../readings` (`field_conditions.record`): 0–500 mm, not in the future. */
export interface RainReadingInput {
  readonly rainMm: number
  readonly observedAt: Date
}

/** Interval `(from, to]` of `GET .../readings`; omitted, the last 7 days up to now (31 days at most). */
export interface RainReadingRange {
  readonly from?: Date
  readonly to?: Date
}

export interface SoilInspection {
  readonly id: string
  readonly fieldId: string
  readonly farmId: string
  readonly state: SoilState
  readonly inspectedAt: Date
  /** Until when it replaces today's calculation (API configuration, 12 h by default). */
  readonly validUntil: Date
  readonly inspectedBy: PersonRef
  readonly note: string | null
}

/** Body of `POST .../soil-inspections` (`field_conditions.record`). `inspectedAt` omitted = now. */
export interface SoilInspectionInput {
  readonly state: InspectableSoilState
  readonly inspectedAt?: Date
  /** Up to 500 characters. */
  readonly note?: string | null
}

export interface SoilProfile {
  readonly fieldId: string
  readonly farmId: string
  readonly soilTexture: SoilTexture | null
  /** Gauge of the same farm serving the field; null = the field uses the weather model. */
  readonly rainGauge: { readonly id: string, readonly name: string } | null
}

/** Body of `PUT .../soil-profile` (`registry.write`): both keys always sent; null clears. */
export interface SoilProfileInput {
  readonly soilTexture: SoilTexture | null
  readonly rainGaugeId: string | null
}
