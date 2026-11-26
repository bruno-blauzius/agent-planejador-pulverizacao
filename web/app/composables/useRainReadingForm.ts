import { computed, reactive, ref, toValue, watch, type ComputedRef, type MaybeRefOrGetter, type Ref } from 'vue'
import type { RainGaugeService } from '~/services/ports'
import type { RainGauge, RainReading } from '~/types/trafficability'
import {
  emptyRainReadingValues,
  normalizeTimeInput,
  RAIN_READING_MAX_MM,
  RAIN_READING_NOTICE,
  rainGaugeOptions,
  rainReadingErrorsFromApi,
  todayInFarmZone,
  toRainReadingInput,
  validateRainReading,
  type FormErrors,
  type RainReadingFormField,
  type RainReadingFormValues,
  type SelectOption,
} from '~/utils/fieldConditionsForm'
import { useFormSubmission } from './internal/formSubmission'
import { useApiServices } from './useApiServices'

export interface RainReadingForm {
  /** Reactive; bind with v-model. `date` is `YYYY-MM-DD` and `time` `HH:MM` (or `H:MM`), local time of the farm. */
  readonly values: RainReadingFormValues
  readonly gaugeOptions: ComputedRef<readonly SelectOption<string>[]>
  /** Local errors (after trying to save) and the 422 of the API (`rain_mm`, `observed_at` → `time`). */
  readonly errors: ComputedRef<FormErrors<RainReadingFormField>>
  /** Failure no field explains (403, 404, 409, network). */
  readonly formError: ComputedRef<string | null>
  readonly saving: Readonly<Ref<boolean>>
  readonly notice: string
  readonly rainMmMax: number
  /** Today in São Paulo (`YYYY-MM-DD`), as of the last `reset`: `max` of the date input. */
  readonly maxDate: ComputedRef<string>
  /**
   * On open: now, and the field's gauge (else the only gauge of the farm). While
   * the user has not picked a gauge, it follows the default as gauges and
   * profile arrive.
   */
  readonly reset: () => void
  /** Picks a gauge as if the user had (e.g. the one just registered in the manager). */
  readonly selectGauge: (gaugeId: string) => void
  /** True = saved; `onSaved` (reload of the trafficability) already ran. */
  readonly submit: () => Promise<boolean>
}

export interface RainReadingFormOptions {
  /** Gauges of the farm (`useRainGauges`). */
  readonly gauges: MaybeRefOrGetter<readonly RainGauge[]>
  /** Gauge preselected (the one linked to the field). */
  readonly defaultGaugeId?: MaybeRefOrGetter<string | null>
  readonly onSaved?: (reading: RainReading) => Promise<void> | void
  readonly now?: () => Date
  /** Test seam; defaults to the app's `rainGauges` service. */
  readonly service?: RainGaugeService
}

/** "Lançar leitura" (`field_conditions.record`): rain since the previous reading of a gauge of the farm. */
export function useRainReadingForm(farmId: MaybeRefOrGetter<string>, options: RainReadingFormOptions): RainReadingForm {
  const service = options.service ?? useApiServices().rainGauges
  const now = options.now ?? (() => new Date())
  const values = reactive<RainReadingFormValues>(emptyRainReadingValues(now(), null))
  const openedAt = ref(now())
  /** The user picked a gauge: the default no longer overrides it. */
  const gaugeTouched = ref(false)
  let applyingDefault = false

  function defaultGauge(): string | null {
    const gauges = toValue(options.gauges)
    const preferred = toValue(options.defaultGaugeId)
    if (preferred && gauges.some(gauge => gauge.id === preferred)) return preferred
    return gauges.length === 1 ? gauges[0]!.id : null
  }

  function applyDefaultGauge(): void {
    applyingDefault = true
    values.gaugeId = defaultGauge()
    applyingDefault = false
  }

  watch(() => values.gaugeId, () => {
    if (!applyingDefault) gaugeTouched.value = true
  }, { flush: 'sync' })

  // Gauges and the field profile load after the form may already be open.
  watch(defaultGauge, () => {
    if (!gaugeTouched.value) applyDefaultGauge()
  })

  const submission = useFormSubmission<RainReadingFormField, NonNullable<ReturnType<typeof toRainReadingInput>>, RainReading>({
    values,
    validate: () => validateRainReading(values, now()),
    toInput: () => toRainReadingInput(values, now()),
    send: ({ gaugeId, input }) => service.recordReading(toValue(farmId), gaugeId, input),
    errorsFromApi: rainReadingErrorsFromApi,
    onSaved: options.onSaved,
  })

  return {
    values,
    gaugeOptions: computed(() => rainGaugeOptions(toValue(options.gauges))),
    errors: submission.errors,
    formError: submission.formError,
    saving: submission.saving,
    notice: RAIN_READING_NOTICE,
    rainMmMax: RAIN_READING_MAX_MM,
    maxDate: computed(() => todayInFarmZone(openedAt.value)),
    reset: () => {
      openedAt.value = now()
      Object.assign(values, emptyRainReadingValues(openedAt.value, null))
      applyDefaultGauge()
      gaugeTouched.value = false
      submission.clear()
    },
    selectGauge: (gaugeId) => {
      values.gaugeId = gaugeId
    },
    submit: async () => {
      values.time = normalizeTimeInput(values.time)
      return (await submission.submit()) !== null
    },
  }
}
