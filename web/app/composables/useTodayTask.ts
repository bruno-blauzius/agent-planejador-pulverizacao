import { computed } from 'vue'
import type { TodayBrief } from '~/types/planner'
import {
  buildTodayHourViews,
  buildTodayMetricViews,
  buildTodayStatusView,
  buildTodayTaskView,
  todayLocationLabel,
} from '~/utils/planner'
import { useAction, useResource } from './internal/resource'
import { useApiServices } from './useApiServices'
import { useGreeting } from './useGreeting'
import { usePermissions } from './usePermissions'

export interface UseTodayTaskOptions {
  now?: () => Date
}

/**
 * Operator "Hoje" screen (18): status now, Delta T/wind/rain tiles, hour line,
 * today's task and "Iniciar aplicação". The task is an application already
 * registered after human confirmation; starting it only changes its status.
 * Scope is enforced by the API (operators see their farms only).
 */
export function useTodayTask(options: UseTodayTaskOptions = {}) {
  const { today } = useApiServices()
  const { can } = usePermissions()
  const { greeting } = useGreeting({ now: options.now })

  const resource = useResource<TodayBrief | null>(
    () => 'planner:today',
    () => today.getBrief(),
    () => null,
  )
  const brief = computed(() => resource.data.value)

  const startAction = useAction(async (taskId: string) => {
    const task = await today.startTask(taskId)
    await resource.refresh()
    return task
  })

  const task = computed(() => (brief.value ? buildTodayTaskView(brief.value) : null))
  const loadCode = computed(() => resource.error.value?.code)
  const startCode = computed(() => startAction.error.value?.code)

  return {
    greeting,
    locationLabel: computed(() => (brief.value ? todayLocationLabel(brief.value) : '')),
    status: computed(() => (brief.value ? buildTodayStatusView(brief.value) : null)),
    metrics: computed(() => (brief.value ? buildTodayMetricViews(brief.value) : [])),
    hours: computed(() => (brief.value ? buildTodayHourViews(brief.value) : [])),
    task,

    pending: resource.pending,
    error: resource.error,
    isForbidden: computed(() => loadCode.value === 'forbidden' || loadCode.value === 'out_of_scope'),
    isWeatherUnavailable: computed(() => loadCode.value === 'weather_unavailable'),
    /** Loaded fine, but there is no application planned for today. */
    isEmpty: computed(() => brief.value !== null && !resource.error.value && task.value === null),
    refresh: resource.refresh,

    canStart: computed(() => can('applications.register') && task.value !== null && !task.value.isStarted),
    starting: startAction.pending,
    startError: startAction.error,
    /** Window closed or already started elsewhere (409). */
    startConflict: computed(() => startCode.value === 'conflict'),
    startForbidden: computed(() => startCode.value === 'forbidden' || startCode.value === 'out_of_scope'),
    /** Resolves `true` when the task is now in progress (see `startError` otherwise; a 409 also reloads the brief). */
    async start(): Promise<boolean> {
      const current = task.value
      if (!current || current.isStarted) return false
      const started = (await startAction.execute(current.id)) !== null
      // 409: someone else started it or the window closed. Reload so the screen shows the truth.
      if (!started && startAction.error.value?.code === 'conflict') await resource.refresh()
      return started
    },
  }
}
