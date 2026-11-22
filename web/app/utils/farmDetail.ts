/**
 * View models of the farm detail screen, "Áreas e talhões" tab (Figma 15:126).
 * Presentation only: conditions come from the API (spray-planner rules), the
 * front never derives them.
 */
import type { FieldBlock, Field } from '~/types/field'
import type { GeoJsonPolygon } from '~/types/geo'
import type { SprayCondition } from '~/types/spray'
import { formatDaysAgo, formatHectares, formatMonthYear } from './format'
import { SPRAY_CONDITION_LABELS } from './labels'

export type FarmDetailTab = 'overview' | 'fields' | 'owners' | 'applications' | 'windows'
export type FarmMapMode = 'crop' | 'status' | 'satellite'

export const FARM_DETAIL_TABS: readonly FarmDetailTab[] = ['overview', 'fields', 'owners', 'applications', 'windows']
export const FARM_MAP_MODES: readonly FarmMapMode[] = ['crop', 'status', 'satellite']

export const FARM_DETAIL_TAB_LABELS: Readonly<Record<FarmDetailTab, string>> = {
  overview: 'Visão geral',
  fields: 'Áreas e talhões',
  owners: 'Proprietários',
  applications: 'Aplicações',
  windows: 'Janelas de pulverização',
}

export const FARM_MAP_MODE_LABELS: Readonly<Record<FarmMapMode, string>> = {
  crop: 'Cultura',
  status: 'Status',
  satellite: 'Satélite',
}

/** "Status" colors by condition; "Cultura" and "Satélite" color by variety. */
export function mapColorByFor(mode: FarmMapMode): 'crop' | 'status' {
  return mode === 'status' ? 'status' : 'crop'
}

export function mapBasemapFor(mode: FarmMapMode): 'vector' | 'satellite' {
  return mode === 'satellite' ? 'satellite' : 'vector'
}

/** Field polygons colored by variety slot (see `cultivarColorSlots`). */
export function toFieldMapFeatures(fields: readonly Field[], colorIndexOf: (field: Field) => number): FieldMapFeature[] {
  return fields.map(field => ({
    id: field.id,
    geometry: field.polygon,
    status: field.sprayCondition,
    cropIndex: colorIndexOf(field),
    label: fieldShortLabel(field.name),
  }))
}

/** Group label of fields without a block. */
export const NO_BLOCK_LABEL = 'Sem área'
const NO_BLOCK_KEY = '__no-block__'
const SPRAY_CONDITION_ORDER: readonly SprayCondition[] = ['suitable', 'attention', 'unsuitable', 'rain']

export interface ConditionView {
  readonly value: SprayCondition
  readonly label: string
}

/** Control shown but not usable yet (missing endpoint or out-of-scope feature). */
export interface DisabledAction {
  readonly disabled: true
  readonly hint: string
}

export const COMING_SOON: DisabledAction = { disabled: true, hint: 'Em breve' }
export const NEEDS_PLANNER: DisabledAction = { disabled: true, hint: 'Disponível com o planejador' }

export interface FarmDetailTabView {
  readonly value: FarmDetailTab
  readonly label: string
  readonly count: number | null
  readonly disabled: boolean
}

export interface FieldsSummaryView {
  readonly blockCount: string
  readonly fieldCount: string
  readonly productiveAreaLabel: string
}

export interface FieldRowView {
  readonly id: string
  readonly name: string
  readonly shortLabel: string
  readonly subtitle: string
  readonly condition: ConditionView | null
  readonly colorIndex: number
  readonly selected: boolean
}

export interface FieldBlockView {
  readonly key: string
  readonly name: string
  readonly areaLabel: string
  readonly collapsedLabel: string
  readonly expanded: boolean
  readonly fields: readonly FieldRowView[]
}

export interface CropLegendItem {
  readonly colorIndex: number
  readonly label: string
}

export type MapLegendView =
  | { readonly kind: 'crop', readonly items: readonly CropLegendItem[] }
  | { readonly kind: 'status', readonly items: readonly ConditionView[] }

/**
 * Structurally compatible with `MapFeature` of `components/map/MapView.vue`.
 * `cropIndex` is the variety color slot (same as `FieldRowView.colorIndex`).
 */
export interface FieldMapFeature {
  readonly id: string
  readonly geometry: GeoJsonPolygon
  readonly status: SprayCondition | null
  readonly cropIndex: number
  /** Map tag, e.g. "T3". */
  readonly label: string
}

export interface FieldPopoverView {
  readonly id: string
  readonly name: string
  readonly subtitle: string
  readonly areaLabel: string
  readonly plantingLabel: string
  readonly lastApplicationLabel: string
  readonly condition: ConditionView | null
  /** The day's window comes from the spray-planner, not modeled yet: always null. */
  readonly todayWindowLabel: null
  /** "Editar": edit form of screen 10. */
  readonly editFieldTo: string
  readonly planApplication: DisabledAction
}

const ENABLED_TABS: ReadonlySet<FarmDetailTab> = new Set(['fields'])

export function isFarmDetailTabEnabled(tab: FarmDetailTab): boolean {
  return ENABLED_TABS.has(tab)
}

/**
 * Tabs of the farm screens (9 and 10). Only "Áreas e talhões" has a screen;
 * applications have no endpoint yet, so they never show an invented count.
 */
export function farmDetailTabs(counts: { fields: number | null, owners: number | null }): FarmDetailTabView[] {
  const countOf: Readonly<Record<FarmDetailTab, number | null>> = {
    overview: null,
    fields: counts.fields,
    owners: counts.owners,
    applications: null,
    windows: null,
  }
  return FARM_DETAIL_TABS.map(value => ({
    value,
    label: FARM_DETAIL_TAB_LABELS[value],
    count: countOf[value],
    disabled: !isFarmDetailTabEnabled(value),
  }))
}

export function toConditionView(condition: SprayCondition | null | undefined): ConditionView | null {
  return condition ? { value: condition, label: SPRAY_CONDITION_LABELS[condition] } : null
}

export function blockKey(name: string | null): string {
  return name ?? NO_BLOCK_KEY
}

/** Field area with one fixed decimal, as in the Figma list: 44 → "44,0 ha". */
export function formatFieldArea(areaHa: number): string {
  const number = new Intl.NumberFormat('pt-BR', { minimumFractionDigits: 1, maximumFractionDigits: 1 }).format(areaHa)
  return `${number} ha`
}

function fieldCountLabel(count: number): string {
  return count === 1 ? '1 talhão' : `${count} talhões`
}

/** "Catuaí 144" or, without a variety, the crop name ("Milho safrinha"). */
export function cultivarLabel(field: Pick<Field, 'crop' | 'variety'>): string {
  return field.variety?.trim() || field.crop.name
}

/** Map tag: "Talhão 3" → "T3"; other names keep their initials ("Lote A" → "LA"). */
export function fieldShortLabel(name: string): string {
  const numbered = /^talh[aã]o\s+(\S+)/i.exec(name.trim())
  if (numbered) return `T${numbered[1]}`
  return name.trim().split(/\s+/).map(word => word.charAt(0)).join('').slice(0, 3).toUpperCase()
}

/**
 * Color slot per variety (crop + variety; the crop alone when there is no
 * variety), in order of first appearance in the grouped list, so the list
 * swatch, the map polygon and the legend share the same color. Stable for the
 * same data; legend labels are the variety names, as in the Figma (15:321).
 */
export function cultivarColorSlots(fields: readonly Field[]): { indexOf: (field: Field) => number, legend: CropLegendItem[] } {
  const slots = new Map<string, number>()
  const legend: CropLegendItem[] = []
  const keyOf = (field: Field) => `${field.crop.code}|${field.variety?.trim() ?? ''}`
  for (const field of fields) {
    const key = keyOf(field)
    if (slots.has(key)) continue
    slots.set(key, legend.length)
    legend.push({ colorIndex: legend.length, label: cultivarLabel(field) })
  }
  return { indexOf: field => slots.get(keyOf(field)) ?? 0, legend }
}

/** Conditions present among the fields, in the badge order of the design system. */
export function statusLegend(fields: readonly Pick<Field, 'sprayCondition'>[]): ConditionView[] {
  const present = new Set(fields.map(field => field.sprayCondition))
  return SPRAY_CONDITION_ORDER.filter(condition => present.has(condition)).map(condition => ({
    value: condition,
    label: SPRAY_CONDITION_LABELS[condition],
  }))
}

export function toFieldRowView(field: Field, colorIndex: number, selectedId: string | null): FieldRowView {
  return {
    id: field.id,
    name: field.name,
    shortLabel: fieldShortLabel(field.name),
    subtitle: `${cultivarLabel(field)} · ${formatFieldArea(field.areaHa)}`,
    condition: toConditionView(field.sprayCondition),
    colorIndex,
    selected: field.id === selectedId,
  }
}

export function toFieldBlockView(
  block: FieldBlock,
  rows: readonly FieldRowView[],
  expanded: boolean,
): FieldBlockView {
  const areaLabel = formatHectares(block.areaHa, 0)
  return {
    key: blockKey(block.name),
    name: block.name ?? NO_BLOCK_LABEL,
    areaLabel,
    collapsedLabel: `${areaLabel} · ${fieldCountLabel(block.fields.length)}`,
    expanded,
    fields: rows,
  }
}

const PLACEHOLDER = '—'

/** Summary cards of the panel; "—" while there is no data to show. */
export function toFieldsSummaryView(blocks: readonly FieldBlock[], hasData: boolean): FieldsSummaryView {
  if (!hasData) return { blockCount: PLACEHOLDER, fieldCount: PLACEHOLDER, productiveAreaLabel: PLACEHOLDER }
  const fieldCount = blocks.reduce((sum, block) => sum + block.fields.length, 0)
  const areaHa = blocks.reduce((sum, block) => sum + block.areaHa, 0)
  return {
    blockCount: String(blocks.filter(block => block.name !== null).length),
    fieldCount: String(fieldCount),
    productiveAreaLabel: formatHectares(areaHa, 0),
  }
}

export function toFieldPopoverView(field: Field, now: Date): FieldPopoverView {
  const cultivar = cultivarLabel(field)
  return {
    id: field.id,
    name: field.name,
    subtitle: field.block ? `${field.block} · ${cultivar}` : cultivar,
    areaLabel: formatFieldArea(field.areaHa),
    plantingLabel: formatMonthYear(field.plantingDate),
    lastApplicationLabel: formatDaysAgo(field.lastApplicationAt, now),
    condition: toConditionView(field.sprayCondition),
    todayWindowLabel: null,
    editFieldTo: editFieldRoute(field.farmId, field.id),
    planApplication: NEEDS_PLANNER,
  }
}

/** Edit route of screen 10 (same form as the new field). */
export function editFieldRoute(farmId: string, fieldId: string): string {
  return `/farms/${encodeURIComponent(farmId)}/fields/${encodeURIComponent(fieldId)}/edit`
}

/** Route of screen 10, optionally pre-filling the block. */
export function newFieldRoute(farmId: string, block: string | null = null): string {
  const base = `/farms/${encodeURIComponent(farmId)}/fields/new`
  return block ? `${base}?block=${encodeURIComponent(block)}` : base
}
