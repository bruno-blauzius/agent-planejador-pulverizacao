import { computed, toValue, type MaybeRefOrGetter } from 'vue'
import type { CurrentWeather } from '~/types/application'
import { formatWeatherLabel } from '~/utils/applications'
import { useResource } from './internal/resource'
import { useApiServices } from './useApiServices'

/**
 * Weather chip of the page header ("Guaxupé · 24 °C · vento 7 km/h"). Without
 * `farmId` the API uses the user's primary farm. A forecast outage is a state of
 * the chip (`isUnavailable`), never made-up numbers, and does not block the page.
 */
export function useCurrentWeather(farmId?: MaybeRefOrGetter<string | null | undefined>) {
  const { weather: weatherService } = useApiServices()
  const resource = useResource<CurrentWeather | null>(
    () => `weather:current:${toValue(farmId) ?? 'primary'}`,
    () => weatherService.getCurrent(toValue(farmId) ?? undefined),
    () => null,
  )
  const weather = computed(() => resource.data.value)

  return {
    weather,
    /** "Guaxupé · 24 °C · vento 7 km/h"; null while loading or on error. */
    label: computed(() => (weather.value ? formatWeatherLabel(weather.value) : null)),
    pending: resource.pending,
    error: resource.error,
    isUnavailable: computed(() => resource.error.value !== null),
    refresh: resource.refresh,
  }
}
