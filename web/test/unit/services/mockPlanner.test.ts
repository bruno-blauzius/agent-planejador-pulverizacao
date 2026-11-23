import { describe, expect, it } from 'vitest'
import { ApiError } from '~/services/errors'
import { createMockServices } from '~/services/mock/mockServices'
import type { PlannerFaults } from '~/services/mock/plannerServices'
import type { ApiServices } from '~/services/ports'
import type { PlannerMessageInput } from '~/types/planner'
import { SCENARIO } from '../../support/planner'

const NOW = new Date('2026-11-21T12:00:00Z')
const TOKENS = { admin: 'mock-token:user-marina', operator: 'mock-token:user-carlos', superAdmin: 'mock-token:user-bruno' }

function apiFor(token: string, faults: PlannerFaults = {}): ApiServices {
  return createMockServices({ latencyMs: 0, getAccessToken: () => token, now: () => NOW, plannerFaults: faults })
}

const ask = (text: string, fieldIds: string[] = [SCENARIO.fieldT3, SCENARIO.fieldT7]): PlannerMessageInput => ({
  conversationId: null,
  text,
  context: { farmId: SCENARIO.farmId, fieldIds, productKind: 'fungicide' },
})

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

describe('mock planner chat', () => {
  it('answers the Figma question with the plan for T3 and T7', async () => {
    const reply = await apiFor(TOKENS.admin).planner.sendMessage(ask('Preciso aplicar fungicida nos talhões 3 e 7 essa semana. Quando faço?'))

    expect(reply.message.text).toContain('A melhor janela é sábado (21/11), das 06h às 10h:')
    expect(reply.message.text).toContain('Os dois talhões somam 90 ha e o pulverizador faz 12 ha/h, ou seja, cerca de 7h30 de trabalho.')
    expect(reply.message.text).toContain('Domingo tem 18 mm de chuva prevista.')
    expect(reply.consulted.map(source => source.title)).toEqual([
      'Previsão do tempo',
      'Condições de aplicação',
      'Regras do produto',
      'Rendimento operacional',
    ])
    expect(reply.suggestions).toEqual(['E se chover no sábado?', 'Comparar com terça', 'Só o talhão 3'])

    const plan = reply.plan!
    expect(plan).toMatchObject({ headline: '2 janelas aptas cobrem os 90 ha', totalAreaHa: 90, capacityHaPerHour: 12 })
    expect(plan.entries.map(entry => [entry.fieldLabel, entry.areaHa, entry.durationMinutes, entry.window.date])).toEqual([
      ['T3', 38.6, 195, '2026-11-21'],
      ['T7', 51.4, 260, '2026-11-23'],
    ])
    expect(plan.entries[0]?.alert).toBeNull()
    expect(plan.entries[1]?.alert).toMatchObject({ kind: 'window_too_short', requiredMinutes: 260, availableMinutes: 240 })
    expect(plan.entries[1]?.alert?.message).toBe('T7 precisa de 4h20 e a janela tem 4h. O agente sugere começar 05:40 ou deixar 3 ha para terça.')
  })

  it('is deterministic: same message, same content', async () => {
    const api = apiFor(TOKENS.admin)
    const first = await api.planner.sendMessage(ask('Quando faço?'))
    const second = await api.planner.sendMessage(ask('Quando faço?'))
    expect(second.message.text).toBe(first.message.text)
    expect(second.plan?.entries).toEqual(first.plan?.entries)
  })

  it('keeps the conversation id and follows the suggested questions', async () => {
    const api = apiFor(TOKENS.admin)
    const first = await api.planner.sendMessage(ask('Quando faço?'))
    const rain = await api.planner.sendMessage({ ...ask('E se chover no sábado?'), conversationId: first.conversationId })
    expect(rain.conversationId).toBe(first.conversationId)
    expect(rain.plan).toBeNull()
    expect(rain.message.text).toContain('próxima janela apta é segunda')

    const compare = await api.planner.sendMessage(ask('Comparar com terça'))
    expect(compare.plan).toBeNull()
    expect(compare.message.text).toContain('Comparando sábado e terça')

    const only = await api.planner.sendMessage(ask('Só o talhão 3'))
    expect(only.plan?.entries.map(entry => entry.fieldLabel)).toEqual(['T3'])
    expect(only.plan?.headline).toBe('1 janela apta cobre os 38,6 ha')
  })

  it('rejects a message without fields or text (422 with field details)', async () => {
    const api = apiFor(TOKENS.admin)
    const error = await failureOf(api.planner.sendMessage(ask('  ', [])))
    expect(error).toMatchObject({ code: 'validation_failed', status: 422 })
    expect(error.details.map(detail => detail.field)).toEqual(['text', 'context.field_ids'])
  })

  it('answers honestly when the agent or the weather are down (502), never with made-up text', async () => {
    const agent = await failureOf(apiFor(TOKENS.admin, { agent: true }).planner.sendMessage(ask('Quando faço?')))
    expect(agent).toMatchObject({ code: 'agent_unavailable', status: 502 })

    const weather = apiFor(TOKENS.admin, { weather: true })
    expect(await failureOf(weather.planner.sendMessage(ask('Quando faço?')))).toMatchObject({ code: 'weather_unavailable', status: 502 })
    expect(await failureOf(weather.planner.getWindowMap({ farmId: SCENARIO.farmId }))).toMatchObject({ code: 'weather_unavailable' })
    expect(await failureOf(weather.today.getBrief())).toMatchObject({ code: 'weather_unavailable' })
  })

  it('applies tenant and capability rules', async () => {
    expect(await failureOf(apiFor(TOKENS.admin).planner.sendMessage({ ...ask('x'), context: { farmId: 'farm-boa-esperanca', fieldIds: ['field-sbe-1'], productKind: null } })))
      .toMatchObject({ code: 'not_found', status: 404 })
    expect(await failureOf(apiFor('mock-token:nobody').planner.getWindowMap({ farmId: SCENARIO.farmId })))
      .toMatchObject({ code: 'unauthorized' })
    // The operator sees the farm released for them.
    expect((await apiFor(TOKENS.operator).planner.getWindowMap({ farmId: SCENARIO.farmId })).farmId).toBe(SCENARIO.farmId)
  })
})

describe('mock window map and conditions', () => {
  it('maps 5 days of hourly conditions per field and groups the apt windows', async () => {
    const map = await apiFor(TOKENS.admin).planner.getWindowMap({ farmId: SCENARIO.farmId, fieldIds: [SCENARIO.fieldT3, SCENARIO.fieldT7] })

    expect(map).toMatchObject({ dayStartHour: 6, dayEndHour: 20 })
    expect(map.forecastUpdatedAt).toEqual(new Date('2026-11-21T08:40:00Z'))
    expect(map.fields.map(row => row.fieldLabel)).toEqual(['T3', 'T7'])
    const days = map.fields[0]!.days
    expect(days.map(day => day.date)).toEqual(['2026-11-21', '2026-11-22', '2026-11-23', '2026-11-24', '2026-11-25'])
    expect(days.every(day => day.hours.length === 15)).toBe(true)
    expect(days[0]!.hours.slice(0, 6).map(hour => hour.condition)).toEqual(['suitable', 'suitable', 'suitable', 'suitable', 'attention', 'attention'])
    expect(days[1]!.hours.every(hour => hour.condition === 'rain')).toBe(true)

    const t3Windows = map.windows.filter(slot => slot.fieldLabel === 'T3')
    expect(t3Windows[0]).toEqual({
      fieldId: SCENARIO.fieldT3,
      fieldLabel: 'T3',
      date: '2026-11-21',
      startTime: '06:00',
      endTime: '10:00',
      durationMinutes: 240,
      condition: 'suitable',
    })
    expect(t3Windows.some(slot => slot.date === '2026-11-22')).toBe(false)
  })

  it('defaults to every field of the farm and rejects fields of another farm', async () => {
    const api = apiFor(TOKENS.admin)
    const map = await api.planner.getWindowMap({ farmId: SCENARIO.farmId, days: 2 })
    expect(map.fields.length).toBeGreaterThan(2)
    expect(map.fields[0]!.days).toHaveLength(2)
    expect(await failureOf(api.planner.getWindowMap({ farmId: SCENARIO.farmId, fieldIds: ['field-sbe-1'] })))
      .toMatchObject({ code: 'validation_failed' })
  })

  it('summarizes the best window of the day with the Figma numbers', async () => {
    const day = await apiFor(TOKENS.admin).planner.getDayConditions({ farmId: SCENARIO.farmId, date: '2026-11-21' })

    expect(day.idealRanges).toEqual({ deltaTC: { min: 2, max: 8 }, windKmh: { min: 3, max: 10 } })
    expect(day.hours.map(hour => hour.hour)).toEqual(Array.from({ length: 16 }, (_, index) => 5 + index))
    expect(day.windowSummary).toMatchObject({
      startTime: '06:00',
      endTime: '10:00',
      deltaTMinC: 2.6,
      deltaTMaxC: 6.2,
      windAvgKmh: 6.25,
      gustMaxKmh: 11,
      rainMm: 0,
      rainUntilHour: 18,
      nextRainAt: new Date('2026-11-22T06:00:00Z'),
    })
  })

  it('has no window on the rainy day and summarizes a requested window', async () => {
    const api = apiFor(TOKENS.admin)
    const rainy = await api.planner.getDayConditions({ farmId: SCENARIO.farmId, date: '2026-11-22' })
    expect(rainy.windowSummary).toBeNull()
    expect(rainy.hours.every(hour => hour.condition === 'rain')).toBe(true)
    expect(rainy.hours.reduce((sum, hour) => sum + hour.rainMm, 0)).toBeCloseTo(18)

    const custom = await api.planner.getDayConditions({ farmId: SCENARIO.farmId, date: '2026-11-21', windowStart: '07:00', windowEnd: '09:00' })
    expect(custom.windowSummary).toMatchObject({ startTime: '07:00', endTime: '09:00', deltaTMinC: 4.2, deltaTMaxC: 5.1 })
    expect(await failureOf(api.planner.getDayConditions({ farmId: SCENARIO.farmId, date: '2026-12-31' })))
      .toMatchObject({ code: 'validation_failed' })
  })
})

async function planOf(api: ApiServices) {
  const reply = await api.planner.sendMessage(ask('Quando faço?'))
  return reply.plan!
}

describe('mock human confirmation', () => {
  it('reviews T3 with the six checks and the Figma numbers', async () => {
    const api = apiFor(TOKENS.admin)
    const plan = await planOf(api)
    const review = await api.applications.review(plan.id, SCENARIO.fieldT3)

    expect(review).toMatchObject({
      fieldLabel: 'T3',
      variety: 'Catuaí 144',
      areaHa: 38.6,
      mixVolumeL: 7720,
      tankCount: 4,
      tankCapacityL: 2000,
      canConfirm: true,
      product: { name: 'Fungicida', dose: { value: 0.5, unit: 'L/ha' }, mixRateLPerHa: 200 },
      operator: { name: 'Carlos Mendes' },
      equipment: { name: 'Pulverizador 2.000 L' },
    })
    expect(review.checks.map(check => [check.code, check.status])).toEqual([
      ['delta_t', 'ok'],
      ['wind', 'ok'],
      ['rain_after', 'ok'],
      ['interval_since_last', 'ok'],
      ['pre_harvest_interval', 'ok'],
      ['operator_equipment', 'ok'],
    ])
    expect(review.checks[0]?.detail).toBe('2,6 a 6,2 °C (ideal 2 a 8 °C)')
    expect(review.checks[1]?.detail).toBe('4 a 8 km/h, rajada máxima 11 km/h')
    expect(review.checks[2]?.detail).toBe('8h livres; próxima chuva domingo 03h')
    expect(review.checks[3]?.detail).toBe('21 dias (mínimo do produto: 14 dias)')
    expect(review.checks[5]?.detail).toBe('Carlos Mendes · Pulverizador 2.000 L')
  })

  it('adds a warning check for the short window of T7 that must be acknowledged', async () => {
    const api = apiFor(TOKENS.admin)
    const plan = await planOf(api)
    const review = await api.applications.review(plan.id, SCENARIO.fieldT7)
    expect(review.checks.at(-1)).toMatchObject({ code: 'window_fits', status: 'warning' })
    expect(review.tankCount).toBe(6)

    const base = { planId: plan.id, fieldId: SCENARIO.fieldT7, reviewId: review.id }
    const missing = await failureOf(api.applications.register({ ...base, confirmation: { confirmed: true, acknowledgedWarnings: [] } }))
    expect(missing).toMatchObject({ code: 'validation_failed' })
    expect(missing.details[0]?.field).toBe('confirmation.acknowledged_warnings')

    const done = await api.applications.register({ ...base, confirmation: { confirmed: true, acknowledgedWarnings: ['window_fits'] } })
    expect(done.status).toBe('planned')
  })

  it('registers only with the review and the explicit confirmation, once', async () => {
    const api = apiFor(TOKENS.admin)
    const plan = await planOf(api)
    const review = await api.applications.review(plan.id, SCENARIO.fieldT3)
    const input = { planId: plan.id, fieldId: SCENARIO.fieldT3, reviewId: review.id, confirmation: { confirmed: true as const, acknowledgedWarnings: [] } }

    const notConfirmed = { ...input, confirmation: { ...input.confirmation, confirmed: false as unknown as true } }
    expect(await failureOf(api.applications.register(notConfirmed))).toMatchObject({ code: 'validation_failed' })
    expect(await failureOf(api.applications.register({ ...input, reviewId: 'review-unknown' }))).toMatchObject({ code: 'not_found' })

    const application = await api.applications.register(input)
    expect(application).toMatchObject({ status: 'planned', fieldLabel: 'T3', areaHa: 38.6, registeredBy: { name: 'Marina Rezende' } })
    expect(application.registeredAt).toEqual(NOW)
    expect(await failureOf(api.applications.register(input))).toMatchObject({ code: 'conflict', status: 409 })
  })

  it('answers conditions_changed when the forecast moved and refuses blocked reviews', async () => {
    const faults: PlannerFaults = {}
    const api = apiFor(TOKENS.admin, faults)
    const plan = await planOf(api)
    const review = await api.applications.review(plan.id, SCENARIO.fieldT3)
    const input = { planId: plan.id, fieldId: SCENARIO.fieldT3, reviewId: review.id, confirmation: { confirmed: true as const, acknowledgedWarnings: [] } }

    faults.conditionsChanged = true
    expect(await failureOf(api.applications.register(input))).toMatchObject({ code: 'conditions_changed', status: 409 })

    faults.conditionsChanged = false
    faults.blockedCheck = 'wind'
    const blocked = await api.applications.review(plan.id, SCENARIO.fieldT3)
    expect(blocked.canConfirm).toBe(false)
    expect(await failureOf(api.applications.register({ ...input, reviewId: blocked.id }))).toMatchObject({ code: 'validation_failed' })
  })

  it('requires applications.register and a plan the caller created', async () => {
    const admin = apiFor(TOKENS.admin)
    const plan = await planOf(admin)
    expect(await failureOf(admin.applications.review('plan-unknown', SCENARIO.fieldT3))).toMatchObject({ code: 'not_found' })
    expect(await failureOf(admin.applications.review(plan.id, 'field-str-1'))).toMatchObject({ code: 'not_found' })
  })
})

describe('mock operator "Hoje"', () => {
  it('serves the Figma brief for the operator', async () => {
    const brief = await apiFor(TOKENS.operator).today.getBrief()

    expect(brief.farm).toEqual({ id: SCENARIO.farmId, name: 'Fazenda Santa Rita' })
    expect(brief.asOf).toEqual(new Date('2026-11-21T09:48:00Z'))
    expect(brief.status).toEqual({
      condition: 'suitable',
      headline: 'Pode pulverizar',
      untilTime: '10:00',
      remainingMinutes: 192,
      windowStart: '06:00',
      windowEnd: '10:00',
    })
    expect(brief.metrics.map(metric => [metric.label, metric.value, metric.unit])).toEqual([
      ['Delta T', 4.2, '°C'],
      ['Vento', 6, 'km/h'],
      ['Chuva 4h', 0, 'mm'],
    ])
    expect(brief.hours.map(hour => hour.hour)).toEqual(Array.from({ length: 13 }, (_, index) => 6 + index))
    expect(brief.hours.filter(hour => hour.isCurrent).map(hour => hour.hour)).toEqual([6])
    expect(brief.task).toMatchObject({
      fieldLabel: 'T3',
      fieldName: 'Talhão 3',
      operation: 'Fungicida',
      areaHa: 38.6,
      dose: { value: 0.5, unit: 'L/ha' },
      tankCount: 4,
      tankCapacityL: 2000,
      status: 'planned',
    })
  })

  it('starts the task once and then reports it in progress', async () => {
    const api = apiFor(TOKENS.operator)
    const brief = await api.today.getBrief()
    const started = await api.today.startTask(brief.task!.id)

    expect(started.status).toBe('in_progress')
    expect((await api.today.getBrief()).task?.status).toBe('in_progress')
    expect(await failureOf(api.today.startTask(brief.task!.id))).toMatchObject({ code: 'conflict', status: 409 })
    expect(await failureOf(api.today.startTask('task-unknown'))).toMatchObject({ code: 'not_found' })
  })
})
