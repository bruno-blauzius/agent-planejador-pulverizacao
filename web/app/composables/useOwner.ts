import { computed, toValue, type MaybeRefOrGetter } from 'vue'
import type { Owner, OwnerFarmLink } from '~/types/owner'
import { useResource } from './internal/resource'
import { useApiServices } from './useApiServices'

/** Owner detail and farm links (owner modal in edit mode). */
export function useOwner(ownerId: MaybeRefOrGetter<string>) {
  const { owners } = useApiServices()
  const ownerResource = useResource<Owner | null>(
    () => `owners:detail:${toValue(ownerId)}`,
    () => owners.get(toValue(ownerId)),
    () => null,
  )
  const linksResource = useResource<readonly OwnerFarmLink[]>(
    () => `owners:links:${toValue(ownerId)}`,
    () => owners.getLinks(toValue(ownerId)),
    () => [],
  )

  return {
    owner: ownerResource.data,
    links: linksResource.data,
    pending: computed(() => ownerResource.pending.value || linksResource.pending.value),
    error: computed(() => ownerResource.error.value ?? linksResource.error.value),
    refresh: async () => {
      await Promise.all([ownerResource.refresh(), linksResource.refresh()])
    },
  }
}
