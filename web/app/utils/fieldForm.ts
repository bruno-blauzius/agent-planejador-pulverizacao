/**
 * Presentation rules of the field form (screen 10, Figma 16:82): parsing,
 * local validation and labels. Area, stand and geometry validity are decided
 * by the API; local checks only spare a round trip for obvious mistakes.
 */
import type { FieldError } from '~/services/errors'
import type { Crop } from '~/types/crop'
import type { Field, FieldInput } from '~/types/field'
import type { GeoJsonPolygon } from '~/types/geo'
import { isDrawablePolygon } from './geoArea'

export type FieldFormStep = 'draw' | 'details'
export type FieldFormField = 'name' | 'block' | 'cropCode' | 'variety' | 'plantingDate' | 'spacing' | 'polygon'
export type FieldFormErrors = Partial<Record<FieldFormField, string>>

export const FIELD_FORM_FIELDS: readonly FieldFormField[] = ['name', 'block', 'cropCode', 'variety', 'plantingDate', 'spacing', 'polygon']

export interface FieldFormValues {
  name: string
  block: string
  cropCode: string | null
  variety: string
  /** ISO `YYYY-MM-DD` or empty. */
  plantingDate: string
  /** "3,5 × 0,7" (row × plant, meters) or empty. */
  spacing: string
}

export interface CropChipView {
  readonly code: string
  readonly label: string
  readonly icon: string
  readonly selected: boolean
}

export const FIELD_FORM_MESSAGES = {
  name: 'Informe o nome do talhão.',
  cropCode: 'Escolha a cultura.',
  polygon: 'Desenhe o talhão no mapa, com ao menos 3 pontos.',
  spacing: 'Use o formato 3,5 × 0,7 (entre linhas × entre plantas, em metros).',
  plantingDate: 'Informe uma data válida.',
  network: 'Não foi possível salvar. Verifique sua conexão e tente novamente.',
  notFound: 'O talhão ou a fazenda não existe mais.',
  generic: 'Não foi possível salvar o talhão. Tente novamente.',
} as const

/** Crops shown as chips, in the Figma order (16:467); the rest goes under "+ Outra". */
export const FEATURED_CROP_CODES: readonly string[] = ['coffee_arabica', 'coffee_conilon', 'soybean', 'corn', 'wheat']

const CROP_ICONS: Readonly<Record<string, string>> = {
  coffee_arabica: '☕',
  coffee_conilon: '☕',
  soybean: '🌱',
  corn: '🌽',
  corn_second_season: '🌽',
  wheat: '🌾',
}
const DEFAULT_CROP_ICON = '🌿'

export function emptyFieldFormValues(block: string | null = null): FieldFormValues {
  return { name: '', block: block ?? '', cropCode: null, variety: '', plantingDate: '', spacing: '' }
}

const decimal = new Intl.NumberFormat('pt-BR', { maximumFractionDigits: 2 })

export function formatSpacing(rowSpacingM: number | null, plantSpacingM: number | null): string {
  if (rowSpacingM === null || plantSpacingM === null) return ''
  return `${decimal.format(rowSpacingM)} × ${decimal.format(plantSpacingM)}`
}

export function fieldToFormValues(field: Field): FieldFormValues {
  return {
    name: field.name,
    block: field.block ?? '',
    cropCode: field.crop.code,
    variety: field.variety ?? '',
    plantingDate: field.plantingDate ?? '',
    spacing: formatSpacing(field.rowSpacingM, field.plantSpacingM),
  }
}

const NUMBER = String.raw`(\d+(?:[.,]\d+)?)`
const SPACING_PATTERN = new RegExp(String.raw`^\s*${NUMBER}\s*[×xX*]\s*${NUMBER}\s*$`)

/** "3,5 × 0,7" → { row: 3.5, plant: 0.7 }; empty → null pair; invalid → undefined. */
export function parseSpacing(text: string): { rowSpacingM: number | null, plantSpacingM: number | null } | undefined {
  if (!text.trim()) return { rowSpacingM: null, plantSpacingM: null }
  const match = SPACING_PATTERN.exec(text)
  if (!match) return undefined
  const [row, plant] = [Number(match[1]!.replace(',', '.')), Number(match[2]!.replace(',', '.'))]
  return row > 0 && plant > 0 ? { rowSpacingM: row, plantSpacingM: plant } : undefined
}

function isIsoDate(value: string): boolean {
  if (!/^\d{4}-\d{2}-\d{2}$/.test(value)) return false
  const date = new Date(`${value}T00:00:00Z`)
  return !Number.isNaN(date.getTime()) && date.toISOString().startsWith(value)
}

export function validateFieldForm(values: FieldFormValues, polygon: GeoJsonPolygon | null): FieldFormErrors {
  const errors: FieldFormErrors = {}
  if (!values.name.trim()) errors.name = FIELD_FORM_MESSAGES.name
  if (!values.cropCode) errors.cropCode = FIELD_FORM_MESSAGES.cropCode
  if (!isDrawablePolygon(polygon)) errors.polygon = FIELD_FORM_MESSAGES.polygon
  if (parseSpacing(values.spacing) === undefined) errors.spacing = FIELD_FORM_MESSAGES.spacing
  if (values.plantingDate && !isIsoDate(values.plantingDate)) errors.plantingDate = FIELD_FORM_MESSAGES.plantingDate
  return errors
}

function blankToNull(value: string): string | null {
  return value.trim() || null
}

/** Payload for create/update. Call only after `validateFieldForm` returned no errors. */
export function toFieldInput(values: FieldFormValues, polygon: GeoJsonPolygon, season: string | null): FieldInput {
  const spacing = parseSpacing(values.spacing) ?? { rowSpacingM: null, plantSpacingM: null }
  return {
    name: values.name.trim(),
    block: blankToNull(values.block),
    cropCode: values.cropCode ?? '',
    variety: blankToNull(values.variety),
    season,
    plantingDate: blankToNull(values.plantingDate),
    ...spacing,
    polygon,
  }
}

const API_FIELD_MAP: Readonly<Record<string, FieldFormField>> = {
  name: 'name',
  block: 'block',
  crop_code: 'cropCode',
  variety: 'variety',
  planting_date: 'plantingDate',
  row_spacing_m: 'spacing',
  plant_spacing_m: 'spacing',
  polygon: 'polygon',
}

/** Maps the API `details` (snake_case) to the form fields; unknown fields are dropped. */
export function fieldErrorsFromApi(details: readonly FieldError[]): FieldFormErrors {
  const errors: FieldFormErrors = {}
  for (const detail of details) {
    const field = API_FIELD_MAP[detail.field]
    if (field && !errors[field]) errors[field] = detail.message
  }
  return errors
}

export function cropChips(crops: readonly Crop[], selectedCode: string | null): { featured: CropChipView[], others: CropChipView[] } {
  const toChip = (crop: Crop): CropChipView => ({
    code: crop.code,
    label: crop.name,
    icon: CROP_ICONS[crop.code] ?? DEFAULT_CROP_ICON,
    selected: crop.code === selectedCode,
  })
  const featured = FEATURED_CROP_CODES
    .map(code => crops.find(crop => crop.code === code))
    .filter((crop): crop is Crop => crop !== undefined)
  const others = crops.filter(crop => !FEATURED_CROP_CODES.includes(crop.code))
  const selectedOther = others.find(crop => crop.code === selectedCode)
  return {
    featured: [...featured, ...(selectedOther ? [selectedOther] : [])].map(toChip),
    others: others.filter(crop => crop !== selectedOther).map(toChip),
  }
}

/** "Talhão 10" after "Talhão 9"; "Talhão 1" for a farm without numbered fields. */
export function nextFieldName(fields: readonly Pick<Field, 'name'>[]): string {
  const numbers = fields
    .map(field => /^talh[aã]o\s+(\d+)\s*$/i.exec(field.name.trim())?.[1])
    .filter((value): value is string => value !== undefined)
    .map(Number)
  return `Talhão ${Math.max(0, ...numbers) + 1}`
}

export function drawHintFor(vertices: number): string {
  if (vertices === 0) return 'Clique no mapa para marcar os vértices · feche no primeiro ponto'
  return `Arraste os vértices para ajustar · ${vertices} ${vertices === 1 ? 'ponto' : 'pontos'}`
}

export function samePolygon(a: GeoJsonPolygon | null, b: GeoJsonPolygon | null): boolean {
  return JSON.stringify(a) === JSON.stringify(b)
}
