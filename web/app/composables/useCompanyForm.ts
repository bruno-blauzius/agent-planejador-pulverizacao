import { computed, reactive, shallowRef, watch, type ComputedRef, type Ref, type WritableComputedRef } from 'vue'
import type { ApiError } from '~/services/errors'
import type { Company, CompanyPlan } from '~/types/admin'
import {
  COMPANY_FORM_FIELDS,
  COMPANY_PLAN_EDIT_HINTS,
  COMPANY_PLAN_HINTS,
  COMPANY_PLAN_OPTIONS,
  companyApiFieldErrors,
  companyCreateInput,
  companyFormErrorMessage,
  companyToFormValues,
  companyUpdateInput,
  emptyCompanyForm,
  maskCnpjInput,
  maskStateInput,
  validateCompanyForm,
  type CompanyFormErrors,
  type CompanyFormField,
} from '~/utils/companyForm'
import { useCompanyActions } from './useCompanyActions'

export type CompanyFormMode = 'create' | 'edit'

export interface CompanyFormOptions {
  readonly mode: CompanyFormMode
  /**
   * Edit only: the company being edited. May start `null` while the list is
   * loading (deep link); the form fills itself once it arrives.
   */
  readonly company?: Company | Ref<Company | null> | ComputedRef<Company | null> | null
}

export type CompanyFormResult =
  | { readonly mode: 'create', readonly company: Company }
  /** `changed: false` when nothing differed: no request was sent. */
  | { readonly mode: 'edit', readonly company: Company, readonly changed: boolean }

export interface CompanyForm {
  readonly mode: CompanyFormMode
  readonly title: string
  readonly submitLabel: string
  readonly name: WritableComputedRef<string>
  /** Masked as typed ("12.345.678/0001-90"). Read-only in edit (the PATCH has no document). */
  readonly document: WritableComputedRef<string>
  readonly city: WritableComputedRef<string>
  /** Letters only, upper case, at most 2. */
  readonly state: WritableComputedRef<string>
  readonly plan: WritableComputedRef<CompanyPlan>
  readonly planOptions: typeof COMPANY_PLAN_OPTIONS
  /** What the chosen plan does (trial starts an evaluation period). */
  readonly planHint: ComputedRef<string>
  readonly isDocumentLocked: boolean
  /** Visible errors: local ones after the field is touched, API ones (422 `details`, 409 duplicate CNPJ) at once. */
  readonly fieldErrors: ComputedRef<CompanyFormErrors>
  /** Footer message when no field explains the failure (403, network, 404, 502...). */
  readonly formError: ComputedRef<string | null>
  /** On blur of an input. */
  readonly touch: (field: CompanyFormField) => void
  readonly submit: () => Promise<CompanyFormResult | null>
  readonly saving: ComputedRef<boolean>
  /** Edit: true while a deep-linked company has not arrived yet. */
  readonly isLoadingCompany: ComputedRef<boolean>
}

/**
 * "Nova empresa" / "Editar empresa" modal of screen 14. `submit()` resolves
 * with the saved company (the page then shows the notice and refreshes) or
 * `null` when invalid or failed, with errors exposed per field.
 */
export function useCompanyForm(options: CompanyFormOptions): CompanyForm {
  const { mode } = options
  const actions = useCompanyActions()
  const source = computed<Company | null>(() => {
    const company = options.company
    if (!company) return null
    return 'value' in company ? company.value : company
  })

  const values = reactive(emptyCompanyForm())
  const touched = reactive(new Set<CompanyFormField>())
  const apiError = shallowRef<ApiError | null>(null)

  if (mode === 'edit') {
    let filled = false
    watch(source, (company) => {
      if (!company || filled) return
      filled = true
      Object.assign(values, companyToFormValues(company))
    }, { immediate: true })
  }

  // An API error stays visible until the user changes something.
  watch(values, () => {
    apiError.value = null
  }, { deep: true })

  const isDocumentLocked = mode === 'edit'
  const localErrors = computed(() => validateCompanyForm(values, { skipDocument: isDocumentLocked }))
  const apiFieldErrors = computed(() => companyApiFieldErrors(apiError.value))

  const fieldErrors = computed<CompanyFormErrors>(() => {
    const visible: CompanyFormErrors = { ...apiFieldErrors.value }
    for (const field of COMPANY_FORM_FIELDS) {
      const message = localErrors.value[field]
      if (touched.has(field) && message) visible[field] = message
    }
    return visible
  })

  function touchAll() {
    COMPANY_FORM_FIELDS.forEach(field => touched.add(field))
  }

  async function submit(): Promise<CompanyFormResult | null> {
    if (actions.creating.value || actions.updating.value) return null
    touchAll()
    if (Object.keys(localErrors.value).length > 0) return null

    if (mode === 'create') {
      const company = await actions.create(companyCreateInput(values))
      if (!company) {
        apiError.value = actions.createError.value
        return null
      }
      return { mode: 'create', company }
    }

    const original = source.value
    if (!original) return null
    const changes = companyUpdateInput(values, original)
    if (Object.keys(changes).length === 0) return { mode: 'edit', company: original, changed: false }
    const company = await actions.update(original.id, changes)
    if (!company) {
      apiError.value = actions.updateError.value
      return null
    }
    return { mode: 'edit', company, changed: true }
  }

  return {
    mode,
    title: mode === 'create' ? 'Nova empresa' : 'Editar empresa',
    submitLabel: mode === 'create' ? 'Cadastrar empresa' : 'Salvar alterações',
    name: computed<string>({ get: () => values.name, set: text => (values.name = text) }),
    document: computed<string>({
      get: () => values.document,
      set: (text) => {
        if (!isDocumentLocked) values.document = maskCnpjInput(text)
      },
    }),
    city: computed<string>({ get: () => values.city, set: text => (values.city = text) }),
    state: computed<string>({ get: () => values.state, set: text => (values.state = maskStateInput(text)) }),
    plan: computed<CompanyPlan>({ get: () => values.plan, set: plan => (values.plan = plan) }),
    planOptions: COMPANY_PLAN_OPTIONS,
    planHint: computed(() => (mode === 'create' ? COMPANY_PLAN_HINTS : COMPANY_PLAN_EDIT_HINTS)[values.plan]),
    isDocumentLocked,
    fieldErrors,
    formError: computed(() => companyFormErrorMessage(apiError.value)),
    touch: field => touched.add(field),
    submit,
    saving: computed(() => actions.creating.value || actions.updating.value),
    isLoadingCompany: computed(() => mode === 'edit' && !source.value),
  }
}
