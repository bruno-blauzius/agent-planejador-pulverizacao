import { computed } from 'vue'
import type { Dashboard } from '~/types/dashboard'
import {
  buildDashboardChart,
  buildDashboardStatCards,
  DASHBOARD_PERIOD_DAYS,
  dashboardDateLine,
  isoDateInZone,
  toActivityView,
  toAgentAlertView,
  toUpcomingWindowView,
} from '~/utils/dashboard'
import { stableKey, useResource } from './internal/resource'
import { useApiServices } from './useApiServices'
import { useFarms } from './useFarms'
import { useGreeting } from './useGreeting'
import { usePermissions } from './usePermissions'

export interface UseDashboardPageOptions {
  now?: () => Date
}

const PLANNER_PATH = '/planner'
const FARM_OPTIONS_PAGE_SIZE = 100

function firstValue(value: unknown): unknown {
  return Array.isArray(value) ? value[0] : value
}

/**
 * Administrator dashboard (15) at `/`: farm selector (`?farm=`), indicators,
 * windows × applications, upcoming windows, agent alerts and recent activity.
 * The route requires `dashboard.view`; 403s still map to honest states.
 */
export function useDashboardPage(options: UseDashboardPageOptions = {}) {
  const { dashboard: dashboardService } = useApiServices()
  const { can, company } = usePermissions()
  const route = useRoute()
  const now = options.now ?? (() => new Date())
  const { greeting } = useGreeting({ now })

  const farmQuery = computed(() => {
    const value = firstValue(route.query.farm)
    return typeof value === 'string' && value ? value : null
  })

  const farms = useFarms({ pageSize: FARM_OPTIONS_PAGE_SIZE })
  const filters = computed(() => ({ farmId: farmQuery.value ?? undefined, periodDays: DASHBOARD_PERIOD_DAYS }))
  const resource = useResource<Dashboard | null>(
    () => stableKey('dashboard', filters),
    () => dashboardService.get(filters.value),
    () => null,
  )
  const dashboard = computed(() => resource.data.value)

  /** Dismissed alerts (no endpoint yet): kept for this browser session only. */
  const dismissed = useState<string[]>('dashboard:dismissed-alerts', () => [])

  const selectedFarmId = computed<string | null>({
    get: () => farmQuery.value,
    set: (farmId) => {
      void navigateTo({ query: { ...route.query, farm: farmId ?? undefined } }, { replace: true })
    },
  })

  const farmOptions = computed(() => [
    { value: null, label: `Todas (${farms.total.value})` },
    ...farms.farms.value.map(farm => ({ value: farm.id, label: farm.name })),
  ])

  const selectedFarmName = computed(() =>
    farms.farms.value.find(farm => farm.id === farmQuery.value)?.name ?? null)

  const reference = computed(() => dashboard.value?.generatedAt ?? now())
  const today = computed(() => isoDateInZone(reference.value))

  return {
    greeting,
    dateLine: computed(() => dashboardDateLine(reference.value, selectedFarmName.value ?? company.value?.name ?? null)),

    farmOptions,
    selectedFarmId,
    canOpenPlanner: computed(() => can('planner.use')),
    plannerPath: PLANNER_PATH,

    pending: resource.pending,
    error: resource.error,
    isForbidden: computed(() => resource.error.value?.code === 'forbidden'),
    isOutOfScope: computed(() => ['out_of_scope', 'not_found'].includes(resource.error.value?.code ?? '')),
    refresh: resource.refresh,

    statCards: computed(() => buildDashboardStatCards(dashboard.value)),
    chart: computed(() => buildDashboardChart(
      dashboard.value?.windowsVsApplications.days ?? [],
      dashboard.value?.windowsVsApplications.summary ?? null,
    )),
    upcomingWindows: computed(() =>
      (dashboard.value?.upcomingWindows ?? []).map(window => toUpcomingWindowView(window, today.value))),
    alerts: computed(() => (dashboard.value?.agentAlerts ?? [])
      .filter(alert => !dismissed.value.includes(alert.id))
      .map(toAgentAlertView)),
    dismissAlert(id: string) {
      if (!dismissed.value.includes(id)) dismissed.value = [...dismissed.value, id]
    },
    activity: computed(() =>
      (dashboard.value?.recentActivity ?? []).map(entry => toActivityView(entry, reference.value))),
  }
}
