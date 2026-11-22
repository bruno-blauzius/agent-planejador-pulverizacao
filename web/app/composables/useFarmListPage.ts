import { computed, type ComputedRef, type WritableComputedRef } from 'vue'
import type { ApiError } from '~/services/errors'
import {
  countCreatedInMonth,
  countFarmTabs,
  FARM_LIST_TAB_LABELS,
  FARM_LIST_TABS,
  matchesFarmTab,
  summarizeFarms,
  type FarmListTab,
} from '~/utils/farm'
import { toFarmCardView, type FarmCardView } from '~/utils/farmCard'
import { formatHectares } from '~/utils/format'
import { useFarms } from './useFarms'
import { usePermissions } from './usePermissions'

export type FarmListView = 'grid' | 'list' | 'map'
const FARM_LIST_VIEWS: readonly FarmListView[] = ['grid', 'list', 'map']

/** Up to this many farms are loaded at once; tabs and counts are computed on it. */
export const FARM_LIST_PAGE_SIZE = 100

export interface FarmListSummaryView {
  readonly farmCount: number
  /** "+N este mês". */
  readonly createdThisMonth: number
  /** "1.284 ha" */
  readonly totalAreaLabel: string
  /** "982 ha" (UI appends "produtivos"). */
  readonly productiveAreaLabel: string
  readonly fieldCount: number
  readonly cropCount: number
  /**
   * "Janelas aptas hoje" is not modeled yet (spray-planner). Always null: the UI
   * shows "—" and "Disponível com o planejador", never an invented number.
   */
  readonly suitableWindowsToday: null
}

/** Ready-to-render summary card (Figma 7:117). */
export interface SummaryCardView {
  readonly key: 'farms' | 'area' | 'fields' | 'windows'
  readonly label: string
  /** "—" during the first load, on error, and for data not modeled yet. */
  readonly value: string
  readonly caption: string | null
  /** Dark highlighted card ("Janelas aptas hoje"). */
  readonly highlight: boolean
}

const PLACEHOLDER = '—'

function cropCaption(count: number): string | null {
  if (count === 0) return null
  return count === 1 ? '1 cultura' : `${count} culturas diferentes`
}

function buildSummaryCards(summary: FarmListSummaryView, hasData: boolean): SummaryCardView[] {
  const value = (text: string) => (hasData ? text : PLACEHOLDER)
  const caption = (text: string | null) => (hasData ? text : null)
  return [
    {
      key: 'farms',
      label: 'Fazendas',
      value: value(String(summary.farmCount)),
      caption: caption(summary.createdThisMonth > 0 ? `+${summary.createdThisMonth} este mês` : null),
      highlight: false,
    },
    {
      key: 'area',
      label: 'Área total',
      value: value(summary.totalAreaLabel),
      caption: caption(`${summary.productiveAreaLabel} produtivos`),
      highlight: false,
    },
    {
      key: 'fields',
      label: 'Talhões',
      value: value(String(summary.fieldCount)),
      caption: caption(cropCaption(summary.cropCount)),
      highlight: false,
    },
    // Not modeled yet (spray-planner): never show an invented number.
    { key: 'windows', label: 'Janelas aptas hoje', value: PLACEHOLDER, caption: 'Disponível com o planejador', highlight: true },
  ]
}

export interface FarmListPage {
  /**
   * `registry.write` of the session (ADR 0013): hide "Nova fazenda", "Novo talhão",
   * "Editar" and similar actions when false. Presentation only; the API answers 403.
   */
  readonly canWrite: ComputedRef<boolean>
  readonly tab: WritableComputedRef<FarmListTab>
  readonly view: WritableComputedRef<FarmListView>
  readonly tabs: ComputedRef<readonly { value: FarmListTab, label: string, count: number }[]>
  readonly cards: ComputedRef<readonly FarmCardView[]>
  readonly summary: ComputedRef<FarmListSummaryView>
  /** The four cards, with "—" instead of false zeros while loading or on error. */
  readonly summaryCards: ComputedRef<readonly SummaryCardView[]>
  /** Farms in the account; when greater than `cards` on "all", show "Mostrando N de total". */
  readonly total: ComputedRef<number>
  readonly pending: ComputedRef<boolean>
  readonly error: ComputedRef<ApiError | null>
  /** No farm registered at all (CTA for the first farm). */
  readonly isEmpty: ComputedRef<boolean>
  /** Farms exist, but none in the selected tab. */
  readonly isTabEmpty: ComputedRef<boolean>
  readonly refresh: () => Promise<void>
}

function oneOf<T extends string>(value: unknown, allowed: readonly T[], fallback: T): T {
  const candidate = Array.isArray(value) ? value[0] : value
  return allowed.find(item => item === candidate) ?? fallback
}

/**
 * Screen "Suas fazendas". `tab` and `view` live in the query (`?tab=&view=`,
 * replaced without history entries; defaults are omitted from the URL).
 */
export function useFarmListPage(options: { now?: () => Date } = {}): FarmListPage {
  const now = options.now ?? (() => new Date())
  const route = useRoute()
  const list = useFarms({ pageSize: FARM_LIST_PAGE_SIZE })
  const permissions = usePermissions()

  function queryParam<T extends string>(name: string, allowed: readonly T[], fallback: T): WritableComputedRef<T> {
    return computed({
      get: () => oneOf(route.query[name], allowed, fallback),
      set: (value) => {
        const query = { ...route.query, [name]: value === fallback ? undefined : value }
        void navigateTo({ query }, { replace: true })
      },
    })
  }

  const tab = queryParam<FarmListTab>('tab', FARM_LIST_TABS, 'all')
  const view = queryParam<FarmListView>('view', FARM_LIST_VIEWS, 'grid')
  const counts = computed(() => countFarmTabs(list.farms.value))
  const visibleFarms = computed(() => list.farms.value.filter(farm => matchesFarmTab(farm, tab.value)))

  const summary = computed<FarmListSummaryView>(() => {
    const totals = summarizeFarms(list.farms.value)
    return {
      farmCount: totals.farmCount,
      createdThisMonth: countCreatedInMonth(list.farms.value, now()),
      totalAreaLabel: formatHectares(totals.totalAreaHa, 0),
      productiveAreaLabel: formatHectares(totals.productiveAreaHa, 0),
      fieldCount: totals.fieldCount,
      cropCount: totals.cropCount,
      suitableWindowsToday: null,
    }
  })

  const isLoaded = computed(() => !list.pending.value && !list.error.value)
  // A refresh keeps the previous numbers; only the first load and errors hide them.
  const hasSummaryData = computed(() => !list.error.value && (isLoaded.value || list.farms.value.length > 0))

  return {
    tab,
    view,
    tabs: computed(() => FARM_LIST_TABS.map(value => ({ value, label: FARM_LIST_TAB_LABELS[value], count: counts.value[value] }))),
    cards: computed(() => visibleFarms.value.map(toFarmCardView)),
    summary,
    summaryCards: computed(() => buildSummaryCards(summary.value, hasSummaryData.value)),
    total: list.total,
    pending: list.pending,
    error: list.error,
    isEmpty: list.isEmpty,
    isTabEmpty: computed(() => isLoaded.value && list.farms.value.length > 0 && visibleFarms.value.length === 0),
    refresh: list.refresh,
    canWrite: computed(() => permissions.can('registry.write')),
  }
}
