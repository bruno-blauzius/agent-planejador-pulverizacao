import { computed, reactive, ref, shallowRef, watch, type ComputedRef, type Ref, type WritableComputedRef } from 'vue'
import type { ApiError } from '~/services/errors'
import type { LinkRole, Owner, OwnerFarmLink, OwnerFarmLinkInput, OwnerInput, PersonType } from '~/types/owner'
import { formatPercent, maskDocument } from '~/utils/format'
import {
  currentSharesLabel,
  isoToBrDate,
  isValidCnpj,
  isValidCpf,
  maskDateInput,
  maskDocumentInput,
  maskPhoneInput,
  OWNER_FORM_MESSAGES,
  parseBrDate,
  parseShare,
  shareAdjustmentHint,
  validateAlertsPhone,
  validateOwnerData,
  validateShare,
  type OwnerFormErrors,
  type OwnerFormField,
} from '~/utils/ownerForm'
import { ownerSavedFlash } from '~/utils/ownerRow'
import { forbiddenMessageFor } from '~/utils/permissions'
import { useFarms } from './useFarms'
import { useFlash } from './useFlash'
import { useOwner } from './useOwner'
import { useOwnerActions } from './useOwnerActions'

export type OwnerFormMode = 'create' | 'edit' | 'wizard'
export type OwnerFormStep = 'data' | 'links'

export interface OwnerFormOptions {
  readonly mode: OwnerFormMode
  /** Required in 'edit'. */
  readonly ownerId?: string
  /** Only in 'wizard': farm being registered and its current shares (wizard state). */
  readonly presetFarm?: { readonly name: string, readonly currentShares: readonly { name: string, sharePercent: number }[] }
  readonly today?: () => Date
}

export interface OwnerLinkRowView {
  readonly farmId: string
  readonly farmName: string
  readonly selected: boolean
  readonly role: LinkRole
  /** Input text, e.g. "50". */
  readonly share: string
  /** Sum of the OTHER owners of the farm. */
  readonly othersShare: number
  /** othersShare + this owner's share (display only; the API decides). */
  readonly totalShare: number
  /** "Sítio Boa Esperança · participação total" */
  readonly totalLabel: string
  /** "100%" */
  readonly totalShareLabel: string
  readonly error: string | null
}

export type OwnerFormResult =
  | { readonly mode: 'create' | 'edit', readonly owner: Owner, readonly links: readonly OwnerFarmLink[] }
  | { readonly mode: 'wizard', readonly owner: Owner, readonly sharePercent: number }

interface LinkDraft {
  selected: boolean
  role: LinkRole
  share: string
}

const API_FIELDS: Readonly<Record<string, OwnerFormField>> = {
  name: 'name',
  document: 'document',
  birth_date: 'birthDate',
  phone: 'phone',
  email: 'email',
}
const DATA_FIELDS: readonly OwnerFormField[] = ['name', 'document', 'birthDate', 'phone', 'email']

function digitsOrNull(value: string): string | null {
  const digits = value.replace(/\D/g, '')
  return digits || null
}

/**
 * Owner modal (screen 4) and the "Cadastrar e vincular" dialog of the wizard.
 * See `.claude/contratos/tela-04.md`.
 */
export function useOwnerForm(options: OwnerFormOptions): OwnerForm {
  const { mode } = options
  const today = options.today ?? (() => new Date())
  const flash = useFlash()
  const actions = useOwnerActions()
  const ownerSource = mode === 'edit' && options.ownerId ? useOwner(options.ownerId) : null
  const farmsSource = mode === 'wizard' ? null : useFarms({ pageSize: 100 })

  const step = ref<OwnerFormStep>('data')
  const values = reactive({
    personType: 'individual' as PersonType,
    name: '',
    document: '',
    birthDate: '',
    phone: '',
    email: '',
    windowAlertsEnabled: true,
  })
  const wizardShare = ref('')
  const drafts = reactive<Record<string, LinkDraft>>({})
  const touched = reactive(new Set<OwnerFormField>())
  const touchedShares = reactive(new Set<string>())
  const apiError = shallowRef<ApiError | null>(null)
  /** Set once the owner exists (edit, or create whose links failed): next submit updates. */
  const savedOwnerId = ref<string | null>(options.ownerId ?? null)
  const originalDocument = ref('')
  const loadedFarmNames = reactive<Record<string, string>>({})

  // ---- edit: fill the form once the owner and links arrive ----------------
  if (ownerSource) {
    let ownerFilled = false
    let linksFilled = false
    watch(ownerSource.owner, (owner) => {
      if (!owner || ownerFilled) return
      ownerFilled = true
      Object.assign(values, {
        personType: owner.personType,
        name: owner.name,
        document: owner.document,
        birthDate: isoToBrDate(owner.birthDate),
        phone: owner.phone ? maskPhoneInput(owner.phone) : '',
        email: owner.email ?? '',
        windowAlertsEnabled: owner.windowAlertsEnabled,
      })
      originalDocument.value = owner.document
    }, { immediate: true })
    watch([ownerSource.links, ownerSource.pending], ([links, pending]) => {
      if (pending || linksFilled || !ownerSource.owner.value) return
      linksFilled = true
      for (const link of links) {
        drafts[link.farmId] = { selected: true, role: link.role, share: link.sharePercent === null ? '' : String(link.sharePercent) }
        loadedFarmNames[link.farmId] = link.farmName
      }
    }, { immediate: true })
  }

  const isDocumentLocked = computed(() => mode === 'edit')

  const personType = computed<PersonType>({
    get: () => values.personType,
    set: (type) => {
      if (isDocumentLocked.value || type === values.personType) return
      values.personType = type
      values.document = maskDocumentInput(values.document, type)
    },
  })
  const document = computed<string>({
    get: () => (isDocumentLocked.value ? maskDocument(originalDocument.value) : values.document),
    set: (text) => {
      if (!isDocumentLocked.value) values.document = maskDocumentInput(text, values.personType)
    },
  })
  const birthDate = computed<string>({ get: () => values.birthDate, set: text => (values.birthDate = maskDateInput(text)) })
  const phone = computed<string>({ get: () => values.phone, set: text => (values.phone = maskPhoneInput(text)) })
  const name = computed<string>({ get: () => values.name, set: text => (values.name = text) })
  const email = computed<string>({ get: () => values.email, set: text => (values.email = text) })
  const windowAlertsEnabled = computed<boolean>({ get: () => values.windowAlertsEnabled, set: on => (values.windowAlertsEnabled = on) })

  // An API error stays visible until the user changes something.
  watch([values, drafts, wizardShare], () => {
    apiError.value = null
  }, { deep: true })

  // ---- validation ---------------------------------------------------------
  const localDataErrors = computed<OwnerFormErrors>(() => {
    const errors = validateOwnerData(values, { skipDocument: isDocumentLocked.value, today: today() })
    if (mode === 'wizard') {
      const shareError = validateShare(wizardShare.value)
      if (shareError) errors.wizardShare = shareError
    }
    return errors
  })

  /** Not in the wizard: it has no toggle, alerts simply follow the phone there. */
  const alertsError = computed(() => (mode === 'wizard' ? null : validateAlertsPhone(values)))

  const apiFieldErrors = computed<OwnerFormErrors>(() => {
    const error = apiError.value
    if (!error) return {}
    if (error.code === 'conflict') return { document: error.message }
    const errors: OwnerFormErrors = {}
    for (const detail of error.details) {
      const field = API_FIELDS[detail.field]
      if (field) errors[field] = detail.message
    }
    return errors
  })

  /** 422 details like `links[1].share_percent`, indexed by the order sent. */
  const lastSentLinks = shallowRef<readonly OwnerFarmLinkInput[]>([])
  const apiLinkErrors = computed<Record<string, string>>(() => {
    const errors: Record<string, string> = {}
    for (const detail of apiError.value?.details ?? []) {
      const match = /^links\[(\d+)\]/.exec(detail.field)
      const link = match ? lastSentLinks.value[Number(match[1])] : undefined
      if (link) errors[link.farmId] = detail.message
    }
    return errors
  })

  const fieldErrors = computed<OwnerFormErrors>(() => {
    const visible: OwnerFormErrors = { ...apiFieldErrors.value }
    for (const field of [...DATA_FIELDS, 'wizardShare'] as const) {
      const message = localDataErrors.value[field]
      if (touched.has(field) && message) visible[field] = message
    }
    if (touched.has('windowAlerts') && alertsError.value) visible.windowAlerts = alertsError.value
    return visible
  })

  const formError = computed(() => {
    const error = apiError.value
    if (!error) return null
    const forbidden = forbiddenMessageFor(error)
    if (forbidden) return forbidden
    const isMapped = Object.keys(apiFieldErrors.value).length > 0 || Object.keys(apiLinkErrors.value).length > 0
    return isMapped ? null : error.message
  })

  function touch(field: OwnerFormField) {
    touched.add(field)
  }

  // ---- links ----------------------------------------------------------------
  const links = computed<readonly OwnerLinkRowView[]>(() => {
    const farms = farmsSource?.farms.value ?? []
    return farms.map((farm) => {
      const draft = drafts[farm.id] ?? { selected: false, role: 'owner' as LinkRole, share: '' }
      const othersShare = farm.owners
        .filter(share => share.ownerId !== savedOwnerId.value && share.role === 'owner')
        .reduce((sum, share) => sum + (share.sharePercent ?? 0), 0)
      const ownShare = draft.selected && draft.role === 'owner' ? parseShare(draft.share) ?? 0 : 0
      const totalShare = Math.round((othersShare + ownShare) * 10) / 10
      const shareError = draft.selected && draft.role === 'owner' && touchedShares.has(farm.id) ? validateShare(draft.share) : null
      return {
        farmId: farm.id,
        farmName: farm.name,
        selected: draft.selected,
        role: draft.role,
        share: draft.share,
        othersShare,
        totalShare,
        totalLabel: `${farm.name} · participação total`,
        totalShareLabel: formatPercent(totalShare),
        error: shareError ?? (draft.selected && totalShare > 100 ? OWNER_FORM_MESSAGES.farmTotalOver : null) ?? apiLinkErrors.value[farm.id] ?? null,
      }
    })
  })

  function draftOf(farmId: string): LinkDraft {
    drafts[farmId] ??= { selected: false, role: 'owner', share: '' }
    return drafts[farmId]
  }

  function farmNameOf(farmId: string): string {
    return farmsSource?.farms.value.find(farm => farm.id === farmId)?.name ?? loadedFarmNames[farmId] ?? ''
  }

  const selectedLinks = computed<OwnerFarmLinkInput[]>(() =>
    Object.entries(drafts)
      .filter(([, draft]) => draft.selected)
      .map(([farmId, draft]) => ({
        farmId,
        role: draft.role,
        sharePercent: draft.role === 'owner' ? parseShare(draft.share) : null,
      })),
  )

  const linksSummary = computed(() => {
    if (selectedLinks.value.length === 0) return 'Sem vínculos'
    const parts = selectedLinks.value.map(link =>
      `${farmNameOf(link.farmId)} · ${link.role === 'owner' ? formatPercent(link.sharePercent) : link.role === 'tenant' ? 'Arrend.' : 'Resp. legal'}`)
    return `Vínculos: ${parts.join(', ')}`
  })

  // ---- wizard ---------------------------------------------------------------
  const wizardShareContext = computed(() => {
    const preset = options.presetFarm
    if (mode !== 'wizard' || !preset) return null
    return {
      currentLabel: currentSharesLabel(preset.currentShares),
      hint: shareAdjustmentHint(preset.currentShares, parseShare(wizardShare.value), values.name),
    }
  })

  const pendingDocumentNotice = computed(() => {
    const owner = ownerSource?.owner.value
    if (!owner?.hasPendingDocuments) return null
    return {
      title: 'Documento com foto pendente',
      description: `Sem ele, ${owner.name.split(/\s+/)[0]} não pode assinar o registro de aplicações.`,
    }
  })

  // ---- navigation and submit -------------------------------------------------
  function touchData() {
    DATA_FIELDS.forEach(touch)
    if (mode === 'wizard') touch('wizardShare')
  }

  function hasDataErrors(): boolean {
    touchData()
    return Object.keys(localDataErrors.value).length > 0
  }

  function goToLinks(): boolean {
    if (hasDataErrors()) return false
    step.value = 'links'
    return true
  }

  /** Touches every selected share and the alerts toggle; true when the step is valid. */
  function validateLinksStep(): boolean {
    touch('windowAlerts')
    Object.entries(drafts).forEach(([farmId, draft]) => draft.selected && touchedShares.add(farmId))
    return !alertsError.value && !links.value.some(link => link.selected && link.error)
  }

  function backToData() {
    step.value = 'data'
  }

  function ownerInput(): OwnerInput {
    return {
      personType: values.personType,
      name: values.name.trim(),
      document: isDocumentLocked.value ? originalDocument.value : values.document,
      birthDate: values.personType === 'individual' ? parseBrDate(values.birthDate) : null,
      phone: digitsOrNull(values.phone),
      email: values.email.trim() || null,
      windowAlertsEnabled: mode === 'wizard' ? Boolean(digitsOrNull(values.phone)) : values.windowAlertsEnabled,
    }
  }

  async function saveOwner(): Promise<Owner | null> {
    const input = ownerInput()
    const owner = savedOwnerId.value
      ? await actions.updateOwner(savedOwnerId.value, input)
      : await actions.createOwner(input)
    if (!owner) {
      apiError.value = actions.error.value
      return null
    }
    savedOwnerId.value = owner.id
    return owner
  }

  async function submit(): Promise<OwnerFormResult | null> {
    if (actions.saving.value) return null
    if (hasDataErrors()) {
      step.value = 'data'
      return null
    }
    if (mode === 'wizard') {
      const owner = await saveOwner()
      return owner ? { mode: 'wizard', owner, sharePercent: parseShare(wizardShare.value) ?? 0 } : null
    }
    if (!validateLinksStep()) {
      step.value = 'links'
      return null
    }

    const owner = await saveOwner()
    if (!owner) return null
    lastSentLinks.value = selectedLinks.value
    const savedLinks = await actions.replaceLinks(owner.id, lastSentLinks.value)
    if (!savedLinks) {
      apiError.value = actions.error.value
      step.value = 'links'
      return null
    }
    flash.show(ownerSavedFlash(owner, savedLinks, mode === 'create' ? 'created' : 'updated'))
    return { mode, owner, links: savedLinks }
  }

  return {
    step,
    personType,
    name,
    document,
    birthDate,
    phone,
    email,
    windowAlertsEnabled,
    isDocumentLocked,
    labels: computed(() => (values.personType === 'individual'
      ? { name: 'Nome completo', document: 'CPF' }
      : { name: 'Razão social', document: 'CNPJ' })),
    fieldErrors,
    documentHint: computed(() => {
      if (isDocumentLocked.value || fieldErrors.value.document) return null
      const valid = values.personType === 'individual' ? isValidCpf(values.document) : isValidCnpj(values.document)
      return valid ? `${values.personType === 'individual' ? 'CPF' : 'CNPJ'} válido` : null
    }),
    formError,
    touch,
    pendingDocumentNotice,
    links,
    toggleFarm(farmId: string, selected: boolean) {
      draftOf(farmId).selected = selected
    },
    setShare(farmId: string, share: string) {
      draftOf(farmId).share = share
      touchedShares.add(farmId)
    },
    touchShare(farmId: string) {
      touchedShares.add(farmId)
    },
    setRole(farmId: string, role: LinkRole) {
      const draft = draftOf(farmId)
      draft.role = role
      if (role !== 'owner') draft.share = ''
    },
    linksSummary,
    wizardShare,
    wizardShareContext,
    goToLinks,
    backToData,
    submit,
    loading: computed(() => ownerSource?.pending.value ?? false),
    linksLoading: computed(() => farmsSource?.pending.value ?? false),
    loadError: computed(() => ownerSource?.error.value ?? farmsSource?.error.value ?? null),
    saving: actions.saving,
  }
}

export interface OwnerForm {
  readonly step: Readonly<Ref<OwnerFormStep>>
  readonly personType: WritableComputedRef<PersonType>
  readonly name: WritableComputedRef<string>
  readonly document: WritableComputedRef<string>
  readonly birthDate: WritableComputedRef<string>
  readonly phone: WritableComputedRef<string>
  readonly email: WritableComputedRef<string>
  readonly windowAlertsEnabled: WritableComputedRef<boolean>
  readonly isDocumentLocked: ComputedRef<boolean>
  readonly labels: ComputedRef<{ name: string, document: string }>
  readonly fieldErrors: ComputedRef<OwnerFormErrors>
  /** "CPF válido" / "CNPJ válido" once the local check passes (never claims a Receita lookup). */
  readonly documentHint: ComputedRef<string | null>
  readonly formError: ComputedRef<string | null>
  readonly touch: (field: OwnerFormField) => void
  readonly pendingDocumentNotice: ComputedRef<{ title: string, description: string } | null>
  readonly links: ComputedRef<readonly OwnerLinkRowView[]>
  readonly toggleFarm: (farmId: string, selected: boolean) => void
  readonly setShare: (farmId: string, share: string) => void
  /** On blur of the share input: shows "Informe a participação." if still empty. */
  readonly touchShare: (farmId: string) => void
  readonly setRole: (farmId: string, role: LinkRole) => void
  readonly linksSummary: ComputedRef<string>
  readonly wizardShare: Ref<string>
  readonly wizardShareContext: ComputedRef<{ currentLabel: string, hint: string | null } | null>
  readonly goToLinks: () => boolean
  readonly backToData: () => void
  readonly submit: () => Promise<OwnerFormResult | null>
  readonly loading: ComputedRef<boolean>
  readonly linksLoading: ComputedRef<boolean>
  readonly loadError: ComputedRef<ApiError | null>
  readonly saving: ComputedRef<boolean>
}
