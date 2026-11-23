/**
 * Presentation helpers of the spray planner (ADR 0016): formatting and view
 * models only. Suitability, durations, alerts and checks arrive computed from
 * the API; nothing here decides whether a window is apt.
 */
import type { SprayCondition } from '~/types/spray'
import type {
  ApplicationReview,
  DayConditions,
  PlannerReply,
  ProductKind,
  RegisteredApplication,
  ReviewCheck,
  ReviewCheckCode,
  ReviewCheckStatus,
  SprayPlan,
  SprayWindowMap,
  TodayBrief,
} from '~/types/planner'
import { DISPLAY_TIME_ZONE } from './timeZone'
import { formatHectares } from './format'
import { SPRAY_CONDITION_LABELS } from './labels'

const DAY_MS = 86_400_000
const WEEKDAYS_LONG = ['domingo', 'segunda', 'terça', 'quarta', 'quinta', 'sexta', 'sábado'] as const
const WEEKDAYS_SHORT = ['dom', 'seg', 'ter', 'qua', 'qui', 'sex', 'sáb'] as const
const HOUR_LABEL_STEP = 3

const pad2 = (value: number) => String(value).padStart(2, '0')

export function capitalize(text: string): string {
  return text.charAt(0).toUpperCase() + text.slice(1)
}

// ---- dates (ISO `YYYY-MM-DD`, calendar math in UTC so there is no zone drift) ----

export function addDaysIso(isoDate: string, days: number): string {
  return new Date(Date.parse(`${isoDate}T00:00:00Z`) + days * DAY_MS).toISOString().slice(0, 10)
}

function weekdayIndex(isoDate: string): number {
  return new Date(`${isoDate}T00:00:00Z`).getUTCDay()
}

/** "quarta" */
export function weekdayLong(isoDate: string): string {
  return WEEKDAYS_LONG[weekdayIndex(isoDate)]!
}

/** "qua" */
export function weekdayShort(isoDate: string): string {
  return WEEKDAYS_SHORT[weekdayIndex(isoDate)]!
}

/** "2026-11-26" → "26/11" */
export function formatDayMonth(isoDate: string): string {
  const [, month = '', day = ''] = isoDate.split('-')
  return `${day}/${month}`
}

/** "na quarta" / "no sábado" */
export function onWeekday(isoDate: string): string {
  const index = weekdayIndex(isoDate)
  return `${index === 0 || index === 6 ? 'no' : 'na'} ${weekdayLong(isoDate)}`
}

/** Date and hour of an instant in the display zone. */
function zonedParts(date: Date, timeZone = DISPLAY_TIME_ZONE): { isoDate: string, hour: number, minute: number } {
  const parts = new Intl.DateTimeFormat('en-CA', {
    timeZone, year: 'numeric', month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', hourCycle: 'h23',
  }).formatToParts(date)
  const get = (type: string) => parts.find(part => part.type === type)?.value ?? '0'
  return { isoDate: `${get('year')}-${get('month')}-${get('day')}`, hour: Number(get('hour')), minute: Number(get('minute')) }
}

export function zonedIsoDate(date: Date): string {
  return zonedParts(date).isoDate
}

/** "05:40" in the display zone. */
export function formatClock(date: Date): string {
  const { hour, minute } = zonedParts(date)
  return `${pad2(hour)}:${pad2(minute)}`
}

// ---- numbers and labels ------------------------------------------------------

/** 4.2 → "4,2"; 6 → "6". */
export function formatDecimal(value: number, maximumFractionDigits = 1): string {
  return new Intl.NumberFormat('pt-BR', { maximumFractionDigits }).format(value)
}

/** 195 → "3h15"; 240 → "4h"; 45 → "45 min". */
export function formatDurationMinutes(minutes: number): string {
  const total = Math.max(0, Math.round(minutes))
  const hours = Math.floor(total / 60)
  const rest = total % 60
  if (hours === 0) return `${rest} min`
  return rest === 0 ? `${hours}h` : `${hours}h${pad2(rest)}`
}

/** "Fazenda Santa Rita" → "Faz. Santa Rita". */
export function abbreviateFarmName(name: string): string {
  return name.replace(/^Fazenda\s+/i, 'Faz. ')
}

/** Agent text: paragraphs separated by a blank line. */
export function splitParagraphs(text: string): string[] {
  return text.split(/\n\s*\n/).map(part => part.trim()).filter(Boolean)
}

/** "06:00" → 6 */
export function hourOfClock(clock: string): number {
  return Number(clock.split(':')[0])
}

export const SPRAY_CONDITIONS: readonly SprayCondition[] = ['suitable', 'attention', 'unsuitable', 'rain']

/** Legend of the window map, in the Figma order. */
export function conditionLegend(): { value: SprayCondition, label: string }[] {
  return SPRAY_CONDITIONS.map(value => ({ value, label: SPRAY_CONDITION_LABELS[value] }))
}

// ---- chat / plan ----------------------------------------------------------------

export interface PlanEntryView {
  readonly fieldId: string
  readonly fieldLabel: string
  /** "38,6 ha · 3h15" */
  readonly detail: string
  /** "Qua 26 · 06:00 – 10:00" */
  readonly windowLabel: string
  readonly condition: SprayCondition
  readonly alertMessage: string | null
}

export interface PlanView {
  readonly id: string
  readonly headline: string
  readonly entries: readonly PlanEntryView[]
  readonly hasAlerts: boolean
  readonly totalAreaLabel: string
}

export function buildPlanView(plan: SprayPlan): PlanView {
  const entries = plan.entries.map<PlanEntryView>(entry => ({
    fieldId: entry.fieldId,
    fieldLabel: entry.fieldLabel,
    detail: `${formatHectares(entry.areaHa)} · ${formatDurationMinutes(entry.durationMinutes)}`,
    windowLabel: `${capitalize(weekdayShort(entry.window.date))} ${entry.window.date.slice(8)} · ${entry.window.startTime} – ${entry.window.endTime}`,
    condition: entry.window.condition,
    alertMessage: entry.alert?.message ?? null,
  }))
  return {
    id: plan.id,
    headline: plan.headline,
    entries,
    hasAlerts: entries.some(entry => entry.alertMessage !== null),
    totalAreaLabel: formatHectares(plan.totalAreaHa),
  }
}

export interface ChatEntry {
  readonly id: string
  readonly role: 'user' | 'agent'
  readonly paragraphs: readonly string[]
  readonly createdAt: Date
  /** Agent only. */
  readonly consulted: PlannerReply['consulted']
  readonly plan: SprayPlan | null
}

export function toUserEntry(id: string, text: string, createdAt: Date): ChatEntry {
  return { id, role: 'user', paragraphs: [text], createdAt, consulted: [], plan: null }
}

export function toAgentEntry(reply: PlannerReply): ChatEntry {
  return {
    id: reply.message.id,
    role: 'agent',
    paragraphs: splitParagraphs(reply.message.text),
    createdAt: reply.message.createdAt,
    consulted: reply.consulted,
    plan: reply.plan,
  }
}

// ---- window map -----------------------------------------------------------------

export interface WindowMapCellView {
  readonly hour: number
  readonly condition: SprayCondition
  readonly ariaLabel: string
}

export interface WindowMapView {
  /** "dados de hoje 05:40" */
  readonly updatedLabel: string
  readonly legend: ReturnType<typeof conditionLegend>
  readonly days: readonly { readonly date: string, readonly label: string, readonly hourLabels: readonly string[] }[]
  readonly rows: readonly {
    readonly fieldId: string
    readonly fieldLabel: string
    readonly days: readonly { readonly date: string, readonly cells: readonly WindowMapCellView[] }[]
  }[]
  readonly isEmpty: boolean
}

function hourLabelsOf(map: SprayWindowMap): string[] {
  const labels: string[] = []
  for (let hour = map.dayStartHour; hour <= map.dayEndHour; hour += HOUR_LABEL_STEP) labels.push(pad2(hour))
  return labels
}

export function dayHeading(isoDate: string): string {
  return `${capitalize(weekdayShort(isoDate))} ${isoDate.slice(8)}`
}

export function buildWindowMapView(map: SprayWindowMap | null): WindowMapView {
  if (!map) return { updatedLabel: '', legend: conditionLegend(), days: [], rows: [], isEmpty: true }
  const sameDay = zonedIsoDate(map.forecastUpdatedAt) === zonedIsoDate(map.generatedAt)
  const dates = map.fields[0]?.days.map(day => day.date) ?? []
  const hourLabels = hourLabelsOf(map)
  return {
    updatedLabel: sameDay
      ? `dados de hoje ${formatClock(map.forecastUpdatedAt)}`
      : `dados de ${formatDayMonth(zonedIsoDate(map.forecastUpdatedAt))} ${formatClock(map.forecastUpdatedAt)}`,
    legend: conditionLegend(),
    days: dates.map(date => ({ date, label: dayHeading(date), hourLabels })),
    rows: map.fields.map(row => ({
      fieldId: row.fieldId,
      fieldLabel: row.fieldLabel,
      days: row.days.map(day => ({
        date: day.date,
        cells: day.hours.map(hour => ({
          hour: hour.hour,
          condition: hour.condition,
          ariaLabel: `${row.fieldLabel}, ${dayHeading(day.date)}, ${pad2(hour.hour)}h: ${SPRAY_CONDITION_LABELS[hour.condition]}`,
        })),
      })),
    })),
    isEmpty: map.fields.length === 0,
  }
}

// ---- hourly conditions chart ------------------------------------------------------

const DELTA_T_AXIS_STEP = 4
const DELTA_T_AXIS_MIN_MAX = 12
const WIND_AXIS_STEP = 5
const WIND_AXIS_MIN_MAX = 20

export interface ConditionPointView {
  readonly hour: number
  /** "5h"; set only on the ticks drawn on the x axis. */
  readonly tickLabel: string | null
  readonly deltaT: number
  readonly windKmh: number
  readonly gustKmh: number
  readonly condition: SprayCondition
  /** 0–1 heights against the axis maxima. */
  readonly deltaTRatio: number
  readonly windRatio: number
  /** Same wind axis as `windRatio` (the axis is sized by the highest gust), so a gust never overflows. */
  readonly gustRatio: number
}

export interface ConditionStatView {
  readonly key: 'delta_t' | 'wind' | 'rain'
  readonly label: string
  readonly value: string
  readonly caption: string
}

export interface DayConditionsView {
  /** "Quarta 26/11 · condições hora a hora" */
  readonly title: string
  readonly points: readonly ConditionPointView[]
  readonly deltaTAxis: { readonly max: number, readonly ticks: readonly number[] }
  readonly windAxis: { readonly max: number, readonly ticks: readonly number[] }
  /** Ideal bands as 0–1 ratios of each axis, for the shaded area ("Faixa ideal"). */
  readonly idealRange: {
    readonly deltaT: { readonly min: number, readonly max: number, readonly minRatio: number, readonly maxRatio: number }
    readonly windKmh: { readonly min: number, readonly max: number, readonly minRatio: number, readonly maxRatio: number }
  }
  /** Chosen window ("Janela 06–10h"); hours are `[startHour, endHour)`. */
  readonly window: { readonly label: string, readonly startHour: number, readonly endHour: number } | null
  readonly stats: readonly ConditionStatView[]
  readonly isEmpty: boolean
}

/** Position of `value` on an axis `0..max`, kept inside 0–1 so nothing is drawn outside the chart. */
function ratioOf(value: number, max: number): number {
  return max > 0 ? Math.min(1, Math.max(0, value / max)) : 0
}

function axisTicks(max: number, step: number): number[] {
  return Array.from({ length: Math.floor(max / step) + 1 }, (_, index) => index * step)
}

function ceilTo(value: number, step: number, minimum: number): number {
  return Math.max(minimum, Math.ceil(value / step) * step)
}

function rainCaption(nextRainAt: Date | null): string {
  if (!nextRainAt) return 'sem chuva prevista'
  const { isoDate, hour } = zonedParts(nextRainAt)
  return `próx. chuva ${weekdayShort(isoDate)} ${pad2(hour)}h`
}

export function buildDayConditionsView(conditions: DayConditions | null): DayConditionsView {
  const empty: DayConditionsView = {
    title: '',
    points: [],
    deltaTAxis: { max: DELTA_T_AXIS_MIN_MAX, ticks: [0, 4, 8, 12] },
    windAxis: { max: WIND_AXIS_MIN_MAX, ticks: axisTicks(WIND_AXIS_MIN_MAX, WIND_AXIS_STEP) },
    idealRange: {
      deltaT: { min: 0, max: 0, minRatio: 0, maxRatio: 0 },
      windKmh: { min: 0, max: 0, minRatio: 0, maxRatio: 0 },
    },
    window: null,
    stats: [],
    isEmpty: true,
  }
  if (!conditions || conditions.hours.length === 0) return empty

  const { deltaTC, windKmh } = conditions.idealRanges
  // Each axis has its own scale and is sized by its highest value (data or ideal band), so nothing overflows.
  const deltaTMax = ceilTo(Math.max(deltaTC.max, ...conditions.hours.map(hour => hour.deltaTC)), DELTA_T_AXIS_STEP, DELTA_T_AXIS_MIN_MAX)
  const windMax = ceilTo(Math.max(windKmh.max, ...conditions.hours.flatMap(hour => [hour.windKmh, hour.gustKmh])), WIND_AXIS_STEP, WIND_AXIS_MIN_MAX)
  const firstHour = conditions.hours[0]!.hour
  const summary = conditions.windowSummary
  const range = summary ? `${pad2(hourOfClock(summary.startTime))}–${pad2(hourOfClock(summary.endTime))}h` : null

  return {
    title: `${capitalize(weekdayLong(conditions.date))} ${formatDayMonth(conditions.date)} · condições hora a hora`,
    points: conditions.hours.map(hour => ({
      hour: hour.hour,
      tickLabel: (hour.hour - firstHour) % HOUR_LABEL_STEP === 0 ? `${hour.hour}h` : null,
      deltaT: hour.deltaTC,
      windKmh: hour.windKmh,
      gustKmh: hour.gustKmh,
      condition: hour.condition,
      deltaTRatio: ratioOf(hour.deltaTC, deltaTMax),
      windRatio: ratioOf(hour.windKmh, windMax),
      gustRatio: ratioOf(hour.gustKmh, windMax),
    })),
    deltaTAxis: {
      max: deltaTMax,
      ticks: axisTicks(deltaTMax, DELTA_T_AXIS_STEP),
    },
    windAxis: { max: windMax, ticks: axisTicks(windMax, WIND_AXIS_STEP) },
    idealRange: {
      deltaT: { ...deltaTC, minRatio: ratioOf(deltaTC.min, deltaTMax), maxRatio: ratioOf(deltaTC.max, deltaTMax) },
      windKmh: { ...windKmh, minRatio: ratioOf(windKmh.min, windMax), maxRatio: ratioOf(windKmh.max, windMax) },
    },
    window: summary && range
      ? { label: `Janela ${range}`, startHour: hourOfClock(summary.startTime), endHour: hourOfClock(summary.endTime) }
      : null,
    stats: summary
      ? [
          {
            key: 'delta_t',
            label: 'Delta T na janela',
            value: `${formatDecimal(summary.deltaTMinC)} – ${formatDecimal(summary.deltaTMaxC)} °C`,
            caption: `ideal ${formatDecimal(deltaTC.min)} a ${formatDecimal(deltaTC.max)}`,
          },
          {
            key: 'wind',
            label: 'Vento médio',
            value: `${formatDecimal(summary.windAvgKmh, 0)} km/h`,
            caption: `rajada máx. ${formatDecimal(summary.gustMaxKmh, 0)}`,
          },
          {
            key: 'rain',
            label: `Chuva até ${summary.rainUntilHour}h`,
            value: `${formatDecimal(summary.rainMm)} mm`,
            caption: rainCaption(summary.nextRainAt),
          },
        ]
      : [],
    isEmpty: false,
  }
}

// ---- confirmation modal ---------------------------------------------------------------

export interface ReviewCheckView {
  readonly code: ReviewCheckCode
  readonly status: ReviewCheckStatus
  readonly icon: 'check' | 'warning' | 'blocked'
  readonly title: string
  readonly detail: string
}

export interface ReviewView {
  readonly id: string
  /** "T3 · Catuaí 144" */
  readonly fieldTitle: string
  readonly areaLabel: string
  /** "Qua · 26/11" */
  readonly dayLabel: string
  /** "06:00 – 10:00" */
  readonly timeLabel: string
  readonly productName: string
  /** "0,5 L/ha · calda 200 L/ha" */
  readonly doseLabel: string
  /** "7.720 L" */
  readonly mixVolumeLabel: string
  /** "· 4 tanques" */
  readonly tanksLabel: string
  readonly checks: readonly ReviewCheckView[]
  readonly operatorEquipment: string
  readonly canConfirm: boolean
  /** Codes the user acknowledges by confirming (every `warning` check). */
  readonly warningCodes: readonly ReviewCheckCode[]
  readonly blockedTitles: readonly string[]
}

const CHECK_ICONS: Record<ReviewCheckStatus, ReviewCheckView['icon']> = { ok: 'check', warning: 'warning', blocked: 'blocked' }

function toCheckView(check: ReviewCheck): ReviewCheckView {
  return { ...check, icon: CHECK_ICONS[check.status] }
}

export function warningCodesOf(checks: readonly ReviewCheck[]): ReviewCheckCode[] {
  return checks.filter(check => check.status === 'warning').map(check => check.code)
}

export function buildReviewView(review: ApplicationReview): ReviewView {
  const { dose } = review.product
  return {
    id: review.id,
    fieldTitle: [review.fieldLabel, review.variety].filter(Boolean).join(' · '),
    areaLabel: formatHectares(review.areaHa),
    dayLabel: `${capitalize(weekdayShort(review.window.date))} · ${formatDayMonth(review.window.date)}`,
    timeLabel: `${review.window.startTime} – ${review.window.endTime}`,
    productName: review.product.name,
    doseLabel: `${formatDecimal(dose.value)} ${dose.unit} · calda ${formatDecimal(review.product.mixRateLPerHa, 0)} L/ha`,
    mixVolumeLabel: `${formatDecimal(review.mixVolumeL, 0)} L`,
    tanksLabel: `· ${review.tankCount} ${review.tankCount === 1 ? 'tanque' : 'tanques'}`,
    checks: review.checks.map(toCheckView),
    operatorEquipment: `${review.operator.name} · ${review.equipment.name}`,
    canConfirm: review.canConfirm,
    warningCodes: warningCodesOf(review.checks),
    blockedTitles: review.checks.filter(check => check.status === 'blocked').map(check => check.title),
  }
}

export interface RegisteredNotice {
  readonly title: string
  readonly message: string
}

/** Success toast after registering: "T3 · 21/11 06:00 – 10:00. Ela entra como planejada." */
export function buildRegisteredNotice(application: RegisteredApplication): RegisteredNotice {
  const { fieldLabel, window } = application
  return {
    title: 'Aplicação registrada',
    message: `${fieldLabel} · ${formatDayMonth(window.date)} ${window.startTime} – ${window.endTime}. Ela entra como planejada.`,
  }
}

/** Key of one plan entry in the registered set. */
export function entryKey(planId: string, fieldId: string): string {
  return `${planId}:${fieldId}`
}

// ---- operator "Hoje" ---------------------------------------------------------------------

export interface TodayStatusView {
  readonly condition: SprayCondition
  /** "Pode pulverizar" */
  readonly headline: string
  /** "AGORA · 06:48" */
  readonly nowLabel: string
  /** "até 10:00"; null without a window */
  readonly untilLabel: string | null
  /** "· faltam 3h12" */
  readonly remainingLabel: string | null
  /** 0–1 position of "now" inside the window. */
  readonly progressRatio: number
  readonly startLabel: string | null
  readonly endLabel: string | null
}

export interface TodayMetricView {
  readonly kind: 'delta_t' | 'wind' | 'rain'
  readonly label: string
  /** "4,2 °C" */
  readonly valueLabel: string
  readonly condition: SprayCondition
  readonly isOk: boolean
}

export interface TodayHourView {
  readonly hour: number
  /** "06" */
  readonly label: string
  readonly condition: SprayCondition
  readonly isCurrent: boolean
}

export interface TodayTaskView {
  readonly id: string
  readonly fieldLabel: string
  /** "Fungicida · 38,6 ha" */
  readonly title: string
  /** "0,5 L/ha · 4 tanques de 2.000 L" */
  readonly detail: string
  readonly isStarted: boolean
}

function clockMinutes(clock: string): number {
  const [hour = '0', minute = '0'] = clock.split(':')
  return Number(hour) * 60 + Number(minute)
}

export function windowProgress(asOf: Date, windowStart: string | null, windowEnd: string | null): number {
  if (!windowStart || !windowEnd) return 0
  const { hour, minute } = zonedParts(asOf)
  const length = clockMinutes(windowEnd) - clockMinutes(windowStart)
  if (length <= 0) return 0
  return Math.min(1, Math.max(0, (hour * 60 + minute - clockMinutes(windowStart)) / length))
}

export function buildTodayStatusView(brief: TodayBrief): TodayStatusView {
  const { status, asOf } = brief
  return {
    condition: status.condition,
    headline: status.headline,
    nowLabel: `AGORA · ${formatClock(asOf)}`,
    untilLabel: status.untilTime ? `até ${status.untilTime}` : null,
    remainingLabel: status.remainingMinutes === null ? null : `· faltam ${formatDurationMinutes(status.remainingMinutes)}`,
    progressRatio: windowProgress(asOf, status.windowStart, status.windowEnd),
    startLabel: status.windowStart,
    endLabel: status.windowEnd,
  }
}

export function buildTodayMetricViews(brief: TodayBrief): TodayMetricView[] {
  return brief.metrics.map(metric => ({
    kind: metric.kind,
    label: metric.label,
    valueLabel: `${formatDecimal(metric.value)} ${metric.unit}`,
    condition: metric.condition,
    isOk: metric.condition === 'suitable',
  }))
}

export function buildTodayHourViews(brief: TodayBrief): TodayHourView[] {
  return brief.hours.map(hour => ({ hour: hour.hour, label: pad2(hour.hour), condition: hour.condition, isCurrent: hour.isCurrent }))
}

export function buildTodayTaskView(brief: TodayBrief): TodayTaskView | null {
  const task = brief.task
  if (!task) return null
  const tanks = `${task.tankCount} ${task.tankCount === 1 ? 'tanque' : 'tanques'} de ${formatDecimal(task.tankCapacityL, 0)} L`
  return {
    id: task.id,
    fieldLabel: task.fieldLabel,
    title: `${task.operation} · ${formatHectares(task.areaHa)}`,
    detail: `${formatDecimal(task.dose.value)} ${task.dose.unit} · ${tanks}`,
    isStarted: task.status === 'in_progress',
  }
}

/** "Faz. Santa Rita · Talhão 3" */
export function todayLocationLabel(brief: TodayBrief): string {
  return [abbreviateFarmName(brief.farm.name), brief.task?.fieldName].filter(Boolean).join(' · ')
}

/** Labels of the "Produto" selector. */
export const PRODUCT_KIND_LABELS: Readonly<Record<ProductKind, string>> = {
  fungicide: 'Fungicida',
  insecticide: 'Inseticida',
  herbicide: 'Herbicida',
  foliar: 'Foliar',
  other: 'Outro',
}
