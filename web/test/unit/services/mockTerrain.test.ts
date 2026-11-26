import { describe, expect, it } from 'vitest'
import { ApiError } from '~/services/errors'
import { createMockServices } from '~/services/mock/mockServices'
import { createMockTerrainService } from '~/services/mock/terrainMock'
import type { FieldService, TerrainService } from '~/services/ports'
import type { Field } from '~/types/field'
import type { FarmTerrain } from '~/types/terrain'

const NOW = new Date('2026-11-21T12:00:00Z')

function worldAs(userId: string): TerrainService {
  return createMockServices({ latencyMs: 0, getAccessToken: () => `mock-token:${userId}`, now: () => NOW }).terrain
}

async function rejection(promise: Promise<unknown>): Promise<ApiError> {
  const error = await promise.then(() => null, (caught: unknown) => caught)
  expect(error).toBeInstanceOf(ApiError)
  return error as ApiError
}

describe('mock terrain: seed of the mock world', () => {
  it('calculates Santa Rita, with a strongly undulating field, an adjusted one and a failed one', async () => {
    const terrain = await worldAs('user-marina').getFarm('farm-santa-rita')
    expect(terrain.progress).toBeNull()
    expect(terrain.items).toHaveLength(9)
    const byId = Object.fromEntries(terrain.items.map(item => [item.fieldId, item]))

    expect(byId['field-str-4']).toMatchObject({
      status: 'ready',
      source: 'calculated',
      hasRestrictedMethod: true,
      elevationMinM: 872,
      elevationMaxM: 941,
      measurement: { slopeMeanPct: 24.3, slopeClass: 'strongly_undulating', position: 'slope' },
    })
    expect(byId['field-str-2']).toMatchObject({ source: 'manual', measurement: { slopeMeanPct: 14.2, slopeP90Pct: null, aspect: null, slopeClass: 'undulating' } })
    expect(byId['field-str-9']).toMatchObject({ status: 'failed', measurement: null, source: null })
    expect(byId['field-str-9']!.failureReason).toMatch(/modelo de elevação/)
    expect(terrain.summary).toMatchObject({ fieldCount: 9, calculatedFieldCount: 8 })
    expect(terrain.summary.restrictedFieldCount).toBeGreaterThanOrEqual(1)
    expect(terrain.summary.calculatedAt).toEqual(new Date('2026-11-12T12:00:00Z'))
  })

  it('answers the field detail of the forte ondulado field: tractor restricted by 62 %, self-propelled unsuitable', async () => {
    const detail = await worldAs('user-marina').getField('farm-santa-rita', 'field-str-4')
    const fit = (method: string) => detail.methodFits.find(item => item.method === method)
    expect(fit('tractor_boom')).toMatchObject({ status: 'restricted', slopeLimitPct: 20, areaAboveLimitPct: 62, limitSource: 'reference' })
    expect(fit('self_propelled')).toMatchObject({ status: 'unsuitable' })
    expect(fit('knapsack')).toMatchObject({ status: 'compatible', slopeLimitPct: null })
    expect(detail.calculated?.classAreas.map(item => item.sharePct)).toEqual([0, 8, 30, 52, 10])
    expect(detail.calculated?.profile.length).toBeGreaterThan(2)
    expect(detail.calculated?.elevationModel).toMatchObject({ source: 'Copernicus DEM GLO-30', resolutionM: 30 })
  })

  it('shows the adjusted field with the calculated value kept for comparison', async () => {
    const detail = await worldAs('user-marina').getField('farm-santa-rita', 'field-str-2')
    expect(detail.source).toBe('manual')
    expect(detail.adjustment).toMatchObject({ origin: 'topographic_survey', adjustedBy: { name: 'Marina Rezende' } })
    expect(detail.calculated).not.toBeNull()
    expect(detail.effective).toEqual(detail.adjustment?.measurement)
    // No distribution of a manual value: the share above the limit is unknown.
    expect(detail.methodFits.every(item => item.areaAboveLimitPct === null)).toBe(true)
  })

  it('keeps a farm entirely not calculated (frame 04e) until someone calculates it', async () => {
    const terrain = await worldAs('user-bruno').getFarm('farm-boa-esperanca')
    expect(terrain.summary).toMatchObject({ fieldCount: 2, calculatedFieldCount: 0, slopeMeanPct: null, elevationMinM: null, calculatedAt: null })
    expect(terrain.items.every(item => item.status === 'not_calculated' && item.measurement === null)).toBe(true)
  })

  it('gives the same relief for the same field every time', async () => {
    const first = await worldAs('user-marina').getFarm('farm-santa-rita')
    const second = await worldAs('user-marina').getFarm('farm-santa-rita')
    expect(second.items).toEqual(first.items)
  })

  it('has a flat field in a lowland', async () => {
    const detail = await worldAs('user-bruno').getField('farm-ouro-verde', 'field-fov-3')
    expect(detail.effective).toMatchObject({ slopeClass: 'flat', position: 'lowland', aspect: 'flat' })
  })
})

describe('mock terrain: access', () => {
  it('requires a session, hides other companies and answers 404 for an unknown field', async () => {
    const signedOut = createMockServices({ latencyMs: 0, getAccessToken: () => null }).terrain
    expect((await rejection(signedOut.getFarm('farm-santa-rita'))).status).toBe(401)
    expect((await rejection(worldAs('user-marina').getFarm('farm-ouro-verde'))).status).toBe(404)
    expect((await rejection(worldAs('user-marina').getField('farm-santa-rita', 'field-nope'))).status).toBe(404)
  })

  it('lets a reader without registry.write see the relief but not change it (403)', async () => {
    const operator = worldAs('user-carlos')
    expect((await operator.getFarm('farm-santa-rita')).items).toHaveLength(9)
    expect((await rejection(operator.calculate('farm-santa-rita', 'all'))).code).toBe('forbidden')
    expect((await rejection(operator.removeAdjustment('farm-santa-rita', 'field-str-2'))).code).toBe('forbidden')
    const input = { origin: 'field_assessment', slopeMeanPct: 10, slopeMaxPct: 12, position: 'slope', justification: 'x' } as const
    expect((await rejection(operator.adjust('farm-santa-rita', 'field-str-1', input))).code).toBe('forbidden')
  })
})

describe('mock terrain: asynchronous calculation', () => {
  it('schedules the calculation of the whole farm and finishes it over the next reads', async () => {
    const terrain = worldAs('user-bruno')
    const scheduled = await terrain.calculate('farm-boa-esperanca', 'missing')
    expect(scheduled.progress).toEqual({ total: 2, done: 0 })
    expect(scheduled.items.map(item => item.status)).toEqual(['calculating', 'pending'])
    expect(scheduled.summary.calculatedFieldCount).toBe(0)

    const first = await terrain.getFarm('farm-boa-esperanca')
    expect(first.progress).toEqual({ total: 2, done: 1 })
    expect(first.items.map(item => item.status)).toEqual(['ready', 'calculating'])

    const last = await terrain.getFarm('farm-boa-esperanca')
    expect(last.progress).toBeNull()
    expect(last.items.every(item => item.status === 'ready' && item.measurement !== null)).toBe(true)
    expect(last.summary.calculatedFieldCount).toBe(2)
    expect(last.summary.calculatedAt).toEqual(NOW)
  })

  it('does not duplicate a calculation requested while another runs', async () => {
    const terrain = worldAs('user-bruno')
    await terrain.calculate('farm-boa-esperanca', 'all')
    await terrain.getFarm('farm-boa-esperanca')
    const again = await terrain.calculate('farm-boa-esperanca', 'all')
    expect(again.progress).toEqual({ total: 2, done: 1 })
  })

  it('with scope missing recalculates only what has no relief, pending or failed', async () => {
    const terrain = worldAs('user-marina')
    const scheduled = await terrain.calculate('farm-santa-rita', 'missing')
    // Only Talhão 9 (failed) needs it.
    expect(scheduled.progress).toEqual({ total: 1, done: 0 })
    const settled = await terrain.getFarm('farm-santa-rita')
    expect(settled.progress).toBeNull()
    // It fails again, with the reason: never a made-up value.
    expect(settled.items.find(item => item.fieldId === 'field-str-9')).toMatchObject({ status: 'failed', measurement: null })
  })

  it('with scope all recalculates every field but keeps the manual adjustments', async () => {
    const terrain = worldAs('user-marina')
    const scheduled = await terrain.calculate('farm-santa-rita', 'all')
    expect(scheduled.progress).toEqual({ total: 9, done: 0 })
    let current: FarmTerrain = scheduled
    for (let reads = 0; current.progress !== null && reads < 20; reads++) current = await terrain.getFarm('farm-santa-rita')
    expect(current.progress).toBeNull()
    const adjusted = await terrain.getField('farm-santa-rita', 'field-str-2')
    expect(adjusted.adjustment?.justification).toMatch(/Levantamento/)
    expect(adjusted.calculated?.calculatedAt).toEqual(NOW)
  })

  it('answers with no progress when there is nothing to calculate', async () => {
    const terrain = worldAs('user-bruno')
    const answer = await terrain.calculate('farm-recanto', 'missing')
    expect(answer.progress).toBeNull()
  })
})

describe('mock terrain: manual adjustment', () => {
  const valid = { origin: 'topographic_survey', slopeMeanPct: 23.5, slopeMaxPct: 40, position: 'lowland', justification: ' RTK de 2026 ' } as const

  it('creates the adjustment, recalculates the class and records who did it', async () => {
    const terrain = worldAs('user-marina')
    const detail = await terrain.adjust('farm-santa-rita', 'field-str-1', valid)
    expect(detail.source).toBe('manual')
    expect(detail.effective).toMatchObject({ slopeMeanPct: 23.5, slopeMaxPct: 40, slopeClass: 'strongly_undulating', position: 'lowland', slopeP90Pct: null, aspect: null })
    expect(detail.adjustment).toMatchObject({ justification: 'RTK de 2026', adjustedBy: { id: 'user-marina', name: 'Marina Rezende' }, adjustedAt: NOW })
    expect(detail.calculated).not.toBeNull()
    const farm = await terrain.getFarm('farm-santa-rita')
    expect(farm.items.find(item => item.fieldId === 'field-str-1')).toMatchObject({ source: 'manual', measurement: { slopeClass: 'strongly_undulating' } })
  })

  it('rejects invalid input with 422 and the API field names', async () => {
    const terrain = worldAs('user-marina')
    const error = await rejection(terrain.adjust('farm-santa-rita', 'field-str-1', { ...valid, slopeMaxPct: 10, justification: '   ' }))
    expect(error.status).toBe(422)
    expect(error.details.map(item => item.field).sort()).toEqual(['justification', 'slope_max_pct'])
    const range = await rejection(terrain.adjust('farm-santa-rita', 'field-str-1', { ...valid, slopeMeanPct: 101 }))
    expect(range.details[0]?.field).toBe('slope_mean_pct')
    // Nothing was saved.
    expect((await terrain.getField('farm-santa-rita', 'field-str-1')).adjustment).toBeNull()
  })

  it('removes the adjustment going back to the calculated value, and 404s when there is none', async () => {
    const terrain = worldAs('user-marina')
    const detail = await terrain.removeAdjustment('farm-santa-rita', 'field-str-2')
    expect(detail.adjustment).toBeNull()
    expect(detail.source).toBe('calculated')
    expect(detail.effective).toEqual(detail.calculated?.measurement)
    expect((await rejection(terrain.removeAdjustment('farm-santa-rita', 'field-str-2'))).status).toBe(404)
  })

  it('flags an adjustment far from the latest calculation as outdated (double of the API flag)', async () => {
    const terrain = worldAs('user-marina')
    const calculated = await terrain.getField('farm-santa-rita', 'field-str-1')
    const far = (calculated.calculated?.measurement.slopeMeanPct ?? 0) + 10
    const detail = await terrain.adjust('farm-santa-rita', 'field-str-1', { ...valid, slopeMeanPct: far, slopeMaxPct: far + 5 })
    expect(detail.adjustmentOutdated).toBe(true)
    const near = await terrain.adjust('farm-santa-rita', 'field-str-1', { ...valid, slopeMeanPct: far - 10, slopeMaxPct: far })
    expect(near.adjustmentOutdated).toBe(false)
  })

  it('can adjust a field that was never calculated', async () => {
    const terrain = worldAs('user-bruno')
    const detail = await terrain.adjust('farm-boa-esperanca', 'field-sbe-1', valid)
    expect(detail).toMatchObject({ status: 'not_calculated', source: 'manual', calculated: null })
    const farm = await terrain.getFarm('farm-boa-esperanca')
    expect(farm.summary.calculatedFieldCount).toBe(1)
    expect(farm.items.find(item => item.fieldId === 'field-sbe-1')).toMatchObject({ elevationMinM: null, source: 'manual' })
  })
})

describe('createMockTerrainService over fields of any source', () => {
  const polygon = { type: 'Polygon', coordinates: [] } as const
  const makeField = (id: string, name: string, areaHa: number): Field => ({
    id, farmId: 'real-farm', name, block: null, areaHa, crop: { code: 'soybean', name: 'Soja' }, variety: null, season: null,
    plantingDate: null, rowSpacingM: null, plantSpacingM: null, plantsPerHa: null, polygon, sprayCondition: null, lastApplicationAt: null,
  })
  const listed: Field[] = [makeField('real-1', 'Talhão 1', 30), makeField('real-2', 'Talhão 2', 70)]
  const fields: Pick<FieldService, 'listByFarm'> = { listByFarm: async () => listed }

  it('starts every farm not calculated and generates the same values for the same ids', async () => {
    const make = () => createMockTerrainService({ fields, canWrite: () => true, now: () => NOW })
    const first = make()
    expect((await first.getFarm('real-farm')).summary.calculatedFieldCount).toBe(0)
    await first.calculate('real-farm', 'all')
    await first.getFarm('real-farm')
    const done = await first.getFarm('real-farm')
    expect(done.progress).toBeNull()

    const other = make()
    await other.calculate('real-farm', 'all')
    await other.getFarm('real-farm')
    expect((await other.getFarm('real-farm')).items.map(item => item.measurement)).toEqual(done.items.map(item => item.measurement))
  })

  it('weights the farm mean by area and takes the elevation range over the fields', async () => {
    const service = createMockTerrainService({
      fields,
      canWrite: () => true,
      now: () => NOW,
      initiallyCalculatedFarmIds: ['real-farm'],
      overrides: {
        'real-1': { slopeMeanPct: 10, slopeMaxPct: 12, elevationMinM: 800 },
        'real-2': { slopeMeanPct: 20, slopeMaxPct: 25, elevationMinM: 900 },
      },
    })
    const farm = await service.getFarm('real-farm')
    // (10 * 30 + 20 * 70) / 100 = 17
    expect(farm.summary.slopeMeanPct).toBe(17)
    expect(farm.summary.elevationMinM).toBe(800)
    expect(farm.summary.elevationMaxM).toBeGreaterThan(900)
  })

  it('treats a field created after the first read as not calculated', async () => {
    const service = createMockTerrainService({ fields, canWrite: () => true, now: () => NOW, initiallyCalculatedFarmIds: ['real-farm'] })
    await service.getFarm('real-farm')
    listed.push(makeField('real-3', 'Talhão 3', 10))
    try {
      const farm = await service.getFarm('real-farm')
      expect(farm.items.find(item => item.fieldId === 'real-3')).toMatchObject({ status: 'not_calculated', measurement: null })
      expect(farm.summary.calculatedFieldCount).toBe(2)
    }
    finally {
      listed.pop()
    }
  })

  it('reads the permission at each write', async () => {
    let canWrite = false
    const service = createMockTerrainService({ fields, canWrite: () => canWrite, now: () => NOW })
    expect((await rejection(service.calculate('real-farm', 'all'))).code).toBe('forbidden')
    canWrite = true
    expect((await service.calculate('real-farm', 'all')).progress).toEqual({ total: 2, done: 0 })
  })

  it('passes the error of the fields service through (unknown farm)', async () => {
    const missing = createMockTerrainService({
      fields: { listByFarm: async () => { throw new ApiError({ code: 'not_found', message: 'fazenda não encontrada', status: 404 }) } },
      canWrite: () => true,
    })
    expect((await rejection(missing.getFarm('nope'))).status).toBe(404)
  })
})
