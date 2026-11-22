import { computed, reactive, ref, toValue, watch, type MaybeRefOrGetter } from 'vue'
import type { ApiError } from '~/services/errors'
import type { UserAccount } from '~/types/admin'
import type { CompanyRef } from '~/types/auth'
import {
  accountToValues,
  emptyUserAccessValues,
  FARM_SCOPE_LABELS,
  roleHasFarmScope,
  toInvitationInput,
  toUserUpdateInput,
  USER_ACCESS_FIELDS,
  USER_ACCESS_MESSAGES,
  userAccessErrorsFromApi,
  userRoleOptions,
  validateUserAccess,
  type FarmScopeMode,
  type UserAccessErrors,
  type UserAccessField,
  type UserAccessValues,
} from '~/utils/userAccessForm'
import { useResource } from './internal/resource'
import { useApiServices } from './useApiServices'
import { useFarms } from './useFarms'
import { useFlash } from './useFlash'
import { usePermissions } from './usePermissions'
import { useSession } from './useSession'
import { useUserActions } from './useUserActions'

/** Farms offered in "Fazendas liberadas" are loaded at once, like the farm list. */
const FARM_OPTIONS_PAGE_SIZE = 100
const COMPANY_OPTIONS_PAGE_SIZE = 100

function formErrorFor(error: ApiError): string | null {
  if (error.code === 'validation_failed' && error.details.length > 0) return null
  if (error.code === 'conflict') return null
  if (error.code === 'forbidden' || error.code === 'out_of_scope') return USER_ACCESS_MESSAGES.forbidden
  if (error.code === 'network_error') return USER_ACCESS_MESSAGES.network
  if (error.code === 'not_found') return USER_ACCESS_MESSAGES.notFound
  return USER_ACCESS_MESSAGES.generic
}

/**
 * Content of the invite modal (screen 13, Figma 52:758), also used to edit the
 * role, company and farm scope of an existing account. On success it leaves a
 * flash for the "Usuários" page (`takeFlash`) and resolves `true`.
 */
export function useUserAccessForm(options: { user?: MaybeRefOrGetter<UserAccount | null | undefined> } = {}) {
  const { companies: companyService } = useApiServices()
  const { user: sessionUser } = useSession()
  const { can, company: sessionCompany } = usePermissions()
  const flash = useFlash()
  const actions = useUserActions()

  const account = computed(() => toValue(options.user) ?? null)
  const mode = computed(() => (account.value ? 'edit' : 'invite'))
  const isSelf = computed(() => Boolean(account.value && account.value.id === sessionUser.value?.id))

  const values = reactive<UserAccessValues>(emptyUserAccessValues(sessionCompany.value?.id ?? null))
  const touched = reactive<Record<UserAccessField, boolean>>(
    Object.fromEntries(USER_ACCESS_FIELDS.map(field => [field, false])) as Record<UserAccessField, boolean>,
  )
  const showApiError = ref(false)

  watch(account, (current) => {
    if (current) Object.assign(values, accountToValues(current))
  }, { immediate: true })

  // Companies other than the caller's own are only listed with `companies.manage`.
  const companies = useResource<readonly CompanyRef[]>(
    () => `users:access:companies:${can('companies.manage') ? 'all' : 'own'}`,
    async () => {
      const own = sessionCompany.value ? [sessionCompany.value] : []
      if (!can('companies.manage')) return own
      const page = await companyService.list({ pageSize: COMPANY_OPTIONS_PAGE_SIZE })
      const others = page.items.filter(item => !own.some(ownCompany => ownCompany.id === item.id))
      return [...own, ...others.map(item => ({ id: item.id, name: item.name }))]
    },
    () => (sessionCompany.value ? [sessionCompany.value] : []),
  )
  const farms = useFarms({ pageSize: FARM_OPTIONS_PAGE_SIZE })

  // Super admins belong to the platform operator, i.e. the caller's company (the API validates).
  const companyLocked = computed(() => !can('companies.manage') || values.role === 'super_admin')
  const scopeLocked = computed(() => !roleHasFarmScope(values.role))

  watch(() => values.role, (role) => {
    if (role === 'super_admin' && sessionCompany.value) values.companyId = sessionCompany.value.id
    if (!roleHasFarmScope(role)) {
      values.farmScopeMode = 'all'
      values.farmIds = []
    }
  })
  // Farms belong to one company: switching companies drops the selection (not when the edited account loads).
  watch(() => values.companyId, (next, previous) => {
    if (next !== previous && next !== account.value?.company.id) values.farmIds = []
  })
  watch(() => values.farmScopeMode, (scopeMode) => {
    if (scopeMode === 'all') values.farmIds = []
  })

  const localErrors = computed(() => validateUserAccess(values))
  const actionError = computed(() => (mode.value === 'edit' ? actions.updateError.value : actions.inviteError.value))
  const apiError = computed(() => (showApiError.value ? actionError.value : null))
  watch(() => ({ ...values, farmIds: [...values.farmIds] }), () => {
    showApiError.value = false
  })

  const fieldErrors = computed<UserAccessErrors>(() => {
    const visibleLocal: UserAccessErrors = {}
    for (const field of USER_ACCESS_FIELDS) {
      if (touched[field] && localErrors.value[field]) visibleLocal[field] = localErrors.value[field]
    }
    const error = apiError.value
    const fromApi = error?.code === 'conflict' ? { email: USER_ACCESS_MESSAGES.emailTaken } : userAccessErrorsFromApi(error?.details ?? [])
    return { ...fromApi, ...visibleLocal }
  })

  function touch(field: UserAccessField): void {
    touched[field] = true
  }

  const saving = computed(() => actions.inviting.value || actions.updating.value)

  async function submit(): Promise<boolean> {
    USER_ACCESS_FIELDS.forEach(touch)
    if (Object.keys(localErrors.value).length > 0 || saving.value) return false

    const editing = account.value
    if (editing) {
      const input = toUserUpdateInput(editing, values)
      const saved = Object.keys(input).length === 0 ? editing : await actions.update(editing.id, input)
      showApiError.value = saved === null
      if (!saved) return false
      flash.show({ kind: 'success', title: 'Acesso atualizado', description: `${saved.name} já pode usar o novo acesso.`, highlightId: saved.id })
      return true
    }

    const invited = await actions.invite(toInvitationInput(values))
    showApiError.value = invited === null
    if (!invited) return false
    flash.show({
      kind: 'success',
      title: 'Convite enviado',
      description: `${invited.name} vai receber um e-mail em ${invited.email} para criar a senha.`,
      highlightId: invited.id,
    })
    return true
  }

  return {
    mode,
    texts: computed(() => (mode.value === 'edit'
      ? {
          eyebrow: 'USUÁRIOS',
          title: 'Editar acesso',
          description: `O papel define o que ${account.value?.name ?? 'a pessoa'} pode ver e fazer.`,
          submitLabel: 'Salvar acesso',
        }
      : {
          eyebrow: 'USUÁRIOS',
          title: 'Convidar usuário',
          description: 'A pessoa recebe um e-mail para criar a senha. O papel define o que ela pode ver e fazer.',
          submitLabel: 'Enviar convite',
        })),
    values,
    identityReadonly: computed(() => mode.value === 'edit'),
    roleOptions: computed(() => userRoleOptions(can('users.manage_all'), isSelf.value ? (account.value?.role ?? null) : null)),
    companyOptions: computed(() => companies.data.value.map(item => ({ value: item.id, label: item.name }))),
    companyLocked,
    scopeOptions: computed(() => (['all', 'selected'] as const).map((value: FarmScopeMode) => ({ value, label: FARM_SCOPE_LABELS[value] }))),
    scopeLocked,
    farmOptions: computed(() => farms.farms.value
      .filter(farm => farm.companyId === values.companyId)
      .map(farm => ({ value: farm.id, label: farm.name }))),
    fieldErrors,
    formError: computed(() => (apiError.value ? formErrorFor(apiError.value) : null)),
    touch,
    submit,
    saving,
    pending: computed(() => companies.pending.value || farms.pending.value),
    loadError: computed(() => companies.error.value ?? farms.error.value),
  }
}
