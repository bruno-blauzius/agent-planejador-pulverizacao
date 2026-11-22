import { computed, onScopeDispose, ref, watch } from 'vue'
import type { CompaniesSummary, Company } from '~/types/admin'
import type { Page } from '~/types/pagination'
import { buildCompanyPagination, buildCompanyStatCards, toCompanyRowView } from '~/utils/company'
import { stableKey, useResource } from './internal/resource'
import { useApiServices } from './useApiServices'
import { usePermissions } from './usePermissions'

export interface UseCompanyListPageOptions {
  debounceMs?: number
  now?: () => Date
}

export const COMPANY_PAGE_SIZES = [10, 20, 50] as const
const DEFAULT_PAGE_SIZE = 10

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
  watch(search, (text) => {
    clearTimeout(debounce)
    const apply = () => replaceQuery({ q: text.trim() || undefined })
    if (debounceMs <= 0) apply()
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

  const hasSearch = computed(() => Boolean(query.value.q))
  const noRows = computed(() => isLoaded.value && list.data.value.items.length === 0)

  return {
    statCards: computed(() => buildCompanyStatCards(summary.data.value, now())),
    summaryError: summary.error,

    search,
    rows: computed(() => list.data.value.items.map(company => toCompanyRowView(company, now()))),
    pending: list.pending,
    error: list.error,
    isForbidden: computed(() => list.error.value?.code === 'forbidden' || summary.error.value?.code === 'forbidden'),
    isEmpty: computed(() => noRows.value && !hasSearch.value),
    isEmptySearch: computed(() => noRows.value && hasSearch.value),
    clearSearch() {
      search.value = ''
      clearTimeout(debounce)
      replaceQuery({ q: undefined })
    },
    async refresh() {
      await Promise.all([list.refresh(), summary.refresh()])
    },

    pagination,
    page,
    pageSize,
    pageSizeOptions: COMPANY_PAGE_SIZES.map(value => ({ value, label: `${value} por página` })),

    canCreate: computed(() => can('companies.manage')),
  }
}
