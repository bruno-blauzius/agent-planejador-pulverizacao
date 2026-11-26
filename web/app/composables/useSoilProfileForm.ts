import { computed, reactive, ref, toValue, watch, type ComputedRef, type MaybeRefOrGetter, type Ref } from 'vue'
import type { ApiError } from '~/services/errors'
import type { SoilConditionsService } from '~/services/ports'
import type { RainGauge, SoilProfile, SoilProfileInput, SoilTexture } from '~/types/trafficability'
import {
  SOIL_PROFILE_NOTICE,
  SOIL_TEXTURE_OPTIONS,
  soilProfileErrorsFromApi,
  soilProfileGaugeOptions,
  toSoilProfileInput,
  validateSoilProfile,
  type FormErrors,
  type SelectOption,
  type SoilProfileFormField,
  type SoilProfileFormValues,
} from '~/utils/fieldConditionsForm'
import { useFormSubmission } from './internal/formSubmission'
import { useResource } from './internal/resource'
import { useApiServices } from './useApiServices'

/** Shown when "Salvar" is tried before the profile arrived (the UI should keep it disabled until `ready`). */
export const SOIL_PROFILE_NOT_READY_MESSAGE = 'Aguarde o perfil de solo do talhão carregar.'

export interface SoilProfileForm {
  /** Profile of the field; null without a field, while disabled or before the load. */
  readonly profile: ComputedRef<SoilProfile | null>
  /** The profile arrived: the values reflect it and "Salvar" can be enabled. */
  readonly ready: ComputedRef<boolean>
  readonly pending: ComputedRef<boolean>
  readonly error: ComputedRef<ApiError | null>
  readonly refresh: () => Promise<void>
  /** Reactive; bind with v-model. Follows the loaded profile until the user edits it. */
  readonly values: SoilProfileFormValues
  /** Arenoso, Médio, Argiloso, Não informada (null). */
  readonly textureOptions: readonly SelectOption<SoilTexture | null>[]
  /** Gauges of the farm (and the profile's, even before the list loads), then "Nenhum (usa o modelo de previsão)" (null). */
  readonly gaugeOptions: ComputedRef<readonly SelectOption<string | null>[]>
  /** The gauge list is loading / failed (for the select's own state). */
  readonly gaugesPending: ComputedRef<boolean>
  readonly gaugesError: ComputedRef<ApiError | null>
  readonly errors: ComputedRef<FormErrors<SoilProfileFormField>>
  readonly formError: ComputedRef<string | null>
  readonly saving: Readonly<Ref<boolean>>
  readonly notice: string
  /**
   * Opening by the button: values back to the loaded profile (or to follow it
   * when it arrives), errors cleared. Coming back from the gauge manager must
   * NOT call it, so the edits are kept.
   */
  readonly open: () => void
  /** Same as `open` (kept for callers of the first version). */
  readonly reset: () => void
  /** Picks a gauge as if the user had (e.g. the one just registered in the manager). */
  readonly selectGauge: (gaugeId: string | null) => void
  /** True = saved; the profile is reloaded and `onSaved` (reload of the trafficability) already ran. */
  readonly submit: () => Promise<boolean>
}

export interface SoilProfileFormOptions {
  /** Gauges of the farm (`useRainGauges`). */
  readonly gauges: MaybeRefOrGetter<readonly RainGauge[]>
  /** State of that list; the gauge is only checked against it once it loaded. */
  readonly gaugesPending?: MaybeRefOrGetter<boolean>
  readonly gaugesError?: MaybeRefOrGetter<ApiError | null>
  /** False keeps it from calling the API. Default true. */
  readonly enabled?: MaybeRefOrGetter<boolean>
  readonly onSaved?: (profile: SoilProfile) => Promise<void> | void
  /** Test seam; defaults to the app's `soilConditions` service. */
  readonly service?: SoilConditionsService
}

/** Soil texture and rain gauge of a field (reading: anyone in scope; saving: `registry.write`). */
export function useSoilProfileForm(
  farmId: MaybeRefOrGetter<string>,
  fieldId: MaybeRefOrGetter<string | null>,
  options: SoilProfileFormOptions,
): SoilProfileForm {
  const service = options.service ?? useApiServices().soilConditions
  const activeFieldId = () => (toValue(options.enabled ?? true) ? toValue(fieldId) : null)

  const resource = useResource<SoilProfile | null>(
    () => `soil-profile:${toValue(farmId)}:${activeFieldId() ?? '-'}`,
    () => {
      const id = activeFieldId()
      return id ? service.getProfile(toValue(farmId), id) : Promise.resolve(null)
    },
    () => null,
  )
  // The cached answer of another field must not be shown as this one's.
  const profile = computed(() => {
    const data = resource.data.value
    return data && data.fieldId === activeFieldId() ? data : null
  })
  const ready = computed(() => profile.value !== null)

  const values = reactive<SoilProfileFormValues>({ soilTexture: null, rainGaugeId: null })
  /** The user changed a value: the loaded profile no longer overwrites the form. */
  const dirty = ref(false)
  let applying = false

  function applyProfile(): void {
    applying = true
    Object.assign(values, { soilTexture: profile.value?.soilTexture ?? null, rainGaugeId: profile.value?.rainGauge?.id ?? null })
    applying = false
  }

  watch(values, () => {
    if (!applying) dirty.value = true
  }, { flush: 'sync' })

  // Edits belong to the field they were made on.
  watch(activeFieldId, () => {
    dirty.value = false
  }, { flush: 'sync' })

  // The modal may open before the GET answers: an untouched form takes the profile when it arrives.
  watch(profile, () => {
    if (!dirty.value) applyProfile()
  }, { immediate: true })

  const gaugesPending = computed(() => Boolean(toValue(options.gaugesPending ?? false)))
  const gaugesError = computed(() => toValue(options.gaugesError ?? null))
  const validationContext = () => ({
    gaugesLoaded: !gaugesPending.value && gaugesError.value === null,
    profileGaugeId: profile.value?.rainGauge?.id ?? null,
  })
  const validate = () => validateSoilProfile(values, toValue(options.gauges), validationContext())

  const submission = useFormSubmission<SoilProfileFormField, SoilProfileInput, SoilProfile>({
    values,
    validate,
    toInput: () => (Object.keys(validate()).length === 0 ? toSoilProfileInput(values) : null),
    send: input => service.updateProfile(toValue(farmId), toValue(fieldId) ?? '', input),
    errorsFromApi: soilProfileErrorsFromApi,
    // Saving before the profile arrived would replace it with empty values.
    unavailable: () => (toValue(fieldId) && ready.value ? null : SOIL_PROFILE_NOT_READY_MESSAGE),
    onSaved: async (saved) => {
      // The saved profile is now the reference again.
      dirty.value = false
      await resource.refresh()
      await options.onSaved?.(saved)
    },
  })

  function open(): void {
    dirty.value = false
    applyProfile()
    submission.clear()
  }

  return {
    profile,
    ready,
    pending: computed(() => resource.pending.value && profile.value === null),
    error: resource.error,
    refresh: resource.refresh,
    values,
    textureOptions: SOIL_TEXTURE_OPTIONS,
    gaugeOptions: computed(() => soilProfileGaugeOptions(toValue(options.gauges), profile.value?.rainGauge ?? null)),
    gaugesPending,
    gaugesError,
    errors: submission.errors,
    formError: submission.formError,
    saving: submission.saving,
    notice: SOIL_PROFILE_NOTICE,
    open,
    reset: open,
    selectGauge: (gaugeId) => {
      values.rainGaugeId = gaugeId
    },
    submit: async () => (await submission.submit()) !== null,
  }
}
