import { computed, onScopeDispose, ref, shallowRef, watch, type ComputedRef, type Ref, type WritableComputedRef } from 'vue'
import type { ApiError } from '~/services/errors'
import type { FlashMessage } from '~/types/flash'
import type { PersonType } from '~/types/owner'
import { toOwnerRowView, type OwnerRowView } from '~/utils/ownerRow'
import { useFlash } from './useFlash'
import { useOwners } from './useOwners'
import { usePermissions } from './usePermissions'
import { useSession } from './useSession'

export type OwnerPersonFilter = 'all' | PersonType
export type OwnerModalStep = 'data' | 'links'

export const OWNER_LIST_PAGE_SIZE = 20
const PERSON_FILTERS: readonly OwnerPersonFilter[] = ['all', 'individual', 'company']

export interface OwnerListPagination {
  /** "Mostrando 6 de 6" */
  readonly showingLabel: string
  /** "Página 1 de 1" */
  readonly pageLabel: string
  readonly page: number
  readonly pageCount: number
}

export interface OwnerListPage {
  /** Search box text; applied to `?q=` (server search) after the debounce. */
  readonly search: Ref<string>
  readonly personType: WritableComputedRef<OwnerPersonFilter>
  readonly page: WritableComputedRef<number>
  readonly rows: ComputedRef<readonly OwnerRowView[]>
  /** "6 pessoas e empresas"; null during the first load and on error (UI hides it). */
  readonly eyebrow: ComputedRef<string | null>
  /** Footer; null during the first load and on error. */
  readonly pagination: ComputedRef<OwnerListPagination | null>
  readonly pending: ComputedRef<boolean>
  readonly error: ComputedRef<ApiError | null>
  /** No owner registered at all. */
  readonly isEmpty: ComputedRef<boolean>
  /** Search/filter without results. */
  readonly isFilterEmpty: ComputedRef<boolean>
  readonly refresh: () => Promise<void>
  /**
   * `registry.write` of the session (ADR 0013): hide "Novo proprietário",
   * "Editar" and "Vínculos" when false. Presentation only; the API answers 403.
   */
  readonly canWrite: ComputedRef<boolean>
  /**
   * Modal of screen 4, driven by `?owner=new|<id>&step=links`. Always null
   * without `registry.write`: the modal is an edit form, so it does not open
   * and a pasted `?owner=` is removed from the URL (replace).
   */
  readonly modalOwnerId: ComputedRef<string | null>
  readonly modalStep: ComputedRef<OwnerModalStep>
  readonly openNew: () => void
  readonly openEdit: (ownerId: string) => void
  readonly openLinks: (ownerId: string) => void
  readonly closeModal: () => void
  /**
   * Consumes the pending flash (saved owner): returns it for the alert,
   * highlights its row and refreshes the list. Call on mount and after the
   * modal closes.
   */
  readonly takeFlash: () => FlashMessage | null
}

function firstValue(value: unknown): unknown {
  return Array.isArray(value) ? value[0] : value
}

/** Screen "Proprietários". Filters live in the query (`?q=&type=&page=`, replace). */
export function useOwnerListPage(options: { debounceMs?: number } = {}): OwnerListPage {
  const debounceMs = options.debounceMs ?? 300
  const route = useRoute()
  const flash = useFlash()
  const highlightId = shallowRef<string | null>(null)

  const query = computed(() => ({
    q: String(firstValue(route.query.q) ?? ''),
    type: PERSON_FILTERS.find(item => item === firstValue(route.query.type)) ?? 'all',
    page: Math.max(1, Number.parseInt(String(firstValue(route.query.page) ?? '1'), 10) || 1),
  }))

  /** Replaces query params; `undefined` removes them. Filter changes go back to page 1. */
  function replaceQuery(changes: Record<string, string | undefined>, resetPage = true) {
    const next = { ...route.query, ...(resetPage ? { page: undefined } : {}), ...changes }
    void navigateTo({ query: next }, { replace: true })
  }

  const search = ref(query.value.q)
  let debounce: ReturnType<typeof setTimeout> | undefined
  watch(search, (text) => {
    clearTimeout(debounce)
    debounce = setTimeout(() => replaceQuery({ q: text.trim() || undefined }), debounceMs)
  })
  onScopeDispose(() => clearTimeout(debounce))

  const personType = computed<OwnerPersonFilter>({
    get: () => query.value.type,
    set: value => replaceQuery({ type: value === 'all' ? undefined : value }),
  })
  const page = computed<number>({
    get: () => query.value.page,
    set: value => replaceQuery({ page: value > 1 ? String(value) : undefined }, false),
  })

  const list = useOwners(() => ({
    search: query.value.q || undefined,
    personType: query.value.type === 'all' ? undefined : query.value.type,
    page: query.value.page,
    pageSize: OWNER_LIST_PAGE_SIZE,
  }))

  const hasFilters = computed(() => Boolean(query.value.q) || query.value.type !== 'all')
  const isLoaded = computed(() => !list.pending.value && !list.error.value)
  const noResults = computed(() => isLoaded.value && list.total.value === 0)
  // A refresh keeps the previous labels; only the first load and errors hide them.
  const hasData = computed(() => !list.error.value && (isLoaded.value || list.owners.value.length > 0))
  const pageCount = computed(() => Math.max(1, Math.ceil(list.total.value / OWNER_LIST_PAGE_SIZE)))

  // `?page=` beyond the last page (bookmark, deletion elsewhere): go to the last valid one.
  watch([isLoaded, () => list.total.value, () => query.value.page], ([loaded, total, current]) => {
    if (loaded && total > 0 && current > pageCount.value) {
      replaceQuery({ page: pageCount.value > 1 ? String(pageCount.value) : undefined }, false)
    }
  }, { immediate: true })

  const pagination = computed<OwnerListPagination | null>(() => {
    if (!hasData.value) return null
    return {
      showingLabel: `Mostrando ${list.owners.value.length} de ${list.total.value}`,
      pageLabel: `Página ${query.value.page} de ${pageCount.value}`,
      page: query.value.page,
      pageCount: pageCount.value,
    }
  })

  const permissions = usePermissions()
  const session = useSession()
  const canWrite = computed(() => permissions.can('registry.write'))
  const requestedOwner = computed(() => {
    const owner = firstValue(route.query.owner)
    return typeof owner === 'string' && owner ? owner : null
  })

  // Only once the session is known (role loaded), so a slow /auth/me never strips a valid link.
  watch([canWrite, () => session.isAuthenticated.value, requestedOwner], ([allowed, signedIn, owner]) => {
    if (signedIn && !allowed && owner) replaceQuery({ owner: undefined, step: undefined }, false)
  }, { immediate: true })

  function openModal(owner: string, step?: OwnerModalStep) {
    if (!canWrite.value) return
    void navigateTo({ query: { ...route.query, owner, step: step === 'links' ? 'links' : undefined } })
  }

  return {
    search,
    personType,
    page,
    rows: computed(() => list.owners.value.map(owner => toOwnerRowView(owner, highlightId.value))),
    eyebrow: computed(() => {
      if (!hasData.value) return null
      return list.total.value === 1 ? '1 pessoa ou empresa' : `${list.total.value} pessoas e empresas`
    }),
    pagination,
    pending: list.pending,
    error: list.error,
    isEmpty: computed(() => noResults.value && !hasFilters.value),
    isFilterEmpty: computed(() => noResults.value && hasFilters.value),
    refresh: list.refresh,
    canWrite,
    modalOwnerId: computed(() => (canWrite.value ? requestedOwner.value : null)),
    modalStep: computed(() => (firstValue(route.query.step) === 'links' ? 'links' : 'data')),
    openNew: () => openModal('new'),
    openEdit: ownerId => openModal(ownerId),
    openLinks: ownerId => openModal(ownerId, 'links'),
    closeModal: () => replaceQuery({ owner: undefined, step: undefined }, false),
    takeFlash() {
      const message = flash.consume()
      if (!message) return null
      highlightId.value = message.highlightId ?? null
      void list.refresh()
      return message
    },
  }
}
