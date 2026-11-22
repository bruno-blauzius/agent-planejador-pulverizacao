import { toValue, type MaybeRefOrGetter } from 'vue'
import type { Farm } from '~/types/farm'
import { useResource } from './internal/resource'
import { useApiServices } from './useApiServices'

/** Farm detail (header, overview, wizard in edit mode). */
export function useFarm(farmId: MaybeRefOrGetter<string>) {
  const { farms } = useApiServices()
  const resource = useResource<Farm | null>(
    () => `farms:detail:${toValue(farmId)}`,
    () => farms.get(toValue(farmId)),
    () => null,
  )
  return { farm: resource.data, pending: resource.pending, error: resource.error, refresh: resource.refresh }
}
