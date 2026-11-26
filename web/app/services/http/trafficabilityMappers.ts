/** DTO <-> model of the Trafficability endpoints (ADR 0022): snake_case to camelCase, instants to `Date`; calendar days stay `YYYY-MM-DD`. */
import type { GeoPoint } from '~/types/geo'
import type {
  DailyRainEntry,
  FieldTrafficability,
  MethodEligibility,
  MethodRelease,
  ModelSoilMoisture,
  RainAccumulation,
  RainGauge,
  RainGaugeInput,
  RainGaugePatch,
  RainReading,
  RainReadingInput,
  SoilInspection,
  SoilInspectionInput,
  SoilInspectionView,
  SoilProfile,
  SoilProfileInput,
  SoilStatus,
  TrafficabilityDay,
} from '~/types/trafficability'
import type {
  DailyRainEntryDto,
  FieldTrafficabilityDto,
  MethodEligibilityDto,
  MethodReleaseDto,
  ModelSoilMoistureDto,
  RainAccumulationDto,
  RainGaugeDto,
  RainGaugeInputDto,
  RainGaugePatchDto,
  RainReadingDto,
  RainReadingInputDto,
  SoilInspectionDto,
  SoilInspectionInputDto,
  SoilInspectionViewDto,
  SoilProfileDto,
  SoilProfileInputDto,
  SoilStatusDto,
  TrafficabilityDayDto,
} from '~/types/trafficabilityDto'

function toRainAccumulation(dto: RainAccumulationDto): RainAccumulation {
  return {
    last24hMm: dto.last_24h_mm,
    last48hMm: dto.last_48h_mm,
    last72hMm: dto.last_72h_mm,
    last7dMm: dto.last_7d_mm,
    source: dto.source,
    sourceName: dto.source_name,
    observedAt: new Date(dto.observed_at),
  }
}

function toModelSoilMoisture(dto: ModelSoilMoistureDto): ModelSoilMoisture {
  return {
    volumetricM3m3: dto.volumetric_m3m3,
    fieldCapacityM3m3: dto.field_capacity_m3m3,
    fieldCapacityPct: dto.field_capacity_pct,
  }
}

function toSoilInspection(dto: SoilInspectionViewDto): SoilInspectionView {
  return {
    state: dto.state,
    inspectedAt: new Date(dto.inspected_at),
    validUntil: new Date(dto.valid_until),
    inspectedBy: { id: dto.inspected_by.id, name: dto.inspected_by.name },
  }
}

function toSoilStatus(dto: SoilStatusDto): SoilStatus {
  return {
    state: dto.state,
    basis: dto.basis,
    texture: dto.texture,
    thresholdsSource: dto.thresholds_source,
    rain: dto.rain ? toRainAccumulation(dto.rain) : null,
    modelMoisture: dto.model_moisture ? toModelSoilMoisture(dto.model_moisture) : null,
    inspection: dto.inspection ? toSoilInspection(dto.inspection) : null,
  }
}

function toDailyRain(dto: DailyRainEntryDto): DailyRainEntry {
  return { date: dto.date, rainMm: dto.rain_mm, kind: dto.kind, source: dto.source }
}

function toMethodEligibility(dto: MethodEligibilityDto): MethodEligibility {
  return {
    method: dto.method,
    verdict: dto.verdict,
    reasons: [...dto.reasons],
    areaAboveLimitPct: dto.area_above_limit_pct,
  }
}

function toTrafficabilityDay(dto: TrafficabilityDayDto): TrafficabilityDay {
  return { date: dto.date, soilState: dto.soil_state, methods: dto.methods.map(toMethodEligibility) }
}

function toMethodRelease(dto: MethodReleaseDto): MethodRelease {
  return { method: dto.method, requiredState: dto.required_state, date: dto.date }
}

export function toFieldTrafficability(dto: FieldTrafficabilityDto): FieldTrafficability {
  return {
    fieldId: dto.field_id,
    farmId: dto.farm_id,
    asOf: new Date(dto.as_of),
    soil: toSoilStatus(dto.soil),
    rainDaily: dto.rain_daily.map(toDailyRain),
    days: dto.days.map(toTrafficabilityDay),
    releases: dto.releases.map(toMethodRelease),
    dataIssues: [...dto.data_issues],
  }
}

// ---- field conditions (phase 2) ------------------------------------------------------

const pointOrNull = (point: GeoPoint | null): GeoPoint | null => (point ? { lat: point.lat, lng: point.lng } : null)

export function toRainGauge(dto: RainGaugeDto): RainGauge {
  return {
    id: dto.id,
    farmId: dto.farm_id,
    name: dto.name,
    location: pointOrNull(dto.location),
    createdAt: new Date(dto.created_at),
    updatedAt: new Date(dto.updated_at),
    version: dto.version,
  }
}

export function toRainGaugeInputDto(input: RainGaugeInput): RainGaugeInputDto {
  return input.location === undefined ? { name: input.name } : { name: input.name, location: pointOrNull(input.location) }
}

/** Only the keys present in the patch are sent: a missing key keeps the value on the API. */
export function toRainGaugePatchDto(patch: RainGaugePatch): RainGaugePatchDto {
  return {
    ...(patch.name === undefined ? {} : { name: patch.name }),
    ...(patch.location === undefined ? {} : { location: pointOrNull(patch.location) }),
  }
}

export function toRainReading(dto: RainReadingDto): RainReading {
  return {
    id: dto.id,
    rainGaugeId: dto.rain_gauge_id,
    rainMm: dto.rain_mm,
    observedAt: new Date(dto.observed_at),
    recordedBy: { id: dto.recorded_by.id, name: dto.recorded_by.name },
    recordedAt: new Date(dto.recorded_at),
  }
}

export function toRainReadingInputDto(input: RainReadingInput): RainReadingInputDto {
  return { rain_mm: input.rainMm, observed_at: input.observedAt.toISOString() }
}

export function toSoilInspectionRecord(dto: SoilInspectionDto): SoilInspection {
  return {
    id: dto.id,
    fieldId: dto.field_id,
    farmId: dto.farm_id,
    state: dto.state,
    inspectedAt: new Date(dto.inspected_at),
    validUntil: new Date(dto.valid_until),
    inspectedBy: { id: dto.inspected_by.id, name: dto.inspected_by.name },
    note: dto.note,
  }
}

export function toSoilInspectionInputDto(input: SoilInspectionInput): SoilInspectionInputDto {
  return {
    state: input.state,
    ...(input.inspectedAt ? { inspected_at: input.inspectedAt.toISOString() } : {}),
    ...(input.note === undefined ? {} : { note: input.note }),
  }
}

export function toSoilProfile(dto: SoilProfileDto): SoilProfile {
  return {
    fieldId: dto.field_id,
    farmId: dto.farm_id,
    soilTexture: dto.soil_texture,
    rainGauge: dto.rain_gauge ? { id: dto.rain_gauge.id, name: dto.rain_gauge.name } : null,
  }
}

export function toSoilProfileInputDto(input: SoilProfileInput): SoilProfileInputDto {
  return { soil_texture: input.soilTexture, rain_gauge_id: input.rainGaugeId }
}
