import { computed, onScopeDispose, ref, shallowRef, watch } from 'vue'
import type { ApiError } from '~/services/errors'
import type { CompaniesSummary, Company } from '~/types/admin'
import type { FlashMessage } from '~/types/flash'
import type { Page } from '~/types/pagination'
import { buildCompanyPagination, buildCompanyStatCards, toCompanyRowView, type CompanyRowActionKind, type CompanyRowView } from '~/utils/company'
import {
  companyActionChangedDespiteError,
  companyActionErrorMessage,
  companyActionErrorNotice,
  companyActionSuccessNotice,
  companyConfirmationCopy,
  companyRowActions,
  companySavedNotice,
  reactivationStatus,
  type CompanyActionKind,
  type CompanyConfirmationCopy,
} from '~/utils/companyForm'
import type { CompanyFormResult } from './useCompanyForm'
import { useDeepLinkedRecord } from './internal/deepLink'
import { stableKey, useResource } from './internal/resource'
import { useApiServices } from './useApiServices'
import { useCompanyActions } from './useCompanyActions'
import { useFlash } from './useFlash'
import { usePermissions } from './usePermissions'

export interface UseCompanyListPageOptions {
  debounceMs?: number
  now?: () => Date
}

export const COMPANY_PAGE_SIZES = [10, 20, 50] as const
const DEFAULT_PAGE_SIZE = 10

/** Modal of screen 14 in the query: `?new=1` (Nova empresa) or `?company=<id>` (Editar empresa). */
export type CompanyListModal = { readonly mode: 'create' } | {
  readonly mode: 'edit'
  readonly companyId: string
  /** The company (listed, or fetched by id when it is not on the current page/filter); null while loading, not found or on a load error. */
  readonly company: Company | null
  /** The list or the fetch by id is still running, so a missing `company` does not mean "not found" yet. */
  readonly loading: boolean
  /** Fetching by id failed for a reason other than 404: a load error, not "not found". `company` and `loading` are then null/false. */
  readonly loadError: ApiError | null
}

/** Confirmation dialog of the destructive row actions (inativar / excluir). */
export interface CompanyConfirmation extends CompanyConfirmationCopy {
  readonly kind: 'deactivate' | 'delete'
  readonly companyId: string
  readonly companyName: string
  readonly pending: boolean
  /** Failure shown inside the dialog (e.g. 409 `company_has_active_users`); the dialog stays open. */
  readonly error: string | null
}

const emptyPage = (): Page<Company> => ({ items: [], total: 0, page: 1, pageSize: 0 })

function firstValue(value: unknown): unknown {
  return Array.isArray(value) ? value[0] : value
}

function positiveInt(value: unknown, fallback: number): number {
  const parsed = Number.parseInt(String(firstValue(value) ?? ''), 10)
  return Number.isFinite(parsed) && parsed > 0 ? parsed : fallback
}

/**
 * Screen "Empresas" (14, super admin): summary cards and the paginated table.
 * Filters live in the query (`?q=&page=&size=`, replace). The route requires
 * `companies.manage`; a 403 from the API still shows the forbidden state.
 */
export function useCompanyListPage(options: UseCompanyListPageOptions = {}) {
  const { companies } = useApiServices()
  const { can } = usePermissions()
  const route = useRoute()
  const flash = useFlash()
  const debounceMs = options.debounceMs ?? 300
  const now = options.now ?? (() => new Date())

  const query = computed(() => {
    const size = positiveInt(route.query.size, DEFAULT_PAGE_SIZE)
    return {
      q: String(firstValue(route.query.q) ?? ''),
      page: positiveInt(route.query.page, 1),
      size: COMPANY_PAGE_SIZES.find(item => item === size) ?? DEFAULT_PAGE_SIZE,
    }
  })

  /** Replaces query params; `undefined` removes them. Filter changes go back to page 1. */
  function replaceQuery(changes: Record<string, string | undefined>, resetPage = true) {
    void navigateTo({ query: { ...route.query, ...(resetPage ? { page: undefined } : {}), ...changes } }, { replace: true })
  }

  const search = ref(query.value.q)
  let debounce: ReturnType<typeof setTimeout> | undefined
  /** Set by clearSearch: the next change applies at once (a single query replace). */
  let applyImmediately = false
  watch(search, (text) => {
    clearTimeout(debounce)
    const apply = () => replaceQuery({ q: text.trim() || undefined })
    if (debounceMs <= 0 || applyImmediately) {
      applyImmediately = false
      apply()
    }
    else debounce = setTimeout(apply, debounceMs)
  })
  onScopeDispose(() => clearTimeout(debounce))

  const filters = computed(() => ({
    search: query.value.q || undefined,
    page: query.value.page,
    pageSize: query.value.size,
  }))

  const list = useResource(() => stableKey('companies:list', filters), () => companies.list(filters.value), emptyPage)
  const summary = useResource<CompaniesSummary | null>('companies:summary', () => companies.summary(), () => null)

  const total = computed(() => list.data.value.total)
  const pagination = computed(() => buildCompanyPagination(query.value.page, query.value.size, total.value))
  const isLoaded = computed(() => !list.pending.value && !list.error.value)

  // `?page=` beyond the last page (bookmark, removal elsewhere): go to the last valid one.
  watch([isLoaded, total, () => query.value.page], ([loaded, count, current]) => {
    const last = pagination.value.pageCount
    if (loaded && count > 0 && current > last) replaceQuery({ page: last > 1 ? String(last) : undefined }, false)
  })

  const page = computed<number>({
    get: () => query.value.page,
    set: value => replaceQuery({ page: value > 1 ? String(value) : undefined }, false),
  })

  const pageSize = computed<number>({
    get: () => query.value.size,
    set: value => replaceQuery({ size: value === DEFAULT_PAGE_SIZE ? undefined : String(value) }),
  })

  // ---- row actions, confirmations, notice --------------------------------
  const actions = useCompanyActions()
  const canManage = computed(() => can('companies.manage'))
  const highlightId = shallowRef<string | null>(null)
  const actionPending = ref<string | null>(null)
  const notice = shallowRef<FlashMessage | null>(null)
  const confirmationState = shallowRef<{ kind: 'deactivate' | 'delete', company: Company, error: string | null } | null>(null)

  async function refreshAll(): Promise<void> {
    await Promise.all([list.refresh(), summary.refresh()])
  }

  function findCompany(companyId: string): Company | null {
    return list.data.value.items.find(item => item.id === companyId) ?? null
  }

  const deepLink = useDeepLinkedRecord<Company>({
    id: () => {
      const id = firstValue(route.query.company)
      return typeof id === 'string' && id ? id : null
    },
    find: findCompany,
    listSettled: () => !list.pending.value,
    enabled: () => canManage.value,
    load: id => companies.get(id),
  })

  function openModal(changes: Record<string, string | undefined>) {
    void navigateTo({ query: { ...route.query, new: undefined, company: undefined, ...changes } })
  }

  async function callAction(company: Company, kind: CompanyActionKind): Promise<{ ok: true } | { ok: false, error: ApiError | null }> {
    if (kind === 'delete') {
      return (await actions.remove(company.id)) ? { ok: true } : { ok: false, error: actions.removeError.value }
    }
    const status = kind === 'deactivate' ? 'inactive' : reactivationStatus(company)
    return (await actions.update(company.id, { status })) ? { ok: true } : { ok: false, error: actions.updateError.value }
  }

  /** Runs the action, publishes the outcome and refreshes; a failure goes to the dialog or the notice. */
  async function executeAction(company: Company, kind: CompanyActionKind, options: { failureInDialog: boolean }): Promise<void> {
    if (actionPending.value) return
    actionPending.value = company.id
    notice.value = null
    try {
      const outcome = await callAction(company, kind)
      if (outcome.ok) {
        confirmationState.value = null
        notice.value = companyActionSuccessNotice(kind, company)
        highlightId.value = kind === 'delete' ? null : company.id
        await refreshAll()
        return
      }
      // The deactivation is saved before the SSO call: show the warning and the new state.
      const changedAnyway = companyActionChangedDespiteError(outcome.error, kind)
      const current = confirmationState.value
      const stale = outcome.error?.code === 'not_found'
      // A vanished company has nothing left to confirm: close the dialog and say so in the notice.
      if (options.failureInDialog && current && !changedAnyway && !stale) {
        confirmationState.value = { ...current, error: companyActionErrorMessage(outcome.error, kind) }
      }
      else {
        confirmationState.value = null
        notice.value = companyActionErrorNotice(outcome.error, kind)
      }
      if (changedAnyway || stale) {
        highlightId.value = changedAnyway ? company.id : null
        await refreshAll()
      }
    }
    finally {
      actionPending.value = null
    }
  }

  /** Row menu entry: edit opens the modal, reactivate runs at once, deactivate/delete ask for confirmation. */
  async function runAction(row: Pick<CompanyRowView, 'id'>, kind: CompanyRowActionKind): Promise<void> {
    const company = findCompany(row.id)
    if (!company || !canManage.value || actionPending.value) return
    if (!companyRowActions(company.status).some(action => action.kind === kind)) return
    if (kind === 'edit') {
      openModal({ company: company.id })
      return
    }
    if (kind === 'reactivate') {
      await executeAction(company, kind, { failureInDialog: false })
      return
    }
    notice.value = null
    confirmationState.value = { kind, company, error: null }
  }

  async function confirmAction(): Promise<void> {
    const current = confirmationState.value
    if (!current) return
    confirmationState.value = { ...current, error: null }
    await executeAction(current.company, current.kind, { failureInDialog: true })
  }

  const confirmation = computed<CompanyConfirmation | null>(() => {
    const current = confirmationState.value
    if (!current) return null
    return {
      ...companyConfirmationCopy(current.kind, current.company),
      kind: current.kind,
      companyId: current.company.id,
      companyName: current.company.name,
      pending: actionPending.value === current.company.id,
      error: current.error,
    }
  })

  const modal = computed<CompanyListModal | null>(() => {
    if (!canManage.value) return null
    if (firstValue(route.query.new) === '1') return { mode: 'create' }
    const companyId = firstValue(route.query.company)
    if (typeof companyId !== 'string' || !companyId) return null
    const { record, loading, loadError } = deepLink.resolve(companyId)
    return { mode: 'edit', companyId, company: record, loading, loadError }
  })

  const hasSearch = computed(() => Boolean(query.value.q))
  const noRows = computed(() => isLoaded.value && list.data.value.items.length === 0)

  return {
    statCards: computed(() => buildCompanyStatCards(summary.data.value, now())),
    summaryError: summary.error,
    summaryPending: summary.pending,

    search,
    rows: computed(() => list.data.value.items.map(company => toCompanyRowView(company, now(), {
      actions: canManage.value ? companyRowActions(company.status) : [],
      highlightId: highlightId.value,
    }))),
    pending: list.pending,
    error: list.error,
    isForbidden: computed(() => list.error.value?.code === 'forbidden' || summary.error.value?.code === 'forbidden'),
    isEmpty: computed(() => noRows.value && !hasSearch.value),
    isEmptySearch: computed(() => noRows.value && hasSearch.value),
    clearSearch() {
      clearTimeout(debounce)
      // Field already empty (a pending debounce was cancelled): drop ?q= directly.
      if (!search.value) {
        replaceQuery({ q: undefined })
        return
      }
      applyImmediately = true
      search.value = ''
    },
    refresh: refreshAll,

    pagination,
    page,
    pageSize,
    pageSizeOptions: COMPANY_PAGE_SIZES.map(value => ({ value, label: `${value} por página` })),

    canCreate: canManage,

    /** Row "…" menu: `row.actions` lists the entries; pass the chosen `kind` here. */
    runAction,
    /** Id of the company with an action in flight (disable its menu), or null. */
    actionPending: computed(() => actionPending.value),
    confirmation,
    confirmAction,
    cancelConfirmation() {
      if (!actionPending.value) confirmationState.value = null
    },
    notice: computed(() => notice.value),
    dismissNotice: () => {
      notice.value = null
    },
    modal,
    openCreate: () => {
      if (canManage.value) openModal({ new: '1' })
    },
    closeModal: () => replaceQuery({ new: undefined, company: undefined }, false),
    /** The form resolved with a saved company: close the modal, announce, refresh. */
    handleSaved(result: CompanyFormResult) {
      replaceQuery({ new: undefined, company: undefined }, false)
      if (result.mode === 'edit' && !result.changed) return
      notice.value = companySavedNotice(result.company, result.mode === 'create' ? 'created' : 'updated')
      highlightId.value = result.company.id
      void refreshAll()
    },
    /** Shows (once) a message left by another screen and refreshes the data. */
    takeFlash(): FlashMessage | null {
      const message = flash.consume()
      if (!message) return null
      highlightId.value = message.highlightId ?? null
      notice.value = message
      void refreshAll()
      return message
    },
  }
}
