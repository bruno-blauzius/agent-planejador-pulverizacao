import { describe, expect, it } from 'vitest'
import { ApiError } from '~/services/errors'
import { createHttpClient, type RawFetchOptions } from '~/services/http/client'
import { createHttpServices } from '~/services/http/services'
import type {
  ApplicationReviewDto,
  DayConditionsDto,
  PlannerReplyDto,
  RegisteredApplicationDto,
  SprayWindowMapDto,
  TodayBriefDto,
} from '~/types/plannerDto'

interface RecordedCall {
  path: string
  options: RawFetchOptions
}

function servicesWith(answer: unknown) {
  const calls: RecordedCall[] = []
  const http = createHttpClient({ baseURL: 'http://api.test' }, async (path, options) => {
    calls.push({ path, options })
    if (answer instanceof Error) throw answer
    return answer
  })
  return { api: createHttpServices(http, 'http://localhost:8002'), calls }
}

const window = { date: '2026-11-26', start_time: '06:00', end_time: '10:00', condition: 'suitable' } as const

const replyDto: PlannerReplyDto = {
  conversation_id: 'c-1',
  message: { id: 'm-1', text: 'A melhor janela é quarta.\n\nDivida.', created_at: '2026-11-26T08:48:00Z' },
  consulted: [{ kind: 'weather', title: 'Previsão do tempo', detail: 'Open-Meteo · 5 dias, hora a hora' }],
  plan: {
    id: 'p-1',
    farm_id: 'farm-santa-rita',
    product_kind: 'fungicide',
    headline: '2 janelas aptas cobrem os 90 ha',
    total_area_ha: 90,
    capacity_ha_per_hour: 12,
    entries: [{
      field_id: 'field-str-7',
      field_label: 'T7',
      area_ha: 51.4,
      window,
      duration_minutes: 260,
      alert: { kind: 'window_too_short', message: 'curta', required_minutes: 260, available_minutes: 240 },
    }],
  },
  suggestions: ['Só o talhão 3'],
}

describe('http planner service', () => {
  it('posts the message in snake_case and maps the reply', async () => {
    const { api, calls } = servicesWith(replyDto)
    const reply = await api.planner.sendMessage({
      conversationId: null,
      text: 'Quando faço?',
      context: { farmId: 'farm-santa-rita', fieldIds: ['field-str-3', 'field-str-7'], productKind: 'fungicide' },
    })

    expect(calls[0]).toMatchObject({
      path: '/planner/messages',
      options: {
        method: 'POST',
        body: {
          conversation_id: null,
          text: 'Quando faço?',
          context: { farm_id: 'farm-santa-rita', field_ids: ['field-str-3', 'field-str-7'], product_kind: 'fungicide' },
        },
      },
    })
    expect(reply.message.createdAt).toEqual(new Date('2026-11-26T08:48:00Z'))
    expect(reply.plan?.entries[0]).toMatchObject({
      fieldLabel: 'T7',
      areaHa: 51.4,
      durationMinutes: 260,
      window: { startTime: '06:00', endTime: '10:00' },
      alert: { kind: 'window_too_short', requiredMinutes: 260, availableMinutes: 240 },
    })
    expect(reply.suggestions).toEqual(['Só o talhão 3'])
  })

  it('keeps a null plan', async () => {
    const { api } = servicesWith({ ...replyDto, plan: null })
    expect((await api.planner.sendMessage({ conversationId: 'c-1', text: 'x', context: { farmId: 'f', fieldIds: ['a'], productKind: null } })).plan).toBeNull()
  })

  it('asks for the window map with a comma separated field list', async () => {
    const dto: SprayWindowMapDto = {
      farm_id: 'f',
      generated_at: '2026-11-26T08:40:00Z',
      forecast_updated_at: '2026-11-26T08:40:00Z',
      day_start_hour: 6,
      day_end_hour: 20,
      fields: [{ field_id: 'a', field_label: 'T3', days: [{ date: '2026-11-26', hours: [{ hour: 6, condition: 'suitable' }] }] }],
      windows: [{ field_id: 'a', field_label: 'T3', date: '2026-11-26', start_time: '06:00', end_time: '10:00', duration_minutes: 240, condition: 'suitable' }],
    }
    const { api, calls } = servicesWith(dto)
    const map = await api.planner.getWindowMap({ farmId: 'f', fieldIds: ['a', 'b'], days: 5 })

    expect(calls[0]).toMatchObject({ path: '/planner/window-map', options: { query: { farm_id: 'f', field_ids: 'a,b', days: 5 } } })
    expect(map.windows[0]).toMatchObject({ fieldLabel: 'T3', durationMinutes: 240, startTime: '06:00' })
    expect(map.fields[0]?.days[0]?.hours).toEqual([{ hour: 6, condition: 'suitable' }])
    expect(map.forecastUpdatedAt).toEqual(new Date('2026-11-26T08:40:00Z'))
  })

  it('maps hourly conditions and the window summary', async () => {
    const dto: DayConditionsDto = {
      farm_id: 'f',
      date: '2026-11-26',
      ideal_ranges: { delta_t_c: { min: 2, max: 8 }, wind_kmh: { min: 3, max: 10 } },
      hours: [{ hour: 6, delta_t_c: 2.6, wind_kmh: 4, gust_kmh: 7, rain_mm: 0, condition: 'suitable' }],
      window_summary: {
        start_time: '06:00',
        end_time: '10:00',
        delta_t_min_c: 2.6,
        delta_t_max_c: 6.2,
        wind_avg_kmh: 6,
        gust_max_kmh: 11,
        rain_mm: 0,
        rain_until_hour: 18,
        next_rain_at: '2026-11-27T06:00:00Z',
      },
    }
    const { api, calls } = servicesWith(dto)
    const day = await api.planner.getDayConditions({ farmId: 'f', date: '2026-11-26', fieldId: 'a', windowStart: '06:00', windowEnd: '10:00' })

    expect(calls[0]).toMatchObject({
      path: '/planner/conditions',
      options: { query: { farm_id: 'f', date: '2026-11-26', field_id: 'a', window_start: '06:00', window_end: '10:00' } },
    })
    expect(day.hours[0]).toEqual({ hour: 6, deltaTC: 2.6, windKmh: 4, gustKmh: 7, rainMm: 0, condition: 'suitable' })
    expect(day.windowSummary).toMatchObject({ deltaTMaxC: 6.2, nextRainAt: new Date('2026-11-27T06:00:00Z') })
    expect(day.idealRanges.deltaTC).toEqual({ min: 2, max: 8 })
  })

  it('maps a day without window', async () => {
    const { api } = servicesWith({ farm_id: 'f', date: 'd', ideal_ranges: { delta_t_c: { min: 2, max: 8 }, wind_kmh: { min: 3, max: 10 } }, hours: [], window_summary: null })
    expect((await api.planner.getDayConditions({ farmId: 'f', date: 'd' })).windowSummary).toBeNull()
  })

  it('turns 502 bodies into honest ApiErrors', async () => {
    const failure = Object.assign(new Error('bad gateway'), {
      statusCode: 502,
      data: { error: { code: 'agent_unavailable', message: 'o agente de pulverização está indisponível' } },
    })
    const { api } = servicesWith(failure)
    const error = await api.planner.sendMessage({ conversationId: null, text: 'x', context: { farmId: 'f', fieldIds: ['a'], productKind: null } }).catch((e: unknown) => e)
    expect(error).toBeInstanceOf(ApiError)
    expect(error).toMatchObject({ code: 'agent_unavailable', status: 502 })
  })
})

describe('http application service', () => {
  const reviewDto: ApplicationReviewDto = {
    id: 'r-1',
    plan_id: 'p/1',
    field_id: 'field-str-3',
    field_label: 'T3',
    variety: 'Catuaí 144',
    area_ha: 38.6,
    window,
    product: { kind: 'fungicide', name: 'Fungicida', dose: { value: 0.5, unit: 'L/ha' }, mix_rate_l_per_ha: 200 },
    mix_volume_l: 7720,
    tank_count: 4,
    tank_capacity_l: 2000,
    checks: [{ code: 'delta_t', status: 'ok', title: 'Delta T dentro da faixa', detail: '2,6 a 6,2 °C' }],
    operator: { id: 'u-1', name: 'Carlos Mendes' },
    equipment: { id: 'e-1', name: 'Pulverizador 2.000 L' },
    can_confirm: true,
    expires_at: '2026-11-26T09:03:00Z',
  }

  it('posts the review request with escaped path segments and maps it', async () => {
    const { api, calls } = servicesWith(reviewDto)
    const review = await api.applications.review('p/1', 'field-str-3')

    expect(calls[0]).toMatchObject({ path: '/planner/plans/p%2F1/entries/field-str-3/review', options: { method: 'POST' } })
    expect(review).toMatchObject({
      planId: 'p/1',
      product: { dose: { value: 0.5, unit: 'L/ha' }, mixRateLPerHa: 200 },
      mixVolumeL: 7720,
      tankCount: 4,
      canConfirm: true,
      expiresAt: new Date('2026-11-26T09:03:00Z'),
    })
  })

  it('sends the explicit confirmation in the registration body', async () => {
    const dto: RegisteredApplicationDto = {
      id: 'a-1',
      status: 'planned',
      field_id: 'field-str-3',
      field_label: 'T3',
      area_ha: 38.6,
      window,
      registered_at: '2026-11-26T09:05:00Z',
      registered_by: { id: 'u-2', name: 'Marina Rezende' },
    }
    const { api, calls } = servicesWith(dto)
    const application = await api.applications.register({
      planId: 'p-1',
      fieldId: 'field-str-3',
      reviewId: 'r-1',
      confirmation: { confirmed: true, acknowledgedWarnings: ['window_fits'] },
    })

    expect(calls[0]).toMatchObject({
      path: '/applications',
      options: {
        method: 'POST',
        body: {
          plan_id: 'p-1',
          field_id: 'field-str-3',
          review_id: 'r-1',
          confirmation: { confirmed: true, acknowledged_warnings: ['window_fits'] },
        },
      },
    })
    expect(application).toMatchObject({ status: 'planned', registeredAt: new Date('2026-11-26T09:05:00Z'), registeredBy: { name: 'Marina Rezende' } })
  })
})

describe('http today service', () => {
  const briefDto: TodayBriefDto = {
    farm: { id: 'farm-santa-rita', name: 'Fazenda Santa Rita' },
    as_of: '2026-11-26T09:48:00Z',
    status: { condition: 'suitable', headline: 'Pode pulverizar', until_time: '10:00', remaining_minutes: 192, window_start: '06:00', window_end: '10:00' },
    metrics: [{ kind: 'delta_t', label: 'Delta T', value: 4.2, unit: '°C', condition: 'suitable' }],
    hours: [{ hour: 6, condition: 'suitable', is_current: true }],
    task: {
      id: 'task-1',
      field_id: 'field-str-3',
      field_label: 'T3',
      field_name: 'Talhão 3',
      operation: 'Fungicida',
      area_ha: 38.6,
      dose: { value: 0.5, unit: 'L/ha' },
      tank_count: 4,
      tank_capacity_l: 2000,
      status: 'planned',
    },
  }

  it('maps the brief', async () => {
    const { api, calls } = servicesWith(briefDto)
    const brief = await api.today.getBrief()

    expect(calls[0]?.path).toBe('/planner/today')
    expect(brief.asOf).toEqual(new Date('2026-11-26T09:48:00Z'))
    expect(brief.status).toMatchObject({ untilTime: '10:00', remainingMinutes: 192, windowStart: '06:00' })
    expect(brief.hours[0]).toEqual({ hour: 6, condition: 'suitable', isCurrent: true })
    expect(brief.task).toMatchObject({ fieldName: 'Talhão 3', tankCount: 4, tankCapacityL: 2000, status: 'planned' })
  })

  it('maps a day without task', async () => {
    const { api } = servicesWith({ ...briefDto, task: null })
    expect((await api.today.getBrief()).task).toBeNull()
  })

  it('starts a task with POST', async () => {
    const { api, calls } = servicesWith({ ...briefDto.task, status: 'in_progress' })
    const task = await api.today.startTask('task-1')

    expect(calls[0]).toMatchObject({ path: '/planner/today/tasks/task-1/start', options: { method: 'POST' } })
    expect(task.status).toBe('in_progress')
  })
})
