import { describe, expect, it } from 'vitest'
import { ApiError } from '~/services/errors'
import {
  createFieldConditionsStore,
  createMockRainGaugeService,
  MOCK_SEDE_GAUGE_ID,
  MOCK_SOUTH_GAUGE_ID,
} from '~/services/mock/fieldConditionsMock'
import { createMockServices, type MockApiServices } from '~/services/mock/mockServices'

/** 07:00 in São Paulo. */
const NOW = new Date('2026-11-26T10:00:00Z')
const HOUR = 3_600_000

function worldAs(userId: string | null): MockApiServices {
  return createMockServices({ latencyMs: 0, getAccessToken: () => (userId ? `mock-token:${userId}` : null), now: () => NOW })
}

async function rejection(promise: Promise<unknown>): Promise<ApiError> {
  const error = await promise.then(() => null, (caught: unknown) => caught)
  expect(error).toBeInstanceOf(ApiError)
  return error as ApiError
}

const fieldsOf = (error: ApiError) => error.details.map(item => item.field)

describe('mock rain gauges', () => {
  it('lists the seeded gauges of Santa Rita by name', async () => {
    const gauges = await worldAs('user-carlos').rainGauges.list('farm-santa-rita')
    expect(gauges.map(gauge => [gauge.id, gauge.name])).toEqual([
      [MOCK_SOUTH_GAUGE_ID, 'Pluviômetro Área Sul'],
      [MOCK_SEDE_GAUGE_ID, 'Pluviômetro da sede'],
    ])
    expect(await worldAs('user-bruno').rainGauges.list('farm-ouro-verde')).toEqual([])
  })

  it('registers and renames with registry.write, refusing a repeated name (case aside)', async () => {
    const api = worldAs('user-marina')
    const created = await api.rainGauges.create('farm-santa-rita', { name: '  Pluviômetro Área Norte ' })
    expect(created).toMatchObject({ farmId: 'farm-santa-rita', name: 'Pluviômetro Área Norte', location: null, version: 1 })

    const renamed = await api.rainGauges.update('farm-santa-rita', created.id, { name: 'Norte' })
    expect(renamed).toMatchObject({ name: 'Norte', version: 2 })

    const repeated = await rejection(api.rainGauges.create('farm-santa-rita', { name: 'PLUVIÔMETRO DA SEDE' }))
    expect(repeated).toMatchObject({ code: 'validation_failed', status: 422 })
    expect(fieldsOf(repeated)).toEqual(['name'])
    expect(fieldsOf(await rejection(api.rainGauges.create('farm-santa-rita', { name: ' ' })))).toEqual(['name'])
  })

  it('answers 403 to an operator registering a gauge, 404 for a gauge of another farm, 401 without a session', async () => {
    expect(await rejection(worldAs('user-carlos').rainGauges.create('farm-santa-rita', { name: 'X' }))).toMatchObject({ code: 'forbidden', status: 403 })
    expect(await rejection(worldAs('user-bruno').rainGauges.update('farm-ouro-verde', MOCK_SEDE_GAUGE_ID, { name: 'X' }))).toMatchObject({ status: 404 })
    expect(await rejection(worldAs('user-marina').rainGauges.list('farm-ouro-verde'))).toMatchObject({ code: 'not_found', status: 404 })
    expect(await rejection(worldAs(null).rainGauges.list('farm-santa-rita'))).toMatchObject({ code: 'unauthorized', status: 401 })
  })

  it('hides a farm out of the operator scope as 404 (contract), not 403', async () => {
    const store = createFieldConditionsStore({ now: NOW })
    const service = createMockRainGaugeService({
      fields: { listByFarm: async () => { throw new ApiError({ code: 'out_of_scope', message: 'fora do escopo', status: 403 }) } },
      store,
      can: () => true,
      actor: () => ({ id: 'u', name: 'U' }),
      now: () => NOW,
    })
    expect(await rejection(service.list('farm-santa-rita'))).toMatchObject({ code: 'not_found', status: 404 })
  })
})

describe('mock rain readings', () => {
  it('lists the last 7 days newest first, and validates the range', async () => {
    const api = worldAs('user-carlos')
    const readings = await api.rainGauges.listReadings('farm-santa-rita', MOCK_SEDE_GAUGE_ID)
    expect(readings.map(item => item.rainMm)).toEqual([6, 18, 31, 6, 0, 10, 0])
    expect(readings[0]!.observedAt).toEqual(new Date(NOW.getTime() - HOUR))

    const inverted = await rejection(api.rainGauges.listReadings('farm-santa-rita', MOCK_SEDE_GAUGE_ID, { from: NOW, to: new Date(NOW.getTime() - HOUR) }))
    expect(fieldsOf(inverted)).toEqual(['from'])
    const tooLong = await rejection(api.rainGauges.listReadings('farm-santa-rita', MOCK_SEDE_GAUGE_ID, { from: new Date('2026-10-01T00:00:00Z'), to: NOW }))
    expect(fieldsOf(tooLong)).toEqual(['from'])
  })

  it('lets an operator record a reading, which enters the rain of the linked field', async () => {
    const api = worldAs('user-carlos')
    const reading = await api.rainGauges.recordReading('farm-santa-rita', MOCK_SEDE_GAUGE_ID, { rainMm: 4.5, observedAt: new Date(NOW.getTime() - 10 * 60_000) })
    expect(reading).toMatchObject({ rainMm: 4.5, recordedBy: { id: 'user-carlos', name: 'Carlos Mendes' }, recordedAt: NOW })

    const view = await api.trafficability.getFieldTrafficability('farm-santa-rita', 'field-str-4')
    expect(view.soil.rain).toMatchObject({ last24hMm: 10.5, last48hMm: 28.5, last7dMm: 75.5, source: 'gauge', sourceName: 'Pluviômetro da sede' })
    expect(view.soil.rain?.observedAt).toEqual(reading.observedAt)
    expect(view.rainDaily.find(entry => entry.date === '2026-11-26')).toMatchObject({ rainMm: 10.5, kind: 'measured', source: 'gauge' })
    // The state is not recalculated by the mock: it stays as the fixture.
    expect(view.soil.state).toBe('waterlogged')
  })

  it('validates mm (0–500), the future and a repeated instant', async () => {
    const api = worldAs('user-carlos')
    const record = (rainMm: number, observedAt: Date) => rejection(api.rainGauges.recordReading('farm-santa-rita', MOCK_SEDE_GAUGE_ID, { rainMm, observedAt }))
    expect(fieldsOf(await record(501, NOW))).toEqual(['rain_mm'])
    expect(fieldsOf(await record(-1, NOW))).toEqual(['rain_mm'])
    expect(fieldsOf(await record(1, new Date(NOW.getTime() + 60_000)))).toEqual(['observed_at'])
    expect(fieldsOf(await record(1, new Date(NOW.getTime() - HOUR)))).toEqual(['observed_at'])
  })
})

describe('mock soil inspections', () => {
  it('replaces today\'s state and basis, with the seeded verdicts of that state', async () => {
    const api = worldAs('user-carlos')
    const inspection = await api.soilConditions.recordInspection('farm-santa-rita', 'field-str-4', { state: 'ideal', note: '  firme  ' })
    expect(inspection).toMatchObject({ state: 'ideal', inspectedAt: NOW, note: 'firme', inspectedBy: { id: 'user-carlos' } })
    expect(inspection.validUntil).toEqual(new Date(NOW.getTime() + 12 * HOUR))

    const view = await api.trafficability.getFieldTrafficability('farm-santa-rita', 'field-str-4')
    expect(view.soil).toMatchObject({ state: 'ideal', basis: 'inspection', inspection: { state: 'ideal', inspectedBy: { name: 'Carlos Mendes' } } })
    expect(view.days[0]!.soilState).toBe('ideal')
    expect(view.days[0]!.methods.find(item => item.method === 'tractor_boom')).toMatchObject({ verdict: 'attention', reasons: ['slope_restricted'] })
    // The projection is the fixture's.
    expect(view.days[1]!.soilState).toBe('waterlogged')
  })

  it('ignores an expired inspection', async () => {
    const api = worldAs('user-carlos')
    await api.soilConditions.recordInspection('farm-santa-rita', 'field-str-4', { state: 'dry', inspectedAt: new Date(NOW.getTime() - 13 * HOUR) })
    const view = await api.trafficability.getFieldTrafficability('farm-santa-rita', 'field-str-4')
    expect(view.soil).toMatchObject({ state: 'waterlogged', basis: 'measured_rain', inspection: null })
  })

  it('validates state, time and note; 404 for a field of another farm', async () => {
    const api = worldAs('user-carlos')
    const unknownState = await rejection(api.soilConditions.recordInspection('farm-santa-rita', 'field-str-4', { state: 'unknown' as never }))
    expect(fieldsOf(unknownState)).toEqual(['state'])
    const future = await rejection(api.soilConditions.recordInspection('farm-santa-rita', 'field-str-4', { state: 'dry', inspectedAt: new Date(NOW.getTime() + HOUR) }))
    expect(fieldsOf(future)).toEqual(['inspected_at'])
    const longNote = await rejection(api.soilConditions.recordInspection('farm-santa-rita', 'field-str-4', { state: 'dry', note: 'x'.repeat(501) }))
    expect(fieldsOf(longNote)).toEqual(['note'])
    expect(await rejection(api.soilConditions.recordInspection('farm-santa-rita', 'field-fov-3', { state: 'dry' }))).toMatchObject({ status: 404 })
  })
})

describe('mock soil profile', () => {
  it('reads the profile of Talhão 4 (clay, sede gauge) and of a field without one', async () => {
    const api = worldAs('user-carlos')
    expect(await api.soilConditions.getProfile('farm-santa-rita', 'field-str-4')).toEqual({
      fieldId: 'field-str-4',
      farmId: 'farm-santa-rita',
      soilTexture: 'clay',
      rainGauge: { id: MOCK_SEDE_GAUGE_ID, name: 'Pluviômetro da sede' },
    })
    expect(await api.soilConditions.getProfile('farm-santa-rita', 'field-str-1')).toMatchObject({ soilTexture: 'medium', rainGauge: null })
  })

  it('needs registry.write, and refuses a gauge of another farm on rain_gauge_id', async () => {
    expect(await rejection(worldAs('user-carlos').soilConditions.updateProfile('farm-santa-rita', 'field-str-1', { soilTexture: 'clay', rainGaugeId: null })))
      .toMatchObject({ code: 'forbidden', status: 403 })
    const api = worldAs('user-bruno')
    const otherFarm = await rejection(api.soilConditions.updateProfile('farm-ouro-verde', 'field-fov-3', { soilTexture: 'clay', rainGaugeId: MOCK_SEDE_GAUGE_ID }))
    expect(fieldsOf(otherFarm)).toEqual(['rain_gauge_id'])
  })

  it('links a gauge to a field without rain source: its readings show up and the gap goes away', async () => {
    const api = worldAs('user-marina')
    const before = await api.trafficability.getFieldTrafficability('farm-santa-rita', 'field-str-1')
    expect(before.dataIssues).toEqual(['no_rain_source'])

    await api.soilConditions.updateProfile('farm-santa-rita', 'field-str-1', { soilTexture: 'medium', rainGaugeId: MOCK_SEDE_GAUGE_ID })
    const after = await api.trafficability.getFieldTrafficability('farm-santa-rita', 'field-str-1')
    expect(after.dataIssues).toEqual([])
    expect(after.soil.rain).toMatchObject({ last48hMm: 24, source: 'gauge' })
    expect(after.rainDaily.filter(entry => entry.kind === 'measured')).toHaveLength(7)
    // Not recalculated: still the fixture's state.
    expect(after.soil.state).toBe('unknown')
  })

  it('reports no_soil_texture once the texture is cleared', async () => {
    const api = worldAs('user-marina')
    await api.soilConditions.updateProfile('farm-santa-rita', 'field-str-4', { soilTexture: null, rainGaugeId: MOCK_SEDE_GAUGE_ID })
    const view = await api.trafficability.getFieldTrafficability('farm-santa-rita', 'field-str-4')
    expect(view.soil.texture).toBeNull()
    expect(view.dataIssues).toEqual(['no_soil_texture'])
  })

  it('stops using a gauge whose latest reading is older than 36 h', async () => {
    let clock = NOW
    const api = createMockServices({ latencyMs: 0, getAccessToken: () => 'mock-token:user-marina', now: () => clock })
    clock = new Date(NOW.getTime() + 40 * HOUR)
    const view = await api.trafficability.getFieldTrafficability('farm-santa-rita', 'field-str-4')
    // Falls back to the fixture numbers (the mock does not model the weather model).
    expect(view.soil.rain?.last24hMm).toBe(6)
    expect(view.soil.rain?.observedAt).toEqual(clock)
  })
})

describe('mock field conditions: review fixes', () => {
  it('checks the farm scope before the capability: another company is 404, not 403', async () => {
    const operator = worldAs('user-carlos')
    expect(await rejection(operator.rainGauges.create('farm-ouro-verde', { name: 'X' }))).toMatchObject({ code: 'not_found', status: 404 })
    expect(await rejection(operator.soilConditions.updateProfile('farm-ouro-verde', 'field-fov-3', { soilTexture: null, rainGaugeId: null })))
      .toMatchObject({ code: 'not_found', status: 404 })
  })

  it('treats a gauge whose latest reading is exactly 36 h old as stale (strict, like the API)', async () => {
    let clock = NOW
    const api = createMockServices({ latencyMs: 0, getAccessToken: () => 'mock-token:user-marina', now: () => clock })
    // Latest seeded reading is one hour before NOW.
    clock = new Date(NOW.getTime() - HOUR + 36 * HOUR)
    const view = await api.trafficability.getFieldTrafficability('farm-santa-rita', 'field-str-4')
    expect(view.soil.rain?.observedAt).toEqual(clock)
  })
})
