import { describe, expect, it } from 'vitest'
import { ApiError } from '~/services/errors'
import { createMockServices } from '~/services/mock/mockServices'
import type { PlannerFaults } from '~/services/mock/plannerServices'
import type { ApiServices } from '~/services/ports'
import { SCENARIO } from '../../support/planner'

const NOW = new Date('2026-11-21T12:00:00Z')
const TOKENS = {
  admin: 'mock-token:user-marina',
  operator: 'mock-token:user-carlos',
  superAdmin: 'mock-token:user-bruno',
  otherCompany: 'mock-token:user-joao',
}

function apiFor(token: string, options: { faults?: PlannerFaults, now?: () => Date } = {}): ApiServices {
  return createMockServices({ latencyMs: 0, getAccessToken: () => token, now: options.now ?? (() => NOW), plannerFaults: options.faults })
}

async function failureOf(promise: Promise<unknown>): Promise<ApiError> {
  try {
    await promise
  }
  catch (error) {
    if (error instanceof ApiError) return error
    throw error
  }
  throw new Error('expected the call to fail')
}

describe('mock application list', () => {
  it('lists the Figma rows in the API order: planned first, then the past from newest to oldest', async () => {
    const page = await apiFor(TOKENS.admin).applications.list()

    expect(page).toMatchObject({ total: 6, page: 1 })
    expect(page.items.map(item => [item.field.label, item.status, item.window.date])).toEqual([
      ['T3', 'planned', '2026-11-26'],
      ['T7', 'planned', '2026-11-28'],
      ['T5', 'completed', '2026-11-21'],
      ['T1', 'completed', '2026-11-19'],
      ['T4', 'cancelled', '2026-11-17'],
      ['T8', 'completed', '2026-11-15'],
    ])
  })

  it('serves the numbers of each row', async () => {
    const byLabel = Object.fromEntries((await apiFor(TOKENS.admin).applications.list()).items.map(item => [item.field.label, item]))

    expect(byLabel.T3).toMatchObject({
      field: { name: 'Talhão 3', variety: 'Catuaí 144', block: 'Área Norte' },
      product: { kind: 'fungicide', name: 'Fungicida', dose: { value: 0.5, unit: 'L/ha' }, mixRateLPerHa: 200 },
      areaHa: 38.6,
      window: { startTime: '06:00', endTime: '10:00' },
      conditions: { source: 'forecast', deltaTC: 3.1, windKmh: 5 },
      operator: { name: 'Carlos Mendes' },
    })
    expect(byLabel.T7?.conditions).toEqual({ source: 'live', deltaTC: null, windKmh: null, rainMm: null })
    expect(byLabel.T5).toMatchObject({ areaHa: 56.3, window: { startTime: '06:12', endTime: '09:40' }, conditions: { source: 'recorded', deltaTC: 4.2, windKmh: 6 } })
    expect(byLabel.T1?.operator.name).toBe('João Silveira')
    expect(byLabel.T4).toMatchObject({ status: 'cancelled', product: { name: 'Herbicida', mixRateLPerHa: 150 }, conditions: { rainMm: 18 } })
    expect(byLabel.T8).toMatchObject({ product: { kind: 'foliar', name: 'Foliar', dose: { value: 2 }, mixRateLPerHa: 300 }, areaHa: 47.7 })
  })

  it('offers the filter options of the user scope, unaffected by the filters', async () => {
    const page = await apiFor(TOKENS.admin).applications.list({ status: 'cancelled' })

    expect(page.total).toBe(1)
    expect(page.facets.fields.map(field => field.label).sort()).toEqual(['T1', 'T3', 'T4', 'T5', 'T7', 'T8'])
    expect(page.facets.operators.map(operator => operator.name).sort()).toEqual(['Carlos Mendes', 'João Silveira'])
  })

  it('filters by field, product, operator and status, and combines them', async () => {
    const api = apiFor(TOKENS.admin)
    const labels = async (filters: Parameters<ApiServices['applications']['list']>[0]) =>
      (await api.applications.list(filters)).items.map(item => item.field.label)

    expect(await labels({ fieldId: 'field-str-5' })).toEqual(['T5'])
    expect(await labels({ productKind: 'fungicide' })).toEqual(['T3', 'T7', 'T1'])
    expect(await labels({ operatorId: 'operator-joao' })).toEqual(['T1'])
    expect(await labels({ status: 'completed' })).toEqual(['T5', 'T1', 'T8'])
    expect(await labels({ status: 'completed', productKind: 'foliar' })).toEqual(['T8'])
    expect(await labels({ status: 'in_progress' })).toEqual([])
  })

  it('applies the period from today minus N days, keeping the planned future ones', async () => {
    let clock = NOW
    const api = apiFor(TOKENS.admin, { now: () => clock })
    clock = new Date(NOW.getTime() + 10 * 86_400_000)

    const week = await api.applications.list({ periodDays: 7 })
    expect(week.items.map(item => item.field.label)).toEqual(['T3', 'T7'])
    expect((await api.applications.list({ periodDays: 30 })).total).toBe(6)
  })

  it('paginates with the API total', async () => {
    const api = apiFor(TOKENS.admin)
    const second = await api.applications.list({ page: 2, pageSize: 4 })

    expect(second).toMatchObject({ total: 6, page: 2, pageSize: 4 })
    expect(second.items.map(item => item.field.label)).toEqual(['T4', 'T8'])
  })

  it('serves the month indicators of the Figma, unaffected by the table filters', async () => {
    const api = apiFor(TOKENS.admin)
    const expected = {
      monthCount: { total: 12, planned: 2, inProgress: 0, completed: 9, cancelled: 1 },
      appliedArea: { areaHa: 412, productiveAreaHa: 982, percent: 42 },
      withinWindow: { percent: 92, within: 11, total: 12 },
      next: { fieldLabel: 'T3', date: '2026-11-26', startTime: '06:00', endTime: '10:00', productName: 'Fungicida', applicationId: 'application-seed-t3' },
    }

    expect((await api.applications.list()).summary).toEqual(expected)
    expect((await api.applications.list({ status: 'cancelled' })).summary).toEqual(expected)
    expect((await api.applications.list({ productKind: 'foliar', page: 2, pageSize: 1 })).summary).toEqual(expected)
    expect((await api.applications.list({ farmId: SCENARIO.farmId })).summary).toEqual(expected)
  })

  it('has empty indicators (no invented numbers) where there is nothing in the month', async () => {
    const { summary } = await apiFor(TOKENS.otherCompany).applications.list()

    expect(summary).toEqual({
      monthCount: { total: 0, planned: 0, inProgress: 0, completed: 0, cancelled: 0 },
      appliedArea: { areaHa: 0, productiveAreaHa: 0, percent: null },
      withinWindow: { percent: null, within: 0, total: 0 },
      next: null,
    })
  })

  it('counts only the current month and moves "Próxima" along with the clock', async () => {
    let clock = NOW
    const api = apiFor(TOKENS.admin, { now: () => clock })
    // 27/11: T3 (26/11) is past, so the next planned one is T7 (28/11); December has none of the seed rows.
    clock = new Date('2026-11-27T12:00:00Z')
    expect((await api.applications.list()).summary.next).toMatchObject({ fieldLabel: 'T7', date: '2026-11-28' })

    clock = new Date('2026-12-02T12:00:00Z')
    const december = (await api.applications.list()).summary
    expect(december.monthCount.total).toBe(6)
    expect(december.next).toBeNull()
  })

  it('applies company and farm scope', async () => {
    expect((await apiFor(TOKENS.operator).applications.list()).total).toBe(6)
    const other = await apiFor(TOKENS.otherCompany).applications.list()
    expect(other).toMatchObject({ total: 0, items: [], facets: { fields: [], operators: [] } })

    expect(await failureOf(apiFor(TOKENS.otherCompany).applications.list({ farmId: SCENARIO.farmId }))).toMatchObject({ code: 'not_found', status: 404 })
    expect(await failureOf(apiFor('mock-token:nobody').applications.list())).toMatchObject({ code: 'unauthorized', status: 401 })
  })
})

describe('mock application detail', () => {
  it('serves the T5 panel of the Figma', async () => {
    const detail = await apiFor(TOKENS.admin).applications.get('application-seed-t5')

    expect(detail).toMatchObject({
      status: 'completed',
      field: { name: 'Talhão 5', variety: 'Catuaí 144' },
      areaHa: 56.3,
      window: { date: '2026-11-21', startTime: '06:12', endTime: '09:40' },
      recordedConditions: { headline: 'Dentro da janela', deltaTC: 4.2, windKmh: 6, rainMm: 0, rainHours: 8, durationMinutes: 208 },
      usage: { lot: '2409-B', doseTotalL: 16.9, mixTotalL: 10134, tankCount: 6, tankCapacityL: 2000 },
      preHarvest: { days: 14, releasedFrom: '2026-12-05' },
      equipment: { name: 'Turbo 2.000 L · bico cone vazio' },
      operator: { name: 'Carlos Mendes' },
      confirmedBy: { person: { name: 'Marina Rezende' }, at: new Date('2026-11-20T20:40:00Z') },
      signed: true,
      cancellation: null,
    })
    expect(detail.registeredAt).toEqual(new Date('2026-11-21T12:52:00Z'))
  })

  it('has no recorded conditions or signature when planned, and a reason when cancelled', async () => {
    const api = apiFor(TOKENS.admin)
    const planned = await api.applications.get('application-seed-t3')
    expect(planned).toMatchObject({ status: 'planned', recordedConditions: null, signed: false, preHarvest: { days: 14, releasedFrom: null } })
    expect(planned.usage).toMatchObject({ lot: null, doseTotalL: 19.3, mixTotalL: 7720, tankCount: 4 })

    const cancelled = await api.applications.get('application-seed-t4')
    expect(cancelled).toMatchObject({ status: 'cancelled', preHarvest: null, cancellation: { reason: 'Chuva de 18 mm prevista na janela' } })
  })

  it('answers 404 for unknown or other-company applications and 401 without a session', async () => {
    expect(await failureOf(apiFor(TOKENS.admin).applications.get('application-unknown'))).toMatchObject({ code: 'not_found' })
    expect(await failureOf(apiFor(TOKENS.otherCompany).applications.get('application-seed-t5'))).toMatchObject({ code: 'not_found', status: 404 })
    expect(await failureOf(apiFor('mock-token:nobody').applications.get('application-seed-t5'))).toMatchObject({ code: 'unauthorized' })
  })
})

describe('registered applications reach the history', () => {
  it('lists, and details, what the planner registered after the human confirmation', async () => {
    const api = apiFor(TOKENS.admin)
    const reply = await api.planner.sendMessage({
      conversationId: null,
      text: 'Quando faço?',
      context: { farmId: SCENARIO.farmId, fieldIds: [SCENARIO.fieldT3], productKind: 'fungicide' },
    })
    const plan = reply.plan!
    const review = await api.applications.review(plan.id, SCENARIO.fieldT3)
    const registered = await api.applications.register({
      planId: plan.id,
      fieldId: SCENARIO.fieldT3,
      reviewId: review.id,
      confirmation: { confirmed: true, acknowledgedWarnings: [] },
    })

    const page = await api.applications.list()
    expect(page.total).toBe(7)
    // Planned today: nearest planned first.
    expect(page.items[0]).toMatchObject({ id: registered.id, status: 'planned', areaHa: 38.6, window: { date: '2026-11-21' }, conditions: { source: 'live' } })

    const detail = await api.applications.get(registered.id)
    expect(detail).toMatchObject({
      registeredAt: NOW,
      usage: { mixTotalL: 7720, tankCount: 4 },
      confirmedBy: { person: { name: 'Marina Rezende' }, at: NOW },
      preHarvest: { days: 30 },
    })

    // The same entry cannot be registered twice.
    expect(await failureOf(api.applications.register({
      planId: plan.id,
      fieldId: SCENARIO.fieldT3,
      reviewId: review.id,
      confirmation: { confirmed: true, acknowledgedWarnings: [] },
    }))).toMatchObject({ code: 'conflict', status: 409 })
  })
})

describe('mock current weather', () => {
  it('serves the chip of the primary farm and of a chosen farm', async () => {
    const weather = await apiFor(TOKENS.admin).weather.getCurrent()
    expect(weather).toMatchObject({ farmId: SCENARIO.farmId, city: 'Guaxupé', state: 'MG', temperatureC: 24, windKmh: 7 })
    expect(weather.observedAt).toEqual(new Date('2026-11-21T09:48:00Z'))
    expect((await apiFor(TOKENS.otherCompany).weather.getCurrent()).farmId).toBe('farm-ouro-verde')
    expect((await apiFor(TOKENS.operator).weather.getCurrent(SCENARIO.farmId)).city).toBe('Guaxupé')
  })

  it('is honest when the forecast is down, and respects tenancy', async () => {
    expect(await failureOf(apiFor(TOKENS.admin, { faults: { weather: true } }).weather.getCurrent())).toMatchObject({ code: 'weather_unavailable', status: 502 })
    expect(await failureOf(apiFor(TOKENS.otherCompany).weather.getCurrent(SCENARIO.farmId))).toMatchObject({ code: 'not_found' })
  })
})
