/**
 * Forms of "Chuva e solo" (ADR 0022, phase 2): rain reading, soil inspection,
 * soil profile and rain gauge name. Presentation and input rules only, mirroring
 * the contract limits so the user hears about a mistake before the round trip;
 * the API validates again and its 422 wins. Nothing here decides a soil state.
 *
 * Date and time are typed as local time of the farm (`America/Sao_Paulo`) and
 * sent as an instant.
 */
import type { ApiError, FieldError } from '~/services/errors'
import {
  INSPECTABLE_SOIL_STATES,
  SOIL_TEXTURES,
  type InspectableSoilState,
  type RainGauge,
  type RainReadingInput,
  type SoilInspectionInput,
  type SoilProfileInput,
  type SoilTexture,
} from '~/types/trafficability'
import { forbiddenMessageFor } from './permissions'
import { parseDecimalInput } from './terrain'
import { DISPLAY_TIME_ZONE } from './timeZone'
import { SOIL_STATE_LABELS, SOIL_TEXTURE_LABELS } from './trafficabilityLabels'

export const RAIN_READING_MAX_MM = 500
export const SOIL_INSPECTION_NOTE_MAX = 500
export const RAIN_GAUGE_NAME_MAX = 100

export const RAIN_READING_NOTICE = 'Informe a chuva acumulada desde a leitura anterior deste pluviômetro. A leitura vale para todos os talhões ligados a ele.'
export const SOIL_INSPECTION_VALIDITY_NOTICE = 'A vistoria vale por 12 horas a partir do horário informado e, enquanto valer, substitui o cálculo do estado do solo de hoje.'
export const SOIL_PROFILE_NOTICE = 'Sem pluviômetro, o talhão usa a chuva do modelo de previsão. Sem textura, o estado do solo fica desconhecido.'

export type FormErrors<F extends string> = Partial<Record<F, string>>

// ---- local date and time ---------------------------------------------------------------

export interface LocalDateTime {
  /** `YYYY-MM-DD`. */
  readonly date: string
  /** `HH:MM`, 24 h. */
  readonly time: string
}

const zoneParts = new Intl.DateTimeFormat('en-US', {
  timeZone: DISPLAY_TIME_ZONE,
  hourCycle: 'h23',
  year: 'numeric',
  month: '2-digit',
  day: '2-digit',
  hour: '2-digit',
  minute: '2-digit',
  second: '2-digit',
})

function wallClockOf(instant: Date): { year: number, month: number, day: number, hour: number, minute: number, second: number } {
  const parts = Object.fromEntries(zoneParts.formatToParts(instant).map(part => [part.type, part.value]))
  return {
    year: Number(parts.year),
    month: Number(parts.month),
    day: Number(parts.day),
    hour: Number(parts.hour),
    minute: Number(parts.minute),
    second: Number(parts.second),
  }
}

/** Offset of the zone at an instant, in ms (São Paulo: −3 h). */
function zoneOffsetMs(instant: Date): number {
  const wall = wallClockOf(instant)
  const wallAsUtc = Date.UTC(wall.year, wall.month - 1, wall.day, wall.hour, wall.minute, wall.second)
  return wallAsUtc - Math.floor(instant.getTime() / 1000) * 1000
}

const pad = (value: number) => String(value).padStart(2, '0')

/** Date and time inputs for an instant, in the farm's zone. */
export function toLocalDateTime(instant: Date): LocalDateTime {
  const wall = wallClockOf(instant)
  return { date: `${wall.year}-${pad(wall.month)}-${pad(wall.day)}`, time: `${pad(wall.hour)}:${pad(wall.minute)}` }
}

/** 24 h time with one or two hour digits ("7:30", "07:30"). */
const TIME_PATTERN = /^([01]?\d|2[0-3]):([0-5]\d)$/

/** "7:30" → "07:30"; anything that is not a time is returned trimmed, for the validation to report. */
export function normalizeTimeInput(time: string): string {
  const match = TIME_PATTERN.exec(time.trim())
  return match ? `${match[1]!.padStart(2, '0')}:${match[2]}` : time.trim()
}

/** Today in the farm's zone (`YYYY-MM-DD`): the `max` of the date inputs. */
export function todayInFarmZone(now: Date): string {
  return toLocalDateTime(now).date
}

/** Instant of a local date (`YYYY-MM-DD`) and time (`H:MM` or `HH:MM`); null when either is not a real value. */
export function fromLocalDateTime(date: string, time: string): Date | null {
  const dateMatch = /^(\d{4})-(\d{2})-(\d{2})$/.exec(date.trim())
  const timeMatch = TIME_PATTERN.exec(time.trim())
  if (!dateMatch || !timeMatch) return null
  const [year, month, day] = [Number(dateMatch[1]), Number(dateMatch[2]), Number(dateMatch[3])]
  const wallAsUtc = Date.UTC(year, month - 1, day, Number(timeMatch[1]), Number(timeMatch[2]))
  const check = new Date(wallAsUtc)
  if (check.getUTCFullYear() !== year || check.getUTCMonth() !== month - 1 || check.getUTCDate() !== day) return null
  // Second pass covers an offset change between the guess and the answer.
  const first = wallAsUtc - zoneOffsetMs(new Date(wallAsUtc))
  return new Date(wallAsUtc - zoneOffsetMs(new Date(first)))
}

function validateWhen(date: string, time: string, now: Date, futureMessage: string): FormErrors<'date' | 'time'> {
  if (!date.trim()) return { date: 'Informe a data.' }
  if (!time.trim()) return { time: 'Informe o horário.' }
  const instant = fromLocalDateTime(date, time)
  if (!instant) return TIME_PATTERN.test(time.trim()) ? { date: 'Use uma data válida.' } : { time: 'Use o formato 07:30.' }
  // Same-format text compares like the dates themselves.
  if (date.trim() > todayInFarmZone(now)) return { date: 'A data não pode estar no futuro.' }
  return instant > now ? { time: futureMessage } : {}
}

/** Message of a failure no field explains: 403 in plain words, 409 asks to reload; null when a field shows it. */
export function submissionErrorMessage(error: ApiError, hasFieldErrors: boolean): string | null {
  const forbidden = forbiddenMessageFor(error)
  if (forbidden) return forbidden
  if (error.code === 'conflict') return 'Outra pessoa alterou este registro. Recarregue e tente de novo.'
  return hasFieldErrors ? null : error.message
}

/** 422 `details` (`rain_mm`, `body.observed_at`, …) → errors by form field; unknown fields are left out. */
export function formErrorsFromApi<F extends string>(details: readonly FieldError[], fieldByApiField: Readonly<Record<string, F>>): FormErrors<F> {
  const errors: FormErrors<F> = {}
  for (const detail of details) {
    const name = detail.field.split('.').pop() ?? detail.field
    const field = fieldByApiField[name]
    if (field && !errors[field]) errors[field] = detail.message
  }
  return errors
}

export interface SelectOption<T> {
  readonly value: T
  readonly label: string
}

// ---- rain reading ------------------------------------------------------------------------

export type RainReadingFormField = 'gaugeId' | 'rainMm' | 'date' | 'time'

export interface RainReadingFormValues {
  gaugeId: string | null
  /** Text of the input ("12,5"). */
  rainMm: string
  date: string
  time: string
}

export function emptyRainReadingValues(now: Date, gaugeId: string | null): RainReadingFormValues {
  return { gaugeId, rainMm: '', ...toLocalDateTime(now) }
}

export function validateRainReading(values: RainReadingFormValues, now: Date): FormErrors<RainReadingFormField> {
  const errors: FormErrors<RainReadingFormField> = {}
  if (!values.gaugeId) errors.gaugeId = 'Escolha o pluviômetro.'
  if (!values.rainMm.trim()) errors.rainMm = 'Informe a chuva em mm.'
  else {
    const mm = parseDecimalInput(values.rainMm)
    if (mm === null) errors.rainMm = 'Use um número, como 12,5.'
    else if (mm > RAIN_READING_MAX_MM) errors.rainMm = `Use um valor de 0 a ${RAIN_READING_MAX_MM} mm.`
  }
  return { ...errors, ...validateWhen(values.date, values.time, now, 'O horário da leitura não pode estar no futuro.') }
}

/** Gauge and body of the request, or null while the form has errors. */
export function toRainReadingInput(values: RainReadingFormValues, now: Date): { readonly gaugeId: string, readonly input: RainReadingInput } | null {
  if (Object.keys(validateRainReading(values, now)).length > 0) return null
  const rainMm = parseDecimalInput(values.rainMm)
  const observedAt = fromLocalDateTime(values.date, values.time)
  if (!values.gaugeId || rainMm === null || !observedAt) return null
  return { gaugeId: values.gaugeId, input: { rainMm, observedAt } }
}

export function rainReadingErrorsFromApi(details: readonly FieldError[]): FormErrors<RainReadingFormField> {
  return formErrorsFromApi(details, { rain_mm: 'rainMm', observed_at: 'time' })
}

export function rainGaugeOptions(gauges: readonly RainGauge[]): readonly SelectOption<string>[] {
  return gauges.map(gauge => ({ value: gauge.id, label: gauge.name }))
}

// ---- soil inspection ---------------------------------------------------------------------

export type SoilInspectionFormField = 'state' | 'date' | 'time' | 'note'

export interface SoilInspectionFormValues {
  state: InspectableSoilState | null
  date: string
  time: string
  note: string
}

export const SOIL_INSPECTION_STATE_OPTIONS: readonly SelectOption<InspectableSoilState>[] = INSPECTABLE_SOIL_STATES.map(state => ({
  value: state,
  label: SOIL_STATE_LABELS[state],
}))

export function emptySoilInspectionValues(now: Date): SoilInspectionFormValues {
  return { state: null, note: '', ...toLocalDateTime(now) }
}

export function validateSoilInspection(values: SoilInspectionFormValues, now: Date): FormErrors<SoilInspectionFormField> {
  const errors: FormErrors<SoilInspectionFormField> = {}
  if (!values.state) errors.state = 'Escolha o estado do solo.'
  if (values.note.trim().length > SOIL_INSPECTION_NOTE_MAX) errors.note = `Use no máximo ${SOIL_INSPECTION_NOTE_MAX} caracteres.`
  return { ...errors, ...validateWhen(values.date, values.time, now, 'O horário da vistoria não pode estar no futuro.') }
}

export function toSoilInspectionInput(values: SoilInspectionFormValues, now: Date): SoilInspectionInput | null {
  if (Object.keys(validateSoilInspection(values, now)).length > 0) return null
  const inspectedAt = fromLocalDateTime(values.date, values.time)
  if (!values.state || !inspectedAt) return null
  return { state: values.state, inspectedAt, note: values.note.trim() || null }
}

export function soilInspectionErrorsFromApi(details: readonly FieldError[]): FormErrors<SoilInspectionFormField> {
  return formErrorsFromApi(details, { state: 'state', inspected_at: 'time', note: 'note' })
}

// ---- soil profile -----------------------------------------------------------------------

export type SoilProfileFormField = 'soilTexture' | 'rainGaugeId'

export interface SoilProfileFormValues {
  soilTexture: SoilTexture | null
  rainGaugeId: string | null
}

export const SOIL_TEXTURE_OPTIONS: readonly SelectOption<SoilTexture | null>[] = [
  ...SOIL_TEXTURES.map(texture => ({ value: texture, label: SOIL_TEXTURE_LABELS[texture] })),
  { value: null, label: 'Não informada' },
]

/**
 * Gauges of the farm plus "none" (the field uses the weather model). The gauge
 * the profile already has is kept even while the list is not loaded, so the
 * select can show it.
 */
export function soilProfileGaugeOptions(
  gauges: readonly RainGauge[],
  current: { readonly id: string, readonly name: string } | null = null,
): readonly SelectOption<string | null>[] {
  const listed = rainGaugeOptions(gauges)
  const extra = current && !gauges.some(gauge => gauge.id === current.id) ? [{ value: current.id, label: current.name }] : []
  return [...listed, ...extra, { value: null, label: 'Nenhum (usa o modelo de previsão)' }]
}

export interface SoilProfileValidationContext {
  /** The gauge list arrived (no pending load, no error); otherwise the API decides. */
  readonly gaugesLoaded: boolean
  /** Gauge the loaded profile already has: always accepted. */
  readonly profileGaugeId: string | null
}

/** Only a gauge of the farm can be chosen, checked once the list is loaded; the API answers 422 anyway. */
export function validateSoilProfile(
  values: SoilProfileFormValues,
  gauges: readonly RainGauge[],
  context: SoilProfileValidationContext = { gaugesLoaded: true, profileGaugeId: null },
): FormErrors<SoilProfileFormField> {
  const id = values.rainGaugeId
  if (id === null || id === context.profileGaugeId || !context.gaugesLoaded) return {}
  return gauges.some(gauge => gauge.id === id) ? {} : { rainGaugeId: 'Escolha um pluviômetro desta fazenda.' }
}

export function toSoilProfileInput(values: SoilProfileFormValues): SoilProfileInput {
  return { soilTexture: values.soilTexture, rainGaugeId: values.rainGaugeId }
}

export function soilProfileErrorsFromApi(details: readonly FieldError[]): FormErrors<SoilProfileFormField> {
  return formErrorsFromApi(details, { soil_texture: 'soilTexture', rain_gauge_id: 'rainGaugeId' })
}

// ---- rain gauge ---------------------------------------------------------------------------

export type RainGaugeFormField = 'name' | 'location'

export function rainGaugeErrorsFromApi(details: readonly FieldError[]): FormErrors<RainGaugeFormField> {
  return formErrorsFromApi(details, { name: 'name', location: 'location' })
}

/** Name of a new or renamed gauge: 1–100 characters, not repeated in the farm (case aside). */
export function validateRainGaugeName(name: string, gauges: readonly RainGauge[], exceptId: string | null = null): string | null {
  const trimmed = name.trim()
  if (!trimmed) return 'Informe o nome do pluviômetro.'
  if (trimmed.length > RAIN_GAUGE_NAME_MAX) return `Use no máximo ${RAIN_GAUGE_NAME_MAX} caracteres.`
  const repeated = gauges.some(gauge => gauge.id !== exceptId && gauge.name.trim().toLowerCase() === trimmed.toLowerCase())
  return repeated ? 'Já existe um pluviômetro com este nome nesta fazenda.' : null
}
