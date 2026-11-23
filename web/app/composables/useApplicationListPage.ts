import { computed, watch } from 'vue'
import type { ApplicationFilters, ApplicationPage, ApplicationStatus } from '~/types/application'
import type { ProductKind } from '~/types/planner'
import {
  APPLICATION_PERIODS,
  APPLICATION_STATUSES,
  APPLICATION_STATUS_CHIPS,
  applicationRangeLabel,
  buildApplicationSummaryCards,
  DEFAULT_APPLICATION_PERIOD_DAYS,
  firstName,
  hasActiveApplicationFilters,
  monthContextLabel,
  PRODUCT_KINDS,
  productKindOptions,
  toApplicationRowView,
} from '~/utils/applications'
import { APPLICATION_STATUS_LABELS } from '~/utils/labels'
import { stableKey, useResource } from './internal/resource'
import { useApiServices } from './useApiServices'
import { useCurrentWeather } from './useCurrentWeather'
import { usePermissions } from './usePermissions'

export const APPLICATION_PAGE_SIZES: readonly number[] = [10, 20, 50]
const DEFAULT_PAGE_SIZE = 10
const SELECTED_KEY = 'application'

export type ApplicationFilterKey = 'period' | 'field' | 'product' | 'operator' | 'status'

const emptyPage = (): ApplicationPage => ({
  items: [],
  total: 0,
  page: 1,
  pageSize: DEFAULT_PAGE_SIZE,
  facets: { fields: [], operators: [] },
  summary: {
    monthCount: { total: 0, planned: 0, inProgress: 0, completed: 0, cancelled: 0 },
    appliedArea: { areaHa: 0, productiveAreaHa: 0, percent: null },
    withinWindow: { percent: null, within: 0, total: 0 },
    next: null,
  },
})

function firstValue(value: unknown): unknown {
  return Array.isArray(value) ? value[0] : value
}

function textOrNull(value: unknown): string | null {
  const text = firstValue(value)
  return typeof text === 'string' && text ? text : null
}

function positiveInt(value: unknown, fallback: number): number {
  const parsed = Number.parseInt(String(firstValue(value) ?? ''), 10)
  return Number.isFinite(parsed) && parsed > 0 ? parsed : fallback
}

/**
 * Screen "Aplicações" (Figma 63:237). Filters, page and the open detail panel live
 * in the query (`?period=&field=&product=&operator=&status=&page=&size=&application=`,
 * replace), so links and reloads keep the view. The order of the rows is the API's.
 */
export function useApplicationListPage(options: { now?: () => Date } = {}) {
  const { applications } = useApiServices()
  const { can, company } = usePermissions()
  const route = useRoute()
  const now = options.now ?? (() => new Date())
  const weather = useCurrentWeather()

  const query = computed(() => {
    const period = positiveInt(route.query.period, DEFAULT_APPLICATION_PERIOD_DAYS)
    const size = positiveInt(route.query.size, DEFAULT_PAGE_SIZE)
    const product = textOrNull(route.query.product)
    const status = textOrNull(route.query.status)
    return {
      periodDays: APPLICATION_PERIODS.some(item => item.value === period) ? period : DEFAULT_APPLICATION_PERIOD_DAYS,
      fieldId: textOrNull(route.query.field),
      productKind: PRODUCT_KINDS.find(kind => kind === product) ?? null,
      operatorId: textOrNull(route.query.operator),
      status: APPLICATION_STATUSES.find(item => item === status) ?? null,
      page: positiveInt(route.query.page, 1),
      size: APPLICATION_PAGE_SIZES.includes(size) ? size : DEFAULT_PAGE_SIZE,
    }
  })

  const filters = computed<ApplicationFilters>(() => ({
    periodDays: query.value.periodDays,
    fieldId: query.value.fieldId ?? undefined,
    productKind: query.value.productKind ?? undefined,
    operatorId: query.value.operatorId ?? undefined,
    status: query.value.status ?? undefined,
    page: query.value.page,
    pageSize: query.value.size,
  }))

  const resource = useResource<ApplicationPage>(
    () => stableKey('applications:list', filters),
    () => applications.list(filters.value),
    emptyPage,
  )
  const page = computed(() => resource.data.value)
  const code = computed(() => resource.error.value?.code)

  function replaceQuery(changes: Record<string, string | undefined>, resetPage = true) {
    void navigateTo({ query: { ...route.query, ...(resetPage ? { page: undefined } : {}), ...changes } }, { replace: true })
  }

  const queryKeys: Record<ApplicationFilterKey, string> = {
    period: 'period',
    field: 'field',
    product: 'product',
    operator: 'operator',
    status: 'status',
  }

  /** Sets one filter (null or the default period clears it) and goes back to page 1. */
  function setFilter(key: ApplicationFilterKey, value: string | number | null) {
    const clear = value === null || value === '' || (key === 'period' && Number(value) === DEFAULT_APPLICATION_PERIOD_DAYS)
    replaceQuery({ [queryKeys[key]]: clear ? undefined : String(value) })
  }

  function clearFilters() {
    replaceQuery({ period: undefined, field: undefined, product: undefined, operator: undefined, status: undefined })
  }

  const isLoaded = computed(() => !resource.pending.value && !resource.error.value)
  const pageCount = computed(() => Math.max(1, Math.ceil(page.value.total / query.value.size)))

  // `?page=` beyond the last page: go to the last valid one.
  watch([isLoaded, () => page.value.total, () => query.value.page], ([loaded, total, current]) => {
    if (loaded && total > 0 && current > pageCount.value) {
      replaceQuery({ page: pageCount.value > 1 ? String(pageCount.value) : undefined }, false)
    }
  }, { immediate: true })

  const hasFilters = computed(() => hasActiveApplicationFilters(filters.value))
  const noResults = computed(() => isLoaded.value && page.value.total === 0)
  const rows = computed(() => page.value.items.map(toApplicationRowView))

  /** Month indicators exist only after a successful load: while loading or on error the cards show dashes. */
  const summary = computed(() => (isLoaded.value ? page.value.summary : null))

  const selectedId = computed(() => textOrNull(route.query[SELECTED_KEY]))

  return {
    /** "Novembro de 2026 · Fazenda Santa Rita" */
    subtitle: computed(() => monthContextLabel(now(), company.value?.name ?? null)),
    weatherLabel: weather.label,
    weatherUnavailable: weather.isUnavailable,

    filters: computed(() => ({
      periodDays: query.value.periodDays,
      fieldId: query.value.fieldId,
      productKind: query.value.productKind as ProductKind | null,
      operatorId: query.value.operatorId,
      status: query.value.status as ApplicationStatus | null,
    })),
    periodOptions: APPLICATION_PERIODS,
    fieldOptions: computed(() => [
      { value: null, label: 'Todos' },
      ...page.value.facets.fields.map(field => ({ value: field.id, label: `${field.label} · ${field.name}` })),
    ]),
    productOptions: productKindOptions(),
    operatorOptions: computed(() => [
      { value: null, label: 'Todos' },
      ...page.value.facets.operators.map(operator => ({ value: operator.id, label: firstName(operator.name) })),
    ]),
    statusOptions: [
      { value: null, label: 'Todos' },
      ...APPLICATION_STATUSES.map(value => ({ value, label: APPLICATION_STATUS_LABELS[value] })),
    ],
    statusChips: APPLICATION_STATUS_CHIPS,
    hasFilters,
    setFilter,
    clearFilters,

    /** The 4 cards of the top ("No mês", "Área aplicada", "Dentro da janela", "Próxima"). */
    summaryCards: computed(() => buildApplicationSummaryCards(summary.value)),

    rows,
    page: computed({
      get: () => query.value.page,
      set: (next: number) => replaceQuery({ page: next > 1 ? String(next) : undefined }, false),
    }),
    pageSize: computed({
      get: () => query.value.size,
      set: (next: number) => replaceQuery({ size: next === DEFAULT_PAGE_SIZE ? undefined : String(next) }),
    }),
    pageSizeOptions: APPLICATION_PAGE_SIZES.map(value => ({ value, label: String(value) })),
    pagination: computed(() => (page.value.total > 0
      ? { rangeLabel: applicationRangeLabel(query.value.page, query.value.size, page.value.total), page: query.value.page, pageCount: pageCount.value }
      : null)),
    total: computed(() => page.value.total),

    pending: resource.pending,
    error: resource.error,
    isForbidden: computed(() => code.value === 'forbidden'),
    isOutOfScope: computed(() => code.value === 'out_of_scope' || code.value === 'not_found'),
    /** Loaded, no filter active and nothing to list: invite to plan with the agent. */
    isEmpty: computed(() => noResults.value && !hasFilters.value),
    /** Loaded, a filter is active and nothing matches: offer "Limpar filtros". */
    isFilterEmpty: computed(() => noResults.value && hasFilters.value),
    refresh: resource.refresh,

    /** "Planejar com o agente" needs `planner.use`. */
    canPlan: computed(() => can('planner.use')),
    /** "Exportar caderno de campo" has no endpoint yet. */
    exportAction: { disabled: true as const, hint: 'Em breve' },

    /** Open detail panel (`?application=`). */
    selectedId,
    openDetail: (id: string) => replaceQuery({ [SELECTED_KEY]: id }, false),
    closeDetail: () => replaceQuery({ [SELECTED_KEY]: undefined }, false),
  }
}
