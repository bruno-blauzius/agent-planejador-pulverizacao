import { computed, reactive, ref, toValue, type ComputedRef, type MaybeRefOrGetter, type Ref } from 'vue'
import type { SoilConditionsService } from '~/services/ports'
import type { InspectableSoilState, SoilInspection, SoilInspectionInput } from '~/types/trafficability'
import {
  emptySoilInspectionValues,
  normalizeTimeInput,
  SOIL_INSPECTION_NOTE_MAX,
  SOIL_INSPECTION_STATE_OPTIONS,
  SOIL_INSPECTION_VALIDITY_NOTICE,
  soilInspectionErrorsFromApi,
  todayInFarmZone,
  toSoilInspectionInput,
  validateSoilInspection,
  type FormErrors,
  type SelectOption,
  type SoilInspectionFormField,
  type SoilInspectionFormValues,
} from '~/utils/fieldConditionsForm'
import { useFormSubmission } from './internal/formSubmission'
import { useApiServices } from './useApiServices'

/** Shown when the form is sent without a selected field. */
export const SOIL_INSPECTION_NO_FIELD_MESSAGE = 'Selecione um talhão para registrar a vistoria.'

export interface SoilInspectionForm {
  /** Reactive; bind with v-model. `date`/`time` are local time of the farm (default: now; `H:MM` accepted). */
  readonly values: SoilInspectionFormValues
  /** Seco, Ideal, Úmido, Encharcado (no "Desconhecido": the API refuses it). */
  readonly stateOptions: readonly SelectOption<InspectableSoilState>[]
  readonly errors: ComputedRef<FormErrors<SoilInspectionFormField>>
  /** Failure no field explains, or `SOIL_INSPECTION_NO_FIELD_MESSAGE` without a field. */
  readonly formError: ComputedRef<string | null>
  readonly saving: Readonly<Ref<boolean>>
  /** "A vistoria vale por 12 horas…". */
  readonly validityNotice: string
  readonly noteMaxLength: number
  /** Today in São Paulo (`YYYY-MM-DD`), as of the last `reset`: `max` of the date input. */
  readonly maxDate: ComputedRef<string>
  /** On open: empty state, now, no note. */
  readonly reset: () => void
  /** True = saved; `onSaved` (reload of the trafficability) already ran. */
  readonly submit: () => Promise<boolean>
}

export interface SoilInspectionFormOptions {
  readonly onSaved?: (inspection: SoilInspection) => Promise<void> | void
  readonly now?: () => Date
  /** Test seam; defaults to the app's `soilConditions` service. */
  readonly service?: SoilConditionsService
}

/** "Registrar vistoria" (`field_conditions.record`): the state a person saw in the field. */
export function useSoilInspectionForm(
  farmId: MaybeRefOrGetter<string>,
  fieldId: MaybeRefOrGetter<string | null>,
  options: SoilInspectionFormOptions = {},
): SoilInspectionForm {
  const service = options.service ?? useApiServices().soilConditions
  const now = options.now ?? (() => new Date())
  const values = reactive<SoilInspectionFormValues>(emptySoilInspectionValues(now()))
  const openedAt = ref(now())

  const submission = useFormSubmission<SoilInspectionFormField, SoilInspectionInput, SoilInspection>({
    values,
    validate: () => validateSoilInspection(values, now()),
    toInput: () => toSoilInspectionInput(values, now()),
    send: input => service.recordInspection(toValue(farmId), toValue(fieldId) ?? '', input),
    errorsFromApi: soilInspectionErrorsFromApi,
    unavailable: () => (toValue(fieldId) ? null : SOIL_INSPECTION_NO_FIELD_MESSAGE),
    onSaved: options.onSaved,
  })

  return {
    values,
    stateOptions: SOIL_INSPECTION_STATE_OPTIONS,
    errors: submission.errors,
    formError: submission.formError,
    saving: submission.saving,
    validityNotice: SOIL_INSPECTION_VALIDITY_NOTICE,
    noteMaxLength: SOIL_INSPECTION_NOTE_MAX,
    maxDate: computed(() => todayInFarmZone(openedAt.value)),
    reset: () => {
      openedAt.value = now()
      Object.assign(values, emptySoilInspectionValues(openedAt.value))
      submission.clear()
    },
    submit: async () => {
      values.time = normalizeTimeInput(values.time)
      return (await submission.submit()) !== null
    },
  }
}
