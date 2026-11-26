/**
 * Submit cycle shared by the "Chuva e solo" forms (same behaviour as the relief
 * adjustment form): local errors show after the first try, the 422 of the API
 * maps to fields and stops applying once the user edits a value, and a failure
 * no field explains becomes `formError`. Not auto-imported (nested folder).
 */
import { computed, ref, watch, type ComputedRef, type Ref } from 'vue'
import type { ApiError, FieldError } from '~/services/errors'
import { submissionErrorMessage } from '~/utils/fieldConditionsForm'
import { useAction } from './resource'

export interface FormSubmission<F extends string, R> {
  /** Local errors (after trying to save) and the 422 of the API, by field. */
  readonly errors: ComputedRef<Partial<Record<F, string>>>
  /** Failure no field explains (403, 404, 409, network), or why the form cannot be sent now. */
  readonly formError: ComputedRef<string | null>
  readonly saving: Readonly<Ref<boolean>>
  /** Resolves with the saved record, or null (invalid form, unavailable or API failure). */
  readonly submit: () => Promise<R | null>
  /** Forgets errors and the "tried to save" mark (on open). */
  readonly clear: () => void
}

export function useFormSubmission<F extends string, I, R>(options: {
  /** Reactive values of the form; any change makes an API error stale. */
  readonly values: object
  readonly validate: () => Partial<Record<F, string>>
  /** Request input, or null while invalid. */
  readonly toInput: () => I | null
  readonly send: (input: I) => Promise<R>
  readonly errorsFromApi: (details: readonly FieldError[]) => Partial<Record<F, string>>
  /** Why the form cannot be sent at all right now (no field selected…); shown as `formError` after a try. */
  readonly unavailable?: () => string | null
  /** After a successful save (e.g. reload the trafficability). */
  readonly onSaved?: (result: R) => Promise<void> | void
}): FormSubmission<F, R> {
  const attempted = ref(false)
  // An API error describes the values that were sent; once they change it no longer applies.
  const apiErrorStale = ref(false)
  const action = useAction(async (input: I) => {
    const result = await options.send(input)
    await options.onSaved?.(result)
    return result
  })

  // Sync: a value typed right before saving must not mark the answer to that save as stale.
  watch(() => options.values, () => {
    apiErrorStale.value = true
  }, { deep: true, flush: 'sync' })

  const apiError = computed<ApiError | null>(() => (apiErrorStale.value ? null : action.error.value))
  const apiErrors = computed(() => (apiError.value ? options.errorsFromApi(apiError.value.details) : {}))

  return {
    errors: computed(() => ({ ...apiErrors.value, ...(attempted.value ? options.validate() : {}) })),
    formError: computed(() => {
      const reason = attempted.value ? (options.unavailable?.() ?? null) : null
      if (reason) return reason
      const error = apiError.value
      return error ? submissionErrorMessage(error, Object.keys(apiErrors.value).length > 0) : null
    }),
    saving: action.pending,
    submit: async () => {
      attempted.value = true
      apiErrorStale.value = false
      if (options.unavailable?.()) return null
      const input = options.toInput()
      if (input === null) return null
      return action.execute(input)
    },
    clear: () => {
      attempted.value = false
      action.clearError()
    },
  }
}
