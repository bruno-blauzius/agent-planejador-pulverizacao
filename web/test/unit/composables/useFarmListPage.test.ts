import { afterEach, describe, expect, it, vi } from 'vitest'
import { useFarmListPage } from '~/composables/useFarmListPage'
import { useFlash } from '~/composables/useFlash'
import type { ApiServices } from '~/services/ports'
import type { FarmSummary } from '~/types/farm'
import { farmCreatedFlash } from '~/utils/farmCard'
import { createTestApi } from '../../support/mockApi'
import { flushPromises, installNuxtFakes, type NuxtFakes } from '../../support/nuxtFakes'

afterEach(() => vi.unstubAllGlobals())

const NOW = () => new Date('2026-11-21T12:00:00Z')

function setup(token?: string | null): NuxtFakes {
  const fakes = installNuxtFakes(createTestApi(token).api)
  fakes.route.path = '/farms'
  fakes.route.fullPath = '/farms'
  return fakes
}

describe('useFarmListPage', () => {
  it('builds the summary cards, leaving the planner card unmodeled', async () => {
    setup()
    const page = useFarmListPage({ now: NOW })
    expect(page.pending.value).toBe(true)
    await flushPromises()

    expect(page.summary.value).toEqual({
      farmCount: 4,
      createdThisMonth: 1,
      totalAreaLabel: '1.284 ha',
      productiveAreaLabel: '932 ha',
      fieldCount: 16,
      cropCount: 4,
      suitableWindowsToday: null,
    })
  })

  it('lists tabs with counts and filters the cards by the tab in the query', async () => {
    const fakes = setup()
    fakes.route.query = { tab: 'pending' }
    const page = useFarmListPage({ now: NOW })
    await flushPromises()

    expect(page.tabs.value).toEqual([
      { value: 'all', label: 'Todas', count: 4 },
      { value: 'coffee', label: 'Café', count: 3 },
      { value: 'grains', label: 'Grãos', count: 1 },
      { value: 'pending', label: 'Com pendências', count: 1 },
    ])
    expect(page.tab.value).toBe('pending')
    expect(page.cards.value.map(card => card.name)).toEqual(['Fazenda Ouro Verde'])
  })

  it('writes tab and view to the query with replace, omitting defaults', async () => {
    const fakes = setup()
    const page = useFarmListPage({ now: NOW })
    await flushPromises()

    page.view.value = 'map'
    page.tab.value = 'grains'
    expect(page.cards.value.map(card => card.id)).toEqual(['farm-recanto'])
    page.tab.value = 'all'

    expect(fakes.route.query).toEqual({ view: 'map' })
    expect(fakes.navigations.at(-1)).toEqual({ target: { query: { view: 'map', tab: undefined } }, options: { replace: true } })
  })

  it('falls back to defaults for unknown query values', async () => {
    const fakes = setup()
    fakes.route.query = { tab: 'hack', view: ['list', 'map'] }
    const page = useFarmListPage({ now: NOW })

    expect(page.tab.value).toBe('all')
    expect(page.view.value).toBe('list')
  })

  it('maps each farm to a card with geometry for the map', async () => {
    setup()
    const page = useFarmListPage({ now: NOW })
    await flushPromises()

    const card = page.cards.value.find(item => item.id === 'farm-santa-rita')
    expect(card).toMatchObject({
      locationLabel: 'Guaxupé · MG',
      condition: { value: 'suitable', label: 'Apta' },
      areaLabel: '486 ha',
      fieldCountLabel: '9 talhões',
      cropLabel: 'Café arábica · Milho safrinha',
      owners: [{ initials: 'BB', name: 'Bruno Blauzius' }, { initials: 'MR', name: 'Marina Rezende' }],
      headline: '2 janelas aptas hoje',
      to: '/farms/farm-santa-rita',
      forecastPoint: { lat: -21.3, lng: -46.71 },
    })
    expect(card?.perimeter?.type).toBe('Polygon')
  })

  it('separates a failed load from empty states', async () => {
    setup(null)
    const page = useFarmListPage({ now: NOW })
    await flushPromises()

    expect(page.error.value?.code).toBe('unauthorized')
    expect(page.isEmpty.value).toBe(false)
    expect(page.isTabEmpty.value).toBe(false)
  })
})

describe('useFlash', () => {
  it('delivers the "farm created" toast once', () => {
    setup()
    useFlash().show(farmCreatedFlash({ name: 'Fazenda Santa Rita', totalAreaHa: 486.2 }))

    const flash = useFlash()
    expect(flash.consume()).toEqual({
      kind: 'success',
      title: 'Fazenda Santa Rita criada',
      description: 'Área de 486 ha cadastrada.',
    })
    expect(flash.consume()).toBeNull()
  })

  it('uses the farm name as is, without a prefix', () => {
    expect(farmCreatedFlash({ name: 'Sítio Boa Esperança', totalAreaHa: 212 }).title).toBe('Sítio Boa Esperança criada')
  })
})

const recanto: FarmSummary = {
  companyId: 'company-recanto',
  id: 'farm-1',
  name: 'Fazenda Recanto',
  code: null,
  city: 'Alfenas',
  state: 'MG',
  status: 'active',
  productionType: 'grains',
  totalAreaHa: 188,
  productiveAreaHa: 95,
  fieldCount: 1,
  crops: [{ code: 'soybean', name: 'Soja' }],
  owners: [],
  sprayOutlook: null,
  forecastPoint: null,
  perimeter: null,
  createdAt: new Date('2025-01-10T12:00:00Z'),
}

/** Mock API whose farm list answers with the given farms. */
function apiListing(farms: FarmSummary[]): ApiServices {
  const api = createTestApi().api
  return { ...api, farms: { ...api.farms, list: async () => ({ items: farms, total: farms.length, page: 1, pageSize: 100 }) } }
}

describe('useFarmListPage summaryCards', () => {
  const windowsCard = { key: 'windows', label: 'Janelas aptas hoje', value: '—', caption: 'Disponível com o planejador', highlight: true }

  it('shows placeholders, not zeros, during the first load', () => {
    setup()
    const { summaryCards } = useFarmListPage({ now: NOW })

    expect(summaryCards.value.map(card => [card.value, card.caption])).toEqual([
      ['—', null], ['—', null], ['—', null], ['—', 'Disponível com o planejador'],
    ])
  })

  it('shows placeholders on error', async () => {
    setup(null)
    const { summaryCards } = useFarmListPage({ now: NOW })
    await flushPromises()

    expect(summaryCards.value.slice(0, 3).every(card => card.value === '—' && card.caption === null)).toBe(true)
  })

  it('fills the cards on success, as in the Figma', async () => {
    setup()
    const { summaryCards } = useFarmListPage({ now: NOW })
    await flushPromises()

    expect(summaryCards.value).toEqual([
      { key: 'farms', label: 'Fazendas', value: '4', caption: '+1 este mês', highlight: false },
      { key: 'area', label: 'Área total', value: '1.284 ha', caption: '932 ha produtivos', highlight: false },
      { key: 'fields', label: 'Talhões', value: '16', caption: '4 culturas diferentes', highlight: false },
      windowsCard,
    ])
  })

  it('handles singular and empty captions', async () => {
    installNuxtFakes(apiListing([recanto]))
    const single = useFarmListPage({ now: NOW })
    await flushPromises()
    expect(single.summaryCards.value.map(card => card.caption)).toEqual([null, '95 ha produtivos', '1 cultura', 'Disponível com o planejador'])
    vi.unstubAllGlobals()

    installNuxtFakes(apiListing([]))
    const none = useFarmListPage({ now: NOW })
    await flushPromises()
    expect(none.summaryCards.value.slice(0, 3).map(card => [card.value, card.caption])).toEqual([
      ['0', null], ['0 ha', '0 ha produtivos'], ['0', null],
    ])
  })
})
