/**
 * Presentation helpers of the application history and detail (screens 19 and 20).
 * Status, conditions, totals and the carência date come computed from the API;
 * these functions only format them and pick labels.
 */
import type {
  ApplicationDetail,
  ApplicationFilters,
  ApplicationMonthSummary,
  ApplicationStatus,
  ApplicationSummary,
  CurrentWeather,
} from '~/types/application'
import type { Dose, ProductKind } from '~/types/planner'
import { formatHectares, initials } from './format'
import { APPLICATION_STATUS_LABELS } from './labels'
import {
  capitalize,
  formatClock,
  formatDayMonth,
  formatDecimal,
  formatDurationMinutes,
  PRODUCT_KIND_LABELS,
  weekdayShort,
  zonedIsoDate,
} from './planner'
import { DISPLAY_TIME_ZONE } from './timeZone'

export const APPLICATION_PERIODS: readonly { value: number, label: string }[] = [
  { value: 7, label: 'Últimos 7 dias' },
  { value: 30, label: 'Últimos 30 dias' },
  { value: 90, label: 'Últimos 90 dias' },
]
export const DEFAULT_APPLICATION_PERIOD_DAYS = 30

export const APPLICATION_STATUSES: readonly ApplicationStatus[] = ['planned', 'in_progress', 'completed', 'cancelled']
export const PRODUCT_KINDS: readonly ProductKind[] = ['fungicide', 'insecticide', 'herbicide', 'foliar', 'other']

export type StatusTone = 'suitable' | 'attention' | 'info' | 'muted'

const STATUS_TONES: Readonly<Record<ApplicationStatus, StatusTone>> = {
  completed: 'suitable',
  in_progress: 'attention',
  planned: 'info',
  cancelled: 'muted',
}

export function statusView(status: ApplicationStatus): { value: ApplicationStatus, label: string, tone: StatusTone } {
  return { value: status, label: APPLICATION_STATUS_LABELS[status], tone: STATUS_TONES[status] }
}

// ---- small formatters --------------------------------------------------------------------------

/** "2026-11-21" → "Sáb 21/11" */
export function formatWeekdayDate(isoDate: string): string {
  return `${capitalize(weekdayShort(isoDate))} ${formatDayMonth(isoDate)}`
}

/** "06:12", "09:40" → "06:12 – 09:40" */
export function formatTimeRange(startTime: string, endTime: string): string {
  return `${startTime} – ${endTime}`
}

/** { 0.3, 'L/ha' } → "0,3 L/ha" */
export function formatDose(dose: Dose): string {
  return `${formatDecimal(dose.value)} ${dose.unit}`
}

/** 180 → "calda 180 L/ha" */
export function formatMixRate(mixRateLPerHa: number): string {
  return `calda ${formatDecimal(mixRateLPerHa, 0)} L/ha`
}

/** 10134 → "10.134 L"; 16.9 → "16,9 L" */
export function formatLiters(value: number): string {
  return `${formatDecimal(value, Number.isInteger(value) ? 0 : 1)} L`
}

/** Table area with one decimal, as in the Figma: 44 → "44,0 ha". */
export function formatTableArea(areaHa: number): string {
  return `${new Intl.NumberFormat('pt-BR', { minimumFractionDigits: 1, maximumFractionDigits: 1 }).format(areaHa)} ha`
}

/** "Carlos Mendes" → "Carlos" */
export function firstName(name: string): string {
  return name.trim().split(/\s+/)[0] ?? ''
}

/** Instant → "21/11 às 09:52" in the display zone. */
export function formatDateTimeLabel(date: Date): string {
  return `${formatDayMonth(zonedIsoDate(date))} às ${formatClock(date)}`
}

/** "Guaxupé · 24 °C · vento 7 km/h" */
export function formatWeatherLabel(weather: CurrentWeather): string {
  return `${weather.city} · ${formatDecimal(weather.temperatureC, 0)} °C · vento ${formatDecimal(weather.windKmh, 0)} km/h`
}

/** "Novembro de 2026 · Fazenda Santa Rita" */
export function monthContextLabel(now: Date, context: string | null): string {
  const month = new Intl.DateTimeFormat('pt-BR', { timeZone: DISPLAY_TIME_ZONE, month: 'long', year: 'numeric' }).format(now)
  return [capitalize(month), context].filter(Boolean).join(' · ')
}

// ---- list ----------------------------------------------------------------------------------------

export type ConditionsCellView =
  | { readonly kind: 'values', readonly parts: readonly string[] }
  | { readonly kind: 'live', readonly label: string }
  | { readonly kind: 'rain', readonly label: string }
  | { readonly kind: 'none', readonly label: string }

/** What the "CONDIÇÕES" cell shows: recorded or forecast numbers, "Previsão ao vivo" or the rain that cancelled it. */
export function buildConditionsCell(application: Pick<ApplicationSummary, 'status' | 'conditions'>): ConditionsCellView {
  const { conditions, status } = application
  if (status === 'cancelled' && conditions.rainMm !== null) {
    return { kind: 'rain', label: `Chuva ${formatDecimal(conditions.rainMm, 0)} mm` }
  }
  const parts = [
    conditions.deltaTC === null ? null : `${formatDecimal(conditions.deltaTC)} °C`,
    conditions.windKmh === null ? null : `${formatDecimal(conditions.windKmh, 0)} km/h`,
  ].filter((part): part is string => part !== null)
  if (parts.length > 0) return { kind: 'values', parts }
  if (conditions.source === 'live') return { kind: 'live', label: 'Previsão ao vivo' }
  return { kind: 'none', label: '—' }
}

export interface ApplicationRowView {
  readonly id: string
  readonly status: ReturnType<typeof statusView>
  /** "Sáb 21/11" */
  readonly dateLabel: string
  /** "06:12 – 09:40" */
  readonly timeLabel: string
  /** "T5" */
  readonly fieldCode: string
  /** "Talhão 5" */
  readonly fieldName: string
  /** "Catuaí 144 · Área Centro" */
  readonly fieldMeta: string | null
  /** "Inseticida" */
  readonly productLabel: string
  /** "0,3 L/ha · calda 180 L/ha" */
  readonly doseLabel: string
  /** "56,3 ha" */
  readonly areaLabel: string
  readonly conditions: ConditionsCellView
  /** "Carlos" */
  readonly operatorName: string
  /** "CM" */
  readonly operatorInitials: string
  /** "Carlos Mendes", for the avatar tooltip and screen readers. */
  readonly operatorFullName: string
}

export function toApplicationRowView(application: ApplicationSummary): ApplicationRowView {
  return {
    id: application.id,
    status: statusView(application.status),
    dateLabel: formatWeekdayDate(application.window.date),
    timeLabel: formatTimeRange(application.window.startTime, application.window.endTime),
    fieldCode: application.field.label,
    fieldName: application.field.name,
    fieldMeta: [application.field.variety, application.field.block].filter(Boolean).join(' · ') || null,
    productLabel: application.product.name,
    doseLabel: `${formatDose(application.product.dose)} · ${formatMixRate(application.product.mixRateLPerHa)}`,
    areaLabel: formatTableArea(application.areaHa),
    conditions: buildConditionsCell(application),
    operatorName: firstName(application.operator.name),
    operatorInitials: initials(application.operator.name),
    operatorFullName: application.operator.name,
  }
}

/** "1–6 de 6" */
export function applicationRangeLabel(page: number, pageSize: number, total: number): string {
  if (total === 0) return '0 de 0'
  const first = (page - 1) * pageSize + 1
  return `${first}–${Math.min(page * pageSize, total)} de ${total}`
}

/** Active filters, for the "limpar filtros" state (period alone does not count). */
export function hasActiveApplicationFilters(filters: ApplicationFilters): boolean {
  return Boolean(filters.fieldId || filters.productKind || filters.operatorId || filters.status
    || (filters.periodDays !== undefined && filters.periodDays !== DEFAULT_APPLICATION_PERIOD_DAYS))
}

export function productKindOptions(): { value: ProductKind | null, label: string }[] {
  return [{ value: null, label: 'Todos' }, ...PRODUCT_KINDS.map(value => ({ value, label: PRODUCT_KIND_LABELS[value] }))]
}

// ---- top indicators ------------------------------------------------------------------------------

export interface ApplicationSummaryCardView {
  readonly key: 'month' | 'area' | 'window' | 'next'
  /** "No mês" (the Figma writes it in caps through the style) */
  readonly label: string
  /** "12"; "—" when the API has nothing to show */
  readonly value: string
  readonly caption: string | null
  /** Only the "Dentro da janela" card is the dark highlight of the Figma. */
  readonly tone: 'default' | 'highlight'
}

const NO_VALUE = '—'

function pluralize(count: number, singular: string, plural: string): string {
  return `${count} ${count === 1 ? singular : plural}`
}

/** "9 concluídas · 2 planejadas · 1 cancelada": zero counts are left out. */
function monthCaption(count: ApplicationMonthSummary['monthCount']): string | null {
  const parts = [
    count.completed > 0 ? pluralize(count.completed, 'concluída', 'concluídas') : null,
    count.inProgress > 0 ? pluralize(count.inProgress, 'em andamento', 'em andamento') : null,
    count.planned > 0 ? pluralize(count.planned, 'planejada', 'planejadas') : null,
    count.cancelled > 0 ? pluralize(count.cancelled, 'cancelada', 'canceladas') : null,
  ].filter((part): part is string => part !== null)
  return parts.length > 0 ? parts.join(' · ') : null
}

/** The 4 cards of the top (screen 19). `null` (loading or error) shows dashes; nothing is derived here. */
export function buildApplicationSummaryCards(summary: ApplicationMonthSummary | null): ApplicationSummaryCardView[] {
  const { monthCount, appliedArea, withinWindow, next } = summary ?? {}
  return [
    {
      key: 'month',
      label: 'No mês',
      value: monthCount ? String(monthCount.total) : NO_VALUE,
      caption: monthCount ? monthCaption(monthCount) : null,
      tone: 'default',
    },
    {
      key: 'area',
      label: 'Área aplicada',
      value: appliedArea ? formatHectares(appliedArea.areaHa, 0) : NO_VALUE,
      caption: appliedArea && appliedArea.percent !== null ? `${appliedArea.percent}% da área produtiva` : null,
      tone: 'default',
    },
    {
      key: 'window',
      label: 'Dentro da janela',
      value: withinWindow && withinWindow.percent !== null ? `${withinWindow.percent}%` : NO_VALUE,
      caption: withinWindow && withinWindow.total > 0
        ? `${withinWindow.within} de ${withinWindow.total} com Delta T e vento ideais`
        : null,
      tone: 'highlight',
    },
    {
      key: 'next',
      label: 'Próxima',
      value: next ? formatWeekdayDate(next.date) : NO_VALUE,
      caption: next ? `${next.fieldLabel} · ${formatTimeRange(next.startTime, next.endTime)} · ${next.productName.toLowerCase()}` : null,
      tone: 'default',
    },
  ]
}

/** Status chips of the table header ("Todas", "Planejadas", "Concluídas", "Canceladas"). */
export const APPLICATION_STATUS_CHIPS: readonly { value: ApplicationStatus | null, label: string }[] = [
  { value: null, label: 'Todas' },
  { value: 'planned', label: 'Planejadas' },
  { value: 'completed', label: 'Concluídas' },
  { value: 'cancelled', label: 'Canceladas' },
]

// ---- detail ----------------------------------------------------------------------------------------

export interface DetailRowView {
  readonly label: string
  readonly value: string
}

export interface ApplicationDetailView {
  readonly status: ReturnType<typeof statusView>
  /** "Registrada em 21/11 às 09:52" */
  readonly registeredLabel: string
  /** "Inseticida · Talhão 5" */
  readonly title: string
  /** "Sáb 21/11 · 06:12 – 09:40 · 56,3 ha · Catuaí 144" */
  readonly subtitle: string
  /** "Condições registradas na aplicação"; null when nothing was recorded. */
  readonly conditions: {
    readonly headline: string
    readonly stats: readonly { readonly key: 'delta_t' | 'wind' | 'rain' | 'duration', readonly label: string, readonly value: string }[]
  } | null
  /** "Produto e calda" */
  readonly productRows: readonly DetailRowView[]
  /** "Equipe e equipamento" */
  readonly teamRows: readonly DetailRowView[]
  /** "Registro assinado digitalmente · válido para o caderno de campo"; null when not signed. */
  readonly signatureLabel: string | null
  /** Why it was cancelled, when it was. */
  readonly cancellationLabel: string | null
}

export const SIGNATURE_LABEL = 'Registro assinado digitalmente · válido para o caderno de campo'

function buildProductRows(detail: ApplicationDetail): DetailRowView[] {
  const { product, usage, preHarvest } = detail
  const rows: DetailRowView[] = [
    { label: 'Produto', value: usage.lot ? `${product.name} · lote ${usage.lot}` : product.name },
    { label: 'Dose', value: `${formatDose(product.dose)} · ${formatLiters(usage.doseTotalL)} no total` },
    {
      label: 'Calda',
      value: `${formatDecimal(product.mixRateLPerHa, 0)} L/ha · ${formatLiters(usage.mixTotalL)} · ${usage.tankCount} ${usage.tankCount === 1 ? 'tanque' : 'tanques'}`,
    },
  ]
  if (preHarvest) {
    const days = `${preHarvest.days} ${preHarvest.days === 1 ? 'dia' : 'dias'}`
    rows.push({
      label: 'Carência',
      value: preHarvest.releasedFrom ? `${days} · colheita liberada a partir de ${formatDayMonth(preHarvest.releasedFrom)}` : days,
    })
  }
  return rows
}

function buildTeamRows(detail: ApplicationDetail): DetailRowView[] {
  const rows: DetailRowView[] = [
    { label: 'Operador', value: detail.operator.name },
    { label: 'Pulverizador', value: detail.equipment.name },
  ]
  if (detail.confirmedBy) {
    rows.push({ label: 'Confirmado por', value: `${detail.confirmedBy.person.name} · ${formatDateTimeLabel(detail.confirmedBy.at)}` })
  }
  return rows
}

export function buildApplicationDetailView(detail: ApplicationDetail): ApplicationDetailView {
  const recorded = detail.recordedConditions
  return {
    status: statusView(detail.status),
    registeredLabel: `Registrada em ${formatDateTimeLabel(detail.registeredAt)}`,
    title: `${detail.product.name} · ${detail.field.name}`,
    subtitle: [
      formatWeekdayDate(detail.window.date),
      formatTimeRange(detail.window.startTime, detail.window.endTime),
      formatTableArea(detail.areaHa),
      detail.field.variety,
    ].filter(Boolean).join(' · '),
    conditions: recorded
      ? {
          headline: recorded.headline,
          stats: [
            { key: 'delta_t', label: 'Delta T', value: `${formatDecimal(recorded.deltaTC)} °C` },
            { key: 'wind', label: 'Vento', value: `${formatDecimal(recorded.windKmh, 0)} km/h` },
            { key: 'rain', label: `Chuva ${recorded.rainHours}h`, value: `${formatDecimal(recorded.rainMm)} mm` },
            { key: 'duration', label: 'Duração', value: formatDurationMinutes(recorded.durationMinutes) },
          ],
        }
      : null,
    productRows: buildProductRows(detail),
    teamRows: buildTeamRows(detail),
    signatureLabel: detail.signed ? SIGNATURE_LABEL : null,
    cancellationLabel: detail.cancellation?.reason ?? null,
  }
}
