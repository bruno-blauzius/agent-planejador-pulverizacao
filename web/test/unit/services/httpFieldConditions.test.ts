import { describe, expect, it } from 'vitest'
import { createHttpClient, type RawFetchOptions } from '~/services/http/client'
import { createHttpServices } from '~/services/http/services'
import { toRainGaugePatchDto, toSoilInspectionInputDto } from '~/services/http/trafficabilityMappers'
import type { RainGaugeDto, RainReadingDto, SoilInspectionDto, SoilProfileDto } from '~/types/trafficabilityDto'

interface RecordedCall {
  path: string
  options: RawFetchOptions
}

function servicesWith(answer: unknown) {
  const calls: RecordedCall[] = []
  const http = createHttpClient({ baseURL: 'http://api.test' }, async (path, options) => {
    calls.push({ path, options })
    return answer
  })
  return { api: createHttpServices(http, 'http://localhost:8002'), calls }
}

const gaugeDto: RainGaugeDto = {
  id: 'gauge-1',
  farm_id: 'farm-1',
  name: 'Pluviômetro da sede',
  location: { lat: -21.3, lng: -46.7 },
  created_at: '2026-11-01T10:00:00Z',
  updated_at: '2026-11-02T10:00:00Z',
  version: 2,
}

const readingDto: RainReadingDto = {
  id: 'reading-1',
  rain_gauge_id: 'gauge-1',
  rain_mm: 12.5,
  observed_at: '2026-11-26T10:00:00Z',
  recorded_by: { id: 'user-1', name: 'Carlos' },
  recorded_at: '2026-11-26T10:05:00Z',
}

const inspectionDto: SoilInspectionDto = {
  id: 'inspection-1',
  field_id: 'field-4',
  farm_id: 'farm-1',
  state: 'moist',
  inspected_at: '2026-11-26T10:00:00Z',
  valid_until: '2026-11-26T22:00:00Z',
  inspected_by: { id: 'user-1', name: 'Carlos' },
  note: null,
}

const profileDto: SoilProfileDto = { field_id: 'field-4', farm_id: 'farm-1', soil_texture: 'clay', rain_gauge: { id: 'gauge-1', name: 'Pluviômetro da sede' } }

describe('rain gauge HTTP service', () => {
  it('lists the gauges of the farm and maps them', async () => {
    const { api, calls } = servicesWith({ items: [gaugeDto, { ...gaugeDto, id: 'gauge-2', location: null }] })
    const gauges = await api.rainGauges.list('farm 1')
    expect(calls[0]).toMatchObject({ path: '/farms/farm%201/rain-gauges', options: { method: 'GET' } })
    expect(gauges[0]).toEqual({
      id: 'gauge-1',
      farmId: 'farm-1',
      name: 'Pluviômetro da sede',
      location: { lat: -21.3, lng: -46.7 },
      createdAt: new Date('2026-11-01T10:00:00Z'),
      updatedAt: new Date('2026-11-02T10:00:00Z'),
      version: 2,
    })
    expect(gauges[1]!.location).toBeNull()
  })

  it('creates with POST and renames with a PATCH that sends only the given keys', async () => {
    const { api, calls } = servicesWith(gaugeDto)
    await api.rainGauges.create('farm-1', { name: 'Sede' })
    await api.rainGauges.update('farm-1', 'gauge-1', { name: 'Sede nova' })
    expect(calls[0]).toMatchObject({ path: '/farms/farm-1/rain-gauges', options: { method: 'POST', body: { name: 'Sede' } } })
    expect(calls[1]).toMatchObject({ path: '/farms/farm-1/rain-gauges/gauge-1', options: { method: 'PATCH', body: { name: 'Sede nova' } } })
    expect(toRainGaugePatchDto({ name: 'x' })).toEqual({ name: 'x' })
    expect(toRainGaugePatchDto({ location: null })).toEqual({ location: null })
    expect(toRainGaugePatchDto({})).toEqual({})
  })

  it('lists readings with the range as RFC 3339 and records one', async () => {
    const { api, calls } = servicesWith({ items: [readingDto] })
    const readings = await api.rainGauges.listReadings('farm-1', 'gauge-1', { from: new Date('2026-11-20T00:00:00Z'), to: new Date('2026-11-26T00:00:00Z') })
    expect(calls[0]).toMatchObject({
      path: '/farms/farm-1/rain-gauges/gauge-1/readings',
      options: { query: { from: '2026-11-20T00:00:00.000Z', to: '2026-11-26T00:00:00.000Z' } },
    })
    expect(readings[0]).toEqual({
      id: 'reading-1',
      rainGaugeId: 'gauge-1',
      rainMm: 12.5,
      observedAt: new Date('2026-11-26T10:00:00Z'),
      recordedBy: { id: 'user-1', name: 'Carlos' },
      recordedAt: new Date('2026-11-26T10:05:00Z'),
    })

    await api.rainGauges.listReadings('farm-1', 'gauge-1')
    expect(calls[1]!.options.query ?? {}).toEqual({})

    const recorder = servicesWith(readingDto)
    await recorder.api.rainGauges.recordReading('farm-1', 'gauge-1', { rainMm: 12.5, observedAt: new Date('2026-11-26T10:00:00Z') })
    expect(recorder.calls[0]).toMatchObject({ options: { method: 'POST', body: { rain_mm: 12.5, observed_at: '2026-11-26T10:00:00.000Z' } } })
  })
})

describe('soil conditions HTTP service', () => {
  it('records an inspection and maps it, nulls included', async () => {
    const { api, calls } = servicesWith(inspectionDto)
    const inspection = await api.soilConditions.recordInspection('farm-1', 'field-4', { state: 'moist', inspectedAt: new Date('2026-11-26T10:00:00Z'), note: 'barro na entrada' })
    expect(calls[0]).toMatchObject({
      path: '/farms/farm-1/fields/field-4/soil-inspections',
      options: { method: 'POST', body: { state: 'moist', inspected_at: '2026-11-26T10:00:00.000Z', note: 'barro na entrada' } },
    })
    expect(inspection).toEqual({
      id: 'inspection-1',
      fieldId: 'field-4',
      farmId: 'farm-1',
      state: 'moist',
      inspectedAt: new Date('2026-11-26T10:00:00Z'),
      validUntil: new Date('2026-11-26T22:00:00Z'),
      inspectedBy: { id: 'user-1', name: 'Carlos' },
      note: null,
    })
    // Without a time the API uses now.
    expect(toSoilInspectionInputDto({ state: 'dry' })).toEqual({ state: 'dry' })
  })

  it('reads and replaces the soil profile, always sending both keys', async () => {
    const { api, calls } = servicesWith(profileDto)
    expect(await api.soilConditions.getProfile('farm-1', 'field-4')).toEqual({
      fieldId: 'field-4',
      farmId: 'farm-1',
      soilTexture: 'clay',
      rainGauge: { id: 'gauge-1', name: 'Pluviômetro da sede' },
    })
    await api.soilConditions.updateProfile('farm-1', 'field-4', { soilTexture: null, rainGaugeId: null })
    expect(calls[0]).toMatchObject({ path: '/farms/farm-1/fields/field-4/soil-profile', options: { method: 'GET' } })
    expect(calls[1]).toMatchObject({ options: { method: 'PUT', body: { soil_texture: null, rain_gauge_id: null } } })

    const empty = servicesWith({ ...profileDto, soil_texture: null, rain_gauge: null })
    expect(await empty.api.soilConditions.getProfile('farm-1', 'field-4')).toMatchObject({ soilTexture: null, rainGauge: null })
  })
})
