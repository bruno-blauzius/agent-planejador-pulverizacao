import { computed } from 'vue'
import type { OwnerFarmLinkInput, OwnerInput } from '~/types/owner'
import { useAction } from './internal/resource'
import { useApiServices } from './useApiServices'

/**
 * Writes of the owner modal ("Dados" then "Vínculos"). Each call resolves with
 * the result or `null`; the failure (409 duplicate document, 422 with field
 * details, share above 100%) lands in `error`.
 */
export function useOwnerActions() {
  const { owners } = useApiServices()

  const create = useAction((input: OwnerInput) => owners.create(input))
  const update = useAction((ownerId: string, input: OwnerInput) => owners.update(ownerId, input))
  const links = useAction((ownerId: string, input: readonly OwnerFarmLinkInput[]) => owners.replaceLinks(ownerId, input))
  const actions = [create, update, links]

  return {
    createOwner: create.execute,
    updateOwner: update.execute,
    replaceLinks: links.execute,
    saving: computed(() => actions.some(action => action.pending.value)),
    error: computed(() => actions.find(action => action.error.value)?.error.value ?? null),
  }
}
