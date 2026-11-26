import { computed, toValue, type ComputedRef, type MaybeRefOrGetter } from 'vue'
import type { ApiError } from '~/services/errors'
import type { TrafficabilityService } from '~/services/ports'
import { DEFAULT_TRAFFICABILITY_DAYS, type FieldTrafficability } from '~/types/trafficability'
import { useResource } from './internal/resource'
import { useApiServices } from './useApiServices'

export interface FieldTrafficabilityResource {
  /** Null without a field id or before the first load. Always the field asked for, never the previous one. */
  readonly trafficability: ComputedRef<FieldTrafficability | null>
  /** Loading with nothing to show yet (skeleton). */
  readonly pending: ComputedRef<boolean>
  /** Reloading while the previous answer stays on screen (after `refresh`). */
  readonly refreshing: ComputedRef<boolean>
  readonly error: ComputedRef<ApiError | null>
  /** Loads again, e.g. after a rain reading or a soil inspection is recorded (phase 2). */
  readonly refresh: () => Promise<void>
  /** Projection days asked for (option `days` or the contract default); not derived from the answer, which lacks them when the forecast is down. */
  readonly days: ComputedRef<number>
}

export interface FieldTrafficabilityOptions {
  /** Projection days after today (1–7); omitted, the API default (5). */
  readonly days?: MaybeRefOrGetter<number | undefined>
  /** Test seam; defaults to the app's `trafficability` service. */
  readonly service?: TrafficabilityService
}

/**
 * Rain, soil state and method verdicts of one field (ADR 0022, view "Chuva e
 * solo" of the Terreno tab). Presentation only: every state comes from the API.
 * Without a field id nothing is called.
 */
export function useFieldTrafficability(
  farmId: MaybeRefOrGetter<string>,
  fieldId: MaybeRefOrGetter<string | null>,
  options: FieldTrafficabilityOptions = {},
): FieldTrafficabilityResource {
  const service = options.service ?? useApiServices().trafficability
  const days = () => toValue(options.days)

  const resource = useResource<FieldTrafficability | null>(
    () => `trafficability:field:${toValue(farmId)}:${toValue(fieldId) ?? '-'}:${days() ?? 'default'}`,
    () => {
      const id = toValue(fieldId)
      return id ? service.getFieldTrafficability(toValue(farmId), id, days()) : Promise.resolve(null)
    },
    () => null,
  )

  // While another field loads, the cached answer of the previous one must not show up as this field's.
  const trafficability = computed(() => {
    const data = resource.data.value
    return data && data.fieldId === toValue(fieldId) && data.farmId === toValue(farmId) ? data : null
  })

  return {
    trafficability,
    pending: computed(() => resource.pending.value && trafficability.value === null),
    refreshing: computed(() => resource.pending.value && trafficability.value !== null),
    error: resource.error,
    refresh: resource.refresh,
    days: computed(() => days() ?? DEFAULT_TRAFFICABILITY_DAYS),
  }
}
