import { afterEach, describe, expect, it, vi } from 'vitest'
import { useFarmDetailPage } from '~/composables/useFarmDetailPage'
import { createTestApi } from '../../support/mockApi'
import { flushPromises, installNuxtFakes, type NuxtFakes } from '../../support/nuxtFakes'
import { SEED_LOGINS } from '../../support/session'
import { AUTH_TOKEN_COOKIE } from '~/composables/useAuthToken'
import { useSession } from '~/composables/useSession'
import { createMockServices } from '~/services/mock/mockServices'
import { createSeed, TEST_PASSWORD } from '~/services/mock/seed'

afterEach(() => vi.unstubAllGlobals())

// Seed date of the mock: "lastApplicationDaysAgo" is relative to it.
const NOW = () => new Date('2026-10-01T12:00:00Z')

function setup(token?: string | null): NuxtFakes {
  const fakes = installNuxtFakes(createTestApi(token).api)
  fakes.route.path = '/farms/farm-santa-rita'
  fakes.route.fullPath = '/farms/farm-santa-rita'
  return fakes
}

describe('useFarmDetailPage', () => {
  it('builds header, tabs and summary of the farm', async () => {
    setup()
    const page = useFarmDetailPage('farm-santa-rita', { now: NOW })
    expect(page.pending.value).toBe(true)
    expect(page.summary.value.fieldCount).toBe('—')
    await flushPromises()

    expect(page.header.value.title).toBe('Fazenda Santa Rita')
    expect(page.header.value.breadcrumbs).toEqual([{ label: 'Fazendas', to: '/farms' }, { label: 'Fazenda Santa Rita' }])
    expect(page.tabs.value).toEqual([
      { value: 'overview', label: 'Visão geral', count: null, disabled: false },
      { value: 'fields', label: 'Áreas e talhões', count: 9, disabled: false },
      { value: 'terrain', label: 'Terreno', count: null, disabled: false },
      { value: 'owners', label: 'Proprietários', count: 2, disabled: false },
      { value: 'applications', label: 'Aplicações', count: null, disabled: true },
      { value: 'windows', label: 'Janelas de pulverização', count: null, disabled: true },
    ])
    expect(page.summary.value).toEqual({ blockCount: '3', fieldCount: '9', productiveAreaLabel: '412 ha' })
    expect(page.isEmpty.value).toBe(false)
  })

  it('keeps the fields tab when a disabled tab is chosen', () => {
    const fakes = setup()
    const page = useFarmDetailPage('farm-santa-rita', { now: NOW })
    page.tab.value = 'applications'
    expect(page.tab.value).toBe('fields')
    fakes.route.query = { tab: 'windows' }
    expect(page.tab.value).toBe('fields')
  })

  it('switches to the overview and owners tabs through the query', async () => {
    const fakes = setup()
    const page = useFarmDetailPage('farm-santa-rita', { now: NOW })
    await flushPromises()

    page.tab.value = 'overview'
    await flushPromises()
    expect(fakes.route.query).toEqual({ tab: 'overview' })
    expect(page.tab.value).toBe('overview')
    expect(page.overview.value.map(section => section.title)).toEqual(['Identificação', 'Localização', 'Áreas e culturas'])
    expect(page.overview.value[0]?.facts[0]).toEqual({ label: 'Nome', value: 'Fazenda Santa Rita', isEmpty: false })

    page.tab.value = 'owners'
    await flushPromises()
    expect(page.tab.value).toBe('owners')
    expect(page.ownerRows.value).toHaveLength(2)
    expect(page.ownerRows.value[0]?.to).toMatch(/^\/owners\?owner=/)

    page.tab.value = 'fields'
    await flushPromises()
    expect(fakes.route.query).toEqual({})
  })

  it('lists blocks with rows sharing colors with the map and the legend', async () => {
    setup()
    const page = useFarmDetailPage('farm-santa-rita', { now: NOW })
    await flushPromises()

    expect(page.blocks.value.map(block => [block.name, block.areaLabel, block.fields.length])).toEqual([
      ['Área Centro', '148 ha', 3],
      ['Área Norte', '162 ha', 4],
      ['Área Sul', '102 ha', 2],
    ])
    expect(page.legend.value).toEqual({
      kind: 'crop',
      items: [
        { colorIndex: 0, label: 'Arara' },
        { colorIndex: 1, label: 'Catuaí 144' },
        { colorIndex: 2, label: 'Milho safrinha' },
        { colorIndex: 3, label: 'Mundo Novo' },
      ],
    })
    const row = page.blocks.value[1]!.fields[0]!
    expect(row).toMatchObject({ name: 'Talhão 1', subtitle: 'Catuaí 144 · 42,1 ha', colorIndex: 1, shortLabel: 'T1' })
    expect(page.mapFeatures.value.find(feature => feature.id === row.id)).toMatchObject({ cropIndex: 1, status: 'suitable' })
  })

  it('switches map color, basemap and legend by mode', async () => {
    setup()
    const page = useFarmDetailPage('farm-santa-rita', { now: NOW })
    await flushPromises()

    page.mapMode.value = 'status'
    expect(page.mapColorBy.value).toBe('status')
    expect(page.legend.value).toEqual({
      kind: 'status',
      items: [
        { value: 'suitable', label: 'Apta' },
        { value: 'attention', label: 'Atenção' },
        { value: 'unsuitable', label: 'Inapta' },
        { value: 'rain', label: 'Chuva' },
      ],
    })
    page.mapMode.value = 'satellite'
    expect(page.mapBasemap.value).toBe('satellite')
    expect(page.mapColorBy.value).toBe('crop')
  })

  it('selects a field through the query, reopening its block and opening the popover', async () => {
    const fakes = setup()
    const page = useFarmDetailPage('farm-santa-rita', { now: NOW })
    await flushPromises()

    page.toggleBlock('Área Norte')
    expect(page.blocks.value[1]!.expanded).toBe(false)

    page.selectField('field-str-3')
    expect(fakes.route.query).toEqual({ field: 'field-str-3' })
    expect(page.blocks.value[1]!.expanded).toBe(true)
    expect(page.blocks.value[1]!.fields.find(row => row.selected)?.name).toBe('Talhão 3')
    expect(page.popover.value).toMatchObject({
      name: 'Talhão 3',
      subtitle: 'Área Norte · Catuaí 144',
      areaLabel: '38,6 ha',
      plantingLabel: 'mar/2019',
      lastApplicationLabel: 'há 12 dias',
      todayWindowLabel: null,
      editFieldTo: '/farms/farm-santa-rita/fields/field-str-3/edit',
    })
    expect(page.addFieldToBlock.value).toEqual({
      label: 'Adicionar talhão à Área Norte',
      to: '/farms/farm-santa-rita/fields/new?block=%C3%81rea%20Norte',
    })

    page.selectField(null)
    expect(fakes.route.query).toEqual({})
    expect(page.popover.value).toBeNull()
    expect(page.addFieldToBlock.value.label).toBe('Adicionar talhão à Área Centro')
  })

  it('ignores an unknown field in the query', async () => {
    const fakes = setup()
    fakes.route.query = { field: 'field-other' }
    const page = useFarmDetailPage('farm-santa-rita', { now: NOW })
    await flushPromises()
    expect(page.selectedFieldId.value).toBeNull()
    expect(page.popover.value).toBeNull()
  })

  it('reports an unknown farm as not found', async () => {
    setup()
    const page = useFarmDetailPage('farm-missing', { now: NOW })
    await flushPromises()
    expect(page.isNotFound.value).toBe(true)
    expect(page.error.value?.code).toBe('not_found')
    expect(page.isEmpty.value).toBe(false)
    expect(page.summary.value.blockCount).toBe('—')
  })

  it('shows a farm outside the operator scope as forbidden, not as not found', async () => {
    // The seed releases Carlos only farm-santa-rita; narrowing it makes that farm out of scope (403).
    const seed = createSeed()
    const carlos = seed.users.find(user => user.email === SEED_LOGINS.operator)!
    carlos.farmScope = { mode: 'selected', farmIds: ['farm-ouro-verde'] }
    const fakes = installNuxtFakes(nuxt => createMockServices({
      latencyMs: 0,
      seed,
      getAccessToken: () => nuxt.cookie(AUTH_TOKEN_COOKIE).value,
    }))
    await useSession().login({ identifier: SEED_LOGINS.operator, password: TEST_PASSWORD, rememberMe: false })
    fakes.route.path = '/farms/farm-santa-rita'

    const page = useFarmDetailPage('farm-santa-rita', { now: NOW })
    await flushPromises()

    expect(page.error.value?.code).toBe('out_of_scope')
    expect(page.isForbidden.value).toBe(true)
    expect(page.isNotFound.value).toBe(false)
  })

  it('does not flag forbidden for a visible farm or an unknown one', async () => {
    setup()
    const visible = useFarmDetailPage('farm-santa-rita', { now: NOW })
    const missing = useFarmDetailPage('farm-missing', { now: NOW })
    await flushPromises()
    expect(visible.isForbidden.value).toBe(false)
    expect(missing.isForbidden.value).toBe(false)
  })

  it('exposes the actions without endpoint as disabled', async () => {
    setup()
    const page = useFarmDetailPage('farm-santa-rita', { now: NOW })
    await flushPromises()
    expect(page.newFieldTo.value).toBe('/farms/farm-santa-rita/fields/new')
    expect(page.newArea).toEqual({ disabled: true, hint: 'Em breve' })
  })
})
