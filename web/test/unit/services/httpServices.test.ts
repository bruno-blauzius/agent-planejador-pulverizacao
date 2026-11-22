import { describe, expect, it } from 'vitest'
import { ApiError, ERROR_MESSAGES, toApiError } from '~/services/errors'
import { createHttpClient, type RawFetchOptions } from '~/services/http/client'
import { createHttpServices } from '~/services/http/services'
import type { FarmDto, FieldDto, OwnerSummaryDto, PageDto } from '~/types/dto'

interface RecordedCall {
  path: string
  options: RawFetchOptions
}

/** Fake transport: records requests and answers with a canned response or error. */
function fakeTransport(answer: (call: RecordedCall) => unknown) {
  const calls: RecordedCall[] = []
  const rawFetch = async (path: string, options: RawFetchOptions) => {
    const call = { path, options }
    calls.push(call)
    const result = answer(call)
    if (result instanceof Error) throw result
    return result
  }
  return { calls, rawFetch }
}

function servicesWith(answer: (call: RecordedCall) => unknown, token: string | null = 'token-123') {
  const transport = fakeTransport(answer)
  const http = createHttpClient({ baseURL: 'http://api.test', getAccessToken: () => token }, transport.rawFetch)
  return { api: createHttpServices(http), calls: transport.calls }
}

const farmDto: FarmDto = {
  company_id: 'company-1',
  id: 'farm-1',
  name: 'Fazenda Santa Rita',
  code: 'STR-01',
  city: 'Guaxupé',
  state: 'MG',
  status: 'active',
  production_type: 'coffee',
  total_area_ha: 486.2,
  productive_area_ha: 412,
  field_count: 9,
  crops: [{ code: 'coffee_arabica', name: 'Café arábica' }],
  owners: [{ owner_id: 'o-1', name: 'Bruno', role: 'owner', share_percent: 60 }],
  spray_outlook: { condition: 'suitable', headline: '2 janelas aptas hoje' },
  forecast_point: { lat: -21.3, lng: -46.71 },
  perimeter: { type: 'Polygon', coordinates: [[[-46.72, -21.31], [-46.70, -21.31], [-46.70, -21.29], [-46.72, -21.31]]] },
  cooperative: 'Cooxupé',
  location: { postal_code: '37800000', car_number: null, forecast_point: { lat: -21.3, lng: -46.71 }, perimeter: null },
  created_at: '2026-10-01T12:00:00Z',
  updated_at: '2026-10-02T12:00:00Z',
}

describe('http client', () => {
  it('sends the bearer token and drops empty query params', async () => {
    const { api, calls } = servicesWith(() => ({ items: [], total: 0, page: 1, page_size: 20 }))
    await api.farms.list({ search: '', status: 'pending', productionType: 'coffee', page: 2 })
    expect(calls[0]).toEqual({
      path: '/farms',
      options: {
        baseURL: 'http://api.test',
        method: 'GET',
        query: { status: 'pending', production_type: 'coffee', page: 2 },
        body: undefined,
        headers: { Accept: 'application/json', Authorization: 'Bearer token-123' },
      },
    })
  })

  it('omits Authorization without a token', async () => {
    const { api, calls } = servicesWith(() => ({ items: [] }), null)
    await api.crops.list()
    expect(calls[0]?.options.headers).toEqual({ Accept: 'application/json' })
  })

  it('turns an API error body into ApiError with field details', async () => {
    const httpError = Object.assign(new Error('[PUT] 422'), {
      name: 'FetchError',
      statusCode: 422,
      data: { error: { code: 'validation_failed', message: 'dados inválidos', details: [{ field: 'name', message: 'obrigatório' }] } },
    })
    const { api } = servicesWith(() => httpError)
    const error = await api.owners.get('o-1').catch((caught: unknown) => caught)
    expect(error).toBeInstanceOf(ApiError)
    expect(error).toMatchObject({ code: 'validation_failed', status: 422, details: [{ field: 'name', message: 'obrigatório' }] })
  })

  it('reports a network failure honestly', async () => {
    const { api } = servicesWith(() => Object.assign(new Error('fetch failed'), { name: 'FetchError' }))
    const error = await api.crops.list().catch((caught: unknown) => caught)
    expect(error).toMatchObject({ code: 'network_error', status: 0 })
  })
})

describe('http auth service', () => {
  it('sends identifier and remember_me and maps the session', async () => {
    const { api, calls } = servicesWith(() => ({
      access_token: 'abc',
      token_type: 'Bearer',
      expires_at: '2026-12-21T12:00:00Z',
      user: {
        id: 'u-1',
        name: 'Bruno',
        email: 'b@x.test',
        role: 'planning_operator',
        primary_farm: null,
        company: { id: 'c-1', name: 'Fazenda Santa Rita' },
        farm_scope: { mode: 'selected', farm_ids: ['f-1'] },
        permissions: ['applications.register', 'planner.use'],
      },
    }), null)
    const session = await api.auth.login({ identifier: ' 123.278.916-05 ', password: 'secret', rememberMe: true })
    expect(calls[0]).toMatchObject({
      path: '/auth/login',
      options: { method: 'POST', body: { identifier: '123.278.916-05', password: 'secret', remember_me: true } },
    })
    expect(session).toMatchObject({
      accessToken: 'abc',
      expiresAt: new Date('2026-12-21T12:00:00Z'),
      user: {
        primaryFarm: null,
        role: 'planning_operator',
        company: { id: 'c-1', name: 'Fazenda Santa Rita' },
        farmScope: { mode: 'selected', farmIds: ['f-1'] },
        permissions: ['applications.register', 'planner.use'],
      },
    })
  })
})

describe('http farm service', () => {
  it('maps the DTO into the model', async () => {
    const { api, calls } = servicesWith(() => farmDto)
    const farm = await api.farms.get('farm/1')
    expect(calls[0]?.path).toBe('/farms/farm%2F1')
    expect(farm).toMatchObject({
      productionType: 'coffee',
      totalAreaHa: 486.2,
      owners: [{ ownerId: 'o-1', sharePercent: 60 }],
      location: { postalCode: '37800000', forecastPoint: { lat: -21.3, lng: -46.71 } },
    })
    expect(farm.createdAt).toEqual(new Date('2026-10-01T12:00:00Z'))
    expect(farm.forecastPoint).toEqual({ lat: -21.3, lng: -46.71 })
    expect(farm.perimeter?.type).toBe('Polygon')
  })

  it('sends the input in snake_case', async () => {
    const { api, calls } = servicesWith(() => farmDto)
    await api.farms.create({
      name: 'Fazenda Nova',
      code: null,
      cooperative: null,
      productionType: 'grains',
      city: 'Alfenas',
      state: 'MG',
      status: 'draft',
      location: { postalCode: null, carNumber: null, forecastPoint: null, perimeter: null },
      owners: [{ ownerId: 'o-1', role: 'owner', sharePercent: 100 }],
    })
    expect(calls[0]?.options).toMatchObject({
      method: 'POST',
      body: {
        production_type: 'grains',
        location: { postal_code: null, car_number: null, forecast_point: null, perimeter: null },
        owners: [{ owner_id: 'o-1', role: 'owner', share_percent: 100 }],
      },
    })
  })

  it('updates a field with PUT and maps the response', async () => {
    const polygon = { type: 'Polygon', coordinates: [[[-46.71, -21.3], [-46.705, -21.3], [-46.705, -21.297], [-46.71, -21.3]]] } as const
    const fieldDto: FieldDto = {
      id: 'field 3',
      farm_id: 'farm-1',
      name: 'Talhão 3',
      block: 'Área Norte',
      area_ha: 38.6,
      crop: { code: 'coffee_arabica', name: 'Café arábica' },
      variety: 'Catuaí 144',
      season: null,
      planting_date: '2019-03-20',
      row_spacing_m: null,
      plant_spacing_m: null,
      plants_per_ha: null,
      polygon,
      spray_condition: 'suitable',
      last_application_at: '2026-09-19T12:00:00Z',
    }
    const { api, calls } = servicesWith(() => fieldDto)
    const field = await api.fields.update('farm-1', 'field 3', {
      name: 'Talhão 3',
      block: 'Área Norte',
      cropCode: 'coffee_arabica',
      variety: 'Catuaí 144',
      season: null,
      plantingDate: '2019-03-20',
      rowSpacingM: null,
      plantSpacingM: null,
      polygon,
    })
    expect(calls[0]).toMatchObject({
      path: '/farms/farm-1/fields/field%203',
      options: { method: 'PUT', body: { crop_code: 'coffee_arabica', variety: 'Catuaí 144', planting_date: '2019-03-20' } },
    })
    expect(field).toMatchObject({ id: 'field 3', variety: 'Catuaí 144', lastApplicationAt: new Date('2026-09-19T12:00:00Z') })
  })

  it('uploads the perimeter file as multipart', async () => {
    const { api, calls } = servicesWith(() => ({
      source_format: 'geojson',
      perimeter: { type: 'Polygon', coordinates: [] },
      area_ha: 10,
      centroid: { lat: 0, lng: 0 },
      municipality: 'Guaxupé',
      datum: 'SIRGAS 2000',
    }))
    const result = await api.farms.importPerimeter(new File(['{}'], 'area.geojson'))
    expect(calls[0]?.path).toBe('/perimeter-imports')
    expect(calls[0]?.options.body).toBeInstanceOf(FormData)
    // Optional fields absent in the response become null.
    expect(result).toMatchObject({ sourceFormat: 'geojson', areaHa: 10, city: 'Guaxupé', state: null, datum: 'SIRGAS 2000', fieldCount: null })
  })
})

describe('http owner service', () => {
  it('maps the page envelope and links', async () => {
    const page: PageDto<OwnerSummaryDto> = {
      items: [{
        id: 'o-1',
        person_type: 'company',
        name: 'Agropecuária Lima Ltda.',
        document: '12345678000190',
        has_pending_documents: false,
        links: [{ farm_id: 'f-1', farm_name: 'Fazenda Recanto', role: 'owner', share_percent: 100 }],
      }],
      total: 1,
      page: 1,
      page_size: 20,
    }
    const { api, calls } = servicesWith(() => page)
    const result = await api.owners.list({ search: 'lima', personType: 'company' })
    expect(calls[0]?.options.query).toEqual({ search: 'lima', person_type: 'company' })
    expect(result).toMatchObject({
      pageSize: 20,
      items: [{ personType: 'company', hasPendingDocuments: false, links: [{ farmId: 'f-1', farmName: 'Fazenda Recanto' }] }],
    })
  })

  it('wraps links in the PUT body', async () => {
    const { api, calls } = servicesWith(() => ({ items: [] }))
    await api.owners.replaceLinks('o-1', [{ farmId: 'f-1', role: 'tenant', sharePercent: null }])
    expect(calls[0]).toMatchObject({
      path: '/owners/o-1/links',
      options: { method: 'PUT', body: { links: [{ farm_id: 'f-1', role: 'tenant', share_percent: null }] } },
    })
  })
})

describe('toApiError', () => {
  it('unwraps the original error kept as cause (useAsyncData)', () => {
    const original = new ApiError({ code: 'not_found', message: 'x', status: 404 })
    expect(toApiError(Object.assign(new Error('wrapped'), { cause: original }))).toBe(original)
  })

  it('rebuilds the ApiError serialized in the SSR payload (NuxtError.toJSON)', () => {
    const original = new ApiError({
      code: 'validation_failed',
      message: 'dados inválidos',
      status: 422,
      details: [{ field: 'name', message: 'obrigatório' }],
    })
    // What h3 createError + toJSON put in the payload, revived on the client by createError(json).
    const payload = JSON.parse(JSON.stringify({ message: original.message, statusCode: original.status, data: original.data }))
    const revived = Object.assign(new Error(payload.message), payload)

    const error = toApiError(Object.assign(new Error('async data failed'), revived))

    expect(error).toMatchObject({ code: 'validation_failed', status: 422, message: 'dados inválidos', details: [{ field: 'name', message: 'obrigatório' }] })
    const notFound = new ApiError({ code: 'not_found', message: 'fazenda não encontrada', status: 404 })
    expect(toApiError({ statusCode: 404, message: notFound.message, data: JSON.parse(JSON.stringify(notFound.data)) }).code).toBe('not_found')
    const network = new ApiError({ code: 'network_error', message: ERROR_MESSAGES.network, status: 0 })
    expect(toApiError({ statusCode: 500, data: JSON.parse(JSON.stringify(network.data)) }).code).toBe('network_error')
  })

  it('does not mistake a programming error for a network failure', () => {
    const error = toApiError(new TypeError('boom'))
    expect(error).toMatchObject({ code: 'unknown_error', status: 0, message: ERROR_MESSAGES.unexpected })
    expect(error.cause).toBeInstanceOf(TypeError)
  })

  it('never exposes raw exception text to the user', () => {
    expect(toApiError('boom').message).toBe(ERROR_MESSAGES.unexpected)
    expect(toApiError(Object.assign(new Error('fetch failed'), { name: 'FetchError' })).message).toBe(ERROR_MESSAGES.network)
    expect(toApiError(Object.assign(new Error('[GET] 502 Bad Gateway'), { name: 'FetchError', statusCode: 502 }))).toMatchObject({
      code: 'unknown_error',
      status: 502,
      message: ERROR_MESSAGES.unavailable,
    })
    expect(toApiError(Object.assign(new Error('[GET] 418'), { name: 'FetchError', statusCode: 418 })).message).toBe(ERROR_MESSAGES.unexpected)
  })
})
