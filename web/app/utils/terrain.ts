/**
 * View models of the "Relevo" tab of the farm screen (Figma 67:960: 04b list
 * and map, 04c field detail, 04d adjustment modal, 04e not calculated and
 * progress). Presentation only: classes, method fit and the outdated flag come
 * from the API (ADR 0021); the front formats them and never classifies relief.
 *
 * Colors are not decided here: the logic hands out keys (`TerrainMapKey`) and
 * the UI maps them to theme tokens (`theme/map-style.ts`).
 *
 * Contract: `.claude/contratos/tela-21.md`.
 */
import type { FieldError } from '~/services/errors'
import type { Field, FieldBlock } from '~/types/field'
import type { GeoJsonPolygon } from '~/types/geo'
import {
  SLOPE_CLASSES,
  TERRAIN_ASPECTS,
  type ElevationProfilePoint,
  type FarmTerrain,
  type FarmTerrainSummary,
  type FieldTerrain,
  type FieldTerrainAdjustmentInput,
  type FieldTerrainSummary,
  type MethodFit,
  type MethodFitStatus,
  type SlopeClass,
  type SprayMethod,
  type TerrainAdjustmentOrigin,
  type TerrainAspect,
  type TerrainCalculationScope,
  type TerrainMeasurement,
  type TerrainPosition,
  type TerrainStatus,
} from '~/types/terrain'
import { blockKey, cultivarLabel, fieldShortLabel, formatFieldArea, NO_BLOCK_LABEL } from './farmDetail'
import { formatHectares } from './format'
import { DISPLAY_TIME_ZONE } from './timeZone'

/** Map layer of the tab (segmented control "Declividade | Altitude | Exposição"). */
export type TerrainMapLayer = 'slope' | 'elevation' | 'aspect'

/** Relief class of a row, badge and polygon; `not_calculated` without an effective value. */
export type TerrainClassKey = SlopeClass | 'not_calculated'

/** Elevation band, 0 = lowest; `TERRAIN_ELEVATION_BAND_COUNT` bands over the farm range. */
export type ElevationBandKey = `elevation-${number}`

/**
 * Color key of a polygon or legend item for the current layer:
 * - `slope`: `TerrainClassKey`;
 * - `elevation`: `ElevationBandKey` | `not_calculated` (no calculated elevation, even with an adjustment);
 * - `aspect`: `TerrainAspect` | `no_aspect` (manual adjustment, no aspect) | `not_calculated`.
 */
export type TerrainMapKey = TerrainClassKey | ElevationBandKey | TerrainAspect | 'no_aspect'

/** Badge of a relief class ("Forte ondulado", "Não calculado"). */
export interface SlopeClassBadgeView {
  readonly key: TerrainClassKey
  readonly label: string
}

/** Indicator card: label, value and an optional line below ("desnível de 69 m"). */
export interface TerrainIndicatorView {
  readonly label: string
  readonly value: string
  readonly hint: string | null
}

/** Panel title of 04b. */
export interface TerrainPanelHeaderView {
  /** "Relevo por talhão". */
  readonly title: string
  /** "Modelo de elevação Copernicus 30 m · calculado em 12/11/2026"; without calculation, the model only. */
  readonly subtitle: string
}

/** The three indicators of 04b; "—" while there is no data. */
export interface FarmTerrainIndicatorsView {
  /** "Declividade média" / "7,9 %". */
  readonly slopeMean: TerrainIndicatorView
  /** "Com restrição" / "3 talhões". */
  readonly restricted: TerrainIndicatorView
  /** "Altitude" / "872–948 m". */
  readonly elevation: TerrainIndicatorView
}

export interface TerrainFieldRowView {
  readonly id: string
  readonly name: string
  /**
   * "6,2 % · máx. 9,8 % · encosta"; "Calculando…" (pending/calculating);
   * "Falha no cálculo" (failed); "Relevo não calculado" (not_calculated).
   */
  readonly detail: string
  readonly badge: SlopeClassBadgeView
  /** Color of the row bar; always the slope class, whatever the map layer. */
  readonly colorKey: TerrainClassKey
  /** Effective value comes from a manual adjustment (small "ajustado" mark). */
  readonly isAdjusted: boolean
  readonly isFailed: boolean
  readonly selected: boolean
}

export interface TerrainBlockView {
  /** Same key as the "Áreas e talhões" block (`blockKey`). */
  readonly key: string
  /** "Área Norte" | "Sem área". */
  readonly name: string
  /** Open: "162 ha · média 7,4 %" (or "162 ha · sem relevo"). */
  readonly areaLabel: string
  /** Closed: "102 ha · 1 talhão sem relevo", or "102 ha · média 7,4 %" when all have relief. */
  readonly collapsedLabel: string
  readonly expanded: boolean
  readonly fields: readonly TerrainFieldRowView[]
}

/** Field polygon of the tab map. Structurally a `MapFeature` once the UI adds `terrainKey`/`dashed`. */
export interface TerrainMapFeature {
  readonly id: string
  readonly geometry: GeoJsonPolygon
  /** Key of the current layer (see `TerrainMapKey`). */
  readonly terrainKey: TerrainMapKey
  /** Field without relief: dashed outline. */
  readonly dashed: boolean
  /** Map tag, e.g. "T4". */
  readonly label: string
}

export interface TerrainLegendItemView {
  readonly key: TerrainMapKey
  /** "Plano" | "872–887 m" | "Norte" | "Não calculado". */
  readonly label: string
}

export interface TerrainLegendView {
  readonly layer: TerrainMapLayer
  /** "Declividade" | "Altitude" | "Exposição". */
  readonly title: string
  readonly items: readonly TerrainLegendItemView[]
}

export interface TerrainMapLayerOptionView {
  readonly value: TerrainMapLayer
  readonly label: string
  /** True while no field has relief (04e): the map is grey and the layers are off. */
  readonly disabled: boolean
}

/** Popover of the selected polygon (04b). */
export interface TerrainPopoverView {
  readonly id: string
  readonly name: string
  /** "Área Centro · Arara · 44,0 ha" (block · variety or crop · area). */
  readonly subtitle: string
  readonly badge: SlopeClassBadgeView
  /** "Média" / "6,2 %". */
  readonly slopeMean: TerrainIndicatorView
  /** "Máxima" / "9,8 %". */
  readonly slopeMax: TerrainIndicatorView
  /** "Desnível" / "69 m" ("—" without calculated elevation). */
  readonly relief: TerrainIndicatorView
  /**
   * Built from the field detail (`methodFits`): "Restrito para tratorizado de barra (62 % da área acima de 20 %).
   * Inadequado para autopropelido." While the detail loads or when it fails, a generic text if the
   * summary says `hasRestrictedMethod`; null when nothing is restricted.
   */
  readonly methodWarning: string | null
  /** "Ver relevo do talhão" opens 04c (`?terrainView=detail`). */
  readonly canOpenDetail: boolean
}

export interface SlopeClassAreaRowView {
  readonly key: SlopeClass
  readonly label: string
  /** "12,3 ha". */
  readonly areaLabel: string
  /** "28 %". */
  readonly shareLabel: string
  /** 0–100, width of the stacked bar segment. */
  readonly sharePct: number
}

/** Profile point normalized for an SVG: x 0 (top point) → 1 (end); y 0 = lowest, 1 = highest elevation. */
export interface ProfilePointView {
  readonly x: number
  readonly y: number
}

export interface ElevationProfileView {
  /** At least two points; null profile when the API sends fewer. */
  readonly points: readonly ProfilePointView[]
  /** "941 m · topo". */
  readonly topLabel: string
  /** "872 m · base (290 m)" (base elevation and profile length). */
  readonly baseLabel: string
}

export interface TerrainFactRowView {
  /** "Posição" | "Exposição" | "Baixada fria". */
  readonly label: string
  /** "Encosta" | "Noroeste" | "Sim" / "Não"; "—" when unknown. */
  readonly value: string
}

export interface MethodFitRowView {
  readonly method: SprayMethod
  /** "Tratorizado de barra". */
  readonly label: string
  /** "até 20 %" | "sem limite". */
  readonly limitLabel: string
  readonly status: MethodFitStatus
  /** "Compatível" | "Restrito · 62 % da área" | "Inadequado". */
  readonly statusLabel: string
}

/** Calculated × adjusted comparison of 04c, shown when there is an adjustment. */
export interface TerrainComparisonRowView {
  /** "Declividade média" | "Declividade máxima" | "Posição" | "Classe". */
  readonly label: string
  /** "—" without calculation. */
  readonly calculated: string
  readonly adjusted: string
}

/** Field detail card (04c). */
export interface FieldTerrainDetailView {
  readonly id: string
  /** "Talhão 4". */
  readonly title: string
  readonly badge: SlopeClassBadgeView
  /**
   * "Área Centro · Arara · 44,0 ha · calculado do Copernicus DEM GLO-30 (30 m) em 12/11/2026", or
   * "… · ajustado manualmente por Marina Souza em 20/11/2026" when `source` is manual.
   */
  readonly subtitle: string
  readonly status: TerrainStatus
  /** "Falha no cálculo: <failure_reason>"; null unless failed. */
  readonly failureMessage: string | null
  readonly hasAdjustment: boolean
  /** "O cálculo mais recente diverge do ajuste. Revise o ajuste."; null unless `adjustmentOutdated`. */
  readonly outdatedWarning: string | null
  /** Empty without an adjustment. */
  readonly comparison: readonly TerrainComparisonRowView[]
  /** "Justificativa: …"; null without an adjustment. */
  readonly adjustmentNote: string | null
  readonly indicators: {
    readonly slopeMean: TerrainIndicatorView
    /** "90 % da área até" / "8,4 %" ("—" with an adjustment only). */
    readonly slopeP90: TerrainIndicatorView
    readonly slopeMax: TerrainIndicatorView
    /** "Altitude" / "872–941 m" / hint "desnível de 69 m". */
    readonly elevation: TerrainIndicatorView
  }
  /** "Área por classe de declividade"; empty without calculation. */
  readonly classAreas: readonly SlopeClassAreaRowView[]
  readonly profile: ElevationProfileView | null
  readonly facts: readonly TerrainFactRowView[]
  /** "Equipamentos para este relevo"; empty without effective relief. */
  readonly methods: readonly MethodFitRowView[]
  /** Fixed note: reference limits vs. the equipment's own limit. */
  readonly methodsNote: string
}

/** 04e: "Relevo ainda não calculado". */
export interface TerrainNotCalculatedView {
  readonly title: string
  /** What will be calculated (declividade, classe, altitude, posição e exposição, métodos). */
  readonly items: readonly string[]
  /** "Calcular relevo dos 14 talhões"; null without `registry.write`. */
  readonly calculateLabel: string | null
  /** Without `registry.write`: "Peça a um responsável pelo cadastro para calcular o relevo."; null otherwise. */
  readonly noPermissionMessage: string | null
}

export interface TerrainProgressView {
  /** "9 de 14 talhões". */
  readonly label: string
  /** 0–100 for the bar. */
  readonly percent: number
}

export interface TerrainFailureView {
  readonly fieldId: string
  readonly name: string
  /** `failure_reason` from the API, or "Motivo não informado". */
  readonly reason: string
}

/** Fields without relief when others already have it: "2 talhões sem relevo" + "Calcular" (scope `missing`). */
export interface TerrainMissingBannerView {
  readonly label: string
  /** Null without `registry.write`. */
  readonly calculateLabel: string | null
}

/** What the tab shows as a whole. */
export type TerrainTabState =
  | 'loading' // first load
  | 'error' // GET /terrain failed (the fields tab data may still be there)
  | 'not_calculated' // 04e, no field calculated and nothing running
  | 'calculating' // progress != null (04e progress when nothing is ready yet, else list with progress)
  | 'ready'

/** `?terrainView=`: map (default) or detail card (04c). */
export type TerrainView = 'map' | 'detail'

/** View of the field detail card (ADR 0022): "Relevo | Chuva e solo". */
export type TerrainDetailView = 'relief' | 'rain'

/** Header buttons of the farm screen; depend on the tab and on `registry.write`. */
export type FarmHeaderActionId =
  | 'newArea'
  | 'newField'
  | 'terrainMethodology' // "Como calculamos": the page opens the dialog (UI state)
  | 'calculateTerrain' // scope `missing`
  | 'recalculateTerrain' // scope `all`

export interface FarmHeaderActionView {
  readonly id: FarmHeaderActionId
  readonly label: string
  /** Icon name of `theme/icons.ts`. */
  readonly icon: string | null
  readonly variant: 'primary' | 'secondary'
  /** Navigation actions ("Novo talhão"). */
  readonly to: string | null
  /** Shown but not usable ("Nova área": "Em breve"). */
  readonly unavailableHint: string | null
  /** Request in flight or calculation running. */
  readonly loading: boolean
}

/** Fields of the adjustment modal (04d). */
export type TerrainAdjustmentFormField = 'origin' | 'slopeMeanPct' | 'slopeMaxPct' | 'position' | 'justification'
export type TerrainAdjustmentFormErrors = Partial<Record<TerrainAdjustmentFormField, string>>

export interface TerrainAdjustmentFormValues {
  origin: TerrainAdjustmentOrigin
  /** Typed text, comma or dot ("23,5"). */
  slopeMeanPct: string
  slopeMaxPct: string
  position: TerrainPosition | null
  justification: string
}

export interface TerrainAdjustmentHintsView {
  /** "Calculado: 23,5 %"; null without calculation. */
  readonly slopeMean: string | null
  readonly slopeMax: string | null
  readonly position: string | null
  /** "Classe atual: Forte ondulado". */
  readonly currentClass: string
  /** "A classe é recalculada ao salvar." */
  readonly classNotice: string
}

/** Scope of the calculation behind each calculate button. */
export const TERRAIN_SCOPE_BY_ACTION = {
  calculateTerrain: 'missing',
  recalculateTerrain: 'all',
} as const satisfies Readonly<Record<'calculateTerrain' | 'recalculateTerrain', TerrainCalculationScope>>

// ---- constants and labels ----------------------------------------------------

/** Polling interval while a calculation runs (client only, active tab only). */
export const TERRAIN_POLL_INTERVAL_MS = 3000

/** Equal elevation bands over the farm range ("Altitude" layer). */
export const TERRAIN_ELEVATION_BAND_COUNT = 5

/** The farm summary does not carry the model; the source is fixed by ADR 0021. */
export const TERRAIN_ELEVATION_MODEL_LABEL = 'Copernicus 30 m'

/** Attribution required by the Copernicus license. */
export const TERRAIN_COPERNICUS_ATTRIBUTION = 'Contém dados modificados do Copernicus DEM GLO-30 © DLR e Airbus'

export const TERRAIN_METHODS_NOTE
  = 'Limites de referência da API, só pela declividade. Quando o equipamento tiver limite próprio cadastrado, ele prevalece. Bula, solo, porte e zona tampão o planejador avalia.'

export const TERRAIN_ADJUSTMENT_JUSTIFICATION_MAX = 1000

export const TERRAIN_ADJUSTMENT_INFO_NOTICE
  = 'O ajuste vale no lugar do cálculo e fica registrado na auditoria. O valor calculado continua guardado para comparação.'

const TERRAIN_NOT_CALCULATED_LABEL = 'Não calculado'
const TERRAIN_FAILURE_FALLBACK = 'Motivo não informado'
const TERRAIN_GENERIC_METHOD_WARNING = 'Algum equipamento tem restrição neste relevo. Veja o detalhe do talhão.'
const PLACEHOLDER = '—'

export const SLOPE_CLASS_LABELS: Readonly<Record<SlopeClass, string>> = {
  flat: 'Plano',
  gently_undulating: 'Suave ondulado',
  undulating: 'Ondulado',
  strongly_undulating: 'Forte ondulado',
  mountainous: 'Montanhoso',
}

export const TERRAIN_POSITION_LABELS: Readonly<Record<TerrainPosition, string>> = {
  hilltop: 'Topo',
  slope: 'Encosta',
  lowland: 'Baixada',
}

export const TERRAIN_ASPECT_LABELS: Readonly<Record<TerrainAspect, string>> = {
  north: 'Norte',
  northeast: 'Nordeste',
  east: 'Leste',
  southeast: 'Sudeste',
  south: 'Sul',
  southwest: 'Sudoeste',
  west: 'Oeste',
  northwest: 'Noroeste',
  flat: 'Plano',
}

export const SPRAY_METHODS: readonly SprayMethod[] = ['knapsack', 'tractor_boom', 'self_propelled', 'airblast', 'aircraft', 'drone']

export const SPRAY_METHOD_LABELS: Readonly<Record<SprayMethod, string>> = {
  knapsack: 'Costal',
  tractor_boom: 'Tratorizado de barra',
  self_propelled: 'Autopropelido',
  airblast: 'Turbopulverizador',
  aircraft: 'Avião',
  drone: 'Drone',
}

export const TERRAIN_ADJUSTMENT_ORIGIN_LABELS: Readonly<Record<TerrainAdjustmentOrigin, string>> = {
  topographic_survey: 'Levantamento topográfico',
  field_assessment: 'Avaliação em campo',
}

const METHOD_FIT_STATUS_LABELS: Readonly<Record<MethodFitStatus, string>> = {
  compatible: 'Compatível',
  restricted: 'Restrito',
  unsuitable: 'Inadequado',
}

export const TERRAIN_MAP_LAYERS: readonly TerrainMapLayer[] = ['slope', 'elevation', 'aspect']

export const TERRAIN_MAP_LAYER_LABELS: Readonly<Record<TerrainMapLayer, string>> = {
  slope: 'Declividade',
  elevation: 'Altitude',
  aspect: 'Exposição',
}

export const TERRAIN_ADJUSTMENT_ORIGIN_OPTIONS: readonly { readonly value: TerrainAdjustmentOrigin, readonly label: string }[] = [
  { value: 'field_assessment', label: TERRAIN_ADJUSTMENT_ORIGIN_LABELS.field_assessment },
  { value: 'topographic_survey', label: TERRAIN_ADJUSTMENT_ORIGIN_LABELS.topographic_survey },
]

export const TERRAIN_POSITION_OPTIONS: readonly { readonly value: TerrainPosition, readonly label: string }[] = [
  { value: 'hilltop', label: TERRAIN_POSITION_LABELS.hilltop },
  { value: 'slope', label: TERRAIN_POSITION_LABELS.slope },
  { value: 'lowland', label: TERRAIN_POSITION_LABELS.lowland },
]

/** Fixed text of the "Como calculamos" dialog. */
export const TERRAIN_METHODOLOGY = {
  title: 'Como calculamos o relevo',
  paragraphs: [
    'O relevo vem do modelo digital de elevação Copernicus DEM GLO-30, com resolução de 30 m, recortado pelo polígono de cada talhão.',
    'Calculamos a declividade em cada ponto do talhão e resumimos em média, máxima e a faixa que cobre 90 % da área. A classe de relevo segue a escala da Embrapa pela declividade média.',
    'A altitude, a posição no relevo (topo, encosta ou baixada) e a exposição também saem do modelo. Os limites de cada método de aplicação são configuração do sistema.',
    'O ajuste manual vale no lugar do cálculo: informe a origem e a justificativa. O valor calculado fica guardado para comparação.',
  ],
  attribution: TERRAIN_COPERNICUS_ATTRIBUTION,
} as const

// ---- formatting ---------------------------------------------------------------

const oneDecimal = new Intl.NumberFormat('pt-BR', { minimumFractionDigits: 1, maximumFractionDigits: 1 })
const flexibleDecimal = new Intl.NumberFormat('pt-BR', { maximumFractionDigits: 1 })
const integer = new Intl.NumberFormat('pt-BR', { maximumFractionDigits: 0 })
const dateInZone = new Intl.DateTimeFormat('pt-BR', {
  timeZone: DISPLAY_TIME_ZONE,
  day: '2-digit',
  month: '2-digit',
  year: 'numeric',
})

/** 23.5 → "23,5 %"; 6.24 → "6,2 %". */
export function formatSlopePct(value: number | null): string {
  return value === null ? PLACEHOLDER : `${oneDecimal.format(value)} %`
}

/** 62.2 → "62 %" (shares of the area). */
export function formatSharePct(value: number): string {
  return `${integer.format(value)} %`
}

/** 872.4 → "872 m". */
export function formatMeters(value: number): string {
  return `${integer.format(value)} m`
}

/** "872–948 m"; one value when both ends round to the same meter. */
export function formatElevationRange(min: number | null, max: number | null): string {
  if (min === null || max === null) return PLACEHOLDER
  const low = integer.format(min)
  const high = integer.format(max)
  return low === high ? `${low} m` : `${low}–${high} m`
}

export function formatTerrainDate(date: Date): string {
  return dateInZone.format(date)
}

const slopeClassBadge = (measurement: TerrainMeasurement | null): SlopeClassBadgeView => (
  measurement
    ? { key: measurement.slopeClass, label: SLOPE_CLASS_LABELS[measurement.slopeClass] }
    : { key: 'not_calculated', label: TERRAIN_NOT_CALCULATED_LABEL }
)

/** Class key of a measurement (`not_calculated` without one). */
export function terrainClassKey(measurement: TerrainMeasurement | null): TerrainClassKey {
  return measurement ? measurement.slopeClass : 'not_calculated'
}

function pluralize(count: number, singular: string, plural: string): string {
  return count === 1 ? `1 ${singular}` : `${count} ${plural}`
}

function firstQueryValue(value: unknown): string | undefined {
  const candidate = Array.isArray(value) ? value[0] : value
  return typeof candidate === 'string' ? candidate : undefined
}

/** `?terrainView=`: only `detail` opens the card; anything else is the map. */
export function terrainViewFrom(value: unknown): TerrainView {
  return firstQueryValue(value) === 'detail' ? 'detail' : 'map'
}

/** `?terrainDetailView=` raw value; anything but `rain` reads as "Relevo". */
export function terrainDetailViewFrom(value: unknown): TerrainDetailView {
  return firstQueryValue(value) === 'rain' ? 'rain' : 'relief'
}

/** `?terrainField=` raw value (validated against the farm fields by the caller). */
export function terrainFieldIdFrom(value: unknown): string | null {
  return firstQueryValue(value) ?? null
}

export function indexTerrainSummaries(items: readonly FieldTerrainSummary[]): ReadonlyMap<string, FieldTerrainSummary> {
  return new Map(items.map(item => [item.fieldId, item]))
}

// ---- elevation bands, map keys and legends ---------------------------------------

export interface ElevationRange {
  readonly min: number
  readonly max: number
}

/** Farm elevation range, or null before any calculation. */
export function elevationRangeOf(summary: Pick<FarmTerrainSummary, 'elevationMinM' | 'elevationMaxM'> | null | undefined): ElevationRange | null {
  if (!summary || summary.elevationMinM === null || summary.elevationMaxM === null) return null
  return { min: summary.elevationMinM, max: summary.elevationMaxM }
}

/**
 * Band (0 = lowest) of the field's mean calculated elevation over the farm
 * range; null without calculated elevation. A zero-width range is one band.
 */
export function elevationBandOf(
  field: Pick<FieldTerrainSummary, 'elevationMinM' | 'elevationMaxM'>,
  range: ElevationRange | null,
  count: number,
): ElevationBandKey | null {
  if (!range || field.elevationMinM === null || field.elevationMaxM === null) return null
  const width = range.max - range.min
  if (width <= 0 || count <= 1) return 'elevation-0'
  const mean = (field.elevationMinM + field.elevationMaxM) / 2
  const index = Math.floor(((mean - range.min) / width) * count)
  return `elevation-${Math.min(count - 1, Math.max(0, index))}`
}

/** Color key of a field for the layer; `not_calculated` when the layer has no value for it. */
export function terrainMapKey(
  layer: TerrainMapLayer,
  summary: FieldTerrainSummary | null,
  range: ElevationRange | null,
): TerrainMapKey {
  if (layer === 'elevation') return summary ? (elevationBandOf(summary, range, TERRAIN_ELEVATION_BAND_COUNT) ?? 'not_calculated') : 'not_calculated'
  const measurement = summary?.measurement ?? null
  if (layer === 'aspect') return measurement ? (measurement.aspect ?? 'no_aspect') : 'not_calculated'
  return terrainClassKey(measurement)
}

function elevationBandLegend(range: ElevationRange | null, count: number): TerrainLegendItemView[] {
  if (!range) return []
  const width = range.max - range.min
  if (width <= 0) return [{ key: 'elevation-0', label: formatElevationRange(range.min, range.max) }]
  const step = width / count
  return Array.from({ length: count }, (_, index) => ({
    key: `elevation-${index}` as const,
    label: formatElevationRange(range.min + index * step, range.min + (index + 1) * step),
  }))
}

/**
 * Legend of the layer. Slope and aspect are scales and always list every
 * value; "Sem exposição" and "Não calculado" appear only when some field has them.
 */
export function terrainLegend(
  layer: TerrainMapLayer,
  summaries: readonly (FieldTerrainSummary | null)[],
  range: ElevationRange | null,
): TerrainLegendView {
  const keys = new Set(summaries.map(summary => terrainMapKey(layer, summary, range)))
  const extras: TerrainLegendItemView[] = []
  if (layer === 'aspect' && keys.has('no_aspect')) extras.push({ key: 'no_aspect', label: 'Sem exposição' })
  if (keys.has('not_calculated')) extras.push({ key: 'not_calculated', label: TERRAIN_NOT_CALCULATED_LABEL })

  let items: TerrainLegendItemView[]
  if (layer === 'slope') items = SLOPE_CLASSES.map(key => ({ key, label: SLOPE_CLASS_LABELS[key] }))
  else if (layer === 'aspect') items = TERRAIN_ASPECTS.map(key => ({ key, label: TERRAIN_ASPECT_LABELS[key] }))
  else items = elevationBandLegend(range, TERRAIN_ELEVATION_BAND_COUNT)
  return { layer, title: TERRAIN_MAP_LAYER_LABELS[layer], items: [...items, ...extras] }
}

export function toTerrainMapFeatures(
  fields: readonly Field[],
  summariesById: ReadonlyMap<string, FieldTerrainSummary>,
  layer: TerrainMapLayer,
  range: ElevationRange | null,
): TerrainMapFeature[] {
  return fields.map((field) => {
    const summary = summariesById.get(field.id) ?? null
    return {
      id: field.id,
      geometry: field.polygon,
      terrainKey: terrainMapKey(layer, summary, range),
      dashed: !summary?.measurement,
      label: fieldShortLabel(field.name),
    }
  })
}

export function toTerrainMapLayerOptions(hasRelief: boolean): TerrainMapLayerOptionView[] {
  return TERRAIN_MAP_LAYERS.map(value => ({ value, label: TERRAIN_MAP_LAYER_LABELS[value], disabled: !hasRelief }))
}

// ---- list (04b) ---------------------------------------------------------------

function fieldRowDetail(summary: FieldTerrainSummary | null): string {
  if (summary?.status === 'pending' || summary?.status === 'calculating') return 'Calculando…'
  if (summary?.status === 'failed') return 'Falha no cálculo'
  const measurement = summary?.measurement
  if (!measurement) return 'Relevo não calculado'
  return `${formatSlopePct(measurement.slopeMeanPct)} · máx. ${formatSlopePct(measurement.slopeMaxPct)} · ${TERRAIN_POSITION_LABELS[measurement.position].toLowerCase()}`
}

function toTerrainFieldRowView(field: Field, summary: FieldTerrainSummary | null, selectedId: string | null): TerrainFieldRowView {
  const measurement = summary?.measurement ?? null
  return {
    id: field.id,
    name: field.name,
    detail: fieldRowDetail(summary),
    badge: slopeClassBadge(measurement),
    colorKey: terrainClassKey(measurement),
    isAdjusted: summary?.source === 'manual',
    isFailed: summary?.status === 'failed',
    selected: field.id === selectedId,
  }
}

/** Area-weighted mean slope of the fields that have relief; null when none has. */
export function weightedSlopeMean(items: readonly { readonly areaHa: number, readonly measurement: TerrainMeasurement | null }[]): number | null {
  const withRelief = items.filter(item => item.measurement !== null)
  const totalArea = withRelief.reduce((sum, item) => sum + item.areaHa, 0)
  if (withRelief.length === 0) return null
  if (totalArea <= 0) return withRelief.reduce((sum, item) => sum + (item.measurement?.slopeMeanPct ?? 0), 0) / withRelief.length
  return withRelief.reduce((sum, item) => sum + (item.measurement?.slopeMeanPct ?? 0) * item.areaHa, 0) / totalArea
}

/** Blocks of the tab: same keys and order as "Áreas e talhões", crossed with the relief rows by field id. */
export function toTerrainBlockViews(
  blocks: readonly FieldBlock[],
  summariesById: ReadonlyMap<string, FieldTerrainSummary>,
  collapsed: ReadonlySet<string>,
  selectedId: string | null,
): TerrainBlockView[] {
  return blocks.map((block) => {
    const entries = block.fields.map(field => ({ field, summary: summariesById.get(field.id) ?? null }))
    const mean = weightedSlopeMean(entries.map(({ field, summary }) => ({ areaHa: field.areaHa, measurement: summary?.measurement ?? null })))
    const withoutRelief = entries.filter(entry => !entry.summary?.measurement).length
    const area = formatHectares(block.areaHa, 0)
    const meanLabel = mean === null ? 'sem relevo' : `média ${formatSlopePct(mean)}`
    return {
      key: blockKey(block.name),
      name: block.name ?? NO_BLOCK_LABEL,
      areaLabel: `${area} · ${meanLabel}`,
      collapsedLabel: withoutRelief > 0 && mean !== null
        ? `${area} · ${pluralize(withoutRelief, 'talhão sem relevo', 'talhões sem relevo')}`
        : `${area} · ${meanLabel}`,
      expanded: !collapsed.has(blockKey(block.name)),
      fields: entries.map(({ field, summary }) => toTerrainFieldRowView(field, summary, selectedId)),
    }
  })
}

/** Panel title and subtitle of 04b. */
export function toTerrainPanelHeader(terrain: FarmTerrain | null): TerrainPanelHeaderView {
  const calculatedAt = terrain?.summary.calculatedAt
  const model = `Modelo de elevação ${TERRAIN_ELEVATION_MODEL_LABEL}`
  return {
    title: 'Relevo por talhão',
    subtitle: calculatedAt ? `${model} · calculado em ${formatTerrainDate(calculatedAt)}` : model,
  }
}

export function toFarmTerrainIndicators(terrain: FarmTerrain | null): FarmTerrainIndicatorsView {
  const summary = terrain?.summary
  const hasRelief = Boolean(summary && summary.calculatedFieldCount > 0)
  return {
    slopeMean: { label: 'Declividade média', value: hasRelief ? formatSlopePct(summary?.slopeMeanPct ?? null) : PLACEHOLDER, hint: null },
    restricted: {
      label: 'Com restrição',
      value: hasRelief && summary ? pluralize(summary.restrictedFieldCount, 'talhão', 'talhões') : PLACEHOLDER,
      hint: null,
    },
    elevation: {
      label: 'Altitude',
      value: hasRelief ? formatElevationRange(summary?.elevationMinM ?? null, summary?.elevationMaxM ?? null) : PLACEHOLDER,
      hint: null,
    },
  }
}

// ---- popover and method fit ------------------------------------------------------

function methodFitPart(fit: MethodFit): string {
  const label = SPRAY_METHOD_LABELS[fit.method].toLowerCase()
  if (fit.areaAboveLimitPct === null || fit.slopeLimitPct === null) return label
  return `${label} (${formatSharePct(fit.areaAboveLimitPct)} da área acima de ${formatSharePct(fit.slopeLimitPct)})`
}

/** Sentences about the methods that are not `compatible` (from the API's `methodFits`); null when none. */
export function methodFitWarning(fits: readonly MethodFit[]): string | null {
  const restricted = fits.filter(fit => fit.status === 'restricted')
  const unsuitable = fits.filter(fit => fit.status === 'unsuitable')
  const sentences: string[] = []
  if (restricted.length) sentences.push(`Restrito para ${restricted.map(methodFitPart).join(' e ')}.`)
  if (unsuitable.length) sentences.push(`Inadequado para ${unsuitable.map(fit => SPRAY_METHOD_LABELS[fit.method].toLowerCase()).join(' e ')}.`)
  return sentences.length ? sentences.join(' ') : null
}

export function toTerrainPopoverView(
  field: Field,
  summary: FieldTerrainSummary | null,
  detail: FieldTerrain | null,
): TerrainPopoverView {
  const measurement = summary?.measurement ?? null
  const relief = summary && summary.elevationMinM !== null && summary.elevationMaxM !== null
    ? formatMeters(summary.elevationMaxM - summary.elevationMinM)
    : PLACEHOLDER
  const place = [field.block, cultivarLabel(field), formatFieldArea(field.areaHa)].filter(Boolean).join(' · ')
  const fallbackWarning = summary?.hasRestrictedMethod ? TERRAIN_GENERIC_METHOD_WARNING : null
  return {
    id: field.id,
    name: field.name,
    subtitle: place,
    badge: slopeClassBadge(measurement),
    slopeMean: { label: 'Média', value: formatSlopePct(measurement?.slopeMeanPct ?? null), hint: null },
    slopeMax: { label: 'Máxima', value: formatSlopePct(measurement?.slopeMaxPct ?? null), hint: null },
    relief: { label: 'Desnível', value: relief, hint: null },
    methodWarning: detail ? methodFitWarning(detail.methodFits) : fallbackWarning,
    canOpenDetail: summary !== null,
  }
}

// ---- detail (04c) -------------------------------------------------------------------

/**
 * Profile normalized for an SVG: x 0 (start, the highest point) → 1; y 0 =
 * lowest, 1 = highest. Null with fewer than two points.
 */
export function normalizeProfile(points: readonly ElevationProfilePoint[]): ProfilePointView[] | null {
  if (points.length < 2) return null
  const sorted = [...points].sort((a, b) => a.distanceM - b.distanceM)
  const first = sorted[0]!.distanceM
  const span = sorted[sorted.length - 1]!.distanceM - first
  const elevations = sorted.map(point => point.elevationM)
  const low = Math.min(...elevations)
  const height = Math.max(...elevations) - low
  return sorted.map((point, index) => ({
    x: span > 0 ? (point.distanceM - first) / span : index / (sorted.length - 1),
    y: height > 0 ? (point.elevationM - low) / height : 0.5,
  }))
}

function toElevationProfileView(points: readonly ElevationProfilePoint[]): ElevationProfileView | null {
  const normalized = normalizeProfile(points)
  if (!normalized) return null
  const elevations = points.map(point => point.elevationM)
  const distances = points.map(point => point.distanceM)
  const length = Math.max(...distances) - Math.min(...distances)
  return {
    points: normalized,
    topLabel: `${formatMeters(Math.max(...elevations))} · topo`,
    baseLabel: `${formatMeters(Math.min(...elevations))} · base (${formatMeters(length)})`,
  }
}

function toMethodFitRowView(fit: MethodFit): MethodFitRowView {
  const label = METHOD_FIT_STATUS_LABELS[fit.status]
  return {
    method: fit.method,
    label: SPRAY_METHOD_LABELS[fit.method],
    limitLabel: fit.slopeLimitPct === null ? 'sem limite' : `até ${flexibleDecimal.format(fit.slopeLimitPct)} %`,
    status: fit.status,
    statusLabel: fit.status === 'restricted' && fit.areaAboveLimitPct !== null
      ? `${label} · ${formatSharePct(fit.areaAboveLimitPct)} da área`
      : label,
  }
}

function comparisonRows(detail: FieldTerrain): TerrainComparisonRowView[] {
  const adjusted = detail.adjustment?.measurement
  if (!adjusted) return []
  const calculated = detail.calculated?.measurement ?? null
  return [
    { label: 'Declividade média', calculated: formatSlopePct(calculated?.slopeMeanPct ?? null), adjusted: formatSlopePct(adjusted.slopeMeanPct) },
    { label: 'Declividade máxima', calculated: formatSlopePct(calculated?.slopeMaxPct ?? null), adjusted: formatSlopePct(adjusted.slopeMaxPct) },
    { label: 'Posição', calculated: calculated ? TERRAIN_POSITION_LABELS[calculated.position] : PLACEHOLDER, adjusted: TERRAIN_POSITION_LABELS[adjusted.position] },
    { label: 'Classe', calculated: calculated ? SLOPE_CLASS_LABELS[calculated.slopeClass] : PLACEHOLDER, adjusted: SLOPE_CLASS_LABELS[adjusted.slopeClass] },
  ]
}

function detailSubtitle(field: Field, detail: FieldTerrain): string {
  const place = [field.block, cultivarLabel(field), formatFieldArea(field.areaHa)].filter(Boolean).join(' · ')
  if (detail.source === 'manual' && detail.adjustment) {
    return `${place} · ajustado manualmente por ${detail.adjustment.adjustedBy.name} em ${formatTerrainDate(detail.adjustment.adjustedAt)}`
  }
  if (detail.calculated) {
    const model = detail.calculated.elevationModel
    return `${place} · calculado do ${model.source} (${integer.format(model.resolutionM)} m) em ${formatTerrainDate(detail.calculated.calculatedAt)}`
  }
  return place
}

function factRows(effective: TerrainMeasurement | null): TerrainFactRowView[] {
  return [
    { label: 'Posição', value: effective ? TERRAIN_POSITION_LABELS[effective.position] : PLACEHOLDER },
    { label: 'Exposição', value: effective?.aspect ? TERRAIN_ASPECT_LABELS[effective.aspect] : PLACEHOLDER },
    { label: 'Baixada fria', value: effective ? (effective.position === 'lowland' ? 'Sim' : 'Não') : PLACEHOLDER },
  ]
}

export function toFieldTerrainDetailView(field: Field, detail: FieldTerrain): FieldTerrainDetailView {
  const effective = detail.effective
  const calculated = detail.calculated
  const reliefM = calculated ? calculated.elevationMaxM - calculated.elevationMinM : null
  return {
    id: field.id,
    title: field.name,
    badge: slopeClassBadge(effective),
    subtitle: detailSubtitle(field, detail),
    status: detail.status,
    failureMessage: detail.status === 'failed' ? `Falha no cálculo: ${detail.failureReason?.trim() || TERRAIN_FAILURE_FALLBACK}` : null,
    hasAdjustment: detail.adjustment !== null,
    outdatedWarning: detail.adjustmentOutdated ? 'O cálculo mais recente diverge do ajuste. Revise o ajuste.' : null,
    comparison: comparisonRows(detail),
    adjustmentNote: detail.adjustment ? `Justificativa: ${detail.adjustment.justification}` : null,
    indicators: {
      slopeMean: { label: 'Declividade média', value: formatSlopePct(effective?.slopeMeanPct ?? null), hint: null },
      slopeP90: { label: '90 % da área até', value: formatSlopePct(effective?.slopeP90Pct ?? null), hint: null },
      slopeMax: { label: 'Declividade máxima', value: formatSlopePct(effective?.slopeMaxPct ?? null), hint: null },
      elevation: {
        label: 'Altitude',
        value: formatElevationRange(calculated?.elevationMinM ?? null, calculated?.elevationMaxM ?? null),
        hint: reliefM === null ? null : `desnível de ${formatMeters(reliefM)}`,
      },
    },
    classAreas: (calculated?.classAreas ?? []).map(item => ({
      key: item.slopeClass,
      label: SLOPE_CLASS_LABELS[item.slopeClass],
      areaLabel: formatFieldArea(item.areaHa),
      shareLabel: formatSharePct(item.sharePct),
      sharePct: item.sharePct,
    })),
    profile: calculated ? toElevationProfileView(calculated.profile) : null,
    facts: factRows(effective),
    methods: detail.methodFits.map(toMethodFitRowView),
    methodsNote: TERRAIN_METHODS_NOTE,
  }
}

// ---- states (04e) -------------------------------------------------------------------

export function toTerrainNotCalculatedView(fieldCount: number, canWrite: boolean): TerrainNotCalculatedView {
  return {
    title: 'Relevo ainda não calculado',
    items: [
      'Declividade média e máxima de cada talhão',
      'Classe de relevo, de plano a montanhoso',
      'Altitude e desnível',
      'Posição no relevo e exposição',
      'Compatibilidade com os métodos de aplicação',
    ],
    calculateLabel: canWrite ? `Calcular relevo ${fieldCount === 1 ? 'do talhão' : `dos ${fieldCount} talhões`}` : null,
    noPermissionMessage: canWrite ? null : 'Peça a um responsável pelo cadastro para calcular o relevo.',
  }
}

export function toTerrainProgressView(progress: { readonly total: number, readonly done: number }): TerrainProgressView {
  const percent = progress.total > 0 ? Math.round((progress.done / progress.total) * 100) : 0
  return {
    label: `${progress.done} de ${pluralize(progress.total, 'talhão', 'talhões')}`,
    percent: Math.min(100, Math.max(0, percent)),
  }
}

export function terrainFailures(summaries: readonly FieldTerrainSummary[]): TerrainFailureView[] {
  return summaries
    .filter(summary => summary.status === 'failed')
    .map(summary => ({
      fieldId: summary.fieldId,
      name: summary.fieldName,
      reason: summary.failureReason?.trim() || TERRAIN_FAILURE_FALLBACK,
    }))
}

/** "2 talhões sem relevo" while others already have it (and nothing runs). */
export function toTerrainMissingBanner(withoutRelief: number, canWrite: boolean): TerrainMissingBannerView | null {
  if (withoutRelief <= 0) return null
  return {
    label: `${pluralize(withoutRelief, 'talhão sem relevo', 'talhões sem relevo')}`,
    calculateLabel: canWrite ? 'Calcular' : null,
  }
}

export function terrainTabState(input: {
  readonly terrain: FarmTerrain | null
  readonly pending: boolean
  readonly hasError: boolean
}): TerrainTabState {
  const { terrain } = input
  if (!terrain) return input.hasError && !input.pending ? 'error' : 'loading'
  if (terrain.progress !== null) return 'calculating'
  return terrain.summary.calculatedFieldCount === 0 ? 'not_calculated' : 'ready'
}

// ---- adjustment form (04d) ----------------------------------------------------------

/** Text for an input: 23.5 → "23,5". */
function decimalText(value: number): string {
  return String(value).replace('.', ',')
}

/** "23,5" or "23.5" → 23.5; null for anything else (negatives, thousands separators, letters). */
export function parseDecimalInput(text: string): number | null {
  const trimmed = text.trim()
  if (!/^\d+([.,]\d+)?$/.test(trimmed)) return null
  return Number(trimmed.replace(',', '.'))
}

/** Initial values on open: the current adjustment, else the calculated value, else empty. */
export function emptyTerrainAdjustmentValues(detail: FieldTerrain | null): TerrainAdjustmentFormValues {
  const measurement = detail?.adjustment?.measurement ?? detail?.calculated?.measurement ?? null
  return {
    origin: detail?.adjustment?.origin ?? 'field_assessment',
    slopeMeanPct: measurement ? decimalText(measurement.slopeMeanPct) : '',
    slopeMaxPct: measurement ? decimalText(measurement.slopeMaxPct) : '',
    position: measurement?.position ?? null,
    justification: detail?.adjustment?.justification ?? '',
  }
}

export function toTerrainAdjustmentHints(detail: FieldTerrain | null): TerrainAdjustmentHintsView {
  const calculated = detail?.calculated?.measurement ?? null
  return {
    slopeMean: calculated ? `Calculado: ${formatSlopePct(calculated.slopeMeanPct)}` : null,
    slopeMax: calculated ? `Calculado: ${formatSlopePct(calculated.slopeMaxPct)}` : null,
    position: calculated ? `Calculado: ${TERRAIN_POSITION_LABELS[calculated.position]}` : null,
    currentClass: `Classe atual: ${detail?.effective ? SLOPE_CLASS_LABELS[detail.effective.slopeClass] : TERRAIN_NOT_CALCULATED_LABEL}`,
    classNotice: 'A classe é recalculada ao salvar.',
  }
}

function validateSlopeField(text: string, label: string): string | null {
  if (!text.trim()) return `Informe a declividade ${label}.`
  const value = parseDecimalInput(text)
  if (value === null) return 'Use um número, como 23,5.'
  return value > 100 ? 'Use um valor entre 0 e 100.' : null
}

/** Mirrors the contract (required, 0–100, max not below mean, justification 1–1000). The class is never computed here. */
export function validateTerrainAdjustment(values: TerrainAdjustmentFormValues): TerrainAdjustmentFormErrors {
  const errors: Record<string, string> = {}
  if (!TERRAIN_ADJUSTMENT_ORIGIN_OPTIONS.some(option => option.value === values.origin)) errors.origin = 'Escolha a origem do ajuste.'
  const meanError = validateSlopeField(values.slopeMeanPct, 'média')
  if (meanError) errors.slopeMeanPct = meanError
  const maxError = validateSlopeField(values.slopeMaxPct, 'máxima')
  if (maxError) errors.slopeMaxPct = maxError
  const mean = parseDecimalInput(values.slopeMeanPct)
  const max = parseDecimalInput(values.slopeMaxPct)
  if (!meanError && !maxError && mean !== null && max !== null && max < mean) {
    errors.slopeMaxPct = 'A máxima não pode ser menor que a média.'
  }
  if (!values.position) errors.position = 'Escolha a posição no relevo.'
  const justification = values.justification.trim()
  if (!justification) errors.justification = 'Informe a justificativa.'
  else if (justification.length > TERRAIN_ADJUSTMENT_JUSTIFICATION_MAX) {
    errors.justification = `Use no máximo ${TERRAIN_ADJUSTMENT_JUSTIFICATION_MAX} caracteres.`
  }
  return errors
}

/** Request body of the form, or null while it has errors. */
export function toTerrainAdjustmentInput(values: TerrainAdjustmentFormValues): FieldTerrainAdjustmentInput | null {
  if (Object.keys(validateTerrainAdjustment(values)).length > 0) return null
  const mean = parseDecimalInput(values.slopeMeanPct)
  const max = parseDecimalInput(values.slopeMaxPct)
  if (mean === null || max === null || !values.position) return null
  return {
    origin: values.origin,
    slopeMeanPct: mean,
    slopeMaxPct: max,
    position: values.position,
    justification: values.justification.trim(),
  }
}

const ADJUSTMENT_FIELD_BY_API_FIELD: Readonly<Record<string, TerrainAdjustmentFormField>> = {
  origin: 'origin',
  slope_mean_pct: 'slopeMeanPct',
  slope_max_pct: 'slopeMaxPct',
  position: 'position',
  justification: 'justification',
}

/** 422 `details` (`slope_max_pct`, `body.justification`, …) → errors by form field; unknown fields are left out. */
export function terrainAdjustmentErrorsFromApi(details: readonly FieldError[]): TerrainAdjustmentFormErrors {
  const errors: Record<string, string> = {}
  for (const detail of details) {
    const name = detail.field.split('.').pop() ?? detail.field
    const field = ADJUSTMENT_FIELD_BY_API_FIELD[name]
    if (field && !errors[field]) errors[field] = detail.message
  }
  return errors
}
