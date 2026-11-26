import { describe, expect, it } from 'vitest'
import { ApiError } from '~/services/errors'
import { createMockServices } from '~/services/mock/mockServices'
import { createPendingMockServices, PENDING_SERVICE_KEYS } from '~/services/mock/pendingServices'
import { createMockTrafficabilityService } from '~/services/mock/trafficabilityMock'
import type { TrafficabilityService } from '~/services/ports'
import type { Field } from '~/types/field'
import type { FieldTrafficability } from '~/types/trafficability'
import { createSsoRealServices } from '../../support/pending'

/** 07:00 in São Paulo: the calendar day there is 2026-11-26. */
const NOW = new Date('2026-11-26T10:00:00Z')

function worldAs(userId: string): TrafficabilityService {
  return createMockServices({ latencyMs: 0, getAccessToken: () => `mock-token:${userId}`, now: () => NOW }).trafficability
}

async function rejection(promise: Promise<unknown>): Promise<ApiError> {
  const error = await promise.then(() => null, (caught: unknown) => caught)
  expect(error).toBeInstanceOf(ApiError)
  return error as ApiError
}

const verdictOf = (view: FieldTrafficability, dayIndex: number, method: string) =>
  view.days[dayIndex]!.methods.find(item => item.method === method)

describe('mock trafficability: Talhão 4, clay and waterlogged (example of the contract)', () => {
  it('answers the soil of today from the gauge, with the model moisture as trend', async () => {
    const view = await worldAs('user-marina').getFieldTrafficability('farm-santa-rita', 'field-str-4')
    expect(view).toMatchObject({ fieldId: 'field-str-4', farmId: 'farm-santa-rita', asOf: NOW, dataIssues: [] })
    expect(view.soil).toMatchObject({
      state: 'waterlogged',
      basis: 'measured_rain',
      texture: 'clay',
      thresholdsSource: 'heuristic',
      rain: { last24hMm: 6, last48hMm: 24, last72hMm: 55, last7dMm: 71, source: 'gauge', sourceName: 'Pluviômetro da sede' },
      modelMoisture: { fieldCapacityPct: 113.9 },
      inspection: null,
    })
  })

  it('has 7 measured days and 5 forecast days, 2 mm tomorrow and 28 mm on the fifth', async () => {
    const view = await worldAs('user-marina').getFieldTrafficability('farm-santa-rita', 'field-str-4')
    const measured = view.rainDaily.filter(entry => entry.kind === 'measured')
    const forecast = view.rainDaily.filter(entry => entry.kind === 'forecast')
    expect(measured.map(entry => entry.date)).toEqual(['2026-11-20', '2026-11-21', '2026-11-22', '2026-11-23', '2026-11-24', '2026-11-25', '2026-11-26'])
    expect(measured.every(entry => entry.source === 'gauge')).toBe(true)
    expect(measured.slice(-2).map(entry => entry.rainMm)).toEqual([18, 6])
    expect(forecast.map(entry => [entry.date, entry.rainMm])).toEqual([
      ['2026-11-27', 2],
      ['2026-11-28', 0],
      ['2026-11-29', 0],
      ['2026-11-30', 0],
      ['2026-12-01', 28],
    ])
    expect(forecast.every(entry => entry.source === 'model')).toBe(true)
  })

  it('lists today to +5 with the six methods in contract order', async () => {
    const view = await worldAs('user-marina').getFieldTrafficability('farm-santa-rita', 'field-str-4')
    expect(view.days.map(day => [day.date, day.soilState])).toEqual([
      ['2026-11-26', 'waterlogged'],
      ['2026-11-27', 'waterlogged'],
      ['2026-11-28', 'moist'],
      ['2026-11-29', 'ideal'],
      ['2026-11-30', 'ideal'],
      ['2026-12-01', 'waterlogged'],
    ])
    for (const day of view.days) {
      expect(day.methods.map(item => item.method)).toEqual(['knapsack', 'tractor_boom', 'self_propelled', 'airblast', 'aircraft', 'drone'])
    }
  })

  it('follows the ADR 0022 matrix: aerial always apt, worst of relief and soil, restricted slope on wet soil unfit', async () => {
    const view = await worldAs('user-marina').getFieldTrafficability('farm-santa-rita', 'field-str-4')
    // Waterlogged today.
    expect(verdictOf(view, 0, 'knapsack')).toMatchObject({ verdict: 'attention', reasons: ['soil_waterlogged'] })
    expect(verdictOf(view, 0, 'tractor_boom')).toMatchObject({ verdict: 'unfit', reasons: ['slope_restricted', 'soil_waterlogged'], areaAboveLimitPct: 62 })
    expect(verdictOf(view, 0, 'drone')).toMatchObject({ verdict: 'apt', reasons: [] })
    expect(verdictOf(view, 0, 'aircraft')).toMatchObject({ verdict: 'apt', reasons: [] })
    // Moist on +2: light machine released, heavy one on a restricted slope unfit.
    expect(verdictOf(view, 2, 'knapsack')).toMatchObject({ verdict: 'apt' })
    expect(verdictOf(view, 2, 'airblast')).toMatchObject({ verdict: 'unfit', reasons: ['slope_restricted', 'soil_moist', 'slope_restricted_on_wet_soil'] })
    // Ideal on +3: only the relief limits.
    expect(verdictOf(view, 3, 'tractor_boom')).toMatchObject({ verdict: 'attention', reasons: ['slope_restricted'] })
    expect(verdictOf(view, 3, 'self_propelled')).toMatchObject({ verdict: 'unfit', reasons: ['slope_unsuitable'] })
  })

  it('releases the heavy methods on +3 and the knapsack on +2, ground methods only', async () => {
    const view = await worldAs('user-marina').getFieldTrafficability('farm-santa-rita', 'field-str-4')
    expect(view.releases).toEqual([
      { method: 'knapsack', requiredState: 'moist', date: '2026-11-28' },
      { method: 'tractor_boom', requiredState: 'ideal', date: '2026-11-29' },
      { method: 'self_propelled', requiredState: 'ideal', date: '2026-11-29' },
      { method: 'airblast', requiredState: 'ideal', date: '2026-11-29' },
    ])
  })

  it('cuts days, forecast and releases to the requested horizon', async () => {
    const view = await worldAs('user-marina').getFieldTrafficability('farm-santa-rita', 'field-str-4', 2)
    expect(view.days).toHaveLength(3)
    expect(view.rainDaily.filter(entry => entry.kind === 'forecast')).toHaveLength(2)
    expect(view.releases.find(item => item.method === 'knapsack')?.date).toBe('2026-11-28')
    expect(view.releases.find(item => item.method === 'tractor_boom')?.date).toBeNull()
  })

  it('rejects a horizon outside 1–7 with 422 on `days`', async () => {
    const error = await rejection(worldAs('user-marina').getFieldTrafficability('farm-santa-rita', 'field-str-4', 8))
    expect(error).toMatchObject({ code: 'validation_failed', status: 422 })
    expect(error.details.map(item => item.field)).toEqual(['days'])
  })
})

describe('mock trafficability: no rain source', () => {
  it('answers unknown with ground methods in attention and nothing released (Talhão 3 of Ouro Verde)', async () => {
    const view = await worldAs('user-bruno').getFieldTrafficability('farm-ouro-verde', 'field-fov-3')
    expect(view.soil).toMatchObject({ state: 'unknown', basis: 'none', rain: null, modelMoisture: null })
    expect(view.dataIssues).toEqual(['no_rain_source'])
    expect(view.rainDaily).toEqual([])
    expect(view.days).toHaveLength(6)
    for (const day of view.days) {
      expect(day.soilState).toBe('unknown')
      for (const item of day.methods) {
        const aerial = item.method === 'aircraft' || item.method === 'drone'
        expect(item).toMatchObject(aerial ? { verdict: 'apt', reasons: [] } : { verdict: 'attention', reasons: ['soil_unknown'] })
      }
    }
    expect(view.releases.every(item => item.date === null)).toBe(true)
  })

  it('is the fallback of a field without a fixture of its own', async () => {
    const view = await worldAs('user-marina').getFieldTrafficability('farm-santa-rita', 'field-str-1')
    expect(view).toMatchObject({ fieldId: 'field-str-1', farmId: 'farm-santa-rita', dataIssues: ['no_rain_source'] })
    expect(view.soil.state).toBe('unknown')
  })
})

describe('mock trafficability: scope', () => {
  it('answers 404 for a farm of another company, never revealing it', async () => {
    const error = await rejection(worldAs('user-marina').getFieldTrafficability('farm-ouro-verde', 'field-fov-3'))
    expect(error).toMatchObject({ code: 'not_found', status: 404 })
  })

  it('answers 404 for a field that is not in the given farm', async () => {
    const error = await rejection(worldAs('user-bruno').getFieldTrafficability('farm-ouro-verde', 'field-str-4'))
    expect(error).toMatchObject({ code: 'not_found', status: 404 })
  })

  it('requires a session', async () => {
    const service = createMockServices({ latencyMs: 0, getAccessToken: () => null, now: () => NOW }).trafficability
    const error = await rejection(service.getFieldTrafficability('farm-santa-rita', 'field-str-4'))
    expect(error).toMatchObject({ code: 'unauthorized', status: 401 })
  })
})

describe('mock trafficability with the real API (NUXT_PUBLIC_API_MOCK_PENDING)', () => {
  const realField = { id: 'fld_real', name: 'Talhão A' } as Field

  it('lists the fields through the given field service, so real ids resolve', async () => {
    const service = createMockTrafficabilityService({ fields: { listByFarm: async () => [realField] }, now: () => NOW })
    const view = await service.getFieldTrafficability('farm_real', 'fld_real')
    expect(view).toMatchObject({ fieldId: 'fld_real', farmId: 'farm_real', soil: { state: 'unknown' } })
  })

  it('is no longer a pending service: the real API serves it (ADR 0022, phase 2)', () => {
    const real = createSsoRealServices()
    const merged = createPendingMockServices(real, { latencyMs: 0, getRole: () => 'admin' })
    expect(PENDING_SERVICE_KEYS).not.toContain('trafficability')
    expect(merged.trafficability).toBe(real.trafficability)
  })
})
