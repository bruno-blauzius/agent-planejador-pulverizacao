import { computed, toValue, type MaybeRefOrGetter } from 'vue'
import type { FarmFilters, FarmSummary } from '~/types/farm'
import type { Page } from '~/types/pagination'
import { summarizeFarms } from '~/utils/farm'
import { stableKey, useResource } from './internal/resource'
import { useApiServices } from './useApiServices'

const emptyPage = (): Page<FarmSummary> => ({ items: [], total: 0, page: 1, pageSize: 0 })

/** Farm list ("Suas fazendas"). Refetches whenever `filters` changes. */
export function useFarms(filters: MaybeRefOrGetter<FarmFilters> = {}) {
  const { farms } = useApiServices()
  const resource = useResource(
    () => stableKey('farms:list', filters),
    () => farms.list(toValue(filters)),
    emptyPage,
  )
  const items = computed(() => resource.data.value.items)

  return {
    farms: items,
    total: computed(() => resource.data.value.total),
    /** Presentation totals of the loaded page (cards and chips). */
    summary: computed(() => summarizeFarms(items.value)),
    pending: resource.pending,
    error: resource.error,
    isEmpty: computed(() => !resource.pending.value && !resource.error.value && items.value.length === 0),
    refresh: resource.refresh,
  }
}
