import { describe, expect, it } from 'vitest'
import { ApiError } from '~/services/errors'
import { createHttpClient, type RawFetchOptions } from '~/services/http/client'
import { createHttpServices } from '~/services/http/services'
import type { FarmTerrainDto, FieldTerrainDto, TerrainMeasurementDto } from '~/types/terrainDto'

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

const measurementDto: TerrainMeasurementDto = {
  slope_mean_pct: 23.5,
  slope_p90_pct: 31,
  slope_max_pct: 41.8,
  slope_class: 'strongly_undulating',
  position: 'slope',
  aspect: 'northwest',
}

const farmDto: FarmTerrainDto = {
  farm_id: 'farm-1',
  summary: {
    field_count: 2,
    calculated_field_count: 1,
    slope_mean_pct: 23.5,
    restricted_field_count: 1,
    elevation_min_m: 872,
    elevation_max_m: 941,
    calculated_at: '2026-11-12T15:00:00Z',
  },
  progress: { total: 2, done: 1 },
  items: [
    {
      field_id: 'field-4',
      field_name: 'Talhão 4',
      block: 'Área Centro',
      area_ha: 44,
      status: 'ready',
      failure_reason: null,
      source: 'calculated',
      measurement: measurementDto,
      elevation_min_m: 872,
      elevation_max_m: 941,
      has_restricted_method: true,
    },
    {
      field_id: 'field-5',
      field_name: 'Talhão 5',
      block: null,
      area_ha: 56.3,
      status: 'failed',
      failure_reason: 'bloco indisponível',
      source: null,
      measurement: null,
      elevation_min_m: null,
      elevation_max_m: null,
      has_restricted_method: false,
    },
  ],
}

const fieldDto: FieldTerrainDto = {
  field_id: 'field-4',
  farm_id: 'farm-1',
  status: 'ready',
  failure_reason: null,
  source: 'manual',
  effective: { ...measurementDto, slope_p90_pct: null, aspect: null },
  calculated: {
    measurement: measurementDto,
    elevation_min_m: 872,
    elevation_max_m: 941,
    class_areas: [{ slope_class: 'strongly_undulating', area_ha: 22.9, share_pct: 52 }],
    profile: [{ distance_m: 0, elevation_m: 941 }, { distance_m: 290, elevation_m: 872 }],
    elevation_model: { source: 'Copernicus DEM GLO-30', resolution_m: 30, version: '2023_1' },
    calculated_at: '2026-11-12T15:00:00Z',
  },
  adjustment: {
    origin: 'topographic_survey',
    measurement: { ...measurementDto, slope_p90_pct: null, aspect: null },
    justification: 'RTK',
    adjusted_by: { id: 'u-1', name: 'Marina Souza' },
    adjusted_at: '2026-11-20T12:00:00Z',
  },
  adjustment_outdated: true,
  method_fits: [{ method: 'tractor_boom', slope_limit_pct: 20, limit_source: 'reference', status: 'restricted', area_above_limit_pct: 62 }],
}

describe('http terrain service', () => {
  it('gets the farm relief and maps snake_case to the model, with Date objects', async () => {
    const { api, calls } = servicesWith(farmDto)
    const terrain = await api.terrain.getFarm('farm 1')
    expect(calls[0]).toMatchObject({ path: '/farms/farm%201/terrain', options: { method: 'GET' } })
    expect(terrain.farmId).toBe('farm-1')
    expect(terrain.summary).toEqual({
      fieldCount: 2,
      calculatedFieldCount: 1,
      slopeMeanPct: 23.5,
      restrictedFieldCount: 1,
      elevationMinM: 872,
      elevationMaxM: 941,
      calculatedAt: new Date('2026-11-12T15:00:00Z'),
    })
    expect(terrain.progress).toEqual({ total: 2, done: 1 })
    expect(terrain.items[0]).toEqual({
      fieldId: 'field-4',
      fieldName: 'Talhão 4',
      block: 'Área Centro',
      areaHa: 44,
      status: 'ready',
      failureReason: null,
      source: 'calculated',
      measurement: { slopeMeanPct: 23.5, slopeP90Pct: 31, slopeMaxPct: 41.8, slopeClass: 'strongly_undulating', position: 'slope', aspect: 'northwest' },
      elevationMinM: 872,
      elevationMaxM: 941,
      hasRestrictedMethod: true,
    })
    expect(terrain.items[1]).toMatchObject({ status: 'failed', failureReason: 'bloco indisponível', measurement: null, elevationMinM: null })
  })

  it('keeps null progress and null calculation date', async () => {
    const { api } = servicesWith({ ...farmDto, progress: null, summary: { ...farmDto.summary, calculated_at: null } })
    const terrain = await api.terrain.getFarm('farm-1')
    expect(terrain.progress).toBeNull()
    expect(terrain.summary.calculatedAt).toBeNull()
  })

  it('posts the calculation scope and maps the 202 answer', async () => {
    const { api, calls } = servicesWith(farmDto)
    const terrain = await api.terrain.calculate('farm-1', 'all')
    expect(calls[0]).toMatchObject({ path: '/farms/farm-1/terrain/calculations', options: { method: 'POST', body: { scope: 'all' } } })
    expect(terrain.progress).toEqual({ total: 2, done: 1 })
  })

  it('gets the field relief with calculation, adjustment, outdated flag and method fits', async () => {
    const { api, calls } = servicesWith(fieldDto)
    const detail = await api.terrain.getField('farm-1', 'field-4')
    expect(calls[0]!.path).toBe('/farms/farm-1/fields/field-4/terrain')
    expect(detail).toMatchObject({
      fieldId: 'field-4',
      farmId: 'farm-1',
      source: 'manual',
      adjustmentOutdated: true,
      effective: { slopeP90Pct: null, aspect: null },
      adjustment: { origin: 'topographic_survey', justification: 'RTK', adjustedBy: { id: 'u-1', name: 'Marina Souza' }, adjustedAt: new Date('2026-11-20T12:00:00Z') },
      methodFits: [{ method: 'tractor_boom', slopeLimitPct: 20, limitSource: 'reference', status: 'restricted', areaAboveLimitPct: 62 }],
    })
    expect(detail.calculated).toMatchObject({
      elevationMinM: 872,
      elevationMaxM: 941,
      classAreas: [{ slopeClass: 'strongly_undulating', areaHa: 22.9, sharePct: 52 }],
      profile: [{ distanceM: 0, elevationM: 941 }, { distanceM: 290, elevationM: 872 }],
      elevationModel: { source: 'Copernicus DEM GLO-30', resolutionM: 30, version: '2023_1' },
      calculatedAt: new Date('2026-11-12T15:00:00Z'),
    })
  })

  it('maps a field without relief', async () => {
    const { api } = servicesWith({
      ...fieldDto, status: 'not_calculated', source: null, effective: null, calculated: null, adjustment: null, adjustment_outdated: false, method_fits: [],
    })
    const detail = await api.terrain.getField('farm-1', 'field-4')
    expect(detail).toMatchObject({ effective: null, calculated: null, adjustment: null, methodFits: [] })
  })

  it('puts the adjustment in snake_case and removes it with DELETE', async () => {
    const { api, calls } = servicesWith(fieldDto)
    await api.terrain.adjust('farm-1', 'field-4', {
      origin: 'field_assessment',
      slopeMeanPct: 14.2,
      slopeMaxPct: 21.5,
      position: 'lowland',
      justification: 'Terraços',
    })
    expect(calls[0]).toMatchObject({
      path: '/farms/farm-1/fields/field-4/terrain/adjustment',
      options: {
        method: 'PUT',
        body: { origin: 'field_assessment', slope_mean_pct: 14.2, slope_max_pct: 21.5, position: 'lowland', justification: 'Terraços' },
      },
    })
    await api.terrain.removeAdjustment('farm-1', 'field-4')
    expect(calls[1]).toMatchObject({ path: '/farms/farm-1/fields/field-4/terrain/adjustment', options: { method: 'DELETE' } })
  })

  it('rejects with ApiError carrying the 422 details of the adjustment', async () => {
    const failure = Object.assign(new Error('unprocessable'), {
      status: 422,
      data: { error: { code: 'validation_failed', message: 'dados inválidos', details: [{ field: 'slope_max_pct', message: 'menor que a média' }] } },
    })
    const { api } = servicesWith(failure)
    const error = await api.terrain.adjust('farm-1', 'field-4', {
      origin: 'field_assessment', slopeMeanPct: 20, slopeMaxPct: 10, position: 'slope', justification: 'x',
    }).catch((caught: unknown) => caught)
    expect(error).toBeInstanceOf(ApiError)
    expect((error as ApiError).details).toEqual([{ field: 'slope_max_pct', message: 'menor que a média' }])
  })
})
