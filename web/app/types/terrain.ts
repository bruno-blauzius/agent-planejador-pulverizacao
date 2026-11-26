/**
 * Relevo do talhão (ADR 0021; `openapi.yaml`, tag `Terrain`). Front models in
 * camelCase; the HTTP mappers convert the snake_case DTOs. Every value here is
 * computed by the API (classes, method fit, outdated adjustment): the front
 * only presents them.
 */
import type { PersonRef } from './planner'

/**
 * `not_calculated`: never calculated. `pending`: waiting (requested or polygon changed).
 * `calculating`: in progress. `ready`: calculated. `failed`: see `failureReason`.
 */
export type TerrainStatus = 'not_calculated' | 'pending' | 'calculating' | 'ready' | 'failed'

/** Embrapa relief classes by mean slope, in contract order (limits are API configuration). */
export const SLOPE_CLASSES = ['flat', 'gently_undulating', 'undulating', 'strongly_undulating', 'mountainous'] as const
export type SlopeClass = (typeof SLOPE_CLASSES)[number]

/** hilltop = topo, slope = encosta, lowland = baixada. */
export type TerrainPosition = 'hilltop' | 'slope' | 'lowland'

/** Predominant exposure; `flat` when the slope defines no direction. */
export const TERRAIN_ASPECTS = [
  'north',
  'northeast',
  'east',
  'southeast',
  'south',
  'southwest',
  'west',
  'northwest',
  'flat',
] as const
export type TerrainAspect = (typeof TERRAIN_ASPECTS)[number]

/** Source of the effective value. */
export type TerrainSource = 'calculated' | 'manual'

export type TerrainAdjustmentOrigin = 'topographic_survey' | 'field_assessment'

/** knapsack = costal, tractor_boom = tratorizado de barra, airblast = turbopulverizador, drone = ARP. */
export type SprayMethod = 'knapsack' | 'tractor_boom' | 'self_propelled' | 'airblast' | 'aircraft' | 'drone'

/** Fit of a method with the relief (slope only; the planner checks the rest). */
export type MethodFitStatus = 'compatible' | 'restricted' | 'unsuitable'

/** `reference`: limit configured in the API. `equipment`: registered equipment (future). */
export type SlopeLimitSource = 'reference' | 'equipment'

/** `missing`: fields without relief, pending or failed. `all`: recalculates every field. */
export type TerrainCalculationScope = 'missing' | 'all'

export interface ElevationModel {
  /** E.g. "Copernicus DEM GLO-30". */
  readonly source: string
  readonly resolutionM: number
  readonly version: string
}

/** Relief measures; `slopeP90Pct` and `aspect` are null in a manual adjustment. */
export interface TerrainMeasurement {
  readonly slopeMeanPct: number
  readonly slopeP90Pct: number | null
  readonly slopeMaxPct: number
  readonly slopeClass: SlopeClass
  readonly position: TerrainPosition
  readonly aspect: TerrainAspect | null
}

export interface SlopeClassArea {
  readonly slopeClass: SlopeClass
  readonly areaHa: number
  /** Share of the field area, 0–100. */
  readonly sharePct: number
}

export interface ElevationProfilePoint {
  /** Distance from the highest point, along the steepest line. */
  readonly distanceM: number
  readonly elevationM: number
}

/** Result of the elevation model calculation. */
export interface TerrainCalculation {
  readonly measurement: TerrainMeasurement
  readonly elevationMinM: number
  readonly elevationMaxM: number
  /** One entry per class, in `SLOPE_CLASSES` order, including zero-area ones. */
  readonly classAreas: readonly SlopeClassArea[]
  readonly profile: readonly ElevationProfilePoint[]
  readonly elevationModel: ElevationModel
  readonly calculatedAt: Date
}

export interface TerrainAdjustment {
  readonly origin: TerrainAdjustmentOrigin
  readonly measurement: TerrainMeasurement
  readonly justification: string
  readonly adjustedBy: PersonRef
  readonly adjustedAt: Date
}

export interface MethodFit {
  readonly method: SprayMethod
  /** Null when the method is not limited by slope. */
  readonly slopeLimitPct: number | null
  readonly limitSource: SlopeLimitSource
  readonly status: MethodFitStatus
  /** Share of the area above the limit, 0–100; null without limit or with a manual adjustment. */
  readonly areaAboveLimitPct: number | null
}

/** One row of the "Relevo" tab (effective value: adjustment, else calculated). */
export interface FieldTerrainSummary {
  readonly fieldId: string
  readonly fieldName: string
  readonly block: string | null
  readonly areaHa: number
  readonly status: TerrainStatus
  readonly failureReason: string | null
  readonly source: TerrainSource | null
  /** Effective value; null without relief. */
  readonly measurement: TerrainMeasurement | null
  /** Calculated elevation ("Altitude" map layer); null without calculation, even with an adjustment. */
  readonly elevationMinM: number | null
  readonly elevationMaxM: number | null
  /** Some slope-limited method is `restricted` or `unsuitable` on this field. */
  readonly hasRestrictedMethod: boolean
}

export interface FarmTerrainSummary {
  readonly fieldCount: number
  readonly calculatedFieldCount: number
  /** Effective mean slope weighted by area (API). */
  readonly slopeMeanPct: number | null
  readonly restrictedFieldCount: number
  readonly elevationMinM: number | null
  readonly elevationMaxM: number | null
  /** Most recent calculation among the fields. */
  readonly calculatedAt: Date | null
}

export interface TerrainProgress {
  readonly total: number
  readonly done: number
}

export interface FarmTerrain {
  readonly farmId: string
  readonly summary: FarmTerrainSummary
  /** Present while a requested calculation is running; the screen polls until it is null. */
  readonly progress: TerrainProgress | null
  /** Ordered by block and name, like the field list. */
  readonly items: readonly FieldTerrainSummary[]
}

/** Detail of one field (screen 04c). */
export interface FieldTerrain {
  readonly fieldId: string
  readonly farmId: string
  readonly status: TerrainStatus
  readonly failureReason: string | null
  readonly source: TerrainSource | null
  /** What drives class and method fit (adjustment, else calculated). */
  readonly effective: TerrainMeasurement | null
  readonly calculated: TerrainCalculation | null
  readonly adjustment: TerrainAdjustment | null
  /** The latest calculation diverges from the adjustment beyond the configured limit. */
  readonly adjustmentOutdated: boolean
  /** Empty without an effective relief. */
  readonly methodFits: readonly MethodFit[]
}

/** Body of `PUT .../terrain/adjustment`. The class is recalculated by the API. */
export interface FieldTerrainAdjustmentInput {
  readonly origin: TerrainAdjustmentOrigin
  /** 0–100. */
  readonly slopeMeanPct: number
  /** 0–100 and not below `slopeMeanPct` (422 `slope_max_pct`). */
  readonly slopeMaxPct: number
  readonly position: TerrainPosition
  /** 1–1000 characters. */
  readonly justification: string
}
