import { computed, toValue, type MaybeRefOrGetter } from 'vue'
import type { OwnerFilters, OwnerSummary } from '~/types/owner'
import type { Page } from '~/types/pagination'
import { stableKey, useResource } from './internal/resource'
import { useApiServices } from './useApiServices'

const emptyPage = (): Page<OwnerSummary> => ({ items: [], total: 0, page: 1, pageSize: 0 })

/**
 * Owner list and search (table "Proprietários" and the owner picker of the
 * wizard). Debounce the search text in the caller before passing it here.
 */
export function useOwners(filters: MaybeRefOrGetter<OwnerFilters> = {}) {
  const { owners } = useApiServices()
  const resource = useResource(
    () => stableKey('owners:list', filters),
    () => owners.list(toValue(filters)),
    emptyPage,
  )
  const items = computed(() => resource.data.value.items)

  return {
    owners: items,
    total: computed(() => resource.data.value.total),
    page: computed(() => resource.data.value.page),
    pageSize: computed(() => resource.data.value.pageSize),
    pending: resource.pending,
    error: resource.error,
    isEmpty: computed(() => !resource.pending.value && !resource.error.value && items.value.length === 0),
    refresh: resource.refresh,
  }
}
