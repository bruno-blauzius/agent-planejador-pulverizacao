import { computed, toValue, type MaybeRefOrGetter } from 'vue'
import type { Field, FieldInput } from '~/types/field'
import { groupFieldsByBlock } from '~/utils/field'
import { useAction, useResource } from './internal/resource'
import { useApiServices } from './useApiServices'

/** Fields of a farm ("Áreas e talhões") plus the "Novo talhão" and "Editar" actions. */
export function useFarmFields(farmId: MaybeRefOrGetter<string>) {
  const { fields: fieldService } = useApiServices()
  const resource = useResource<readonly Field[]>(
    () => `farms:fields:${toValue(farmId)}`,
    () => fieldService.listByFarm(toValue(farmId)),
    () => [],
  )

  const create = useAction(async (input: FieldInput) => {
    const field = await fieldService.create(toValue(farmId), input)
    await resource.refresh()
    return field
  })

  const update = useAction(async (fieldId: string, input: FieldInput) => {
    const field = await fieldService.update(toValue(farmId), fieldId, input)
    await resource.refresh()
    return field
  })

  return {
    fields: resource.data,
    blocks: computed(() => groupFieldsByBlock(resource.data.value)),
    pending: resource.pending,
    error: resource.error,
    isEmpty: computed(() => !resource.pending.value && !resource.error.value && resource.data.value.length === 0),
    refresh: resource.refresh,
    /** Resolves with the created field (list already refreshed) or `null` (see `createError`). */
    createField: create.execute,
    creating: create.pending,
    createError: create.error,
    /** Resolves with the updated field (list already refreshed) or `null` (see `updateError`). */
    updateField: update.execute,
    updating: update.pending,
    updateError: update.error,
  }
}
