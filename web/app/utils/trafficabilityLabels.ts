/**
 * Portuguese labels of the trafficability codes (ADR 0022, view "Chuva e solo").
 * Translation is presentation, not rule: states, verdicts and reasons come from
 * the API and are never derived here. Method names reuse `SPRAY_METHOD_LABELS`
 * of the relief, so both views name the machines the same way.
 */
import type { SprayMethod } from '~/types/terrain'
import type {
  EligibilityReason,
  EligibilityVerdict,
  MethodEligibility,
  RainAccumulation,
  RainSource,
  SoilBasis,
  SoilState,
  SoilTexture,
  ThresholdsSource,
  TrafficabilityDataIssue,
} from '~/types/trafficability'
import { SPRAY_METHOD_LABELS } from './terrain'
import { DISPLAY_TIME_ZONE } from './timeZone'

export const SOIL_STATE_LABELS: Readonly<Record<SoilState, string>> = {
  dry: 'Seco',
  ideal: 'Ideal',
  moist: 'Úmido',
  waterlogged: 'Encharcado',
  unknown: 'Desconhecido',
}

export const ELIGIBILITY_VERDICT_LABELS: Readonly<Record<EligibilityVerdict, string>> = {
  apt: 'Apto',
  attention: 'Atenção',
  unfit: 'Inapto',
}

/** Short reasons shown under a verdict ("Inapto · solo encharcado"). */
export const ELIGIBILITY_REASON_LABELS: Readonly<Record<EligibilityReason, string>> = {
  slope_restricted: 'só na área permitida',
  slope_unsuitable: 'declive acima do limite',
  terrain_unknown: 'relevo não calculado',
  soil_moist: 'solo úmido',
  soil_waterlogged: 'solo encharcado',
  soil_unknown: 'sem dado de chuva',
  slope_restricted_on_wet_soil: 'encosta com solo úmido',
}

export const RAIN_SOURCE_LABELS: Readonly<Record<RainSource, string>> = {
  gauge: 'pluviômetro',
  station: 'estação',
  model: 'modelo (não é o talhão)',
}

export const TRAFFICABILITY_DATA_ISSUE_LABELS: Readonly<Record<TrafficabilityDataIssue, string>> = {
  no_rain_source: 'Sem pluviômetro, estação ou modelo de chuva para este talhão.',
  no_soil_texture: 'Talhão sem textura do solo cadastrada.',
  weather_unavailable: 'Previsão do tempo indisponível: sem projeção dos próximos dias.',
  terrain_not_calculated: 'Relevo não calculado: métodos pesados ficam em atenção.',
}

/** Title of the alert of each data gap; the message is `TRAFFICABILITY_DATA_ISSUE_LABELS`. */
export const TRAFFICABILITY_DATA_ISSUE_TITLES: Readonly<Record<TrafficabilityDataIssue, string>> = {
  no_rain_source: 'Estado do solo desconhecido: métodos terrestres em atenção',
  no_soil_texture: 'Textura do solo não cadastrada',
  weather_unavailable: 'Previsão indisponível',
  terrain_not_calculated: 'Relevo não calculado',
}

export const SOIL_TEXTURE_LABELS: Readonly<Record<SoilTexture, string>> = {
  sandy: 'Arenoso',
  medium: 'Médio',
  clay: 'Argiloso',
}

export const SOIL_BASIS_LABELS: Readonly<Record<SoilBasis, string>> = {
  inspection: 'vistoria em campo',
  measured_rain: 'chuva medida',
  model: 'modelo de previsão',
  none: 'sem dados',
}

export const THRESHOLDS_SOURCE_LABELS: Readonly<Record<ThresholdsSource, string>> = {
  heuristic: 'limites heurísticos, ainda não calibrados',
  calibrated: 'limites calibrados pelo agrônomo',
}

/** Reasons joined for a cell or tooltip: "declive acima do limite · solo encharcado"; empty when there are none. */
export function eligibilityReasonsLabel(reasons: readonly EligibilityReason[]): string {
  return reasons.map(reason => ELIGIBILITY_REASON_LABELS[reason]).join(' · ')
}

/** Verdict with its reasons: "Inapto · solo encharcado", or just "Apto". */
export function methodEligibilityLabel(eligibility: Pick<MethodEligibility, 'verdict' | 'reasons'>): string {
  const reasons = eligibilityReasonsLabel(eligibility.reasons)
  const verdict = ELIGIBILITY_VERDICT_LABELS[eligibility.verdict]
  return reasons ? `${verdict} · ${reasons}` : verdict
}

/** Method name of the table rows; same labels as the relief view. */
export function sprayMethodLabel(method: SprayMethod): string {
  return SPRAY_METHOD_LABELS[method]
}

/** Where a number came from: "Pluviômetro da sede (pluviômetro)", or the kind alone without a name ("modelo (não é o talhão)"). */
export function rainSourceLabel(source: RainSource, sourceName: string | null = null): string {
  const kind = RAIN_SOURCE_LABELS[source]
  const name = sourceName?.trim()
  return name ? `${name} (${kind})` : kind
}

const readingHour = new Intl.DateTimeFormat('pt-BR', { hour: '2-digit', minute: '2-digit', timeZone: DISPLAY_TIME_ZONE })

/**
 * Legend of the measured rain, by source: "Medida: Pluviômetro da sede, lançado 07:00"
 * (gauge, entered by a person), "Medida: Estação Sede, observado 07:00" (station), or
 * "Estimativa do modelo (não é o talhão)". Without a name, the kind of source stands in.
 */
export function measuredRainCaption(rain: Pick<RainAccumulation, 'source' | 'sourceName' | 'observedAt'>): string {
  if (rain.source === 'model') return 'Estimativa do modelo (não é o talhão)'
  const name = rain.sourceName?.trim() || RAIN_SOURCE_LABELS[rain.source]
  const verb = rain.source === 'gauge' ? 'lançado' : 'observado'
  return `Medida: ${name}, ${verb} ${readingHour.format(rain.observedAt)}`
}
