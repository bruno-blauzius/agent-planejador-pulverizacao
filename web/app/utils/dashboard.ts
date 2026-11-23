/**
 * Presentation rules of the administrator dashboard (15). Every number comes
 * from `GET /dashboard` (windows and alerts from the spray-planner); this file
 * only formats them. Dates use one fixed display time zone so the server and
 * the browser render the same text (no hydration mismatch).
 */
import type { SprayCondition } from '~/types/spray'
import type {
  ActivityEntry,
  AgentAlert,
  AgentAlertKind,
  Dashboard,
  DashboardDay,
  UpcomingWindow,
} from '~/types/dashboard'
import { formatHectares, initials } from './format'
import { SPRAY_CONDITION_LABELS } from './labels'
import { DISPLAY_TIME_ZONE } from './timeZone'

export const DASHBOARD_PERIOD_DAYS = 14

const PLACEHOLDER = '—'
const DAY_MS = 86_400_000
const integer = new Intl.NumberFormat('pt-BR')

export interface DashboardStatCardView {
  readonly key: 'windows' | 'applications' | 'area' | 'pending'
  readonly label: string
  readonly value: string
  readonly caption: string | null
  readonly highlight: boolean
}

export interface DashboardChartDayView {
  readonly date: string
  readonly label: string
  readonly showLabel: boolean
  readonly hoursRatio: number
  readonly applicationsRatio: number
  readonly ariaLabel: string
}

export interface DashboardChartView {
  readonly title: string
  readonly legend: readonly { readonly key: 'hours' | 'applications', readonly label: string }[]
  readonly days: readonly DashboardChartDayView[]
  readonly summary: string | null
  readonly isEmpty: boolean
}

export interface UpcomingWindowView {
  readonly key: string
  readonly fieldLabel: string
  readonly whenLabel: string
  readonly detail: string | null
  readonly condition: { readonly value: SprayCondition, readonly label: string }
  readonly isHighlighted: boolean
}

export interface AgentAlertView {
  readonly id: string
  readonly title: string
  readonly description: string
  readonly tone: AgentAlert['tone']
  readonly icon: 'drop' | 'rain' | 'info'
}

export interface ActivityView {
  readonly id: string
  readonly initials: string
  readonly isAgent: boolean
  readonly actorName: string
  readonly description: string
  readonly timeLabel: string
}

/** "2026-11-21" of `date` in the display time zone. */
export function isoDateInZone(date: Date, timeZone = DISPLAY_TIME_ZONE): string {
  return new Intl.DateTimeFormat('en-CA', { timeZone, year: 'numeric', month: '2-digit', day: '2-digit' }).format(date)
}

function timeInZone(date: Date, timeZone = DISPLAY_TIME_ZONE): string {
  return new Intl.DateTimeFormat('pt-BR', { timeZone, hour: '2-digit', minute: '2-digit', hour12: false }).format(date)
}

/** Whole days between two ISO dates (b − a). */
function daysBetween(a: string, b: string): number {
  return Math.round((Date.parse(`${b}T00:00:00Z`) - Date.parse(`${a}T00:00:00Z`)) / DAY_MS)
}

function capitalize(text: string): string {
  return text.charAt(0).toUpperCase() + text.slice(1)
}

/** "Sábado, 21 de novembro · Fazenda Santa Rita". */
export function dashboardDateLine(now: Date, context: string | null): string {
  const date = new Intl.DateTimeFormat('pt-BR', {
    timeZone: DISPLAY_TIME_ZONE, weekday: 'long', day: 'numeric', month: 'long',
  }).format(now)
  return [capitalize(date), context].filter(Boolean).join(' · ')
}

export function buildDashboardStatCards(dashboard: Dashboard | null): DashboardStatCardView[] {
  const indicators = dashboard?.indicators
  const windows = indicators?.suitableWindowsToday
  const area = indicators?.sprayedArea
  const windowsCaption = windows && windows.count > 0
    ? [...windows.fieldLabels, windows.nextStartAt ? `próxima às ${timeInZone(windows.nextStartAt)}` : null]
        .filter(Boolean).join(' · ')
    : null
  const areaPercent = area && area.productiveAreaHa > 0 ? Math.round((area.areaHa / area.productiveAreaHa) * 100) : null
  return [
    {
      key: 'windows',
      label: 'Janelas aptas hoje',
      value: windows ? integer.format(windows.count) : PLACEHOLDER,
      caption: windows ? windowsCaption ?? 'Nenhuma janela apta hoje' : null,
      highlight: true,
    },
    {
      key: 'applications',
      label: 'Aplicações no mês',
      value: indicators ? integer.format(indicators.applicationsThisMonth.count) : PLACEHOLDER,
      caption: indicators
        ? `${integer.format(indicators.applicationsThisMonth.confirmedThisWeek)} ${indicators.applicationsThisMonth.confirmedThisWeek === 1 ? 'confirmada' : 'confirmadas'} esta semana`
        : null,
      highlight: false,
    },
    {
      key: 'area',
      label: 'Área pulverizada',
      value: area ? formatHectares(area.areaHa, 0) : PLACEHOLDER,
      caption: area
        ? [`de ${formatHectares(area.productiveAreaHa, 0)} produtivos`, areaPercent !== null ? `${areaPercent}%` : null].filter(Boolean).join(' · ')
        : null,
      highlight: false,
    },
    {
      key: 'pending',
      label: 'Pendências',
      value: indicators ? integer.format(indicators.pendingItems.count) : PLACEHOLDER,
      caption: indicators ? indicators.pendingItems.headline ?? (indicators.pendingItems.count === 0 ? 'Nada pendente' : null) : null,
      highlight: false,
    },
  ]
}

function dayLabel(isoDate: string): string {
  const [, month, day] = isoDate.split('-')
  return `${Number(day)}/${Number(month)}`
}

/** Each series has its own scale (hours vs. count); the aria label carries the real values. */
export function buildDashboardChart(days: readonly DashboardDay[], summary: string | null): DashboardChartView {
  const maxHours = Math.max(0, ...days.map(day => day.suitableHours))
  const maxApplications = Math.max(0, ...days.map(day => day.applications))
  const middle = Math.floor((days.length - 1) / 2)
  return {
    // The period is fixed (14 days): the title must not depend on what has loaded.
    title: `Janelas aptas × aplicações · últimos ${DASHBOARD_PERIOD_DAYS} dias`,
    legend: [{ key: 'hours', label: 'Horas aptas' }, { key: 'applications', label: 'Aplicações' }],
    days: days.map((day, index) => ({
      date: day.date,
      label: dayLabel(day.date),
      showLabel: index === 0 || index === middle || index === days.length - 1,
      hoursRatio: maxHours > 0 ? day.suitableHours / maxHours : 0,
      applicationsRatio: maxApplications > 0 ? day.applications / maxApplications : 0,
      ariaLabel: `${dayLabel(day.date)}: ${integer.format(day.suitableHours)} h aptas, ${day.applications === 1 ? '1 aplicação' : `${integer.format(day.applications)} aplicações`}`,
    })),
    summary,
    // Only loaded days can be "empty"; no days means loading or failure, not zero activity.
    isEmpty: days.length > 0 && maxHours === 0 && maxApplications === 0,
  }
}

const weekdayShort = new Intl.DateTimeFormat('pt-BR', { timeZone: 'UTC', weekday: 'short' })

/** "Hoje", "Amanhã" or "Qui" for an ISO date relative to `today` (ISO). */
function relativeDay(isoDate: string, today: string): string {
  const diff = daysBetween(today, isoDate)
  if (diff === 0) return 'Hoje'
  if (diff === 1) return 'Amanhã'
  return capitalize(weekdayShort.format(new Date(`${isoDate}T12:00:00Z`)).replace('.', ''))
}

export function toUpcomingWindowView(window: UpcomingWindow, today: string): UpcomingWindowView {
  const hours = window.startTime && window.endTime ? `${window.startTime} – ${window.endTime}` : PLACEHOLDER
  const detail = [window.cropVariety, window.operation].filter(Boolean).join(' · ')
  return {
    key: `${window.fieldId}:${window.date}`,
    fieldLabel: window.fieldLabel,
    whenLabel: `${relativeDay(window.date, today)} · ${hours}`,
    detail: detail || null,
    condition: { value: window.condition, label: SPRAY_CONDITION_LABELS[window.condition] },
    isHighlighted: window.condition === 'suitable',
  }
}

const ALERT_ICONS: Readonly<Record<AgentAlertKind, AgentAlertView['icon']>> = {
  pre_harvest_interval: 'drop',
  rain: 'rain',
  forecast_update: 'info',
  other: 'info',
}

export function toAgentAlertView(alert: AgentAlert): AgentAlertView {
  return { id: alert.id, title: alert.title, description: alert.description, tone: alert.tone, icon: ALERT_ICONS[alert.kind] }
}

/** "hoje, 10:12" | "ontem, 17:40" | "18/11, 09:00". */
export function activityTimeLabel(occurredAt: Date, now: Date): string {
  const day = isoDateInZone(occurredAt)
  const diff = daysBetween(day, isoDateInZone(now))
  const prefix = diff === 0 ? 'hoje' : diff === 1 ? 'ontem' : dayLabel(day).split('/').map(part => part.padStart(2, '0')).join('/')
  return `${prefix}, ${timeInZone(occurredAt)}`
}

export function toActivityView(entry: ActivityEntry, now: Date): ActivityView {
  const isAgent = entry.actor.kind === 'agent'
  return {
    id: entry.id,
    initials: isAgent ? 'IA' : initials(entry.actor.name),
    isAgent,
    actorName: entry.actor.name,
    description: entry.description,
    timeLabel: activityTimeLabel(entry.occurredAt, now),
  }
}
