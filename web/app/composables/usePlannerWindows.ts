import { computed, ref } from 'vue'
import type { DayConditions, SprayWindowMap } from '~/types/planner'
import { buildDayConditionsView, buildWindowMapView } from '~/utils/planner'
import { stableKey, useResource } from './internal/resource'
import { useApiServices } from './useApiServices'
import { usePlannerContext } from './usePlannerContext'

export interface UsePlannerWindowsOptions {
  /** Defaults to the shared selection (`usePlannerContext()`). */
  context?: ReturnType<typeof usePlannerContext>
}

const FORECAST_DAYS = 5

/**
 * Window map of the next 5 days and the hourly conditions of the chosen day
 * (screen 16). Everything is computed by the API; the front only picks the day
 * and, optionally, a window to summarize.
 */
export function usePlannerWindows(options: UsePlannerWindowsOptions = {}) {
  const { planner } = useApiServices()
  const context = options.context ?? usePlannerContext()

  const mapResource = useResource<SprayWindowMap | null>(
    () => stableKey('planner:window-map', { farm: context.farmId.value, fields: context.fieldIds.value.join(','), days: FORECAST_DAYS }),
    () => {
      const farmId = context.farmId.value
      if (!farmId) return Promise.resolve(null)
      const fieldIds = context.fieldIds.value
      return planner.getWindowMap({ farmId, fieldIds: fieldIds.length ? fieldIds : undefined, days: FORECAST_DAYS })
    },
    () => null,
  )

  const pickedDate = ref<string | null>(null)
  const pickedWindow = ref<{ startTime: string, endTime: string } | null>(null)
  const mapView = computed(() => buildWindowMapView(mapResource.data.value))
  /** The picked day, or the first day of the map. */
  const selectedDate = computed(() => pickedDate.value ?? mapView.value.days[0]?.date ?? null)

  const conditionsResource = useResource<DayConditions | null>(
    () => stableKey('planner:conditions', {
      farm: context.farmId.value,
      date: selectedDate.value,
      from: pickedWindow.value?.startTime,
      to: pickedWindow.value?.endTime,
    }),
    () => {
      const farmId = context.farmId.value
      const date = selectedDate.value
      if (!farmId || !date) return Promise.resolve(null)
      return planner.getDayConditions({
        farmId,
        date,
        windowStart: pickedWindow.value?.startTime,
        windowEnd: pickedWindow.value?.endTime,
      })
    },
    () => null,
  )

  const errorCodes = computed(() => [mapResource.error.value?.code, conditionsResource.error.value?.code])

  return {
    map: mapView,
    conditions: computed(() => buildDayConditionsView(conditionsResource.data.value)),
    /** Grouped suitable windows (start/end/duration), for lists and shortcuts. */
    windows: computed(() => mapResource.data.value?.windows ?? []),

    selectedDate,
    /** Picks a day of the map; drops any picked window so the API summarizes that day's best one. */
    selectDay(date: string) {
      pickedDate.value = date
      pickedWindow.value = null
    },
    /** Summarizes a specific window of the selected day (e.g. one from the plan). */
    selectWindow(date: string, startTime: string, endTime: string) {
      pickedDate.value = date
      pickedWindow.value = { startTime, endTime }
    },

    pending: computed(() => mapResource.pending.value),
    conditionsPending: computed(() => conditionsResource.pending.value),
    error: computed(() => mapResource.error.value ?? conditionsResource.error.value),
    isForbidden: computed(() => errorCodes.value.some(code => code === 'forbidden' || code === 'out_of_scope')),
    isWeatherUnavailable: computed(() => errorCodes.value.includes('weather_unavailable')),
    /** Loaded without error but nothing to draw (no farm picked or no fields). */
    isEmpty: computed(() => !mapResource.pending.value && !mapResource.error.value && mapView.value.isEmpty),
    refresh: async () => {
      await Promise.all([mapResource.refresh(), conditionsResource.refresh()])
    },
  }
}
