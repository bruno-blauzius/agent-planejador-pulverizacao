import { computed, toValue, type MaybeRefOrGetter } from 'vue'
import type { UserAccount, UserFilters, UsersSummary } from '~/types/admin'
import type { Page } from '~/types/pagination'
import { stableKey, useResource } from './internal/resource'
import { useApiServices } from './useApiServices'

const emptyPage = (): Page<UserAccount> => ({ items: [], total: 0, page: 1, pageSize: 0 })

/** Users and pending invitations the signed-in user manages (`users.manage`). */
export function useUsers(filters: MaybeRefOrGetter<UserFilters> = {}) {
  const { users } = useApiServices()
  const resource = useResource(
    () => stableKey('users:list', filters),
    () => users.list(toValue(filters)),
    emptyPage,
  )
  const items = computed(() => resource.data.value.items)

  return {
    users: items,
    total: computed(() => resource.data.value.total),
    pending: resource.pending,
    error: resource.error,
    refresh: resource.refresh,
  }
}

/** Counts by role for the role cards and the page eyebrow. */
export function useUsersSummary() {
  const { users } = useApiServices()
  const resource = useResource<UsersSummary | null>('users:summary', () => users.summary(), () => null)
  return { summary: resource.data, pending: resource.pending, error: resource.error, refresh: resource.refresh }
}
