import { computed, reactive, ref, toValue, watch, type ComputedRef, type MaybeRefOrGetter, type Ref } from 'vue'
import { forbiddenMessageFor } from '~/utils/permissions'
import type { TerrainAdjustmentOrigin, TerrainPosition } from '~/types/terrain'
import {
  emptyTerrainAdjustmentValues,
  TERRAIN_ADJUSTMENT_INFO_NOTICE,
  TERRAIN_ADJUSTMENT_JUSTIFICATION_MAX,
  TERRAIN_ADJUSTMENT_ORIGIN_OPTIONS,
  TERRAIN_POSITION_OPTIONS,
  terrainAdjustmentErrorsFromApi,
  toTerrainAdjustmentHints,
  toTerrainAdjustmentInput,
  validateTerrainAdjustment,
  type TerrainAdjustmentFormErrors,
  type TerrainAdjustmentFormValues,
  type TerrainAdjustmentHintsView,
} from '~/utils/terrain'
import type { FieldTerrainResource } from './useFieldTerrain'

export interface TerrainAdjustmentForm {
  /** "Ajustar relevo · Talhão 4". */
  readonly title: ComputedRef<string>
  /** Reactive; bind with v-model. */
  readonly values: TerrainAdjustmentFormValues
  /** Local errors (after trying to save) and the 422 of the API, by field. */
  readonly errors: ComputedRef<TerrainAdjustmentFormErrors>
  /** Failure no field explains (403, network, 404). */
  readonly formError: ComputedRef<string | null>
  readonly hints: ComputedRef<TerrainAdjustmentHintsView>
  readonly originOptions: readonly { readonly value: TerrainAdjustmentOrigin, readonly label: string }[]
  readonly positionOptions: readonly { readonly value: TerrainPosition, readonly label: string }[]
  readonly justificationMaxLength: typeof TERRAIN_ADJUSTMENT_JUSTIFICATION_MAX
  readonly infoNotice: string
  readonly saving: Readonly<Ref<boolean>>
  /** On open: current adjustment, else the calculated value. */
  readonly reset: () => void
  /** True = saved (the page closes the modal). */
  readonly submit: () => Promise<boolean>
}

/**
 * Adjustment modal (04d). It never computes the class: the API does, and the
 * form shows the current one with the "recalculated on save" notice. The field
 * resource comes in as a parameter so tests can hand a fake.
 */
export function useTerrainAdjustmentForm(options: {
  readonly fieldName: MaybeRefOrGetter<string>
  readonly field: Pick<FieldTerrainResource, 'terrain' | 'adjust' | 'adjusting' | 'adjustError'>
}): TerrainAdjustmentForm {
  const { field } = options
  const values = reactive<TerrainAdjustmentFormValues>(emptyTerrainAdjustmentValues(null))
  const attempted = ref(false)
  // An API error describes the values that were sent; once they change it no longer applies.
  const apiErrorStale = ref(false)

  // Sync: a value typed right before saving must not mark the answer to that save as stale.
  watch(values, () => {
    apiErrorStale.value = true
  }, { flush: 'sync' })

  // A new answer of the API is always about what was just sent.
  watch(field.adjustError, () => {
    apiErrorStale.value = false
  })

  const apiErrors = computed(() => (field.adjustError.value && !apiErrorStale.value
    ? terrainAdjustmentErrorsFromApi(field.adjustError.value.details)
    : {}))
  const errors = computed<TerrainAdjustmentFormErrors>(() => ({
    ...apiErrors.value,
    ...(attempted.value ? validateTerrainAdjustment(values) : {}),
  }))

  const formError = computed(() => {
    const error = field.adjustError.value
    if (!error || apiErrorStale.value) return null
    const forbidden = forbiddenMessageFor(error)
    if (forbidden) return forbidden
    return Object.keys(apiErrors.value).length > 0 ? null : error.message
  })

  function reset(): void {
    Object.assign(values, emptyTerrainAdjustmentValues(field.terrain.value))
    attempted.value = false
    apiErrorStale.value = true
  }

  async function submit(): Promise<boolean> {
    attempted.value = true
    apiErrorStale.value = false
    const input = toTerrainAdjustmentInput(values)
    if (!input) return false
    return (await field.adjust(input)) !== null
  }

  return {
    title: computed(() => `Ajustar relevo · ${toValue(options.fieldName)}`),
    values,
    errors,
    formError,
    hints: computed(() => toTerrainAdjustmentHints(field.terrain.value)),
    originOptions: TERRAIN_ADJUSTMENT_ORIGIN_OPTIONS,
    positionOptions: TERRAIN_POSITION_OPTIONS,
    justificationMaxLength: TERRAIN_ADJUSTMENT_JUSTIFICATION_MAX,
    infoNotice: TERRAIN_ADJUSTMENT_INFO_NOTICE,
    saving: field.adjusting,
    reset,
    submit,
  }
}
