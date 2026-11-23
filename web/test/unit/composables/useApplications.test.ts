import { ref } from 'vue'
import { describe, expect, it } from 'vitest'
import { useApplicationDetail } from '~/composables/useApplicationDetail'
import { useApplicationListPage } from '~/composables/useApplicationListPage'
import { AUTH_TOKEN_COOKIE } from '~/composables/useAuthToken'
import { useCurrentWeather } from '~/composables/useCurrentWeather'
import { usePlannerContext } from '~/composables/usePlannerContext'
import { useSession } from '~/composables/useSession'
import { ApiError } from '~/services/errors'
import { createMockServices } from '~/services/mock/mockServices'
import type { ApiServices } from '~/services/ports'
import { TEST_PASSWORD } from '~/services/mock/seed'
import { flushPromises, installNuxtFakes, type NuxtFakes } from '../../support/nuxtFakes'
import { SCENARIO, signInWithPlannerFaults } from '../../support/planner'
import { SEED_LOGINS, SEED_NOW, signInAs } from '../../support/session'

const now = () => SEED_NOW

async function setupList(login: string = SEED_LOGINS.admin, query: Record<string, unknown> = {}) {
  const fakes = await signInAs(login)
  fakes.route.path = '/applications'
  fakes.route.query = query
  const page = useApplicationListPage({ now })
  await flushPromises()
  return { page, fakes }
}

/** Mock services with the application list replaced by a failing one (403, 5xx...). */
async function setupFailingList(error: ApiError): Promise<ReturnType<typeof useApplicationListPage>> {
  const fakes: NuxtFakes = installNuxtFakes((nuxt) => {
    const real = createMockServices({ latencyMs: 0, getAccessToken: () => nuxt.cookie(AUTH_TOKEN_COOKIE).value, now })
    const failing: ApiServices = {
      ...real,
      applications: { ...real.applications, list: () => Promise.reject(error) },
    }
    return failing
  })
  await useSession().login({ identifier: SEED_LOGINS.admin, password: TEST_PASSWORD, rememberMe: false })
  fakes.route.path = '/applications'
  const page = useApplicationListPage({ now })
  await flushPromises()
  return page
}

describe('useApplicationListPage', () => {
  it('builds header, filters and the Figma rows', async () => {
    const { page } = await setupList()

    expect(page.subtitle.value).toBe('Novembro de 2026 · Fazenda Santa Rita')
    expect(page.weatherLabel.value).toBe('Guaxupé · 24 °C · vento 7 km/h')
    expect(page.filters.value).toEqual({ periodDays: 30, fieldId: null, productKind: null, operatorId: null, status: null })
    expect(page.periodOptions.map(option => option.label)).toEqual(['Últimos 7 dias', 'Últimos 30 dias', 'Últimos 90 dias'])
    expect(page.fieldOptions.value[0]).toEqual({ value: null, label: 'Todos' })
    expect(page.fieldOptions.value.map(option => option.label)).toContain('T5 · Talhão 5')
    expect(page.operatorOptions.value.map(option => option.label)).toEqual(['Todos', 'Carlos', 'João'])
    expect(page.productOptions.map(option => option.label)).toEqual(['Todos', 'Fungicida', 'Inseticida', 'Herbicida', 'Foliar', 'Outro'])
    expect(page.statusOptions.map(option => option.label)).toEqual(['Todos', 'Planejada', 'Em andamento', 'Concluída', 'Cancelada'])

    expect(page.rows.value.map(row => [row.dateLabel, row.fieldCode, row.status.label])).toEqual([
      ['Qui 26/11', 'T3', 'Planejada'],
      ['Sáb 28/11', 'T7', 'Planejada'],
      ['Sáb 21/11', 'T5', 'Concluída'],
      ['Qui 19/11', 'T1', 'Concluída'],
      ['Ter 17/11', 'T4', 'Cancelada'],
      ['Dom 15/11', 'T8', 'Concluída'],
    ])
    expect(page.rows.value.map(row => row.conditions)).toEqual([
      { kind: 'values', parts: ['3,1 °C', '5 km/h'] },
      { kind: 'live', label: 'Previsão ao vivo' },
      { kind: 'values', parts: ['4,2 °C', '6 km/h'] },
      { kind: 'values', parts: ['6,8 °C', '9 km/h'] },
      { kind: 'rain', label: 'Chuva 18 mm' },
      { kind: 'values', parts: ['9,1 °C', '11 km/h'] },
    ])
    expect(page.rows.value[2]).toMatchObject({ doseLabel: '0,3 L/ha · calda 180 L/ha', areaLabel: '56,3 ha', operatorName: 'Carlos', operatorInitials: 'CM' })
    expect(page.pagination.value).toEqual({ rangeLabel: '1–6 de 6', page: 1, pageCount: 1 })
    expect(page.pending.value).toBe(false)
    expect(page.error.value).toBeNull()
    expect(page.isEmpty.value).toBe(false)
    expect(page.canPlan.value).toBe(true)
    expect(page.exportAction.disabled).toBe(true)
  })

  it('builds the four indicator cards of the top', async () => {
    const { page } = await setupList()

    expect(page.summaryCards.value).toEqual([
      { key: 'month', label: 'No mês', value: '12', caption: '9 concluídas · 2 planejadas · 1 cancelada', tone: 'default' },
      { key: 'area', label: 'Área aplicada', value: '412 ha', caption: '42% da área produtiva', tone: 'default' },
      { key: 'window', label: 'Dentro da janela', value: '92%', caption: '11 de 12 com Delta T e vento ideais', tone: 'highlight' },
      { key: 'next', label: 'Próxima', value: 'Qui 26/11', caption: 'T3 · 06:00 – 10:00 · fungicida', tone: 'default' },
    ])
  })

  it('keeps the cards when a filter is applied', async () => {
    const { page } = await setupList(SEED_LOGINS.admin, { status: 'cancelled' })
    expect(page.rows.value).toHaveLength(1)
    expect(page.summaryCards.value[0]?.value).toBe('12')
  })

  it('shows dashes while there is no data or when it fails', async () => {
    const dashes = [
      { key: 'month', label: 'No mês', value: '—', caption: null, tone: 'default' },
      { key: 'area', label: 'Área aplicada', value: '—', caption: null, tone: 'default' },
      { key: 'window', label: 'Dentro da janela', value: '—', caption: null, tone: 'highlight' },
      { key: 'next', label: 'Próxima', value: '—', caption: null, tone: 'default' },
    ]
    const broken = await setupFailingList(new ApiError({ code: 'internal_error', message: 'erro', status: 500 }))
    expect(broken.summaryCards.value).toEqual(dashes)

    const { page } = await setupList('joao@ouroverde.test')
    expect(page.summaryCards.value.map(card => card.value)).toEqual(['0', '0 ha', '—', '—'])
    expect(page.summaryCards.value.map(card => card.caption)).toEqual([null, null, null, null])
  })

  it('offers the status chips of the table header', async () => {
    const { page, fakes } = await setupList()
    expect(page.statusChips.map(chip => chip.label)).toEqual(['Todas', 'Planejadas', 'Concluídas', 'Canceladas'])

    page.setFilter('status', page.statusChips[2]!.value)
    await flushPromises()
    expect(fakes.route.query).toEqual({ status: 'completed' })
  })

  it('keeps filters in the query, goes back to page 1 and reloads', async () => {
    const { page, fakes } = await setupList(SEED_LOGINS.admin, { page: '2', size: '10' })

    page.setFilter('status', 'completed')
    await flushPromises()
    expect(fakes.route.query).toEqual({ size: '10', status: 'completed' })
    expect(page.rows.value.map(row => row.fieldCode)).toEqual(['T5', 'T1', 'T8'])

    page.setFilter('product', 'foliar')
    page.setFilter('operator', 'operator-joao')
    await flushPromises()
    expect(fakes.route.query).toMatchObject({ status: 'completed', product: 'foliar', operator: 'operator-joao' })
    expect(page.hasFilters.value).toBe(true)
    expect(page.isFilterEmpty.value).toBe(true)
    expect(page.isEmpty.value).toBe(false)

    page.clearFilters()
    await flushPromises()
    expect(fakes.route.query).toEqual({ size: '10' })
    expect(page.rows.value).toHaveLength(6)
    expect(page.hasFilters.value).toBe(false)
  })

  it('reads the filters from the query and ignores invalid values', async () => {
    const { page } = await setupList(SEED_LOGINS.admin, { period: '7', field: 'field-str-5', product: 'insecticide', status: 'bogus', size: '999', page: 'x' })

    expect(page.filters.value).toEqual({ periodDays: 7, fieldId: 'field-str-5', productKind: 'insecticide', operatorId: null, status: null })
    expect(page.pageSize.value).toBe(10)
    expect(page.page.value).toBe(1)
    expect(page.rows.value.map(row => row.fieldCode)).toEqual(['T5'])
    expect(page.hasFilters.value).toBe(true)
  })

  it('the default period is not a filter and clears itself from the query', async () => {
    const { page, fakes } = await setupList(SEED_LOGINS.admin, { period: '7' })
    expect(page.hasFilters.value).toBe(true)

    page.setFilter('period', 30)
    await flushPromises()
    expect(fakes.route.query).toEqual({})
    expect(page.filters.value.periodDays).toBe(30)
  })

  it('paginates and sends an out-of-range page to the last one', async () => {
    const { page, fakes } = await setupList(SEED_LOGINS.admin, { size: '20' })
    page.pageSize.value = 10
    await flushPromises()
    expect(fakes.route.query).toEqual({})

    fakes.route.query = { page: '9' }
    await flushPromises()
    // 6 rows fit in one page of 10: the query goes back to page 1.
    expect(fakes.route.query).toEqual({})
    expect(page.page.value).toBe(1)

    // The setter writes the query; with 6 rows page 2 does not exist, so it is sent back to page 1.
    page.page.value = 2
    expect(fakes.route.query).toEqual({ page: '2' })
    await flushPromises()
    expect(fakes.route.query).toEqual({})
  })

  it('is empty (no filters) for a user whose farms have no applications', async () => {
    const { page } = await setupList('joao@ouroverde.test')

    expect(page.rows.value).toEqual([])
    expect(page.isEmpty.value).toBe(true)
    expect(page.isFilterEmpty.value).toBe(false)
    expect(page.pagination.value).toBeNull()
    expect(page.isForbidden.value).toBe(false)
  })

  it('opens and closes the detail panel through the query', async () => {
    const { page, fakes } = await setupList(SEED_LOGINS.admin, { status: 'completed' })
    expect(page.selectedId.value).toBeNull()

    page.openDetail('application-seed-t5')
    await flushPromises()
    expect(fakes.route.query).toEqual({ status: 'completed', application: 'application-seed-t5' })
    expect(page.selectedId.value).toBe('application-seed-t5')

    page.closeDetail()
    await flushPromises()
    expect(fakes.route.query).toEqual({ status: 'completed' })
    expect(page.selectedId.value).toBeNull()
  })

  it('maps 403 and out-of-scope to their states and keeps a generic error', async () => {
    const forbidden = await setupFailingList(new ApiError({ code: 'forbidden', message: 'sem permissão', status: 403 }))
    expect(forbidden.isForbidden.value).toBe(true)
    expect(forbidden.isOutOfScope.value).toBe(false)
    expect(forbidden.isEmpty.value).toBe(false)
    expect(forbidden.rows.value).toEqual([])

    const outOfScope = await setupFailingList(new ApiError({ code: 'out_of_scope', message: 'fora do escopo', status: 403 }))
    expect(outOfScope.isOutOfScope.value).toBe(true)
    expect(outOfScope.isForbidden.value).toBe(false)

    const broken = await setupFailingList(new ApiError({ code: 'internal_error', message: 'erro', status: 500 }))
    expect(broken.error.value?.code).toBe('internal_error')
    expect(broken.isForbidden.value).toBe(false)
    expect(broken.isEmpty.value).toBe(false)
    expect(broken.isFilterEmpty.value).toBe(false)
  })

  it('shows the list even when the forecast is down (no weather chip)', async () => {
    await signInWithPlannerFaults({ weather: true })
    const page = useApplicationListPage({ now })
    await flushPromises()

    expect(page.rows.value).toHaveLength(6)
    expect(page.weatherLabel.value).toBeNull()
    expect(page.weatherUnavailable.value).toBe(true)
  })
})

describe('useApplicationDetail', () => {
  it('loads and formats the T5 panel', async () => {
    await signInAs(SEED_LOGINS.admin)
    const id = ref<string | null>('application-seed-t5')
    const detail = useApplicationDetail(id)
    await flushPromises()

    expect(detail.error.value).toBeNull()
    expect(detail.view.value?.title).toBe('Inseticida · Talhão 5')
    expect(detail.view.value?.registeredLabel).toBe('Registrada em 21/11 às 09:52')
    expect(detail.view.value?.productRows.map(row => row.value)).toEqual([
      'Inseticida · lote 2409-B',
      '0,3 L/ha · 16,9 L no total',
      '180 L/ha · 10.134 L · 6 tanques',
      '14 dias · colheita liberada a partir de 05/12',
    ])
    expect(detail.downloadAction.disabled).toBe(true)
    expect(detail.canRepeat.value).toBe(true)
  })

  it('does not request anything while closed and follows the id', async () => {
    await signInAs(SEED_LOGINS.admin)
    const id = ref<string | null>(null)
    const detail = useApplicationDetail(id)
    await flushPromises()
    expect(detail.view.value).toBeNull()
    expect(detail.error.value).toBeNull()
    expect(detail.canRepeat.value).toBe(false)

    id.value = 'application-seed-t4'
    await flushPromises()
    expect(detail.view.value?.status.label).toBe('Cancelada')
    expect(detail.view.value?.cancellationLabel).toBe('Chuva de 18 mm prevista na janela')
    expect(detail.view.value?.conditions).toBeNull()
  })

  it('reports not found and other-company applications as 404', async () => {
    await signInAs(SEED_LOGINS.admin)
    const missing = useApplicationDetail('application-unknown')
    await flushPromises()
    expect(missing.isNotFound.value).toBe(true)
    expect(missing.isForbidden.value).toBe(false)
    expect(missing.view.value).toBeNull()

    await signInAs('joao@ouroverde.test')
    const foreign = useApplicationDetail('application-seed-t5')
    await flushPromises()
    expect(foreign.isNotFound.value).toBe(true)
  })

  it('"Planejar repetição" fills the planner bar and returns the planner path', async () => {
    await signInAs(SEED_LOGINS.admin)
    const detail = useApplicationDetail('application-seed-t8')
    await flushPromises()

    expect(detail.planRepeat()).toBe('/planner')

    const context = usePlannerContext()
    expect(context.farmId.value).toBe(SCENARIO.farmId)
    expect(context.fieldIds.value).toEqual(['field-str-8'])
    expect(context.productKind.value).toBe('foliar')
    expect(context.requestContext.value).toEqual({ farmId: SCENARIO.farmId, fieldIds: ['field-str-8'], productKind: 'foliar' })
  })

  it('cannot repeat without planner.use or before it loads', async () => {
    await signInAs(SEED_LOGINS.admin)
    const detail = useApplicationDetail('application-unknown')
    await flushPromises()
    expect(detail.planRepeat()).toBeNull()

    const loaded = useApplicationDetail('application-seed-t5')
    await flushPromises()
    const session = useState<{ permissions: string[] } | null>('session:user', () => null)
    session.value = { ...session.value!, permissions: [] }
    expect(loaded.canRepeat.value).toBe(false)
    expect(loaded.planRepeat()).toBeNull()
  })
})

describe('useCurrentWeather', () => {
  it('serves the chip label', async () => {
    await signInAs(SEED_LOGINS.admin)
    const weather = useCurrentWeather()
    await flushPromises()

    expect(weather.label.value).toBe('Guaxupé · 24 °C · vento 7 km/h')
    expect(weather.isUnavailable.value).toBe(false)
  })

  it('is unavailable, never made up, when the forecast is down', async () => {
    await signInWithPlannerFaults({ weather: true })
    const weather = useCurrentWeather()
    await flushPromises()

    expect(weather.label.value).toBeNull()
    expect(weather.isUnavailable.value).toBe(true)
    expect(weather.error.value?.code).toBe('weather_unavailable')
  })
})
