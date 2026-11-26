/** DTO <-> model of the Terrain endpoints (ADR 0021): snake_case to camelCase, ISO dates to `Date`. */
import type {
  FarmTerrain,
  FieldTerrain,
  FieldTerrainAdjustmentInput,
  FieldTerrainSummary,
  MethodFit,
  TerrainAdjustment,
  TerrainCalculation,
  TerrainCalculationScope,
  TerrainMeasurement,
} from '~/types/terrain'
import type {
  FarmTerrainDto,
  FieldTerrainAdjustmentInputDto,
  FieldTerrainDto,
  FieldTerrainSummaryDto,
  MethodFitDto,
  TerrainAdjustmentDto,
  TerrainCalculationDto,
  TerrainCalculationRequestDto,
  TerrainMeasurementDto,
} from '~/types/terrainDto'

const dateOrNull = (value: string | null) => (value ? new Date(value) : null)

export function toTerrainMeasurement(dto: TerrainMeasurementDto): TerrainMeasurement {
  return {
    slopeMeanPct: dto.slope_mean_pct,
    slopeP90Pct: dto.slope_p90_pct,
    slopeMaxPct: dto.slope_max_pct,
    slopeClass: dto.slope_class,
    position: dto.position,
    aspect: dto.aspect,
  }
}

const measurementOrNull = (dto: TerrainMeasurementDto | null) => (dto ? toTerrainMeasurement(dto) : null)

function toTerrainCalculation(dto: TerrainCalculationDto): TerrainCalculation {
  return {
    measurement: toTerrainMeasurement(dto.measurement),
    elevationMinM: dto.elevation_min_m,
    elevationMaxM: dto.elevation_max_m,
    classAreas: dto.class_areas.map(item => ({ slopeClass: item.slope_class, areaHa: item.area_ha, sharePct: item.share_pct })),
    profile: dto.profile.map(point => ({ distanceM: point.distance_m, elevationM: point.elevation_m })),
    elevationModel: {
      source: dto.elevation_model.source,
      resolutionM: dto.elevation_model.resolution_m,
      version: dto.elevation_model.version,
    },
    calculatedAt: new Date(dto.calculated_at),
  }
}

function toTerrainAdjustment(dto: TerrainAdjustmentDto): TerrainAdjustment {
  return {
    origin: dto.origin,
    measurement: toTerrainMeasurement(dto.measurement),
    justification: dto.justification,
    adjustedBy: { id: dto.adjusted_by.id, name: dto.adjusted_by.name },
    adjustedAt: new Date(dto.adjusted_at),
  }
}

function toMethodFit(dto: MethodFitDto): MethodFit {
  return {
    method: dto.method,
    slopeLimitPct: dto.slope_limit_pct,
    limitSource: dto.limit_source,
    status: dto.status,
    areaAboveLimitPct: dto.area_above_limit_pct,
  }
}

function toFieldTerrainSummary(dto: FieldTerrainSummaryDto): FieldTerrainSummary {
  return {
    fieldId: dto.field_id,
    fieldName: dto.field_name,
    block: dto.block,
    areaHa: dto.area_ha,
    status: dto.status,
    failureReason: dto.failure_reason,
    source: dto.source,
    measurement: measurementOrNull(dto.measurement),
    elevationMinM: dto.elevation_min_m,
    elevationMaxM: dto.elevation_max_m,
    hasRestrictedMethod: dto.has_restricted_method,
  }
}

export function toFarmTerrain(dto: FarmTerrainDto): FarmTerrain {
  return {
    farmId: dto.farm_id,
    summary: {
      fieldCount: dto.summary.field_count,
      calculatedFieldCount: dto.summary.calculated_field_count,
      slopeMeanPct: dto.summary.slope_mean_pct,
      restrictedFieldCount: dto.summary.restricted_field_count,
      elevationMinM: dto.summary.elevation_min_m,
      elevationMaxM: dto.summary.elevation_max_m,
      calculatedAt: dateOrNull(dto.summary.calculated_at),
    },
    progress: dto.progress ? { total: dto.progress.total, done: dto.progress.done } : null,
    items: dto.items.map(toFieldTerrainSummary),
  }
}

export function toFieldTerrain(dto: FieldTerrainDto): FieldTerrain {
  return {
    fieldId: dto.field_id,
    farmId: dto.farm_id,
    status: dto.status,
    failureReason: dto.failure_reason,
    source: dto.source,
    effective: measurementOrNull(dto.effective),
    calculated: dto.calculated ? toTerrainCalculation(dto.calculated) : null,
    adjustment: dto.adjustment ? toTerrainAdjustment(dto.adjustment) : null,
    adjustmentOutdated: dto.adjustment_outdated,
    methodFits: dto.method_fits.map(toMethodFit),
  }
}

export function toTerrainCalculationRequestDto(scope: TerrainCalculationScope): TerrainCalculationRequestDto {
  return { scope }
}

export function toFieldTerrainAdjustmentInputDto(input: FieldTerrainAdjustmentInput): FieldTerrainAdjustmentInputDto {
  return {
    origin: input.origin,
    slope_mean_pct: input.slopeMeanPct,
    slope_max_pct: input.slopeMaxPct,
    position: input.position,
    justification: input.justification,
  }
}
