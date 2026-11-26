import { computed, toValue, type ComputedRef, type MaybeRefOrGetter, type Ref } from 'vue'
import type { ApiError } from '~/services/errors'
import type { RainGaugeService } from '~/services/ports'
import type { RainGauge, RainGaugeInput } from '~/types/trafficability'
import { rainGaugeErrorsFromApi, submissionErrorMessage, type FormErrors, type RainGaugeFormField } from '~/utils/fieldConditionsForm'
import { useAction, useResource } from './internal/resource'
import { useApiServices } from './useApiServices'
import { usePermissions } from './usePermissions'

export interface RainGaugesResource {
  /** By name; empty before the load or while disabled. */
  readonly gauges: ComputedRef<readonly RainGauge[]>
  readonly pending: ComputedRef<boolean>
  readonly error: ComputedRef<ApiError | null>
  readonly refresh: () => Promise<void>
  /** `registry.write`: register and rename. Presentation only; the API answers 403. */
  readonly canWrite: ComputedRef<boolean>
  /** Resolves with the new gauge (list already reloaded) or null (see `createErrors`/`createFormError`). */
  readonly create: (input: RainGaugeInput) => Promise<RainGauge | null>
  readonly creating: Readonly<Ref<boolean>>
  readonly createError: Readonly<Ref<ApiError | null>>
  /** 422 of the API by field (`name`, `location`). */
  readonly createErrors: ComputedRef<FormErrors<RainGaugeFormField>>
  /** Failure no field explains: 403 and 409 in plain words, else the API message. */
  readonly createFormError: ComputedRef<string | null>
  /** Call on opening the "new gauge" form and when its text changes. */
  readonly clearCreateError: () => void
  /**
   * Resolves with the renamed gauge or null (see `renameErrors`/`renameFormError`).
   * The list is reloaded and `onRenamed` runs (soil profile and trafficability show the name).
   */
  readonly rename: (gaugeId: string, name: string) => Promise<RainGauge | null>
  readonly renaming: Readonly<Ref<boolean>>
  readonly renameError: Readonly<Ref<ApiError | null>>
  readonly renameErrors: ComputedRef<FormErrors<RainGaugeFormField>>
  readonly renameFormError: ComputedRef<string | null>
  /** Call on starting a rename and when its text changes. */
  readonly clearRenameError: () => void
}

export interface RainGaugesOptions {
  /** False keeps it from calling the API (e.g. while "Chuva e solo" was never opened). Default true. */
  readonly enabled?: MaybeRefOrGetter<boolean>
  /** After a rename: whatever shows the gauge name elsewhere reloads. */
  readonly onRenamed?: (gauge: RainGauge) => Promise<void> | void
  /** Test seam; defaults to the app's `rainGauges` service. */
  readonly service?: RainGaugeService
}

function errorViews(source: Readonly<Ref<ApiError | null>>) {
  const errors = computed(() => (source.value ? rainGaugeErrorsFromApi(source.value.details) : {}))
  const formError = computed(() => {
    const error = source.value
    return error ? submissionErrorMessage(error, Object.keys(errors.value).length > 0) : null
  })
  return { errors, formError }
}

/** Rain gauges of a farm (ADR 0022, phase 2), with register and rename. */
export function useRainGauges(farmId: MaybeRefOrGetter<string>, options: RainGaugesOptions = {}): RainGaugesResource {
  const service = options.service ?? useApiServices().rainGauges
  const permissions = usePermissions()
  const enabled = () => toValue(options.enabled ?? true)

  const resource = useResource<readonly RainGauge[]>(
    () => (enabled() ? `rain-gauges:${toValue(farmId)}` : 'rain-gauges:-'),
    () => (enabled() ? service.list(toValue(farmId)) : Promise.resolve([])),
    () => [],
  )

  const create = useAction(async (input: RainGaugeInput) => {
    const gauge = await service.create(toValue(farmId), input)
    await resource.refresh()
    return gauge
  })
  const rename = useAction(async (gaugeId: string, name: string) => {
    const gauge = await service.update(toValue(farmId), gaugeId, { name })
    await resource.refresh()
    await options.onRenamed?.(gauge)
    return gauge
  })
  const createViews = errorViews(create.error)
  const renameViews = errorViews(rename.error)

  return {
    gauges: computed(() => resource.data.value),
    pending: resource.pending,
    error: resource.error,
    refresh: resource.refresh,
    canWrite: computed(() => permissions.can('registry.write')),
    create: create.execute,
    creating: create.pending,
    createError: create.error,
    createErrors: createViews.errors,
    createFormError: createViews.formError,
    clearCreateError: create.clearError,
    rename: rename.execute,
    renaming: rename.pending,
    renameError: rename.error,
    renameErrors: renameViews.errors,
    renameFormError: renameViews.formError,
    clearRenameError: rename.clearError,
  }
}
