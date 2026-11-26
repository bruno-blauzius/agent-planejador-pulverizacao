import { computed, toValue, type ComputedRef, type MaybeRefOrGetter, type Ref } from 'vue'
import type { ApiError } from '~/services/errors'
import type { TerrainService } from '~/services/ports'
import type { FieldTerrain, FieldTerrainAdjustmentInput } from '~/types/terrain'
import { useAction, useResource } from './internal/resource'
import { useLatestValue } from './internal/latest'
import { useApiServices } from './useApiServices'

export interface FieldTerrainResource {
  /** Null without a field id or before the load. */
  readonly terrain: Readonly<Ref<FieldTerrain | null>>
  readonly pending: ComputedRef<boolean>
  readonly error: ComputedRef<ApiError | null>
  readonly refresh: () => Promise<void>
  /** Resolves with the field relief after the change, or null on failure (see `adjustError`; 422 carries `details`). */
  readonly adjust: (input: FieldTerrainAdjustmentInput) => Promise<FieldTerrain | null>
  readonly adjusting: Readonly<Ref<boolean>>
  readonly adjustError: Readonly<Ref<ApiError | null>>
  readonly removeAdjustment: () => Promise<FieldTerrain | null>
  readonly removing: Readonly<Ref<boolean>>
  readonly removeError: Readonly<Ref<ApiError | null>>
}

export interface FieldTerrainOptions {
  /** Called after a change so the farm list and map reload. */
  readonly onChanged?: () => Promise<void>
  /** Test seam; defaults to the app's `terrain` service. */
  readonly service?: TerrainService
}

/** Relief detail of one field (popover, card 04c and adjustment modal). Without a field id nothing is called. */
export function useFieldTerrain(
  farmId: MaybeRefOrGetter<string>,
  fieldId: MaybeRefOrGetter<string | null>,
  options: FieldTerrainOptions = {},
): FieldTerrainResource {
  const service = options.service ?? useApiServices().terrain

  const resource = useResource<FieldTerrain | null>(
    () => `terrain:field:${toValue(farmId)}:${toValue(fieldId) ?? '-'}`,
    () => {
      const id = toValue(fieldId)
      return id ? service.getField(toValue(farmId), id) : Promise.resolve(null)
    },
    () => null,
  )
  const latest = useLatestValue(resource.data, () => `${toValue(farmId)}:${toValue(fieldId)}`)

  async function applyChange(change: (farm: string, field: string) => Promise<FieldTerrain>): Promise<FieldTerrain> {
    const id = toValue(fieldId)
    const farm = toValue(farmId)
    if (!id) throw new Error('no field selected')
    const next = await change(farm, id)
    // The user moved to another field/farm while the request ran: the answer is not theirs anymore.
    if (toValue(farmId) !== farm || toValue(fieldId) !== id) return next
    latest.set(next)
    await options.onChanged?.()
    return next
  }

  const adjustment = useAction((input: FieldTerrainAdjustmentInput) => applyChange((farm, field) => service.adjust(farm, field, input)))
  const removal = useAction(() => applyChange((farm, field) => service.removeAdjustment(farm, field)))

  return {
    terrain: latest.value,
    pending: computed(() => resource.pending.value && latest.value.value === null),
    error: resource.error,
    refresh: () => latest.trackLoad(resource.refresh),
    adjust: adjustment.execute,
    adjusting: adjustment.pending,
    adjustError: adjustment.error,
    removeAdjustment: removal.execute,
    removing: removal.pending,
    removeError: removal.error,
  }
}
