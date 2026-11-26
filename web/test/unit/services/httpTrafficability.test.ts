import { describe, expect, it } from 'vitest'
import { ApiError } from '~/services/errors'
import { createHttpClient, type RawFetchOptions } from '~/services/http/client'
import { createHttpServices } from '~/services/http/services'
import { toFieldTrafficability } from '~/services/http/trafficabilityMappers'
import type { FieldTrafficabilityDto } from '~/types/trafficabilityDto'

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

/** Example `waterloggedClay` of the contract. */
const waterloggedDto: FieldTrafficabilityDto = {
  field_id: 'fld_4',
  farm_id: 'farm_1',
  as_of: '2026-11-26T07:00:00-03:00',
  soil: {
    state: 'waterlogged',
    basis: 'measured_rain',
    texture: 'clay',
    thresholds_source: 'heuristic',
    rain: {
      last_24h_mm: 6,
      last_48h_mm: 24,
      last_72h_mm: 55,
      last_7d_mm: 71,
      source: 'gauge',
      source_name: 'Pluviômetro da sede',
      observed_at: '2026-11-26T07:00:00-03:00',
    },
    model_moisture: { volumetric_m3m3: 0.41, field_capacity_m3m3: 0.36, field_capacity_pct: 113.9 },
    inspection: null,
  },
  rain_daily: [
    { date: '2026-11-25', rain_mm: 18, kind: 'measured', source: 'gauge' },
    { date: '2026-11-26', rain_mm: 6, kind: 'measured', source: 'gauge' },
    { date: '2026-11-27', rain_mm: 2, kind: 'forecast', source: 'model' },
  ],
  days: [
    {
      date: '2026-11-26',
      soil_state: 'waterlogged',
      methods: [
        { method: 'drone', verdict: 'apt', reasons: [], area_above_limit_pct: null },
        { method: 'airblast', verdict: 'unfit', reasons: ['soil_waterlogged'], area_above_limit_pct: 2 },
      ],
    },
  ],
  releases: [
    { method: 'airblast', required_state: 'ideal', date: '2026-11-29' },
    { method: 'knapsack', required_state: 'moist', date: '2026-11-28' },
  ],
  data_issues: [],
}

/** No rain source, with an inspection: every nullable block at its other value. */
const unknownDto: FieldTrafficabilityDto = {
  ...waterloggedDto,
  soil: {
    state: 'unknown',
    basis: 'none',
    texture: null,
    thresholds_source: 'calibrated',
    rain: null,
    model_moisture: null,
    inspection: {
      state: 'moist',
      inspected_at: '2026-11-26T06:00:00-03:00',
      valid_until: '2026-11-26T18:00:00-03:00',
      inspected_by: { id: 'user-1', name: 'Carlos' },
    },
  },
  rain_daily: [],
  releases: [{ method: 'tractor_boom', required_state: 'ideal', date: null }],
  data_issues: ['no_rain_source', 'no_soil_texture'],
}

describe('toFieldTrafficability', () => {
  it('maps the contract example to camelCase, with instants as Date and calendar days as text', () => {
    const model = toFieldTrafficability(waterloggedDto)
    expect(model.fieldId).toBe('fld_4')
    expect(model.farmId).toBe('farm_1')
    expect(model.asOf).toEqual(new Date('2026-11-26T10:00:00Z'))
    expect(model.soil).toEqual({
      state: 'waterlogged',
      basis: 'measured_rain',
      texture: 'clay',
      thresholdsSource: 'heuristic',
      rain: {
        last24hMm: 6,
        last48hMm: 24,
        last72hMm: 55,
        last7dMm: 71,
        source: 'gauge',
        sourceName: 'Pluviômetro da sede',
        observedAt: new Date('2026-11-26T10:00:00Z'),
      },
      modelMoisture: { volumetricM3m3: 0.41, fieldCapacityM3m3: 0.36, fieldCapacityPct: 113.9 },
      inspection: null,
    })
    expect(model.rainDaily[2]).toEqual({ date: '2026-11-27', rainMm: 2, kind: 'forecast', source: 'model' })
    expect(model.days[0]).toEqual({
      date: '2026-11-26',
      soilState: 'waterlogged',
      methods: [
        { method: 'drone', verdict: 'apt', reasons: [], areaAboveLimitPct: null },
        { method: 'airblast', verdict: 'unfit', reasons: ['soil_waterlogged'], areaAboveLimitPct: 2 },
      ],
    })
    expect(model.releases).toEqual([
      { method: 'airblast', requiredState: 'ideal', date: '2026-11-29' },
      { method: 'knapsack', requiredState: 'moist', date: '2026-11-28' },
    ])
    expect(model.dataIssues).toEqual([])
  })

  it('keeps nulls (rain, model moisture, texture, release date) and maps an inspection', () => {
    const model = toFieldTrafficability(unknownDto)
    expect(model.soil).toMatchObject({ state: 'unknown', basis: 'none', texture: null, thresholdsSource: 'calibrated', rain: null, modelMoisture: null })
    expect(model.soil.inspection).toEqual({
      state: 'moist',
      inspectedAt: new Date('2026-11-26T09:00:00Z'),
      validUntil: new Date('2026-11-26T21:00:00Z'),
      inspectedBy: { id: 'user-1', name: 'Carlos' },
    })
    expect(model.rainDaily).toEqual([])
    expect(model.releases).toEqual([{ method: 'tractor_boom', requiredState: 'ideal', date: null }])
    expect(model.dataIssues).toEqual(['no_rain_source', 'no_soil_texture'])
  })
})

describe('trafficability HTTP service', () => {
  it('calls the nested field route with the horizon and maps the answer', async () => {
    const { api, calls } = servicesWith(waterloggedDto)
    const result = await api.trafficability.getFieldTrafficability('farm 1', 'fld/4', 3)
    expect(calls).toHaveLength(1)
    expect(calls[0]!.path).toBe('/farms/farm%201/fields/fld%2F4/trafficability')
    expect(calls[0]!.options.method).toBe('GET')
    expect(calls[0]!.options.query).toEqual({ days: 3 })
    expect(result.soil.state).toBe('waterlogged')
  })

  it('leaves `days` out when not given, so the API applies its default', async () => {
    const { api, calls } = servicesWith(waterloggedDto)
    await api.trafficability.getFieldTrafficability('farm-1', 'field-4')
    expect(calls[0]!.options.query ?? {}).not.toHaveProperty('days')
  })

  it('rejects with the contract error', async () => {
    const failure = Object.assign(new Error('Not Found'), {
      status: 404,
      data: { error: { code: 'not_found', message: 'talhão não encontrado' } },
    })
    const { api } = servicesWith(failure)
    const error = await api.trafficability.getFieldTrafficability('farm-1', 'field-x').catch((caught: unknown) => caught)
    expect(error).toBeInstanceOf(ApiError)
    expect(error).toMatchObject({ code: 'not_found', status: 404 })
  })
})
