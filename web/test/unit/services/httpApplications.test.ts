import { describe, expect, it } from 'vitest'
import { createHttpClient, type RawFetchOptions } from '~/services/http/client'
import { createHttpServices } from '~/services/http/services'
import type { ApplicationDetailDto, ApplicationPageDto, CurrentWeatherDto } from '~/types/applicationDto'

function servicesWith(answer: unknown) {
  const calls: { path: string, options: RawFetchOptions }[] = []
  const http = createHttpClient({ baseURL: 'http://api.test' }, async (path, options) => {
    calls.push({ path, options })
    return answer
  })
  return { api: createHttpServices(http, 'http://localhost:8002'), calls }
}

const summary: ApplicationPageDto['items'][number] = {
  id: 'a-5',
  status: 'completed',
  farm_id: 'farm-santa-rita',
  window: { date: '2026-11-21', start_time: '06:12', end_time: '09:40' },
  field: { id: 'field-str-5', label: 'T5', name: 'Talhão 5', variety: 'Catuaí 144', block: 'Área Centro' },
  product: { kind: 'insecticide', name: 'Inseticida', dose: { value: 0.3, unit: 'L/ha' }, mix_rate_l_per_ha: 180 },
  area_ha: 56.3,
  conditions: { source: 'recorded', delta_t_c: 4.2, wind_kmh: 6, rain_mm: null },
  operator: { id: 'user-carlos', name: 'Carlos Mendes' },
}

describe('http application list', () => {
  it('sends the filters in snake_case and maps the page and the facets', async () => {
    const dto: ApplicationPageDto = {
      items: [summary],
      total: 1,
      page: 2,
      page_size: 10,
      facets: { fields: [{ id: 'field-str-5', label: 'T5', name: 'Talhão 5' }], operators: [{ id: 'user-carlos', name: 'Carlos Mendes' }] },
      summary: {
        month_count: { total: 12, planned: 2, in_progress: 0, completed: 9, cancelled: 1 },
        applied_area: { area_ha: 412, productive_area_ha: 982, percent: 42 },
        within_window: { percent: 92, within: 11, total: 12 },
        next: { application_id: 'a-3', date: '2026-11-26', start_time: '06:00', end_time: '10:00', field_label: 'T3', product_name: 'Fungicida' },
      },
    }
    const { api, calls } = servicesWith(dto)
    const page = await api.applications.list({
      periodDays: 30,
      farmId: 'farm-santa-rita',
      fieldId: 'field-str-5',
      productKind: 'insecticide',
      operatorId: 'user-carlos',
      status: 'completed',
      page: 2,
      pageSize: 10,
    })

    expect(calls[0]).toMatchObject({
      path: '/applications',
      options: {
        query: {
          period_days: 30,
          farm_id: 'farm-santa-rita',
          field_id: 'field-str-5',
          product_kind: 'insecticide',
          operator_id: 'user-carlos',
          status: 'completed',
          page: 2,
          page_size: 10,
        },
      },
    })
    expect(calls[0]?.options.method).toBe('GET')
    expect(page).toMatchObject({ total: 1, page: 2, pageSize: 10 })
    expect(page.items[0]).toEqual({
      id: 'a-5',
      status: 'completed',
      farmId: 'farm-santa-rita',
      window: { date: '2026-11-21', startTime: '06:12', endTime: '09:40' },
      field: { id: 'field-str-5', label: 'T5', name: 'Talhão 5', variety: 'Catuaí 144', block: 'Área Centro' },
      product: { kind: 'insecticide', name: 'Inseticida', dose: { value: 0.3, unit: 'L/ha' }, mixRateLPerHa: 180 },
      areaHa: 56.3,
      conditions: { source: 'recorded', deltaTC: 4.2, windKmh: 6, rainMm: null },
      operator: { id: 'user-carlos', name: 'Carlos Mendes' },
    })
    expect(page.facets).toEqual({ fields: [{ id: 'field-str-5', label: 'T5', name: 'Talhão 5' }], operators: [{ id: 'user-carlos', name: 'Carlos Mendes' }] })
    expect(page.summary).toEqual({
      monthCount: { total: 12, planned: 2, inProgress: 0, completed: 9, cancelled: 1 },
      appliedArea: { areaHa: 412, productiveAreaHa: 982, percent: 42 },
      withinWindow: { percent: 92, within: 11, total: 12 },
      next: { applicationId: 'a-3', date: '2026-11-26', startTime: '06:00', endTime: '10:00', fieldLabel: 'T3', productName: 'Fungicida' },
    })
  })

  it('keeps the nulls of an empty month', async () => {
    const { api } = servicesWith({
      items: [],
      total: 0,
      page: 1,
      page_size: 10,
      facets: { fields: [], operators: [] },
      summary: {
        month_count: { total: 0, planned: 0, in_progress: 0, completed: 0, cancelled: 0 },
        applied_area: { area_ha: 0, productive_area_ha: 0, percent: null },
        within_window: { percent: null, within: 0, total: 0 },
        next: null,
      },
    })
    const page = await api.applications.list()
    expect(page.summary).toMatchObject({ appliedArea: { percent: null }, withinWindow: { percent: null }, next: null })
  })

  it('omits the filters that were not given', async () => {
    const { api, calls } = servicesWith({
      items: [],
      total: 0,
      page: 1,
      page_size: 10,
      facets: { fields: [], operators: [] },
      summary: {
        month_count: { total: 0, planned: 0, in_progress: 0, completed: 0, cancelled: 0 },
        applied_area: { area_ha: 0, productive_area_ha: 0, percent: null },
        within_window: { percent: null, within: 0, total: 0 },
        next: null,
      },
    })
    await api.applications.list()
    expect(calls[0]?.options.query).toBeUndefined()
  })
})

describe('http application detail', () => {
  const dto: ApplicationDetailDto = {
    ...summary,
    registered_at: '2026-11-21T12:52:00Z',
    recorded_conditions: { headline: 'Dentro da janela', delta_t_c: 4.2, wind_kmh: 6, rain_mm: 0, rain_hours: 8, duration_minutes: 208 },
    usage: { lot: '2409-B', dose_total_l: 16.9, mix_total_l: 10134, tank_count: 6, tank_capacity_l: 2000 },
    pre_harvest: { days: 14, released_from: '2026-12-05' },
    equipment: { id: 'e-1', name: 'Turbo 2.000 L · bico cone vazio' },
    confirmed_by: { person: { id: 'user-marina', name: 'Marina Rezende' }, at: '2026-11-20T20:40:00Z' },
    signed: true,
    cancellation: null,
  }

  it('maps the detail with an escaped id', async () => {
    const { api, calls } = servicesWith(dto)
    const detail = await api.applications.get('a/5')

    expect(calls[0]?.path).toBe('/applications/a%2F5')
    expect(detail).toMatchObject({
      id: 'a-5',
      registeredAt: new Date('2026-11-21T12:52:00Z'),
      recordedConditions: { headline: 'Dentro da janela', deltaTC: 4.2, windKmh: 6, rainMm: 0, rainHours: 8, durationMinutes: 208 },
      usage: { lot: '2409-B', doseTotalL: 16.9, mixTotalL: 10134, tankCount: 6, tankCapacityL: 2000 },
      preHarvest: { days: 14, releasedFrom: '2026-12-05' },
      equipment: { name: 'Turbo 2.000 L · bico cone vazio' },
      confirmedBy: { person: { name: 'Marina Rezende' }, at: new Date('2026-11-20T20:40:00Z') },
      signed: true,
      cancellation: null,
    })
  })

  it('keeps the nulls of a planned or cancelled application', async () => {
    const { api } = servicesWith({
      ...dto,
      status: 'cancelled',
      recorded_conditions: null,
      pre_harvest: null,
      confirmed_by: null,
      signed: false,
      cancellation: { reason: 'Chuva de 18 mm prevista na janela' },
    })
    expect(await api.applications.get('a-4')).toMatchObject({
      recordedConditions: null,
      preHarvest: null,
      confirmedBy: null,
      signed: false,
      cancellation: { reason: 'Chuva de 18 mm prevista na janela' },
    })
  })
})

describe('http weather service', () => {
  const dto: CurrentWeatherDto = {
    farm_id: 'farm-santa-rita',
    city: 'Guaxupé',
    state: 'MG',
    temperature_c: 24,
    wind_kmh: 7,
    observed_at: '2026-11-21T09:48:00Z',
  }

  it('asks for the current weather with or without a farm', async () => {
    const { api, calls } = servicesWith(dto)
    const weather = await api.weather.getCurrent('farm-santa-rita')
    await api.weather.getCurrent()

    expect(calls[0]).toMatchObject({ path: '/weather/current', options: { query: { farm_id: 'farm-santa-rita' } } })
    expect(calls[1]?.options.query).toBeUndefined()
    expect(weather).toEqual({
      farmId: 'farm-santa-rita',
      city: 'Guaxupé',
      state: 'MG',
      temperatureC: 24,
      windKmh: 7,
      observedAt: new Date('2026-11-21T09:48:00Z'),
    })
  })
})
