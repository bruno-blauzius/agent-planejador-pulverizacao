/** Wire format (snake_case) of the Trafficability endpoint, ADR 0022. Only `services/http/` uses it. */
import type { PersonRefDto } from './applicationDto'
import type { GeoPoint } from './geo'
import type { SprayMethod } from './terrain'
import type {
  DailyRainKind,
  EligibilityReason,
  EligibilityVerdict,
  InspectableSoilState,
  RainSource,
  SoilBasis,
  SoilState,
  SoilTexture,
  ThresholdsSource,
  TrafficabilityDataIssue,
} from './trafficability'

export interface RainAccumulationDto {
  readonly last_24h_mm: number
  readonly last_48h_mm: number
  readonly last_72h_mm: number
  readonly last_7d_mm: number
  readonly source: RainSource
  readonly source_name: string | null
  readonly observed_at: string
}

export interface ModelSoilMoistureDto {
  readonly volumetric_m3m3: number
  readonly field_capacity_m3m3: number
  readonly field_capacity_pct: number
}

export interface SoilInspectionViewDto {
  readonly state: SoilState
  readonly inspected_at: string
  readonly valid_until: string
  readonly inspected_by: PersonRefDto
}

export interface SoilStatusDto {
  readonly state: SoilState
  readonly basis: SoilBasis
  readonly texture: SoilTexture | null
  readonly thresholds_source: ThresholdsSource
  readonly rain: RainAccumulationDto | null
  readonly model_moisture: ModelSoilMoistureDto | null
  readonly inspection: SoilInspectionViewDto | null
}

export interface DailyRainEntryDto {
  readonly date: string
  readonly rain_mm: number
  readonly kind: DailyRainKind
  readonly source: RainSource
}

export interface MethodEligibilityDto {
  readonly method: SprayMethod
  readonly verdict: EligibilityVerdict
  readonly reasons: readonly EligibilityReason[]
  readonly area_above_limit_pct: number | null
}

export interface TrafficabilityDayDto {
  readonly date: string
  readonly soil_state: SoilState
  readonly methods: readonly MethodEligibilityDto[]
}

export interface MethodReleaseDto {
  readonly method: SprayMethod
  readonly required_state: SoilState
  readonly date: string | null
}

export interface FieldTrafficabilityDto {
  readonly field_id: string
  readonly farm_id: string
  readonly as_of: string
  readonly soil: SoilStatusDto
  readonly rain_daily: readonly DailyRainEntryDto[]
  readonly days: readonly TrafficabilityDayDto[]
  readonly releases: readonly MethodReleaseDto[]
  readonly data_issues: readonly TrafficabilityDataIssue[]
}

export interface RainGaugeDto {
  readonly id: string
  readonly farm_id: string
  readonly name: string
  readonly location: GeoPoint | null
  readonly created_at: string
  readonly updated_at: string
  readonly version: number
}

export interface RainGaugeInputDto {
  readonly name: string
  readonly location?: GeoPoint | null
}

export interface RainGaugePatchDto {
  readonly name?: string
  readonly location?: GeoPoint | null
}

export interface RainReadingDto {
  readonly id: string
  readonly rain_gauge_id: string
  readonly rain_mm: number
  readonly observed_at: string
  readonly recorded_by: PersonRefDto
  readonly recorded_at: string
}

export interface RainReadingInputDto {
  readonly rain_mm: number
  readonly observed_at: string
}

export interface SoilInspectionDto {
  readonly id: string
  readonly field_id: string
  readonly farm_id: string
  readonly state: SoilState
  readonly inspected_at: string
  readonly valid_until: string
  readonly inspected_by: PersonRefDto
  readonly note: string | null
}

export interface SoilInspectionInputDto {
  readonly state: InspectableSoilState
  readonly inspected_at?: string
  readonly note?: string | null
}

export interface SoilProfileDto {
  readonly field_id: string
  readonly farm_id: string
  readonly soil_texture: SoilTexture | null
  readonly rain_gauge: { readonly id: string, readonly name: string } | null
}

export interface SoilProfileInputDto {
  readonly soil_texture: SoilTexture | null
  readonly rain_gauge_id: string | null
}
