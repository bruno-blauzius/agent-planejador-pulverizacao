/**
 * Administrator dashboard (screen 15, ADR 0013). Every number and sentence is
 * computed by the backend (windows and alerts by the spray-planner).
 */
import type { SprayCondition } from './spray'

export interface DashboardIndicators {
  readonly suitableWindowsToday: { readonly count: number, readonly fieldLabels: readonly string[], readonly nextStartAt: Date | null }
  readonly applicationsThisMonth: { readonly count: number, readonly confirmedThisWeek: number }
  readonly sprayedArea: { readonly areaHa: number, readonly productiveAreaHa: number }
  readonly pendingItems: { readonly count: number, readonly headline: string | null }
}

export interface DashboardDay {
  /** ISO date `YYYY-MM-DD`. */
  readonly date: string
  readonly suitableHours: number
  readonly applications: number
}

export interface UpcomingWindow {
  readonly fieldId: string
  /** "T3" */
  readonly fieldLabel: string
  readonly farmId: string
  /** ISO date `YYYY-MM-DD`. */
  readonly date: string
  /** Farm local time "HH:mm"; null when there is no window that day. */
  readonly startTime: string | null
  readonly endTime: string | null
  readonly cropVariety: string | null
  readonly operation: string | null
  readonly condition: SprayCondition
}

export type AgentAlertKind = 'pre_harvest_interval' | 'rain' | 'forecast_update' | 'other'
export type AgentAlertTone = 'attention' | 'highlight' | 'info'

export interface AgentAlert {
  readonly id: string
  readonly kind: AgentAlertKind
  readonly tone: AgentAlertTone
  readonly title: string
  readonly description: string
}

export interface ActivityEntry {
  readonly id: string
  readonly actor: { readonly kind: 'user' | 'agent', readonly name: string }
  /** Text after the actor name, e.g. "registrou aplicação no T3 · 38,6 ha". */
  readonly description: string
  readonly occurredAt: Date
}

export interface Dashboard {
  readonly generatedAt: Date
  /** Filtered farm, or null for every farm in scope. */
  readonly farmId: string | null
  /** Farms considered ("Todas (N)"). */
  readonly farmCount: number
  readonly indicators: DashboardIndicators
  readonly windowsVsApplications: { readonly days: readonly DashboardDay[], readonly summary: string | null }
  readonly upcomingWindows: readonly UpcomingWindow[]
  readonly agentAlerts: readonly AgentAlert[]
  readonly recentActivity: readonly ActivityEntry[]
}

export interface DashboardFilters {
  readonly farmId?: string
  readonly periodDays?: number
}
