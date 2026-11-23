import { computed, onScopeDispose, ref, shallowRef, watch, type ComputedRef, type Ref, type WritableComputedRef } from 'vue'
import { hasCuratedMessage, type ApiError } from '~/services/errors'
import type { UserAccount } from '~/types/admin'
import type { FlashMessage } from '~/types/flash'
import {
  filterToUserFilters,
  toRoleCards,
  toUserRowView,
  USER_LIST_FILTER_LABELS,
  USER_LIST_FILTERS,
  USER_ROLES,
  userFilterCount,
  userRangeLabel,
  usersEyebrow,
  type RoleCardView,
  type UserListFilter,
  type UserRowActionKind,
  type UserRowView,
} from '~/utils/userRow'
import { invitationResentNotice } from '~/utils/invitation'
import { useDeepLinkedRecord } from './internal/deepLink'
import { useApiServices } from './useApiServices'
import { useFlash } from './useFlash'
import { usePermissions } from './usePermissions'
import { useSession } from './useSession'
import { useUserActions } from './useUserActions'
import { useUsers, useUsersSummary } from './useUsers'

export const USER_PAGE_SIZES: readonly number[] = [10, 20, 50]
const DEFAULT_PAGE_SIZE = 10

export type UserListModal = { readonly mode: 'invite' } | {
  readonly mode: 'edit'
  /** The user (listed, or fetched by id when not on the current page/filter); null while loading, not found or on a load error. */
  readonly user: UserAccount | null
  /** The list or the fetch by id is still running, so a missing `user` does not mean "not found" yet. */
  readonly loading: boolean
  /** Fetching by id failed for a reason other than 404: a load error, not "not found". */
  readonly loadError: ApiError | null
}

export interface UserListPage {
  readonly eyebrow: ComputedRef<string | null>
  readonly roleCards: ComputedRef<readonly RoleCardView[]>
  readonly search: Ref<string>
  readonly filter: WritableComputedRef<UserListFilter>
  readonly filterChips: ComputedRef<readonly { value: UserListFilter, label: string, count: number | null }[]>
  readonly page: WritableComputedRef<number>
  readonly pageSize: WritableComputedRef<number>
  readonly pageSizeOptions: readonly { value: number, label: string }[]
  readonly rows: ComputedRef<readonly UserRowView[]>
  readonly pagination: ComputedRef<{ rangeLabel: string, page: number, pageCount: number } | null>
  readonly pending: ComputedRef<boolean>
  readonly error: ComputedRef<ApiError | null>
  readonly isEmpty: ComputedRef<boolean>
  readonly isFilterEmpty: ComputedRef<boolean>
  readonly refresh: () => Promise<void>
  readonly canInvite: ComputedRef<boolean>
  readonly exportAction: { readonly disabled: true, readonly hint: string }
  readonly runAction: (row: UserRowView, kind: UserRowActionKind) => Promise<void>
  readonly actionPending: ComputedRef<string | null>
  readonly notice: ComputedRef<FlashMessage | null>
  readonly dismissNotice: () => void
  readonly modal: ComputedRef<UserListModal | null>
  readonly openInvite: () => void
  readonly closeModal: () => void
  readonly takeFlash: () => FlashMessage | null
}

const ACTION_ERROR_TITLE = 'Não foi possível concluir'
const FORBIDDEN_MESSAGE = 'Você não tem permissão para essa ação.'
const NETWORK_MESSAGE = 'Não foi possível conectar ao servidor. Verifique sua conexão e tente novamente.'
const GENERIC_MESSAGE = 'Tente novamente em instantes.'

function firstValue(value: unknown): unknown {
  return Array.isArray(value) ? value[0] : value
}

function positiveInt(value: unknown, fallback: number): number {
  const parsed = Number.parseInt(String(firstValue(value) ?? ''), 10)
  return Number.isFinite(parsed) && parsed > 0 ? parsed : fallback
}

/** Honest message for a failed menu action; API messages are pt-BR contract text. */
function actionErrorNotice(error: ApiError | null): FlashMessage {
  if (error?.code === 'forbidden' || error?.code === 'out_of_scope') {
    return { kind: 'error', title: FORBIDDEN_MESSAGE, description: error.message }
  }
  if (error && hasCuratedMessage(error.code)) return { kind: 'error', title: ACTION_ERROR_TITLE, description: error.message }
  if (error?.code === 'network_error') return { kind: 'error', title: ACTION_ERROR_TITLE, description: NETWORK_MESSAGE }
  if (error?.code === 'not_found') return { kind: 'error', title: ACTION_ERROR_TITLE, description: 'O usuário ou convite não existe mais.' }
  return { kind: 'error', title: ACTION_ERROR_TITLE, description: GENERIC_MESSAGE }
}

/**
 * Screen "Usuários" (Figma 52:289). Filters live in the query
 * (`?q=&filter=&page=&size=`, replace); the modal of screen 13 in
 * `?invite=1` or `?user=<id>`.
 */
export function useUserListPage(options: { debounceMs?: number, now?: () => Date } = {}): UserListPage {
  const debounceMs = options.debounceMs ?? 300
  const now = options.now ?? (() => new Date())
  const route = useRoute()
  const flash = useFlash()
  const { user: sessionUser } = useSession()
  const { can, company } = usePermissions()
  const actions = useUserActions()
  const highlightId = shallowRef<string | null>(null)
  const actionPending = ref<string | null>(null)
  const notice = shallowRef<FlashMessage | null>(null)

  const managesAll = computed(() => can('users.manage_all'))
  const allowedFilters = computed(() => USER_LIST_FILTERS.filter(item => item !== 'super_admin' || managesAll.value))

  const query = computed(() => {
    const filter = allowedFilters.value.find(item => item === firstValue(route.query.filter)) ?? 'all'
    const size = positiveInt(route.query.size, DEFAULT_PAGE_SIZE)
    return {
      q: String(firstValue(route.query.q) ?? ''),
      filter,
      page: positiveInt(route.query.page, 1),
      size: USER_PAGE_SIZES.includes(size) ? size : DEFAULT_PAGE_SIZE,
    }
  })

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

  const list = useUsers(() => ({
    search: query.value.q || undefined,
    ...filterToUserFilters(query.value.filter),
    page: query.value.page,
    pageSize: query.value.size,
  }))
  const summary = useUsersSummary()

  const { users: userService } = useApiServices()
  const deepLink = useDeepLinkedRecord<UserAccount>({
    id: () => {
      const id = firstValue(route.query.user)
      return typeof id === 'string' && id ? id : null
    },
    find: id => list.users.value.find(item => item.id === id) ?? null,
    listSettled: () => !list.pending.value,
    load: id => userService.get(id),
  })

  const hasFilters = computed(() => Boolean(query.value.q) || query.value.filter !== 'all')
  const isLoaded = computed(() => !list.pending.value && !list.error.value)
  const noResults = computed(() => isLoaded.value && list.total.value === 0)
  const hasData = computed(() => !list.error.value && (isLoaded.value || list.users.value.length > 0))
  const pageCount = computed(() => Math.max(1, Math.ceil(list.total.value / query.value.size)))

  // `?page=` beyond the last page: go to the last valid one.
  watch([isLoaded, () => list.total.value, () => query.value.page], ([loaded, total, current]) => {
    if (loaded && total > 0 && current > pageCount.value) {
      replaceQuery({ page: pageCount.value > 1 ? String(pageCount.value) : undefined }, false)
    }
  }, { immediate: true })

  async function refreshAll(): Promise<void> {
    await Promise.all([list.refresh(), summary.refresh()])
  }

  function openModal(changes: Record<string, string | undefined>) {
    void navigateTo({ query: { ...route.query, invite: undefined, user: undefined, ...changes } })
  }

  async function runAction(row: UserRowView, kind: UserRowActionKind): Promise<void> {
    const account = list.users.value.find(item => item.id === row.id)
    if (!account) return
    if (kind === 'edit_access') {
      openModal({ user: account.id })
      return
    }
    const action = row.actions.find(item => item.kind === kind)
    if (!action || action.disabledHint) return

    actionPending.value = account.id
    notice.value = null
    try {
      const outcome = await performAction(account, kind)
      notice.value = outcome
      if (outcome.kind === 'success') {
        highlightId.value = kind === 'cancel_invitation' ? null : account.id
        await refreshAll()
      }
    }
    finally {
      actionPending.value = null
    }
  }

  async function performAction(account: UserAccount, kind: Exclude<UserRowActionKind, 'edit_access'>): Promise<FlashMessage> {
    const invitationId = account.invitation?.id
    switch (kind) {
      case 'deactivate':
        return (await actions.update(account.id, { status: 'inactive' }))
          ? { kind: 'success', title: `${account.name} inativado`, description: 'O acesso ao sistema foi suspenso.' }
          : actionErrorNotice(actions.updateError.value)
      case 'activate':
        return (await actions.update(account.id, { status: 'active' }))
          ? { kind: 'success', title: `${account.name} reativado`, description: 'O acesso ao sistema foi liberado.' }
          : actionErrorNotice(actions.updateError.value)
      case 'resend_invitation':
        if (!invitationId) return actionErrorNotice(null)
      {
        const emailStatus = await actions.resendInvitation(invitationId)
        return emailStatus ? invitationResentNotice(account.email, emailStatus) : actionErrorNotice(actions.resendError.value)
      }
      case 'cancel_invitation':
        if (!invitationId) return actionErrorNotice(null)
        return (await actions.cancelInvitation(invitationId))
          ? { kind: 'success', title: 'Convite cancelado', description: `O link enviado para ${account.email} não vale mais.` }
          : actionErrorNotice(actions.cancelError.value)
    }
  }

  return {
    eyebrow: computed(() => {
      const value = summary.summary.value
      if (!value || summary.error.value) return null
      return usersEyebrow(value, managesAll.value, company.value?.name ?? null)
    }),
    roleCards: computed(() => toRoleCards(
      USER_ROLES.filter(role => role !== 'super_admin' || managesAll.value),
      summary.error.value ? null : summary.summary.value,
    )),
    search,
    filter: computed<UserListFilter>({
      get: () => query.value.filter,
      set: value => replaceQuery({ filter: value === 'all' ? undefined : value }),
    }),
    filterChips: computed(() => allowedFilters.value.map(value => ({
      value,
      label: USER_LIST_FILTER_LABELS[value],
      count: userFilterCount(value, summary.summary.value),
    }))),
    page: computed<number>({
      get: () => query.value.page,
      set: value => replaceQuery({ page: value > 1 ? String(value) : undefined }, false),
    }),
    pageSize: computed<number>({
      get: () => query.value.size,
      set: value => replaceQuery({ size: value === DEFAULT_PAGE_SIZE ? undefined : String(value) }),
    }),
    pageSizeOptions: USER_PAGE_SIZES.map(value => ({ value, label: `${value} por página` })),
    rows: computed(() => list.users.value.map(account => toUserRowView(account, {
      now: now(),
      selfId: sessionUser.value?.id ?? null,
      highlightId: highlightId.value,
    }))),
    pagination: computed(() => {
      if (!hasData.value) return null
      return {
        rangeLabel: userRangeLabel(query.value.page, query.value.size, list.users.value.length, list.total.value),
        page: query.value.page,
        pageCount: pageCount.value,
      }
    }),
    pending: list.pending,
    error: list.error,
    isEmpty: computed(() => noResults.value && !hasFilters.value),
    isFilterEmpty: computed(() => noResults.value && hasFilters.value),
    refresh: refreshAll,
    canInvite: computed(() => can('users.manage')),
    exportAction: { disabled: true, hint: 'Em breve' },
    runAction,
    actionPending: computed(() => actionPending.value),
    notice: computed(() => notice.value),
    dismissNotice: () => {
      notice.value = null
    },
    modal: computed<UserListModal | null>(() => {
      if (firstValue(route.query.invite) === '1') return { mode: 'invite' }
      const userId = firstValue(route.query.user)
      if (typeof userId !== 'string' || !userId) return null
      const { record, loading, loadError } = deepLink.resolve(userId)
      return { mode: 'edit', user: record, loading, loadError }
    }),
    openInvite: () => openModal({ invite: '1' }),
    closeModal: () => replaceQuery({ invite: undefined, user: undefined }, false),
    takeFlash() {
      const message = flash.consume()
      if (!message) return null
      highlightId.value = message.highlightId ?? null
      notice.value = message
      void refreshAll()
      return message
    },
  }
}
