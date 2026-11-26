/** Wire format (snake_case) of the Terrain endpoints, ADR 0021. Only `services/http/` uses it. */
import type { PersonRefDto } from './applicationDto'
import type {
  MethodFitStatus,
  SlopeClass,
  SlopeLimitSource,
  SprayMethod,
  TerrainAdjustmentOrigin,
  TerrainAspect,
  TerrainCalculationScope,
  TerrainPosition,
  TerrainSource,
  TerrainStatus,
} from './terrain'

export interface TerrainMeasurementDto {
  readonly slope_mean_pct: number
  readonly slope_p90_pct: number | null
  readonly slope_max_pct: number
  readonly slope_class: SlopeClass
  readonly position: TerrainPosition
  readonly aspect: TerrainAspect | null
}

export interface TerrainCalculationDto {
  readonly measurement: TerrainMeasurementDto
  readonly elevation_min_m: number
  readonly elevation_max_m: number
  readonly class_areas: readonly { readonly slope_class: SlopeClass, readonly area_ha: number, readonly share_pct: number }[]
  readonly profile: readonly { readonly distance_m: number, readonly elevation_m: number }[]
  readonly elevation_model: { readonly source: string, readonly resolution_m: number, readonly version: string }
  readonly calculated_at: string
}

export interface TerrainAdjustmentDto {
  readonly origin: TerrainAdjustmentOrigin
  readonly measurement: TerrainMeasurementDto
  readonly justification: string
  readonly adjusted_by: PersonRefDto
  readonly adjusted_at: string
}

export interface MethodFitDto {
  readonly method: SprayMethod
  readonly slope_limit_pct: number | null
  readonly limit_source: SlopeLimitSource
  readonly status: MethodFitStatus
  readonly area_above_limit_pct: number | null
}

export interface FieldTerrainSummaryDto {
  readonly field_id: string
  readonly field_name: string
  readonly block: string | null
  readonly area_ha: number
  readonly status: TerrainStatus
  readonly failure_reason: string | null
  readonly source: TerrainSource | null
  readonly measurement: TerrainMeasurementDto | null
  readonly elevation_min_m: number | null
  readonly elevation_max_m: number | null
  readonly has_restricted_method: boolean
}

export interface FarmTerrainDto {
  readonly farm_id: string
  readonly summary: {
    readonly field_count: number
    readonly calculated_field_count: number
    readonly slope_mean_pct: number | null
    readonly restricted_field_count: number
    readonly elevation_min_m: number | null
    readonly elevation_max_m: number | null
    readonly calculated_at: string | null
  }
  readonly progress: { readonly total: number, readonly done: number } | null
  readonly items: readonly FieldTerrainSummaryDto[]
}

export interface FieldTerrainDto {
  readonly field_id: string
  readonly farm_id: string
  readonly status: TerrainStatus
  readonly failure_reason: string | null
  readonly source: TerrainSource | null
  readonly effective: TerrainMeasurementDto | null
  readonly calculated: TerrainCalculationDto | null
  readonly adjustment: TerrainAdjustmentDto | null
  readonly adjustment_outdated: boolean
  readonly method_fits: readonly MethodFitDto[]
}

export interface TerrainCalculationRequestDto {
  readonly scope: TerrainCalculationScope
}

export interface FieldTerrainAdjustmentInputDto {
  readonly origin: TerrainAdjustmentOrigin
  readonly slope_mean_pct: number
  readonly slope_max_pct: number
  readonly position: TerrainPosition
  readonly justification: string
}
