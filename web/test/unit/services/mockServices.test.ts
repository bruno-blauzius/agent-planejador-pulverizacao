import { describe, expect, it } from 'vitest'
import { ApiError } from '~/services/errors'
import { approximateAreaHa } from '~/services/mock/geometry'
import type { FarmInput } from '~/types/farm'
import type { OwnerInput } from '~/types/owner'
import { createTestApi, SEED_USER_EMAIL, SEED_USER_PASSWORD } from '../../support/mockApi'

async function rejection(promise: Promise<unknown>): Promise<ApiError> {
  const error = await promise.then(() => null, (caught: unknown) => caught)
  expect(error).toBeInstanceOf(ApiError)
  return error as ApiError
}

const draftFarm: FarmInput = {
  name: 'Fazenda Nova',
  code: 'FNV-01',
  cooperative: null,
  productionType: 'coffee',
  city: 'Guaxupé',
  state: 'mg',
  status: 'draft',
  location: { postalCode: null, carNumber: null, forecastPoint: null, perimeter: null },
  owners: [],
}

const newOwner: OwnerInput = {
  personType: 'individual',
  name: 'Ana Souza',
  document: '111.222.333-44',
  birthDate: null,
  phone: null,
  email: 'ana@email.test',
  windowAlertsEnabled: true,
}

describe('mock auth', () => {
  it('issues a token for the seeded user', async () => {
    const { api } = createTestApi(null)
    const session = await api.auth.login({ identifier: SEED_USER_EMAIL, password: SEED_USER_PASSWORD, rememberMe: false })
    expect(session.user.name).toBe('Bruno Blauzius')
    expect(session.accessToken).toBe('mock-token:user-bruno')
  })

  it('extends the session with remember me and no longer accepts the CPF (ADR 0015)', async () => {
    const { api } = createTestApi(null)
    const short = await api.auth.login({ identifier: SEED_USER_EMAIL, password: SEED_USER_PASSWORD, rememberMe: false })
    const long = await api.auth.login({ identifier: SEED_USER_EMAIL.toUpperCase(), password: SEED_USER_PASSWORD, rememberMe: true })
    expect((await rejection(api.auth.login({ identifier: '123.278.916-05', password: SEED_USER_PASSWORD, rememberMe: false }))).code).toBe('unauthorized')
    expect(long.user.id).toBe('user-bruno')
    expect(long.expiresAt.getTime()).toBeGreaterThan(short.expiresAt.getTime())
  })

  it('rejects a wrong password with 401', async () => {
    const { api } = createTestApi(null)
    const error = await rejection(api.auth.login({ identifier: SEED_USER_EMAIL, password: 'wrong', rememberMe: false }))
    expect(error).toMatchObject({ code: 'unauthorized', status: 401 })
  })

  it('requires the bearer token on business endpoints', async () => {
    const { api } = createTestApi(null)
    expect((await rejection(api.farms.list())).code).toBe('unauthorized')
  })
})

describe('mock farms', () => {
  it('lists the four Figma farms with derived counts', async () => {
    const { api } = createTestApi()
    const page = await api.farms.list()
    expect(page.total).toBe(4)
    const santaRita = page.items.find(farm => farm.id === 'farm-santa-rita')
    expect(santaRita).toMatchObject({ fieldCount: 9, productiveAreaHa: 412, totalAreaHa: 486.2 })
    expect(santaRita?.owners.map(owner => owner.sharePercent)).toEqual([60, 40])
  })

  it('exposes perimeters coherent with the farm areas and fields near the farm', async () => {
    const { api } = createTestApi()
    const { items } = await api.farms.list()
    for (const farm of items) {
      expect(farm.perimeter).not.toBeNull()
      expect(Math.abs(approximateAreaHa(farm.perimeter!) - farm.totalAreaHa) / farm.totalAreaHa).toBeLessThan(0.02)
    }
    const fields = await api.fields.listByFarm('farm-santa-rita')
    const area = fields.reduce((sum, field) => sum + approximateAreaHa(field.polygon), 0)
    expect(Math.abs(area - 412)).toBeLessThan(2)
  })

  it('reports creation dates (one farm this month)', async () => {
    const { api } = createTestApi()
    const { items } = await api.farms.list()
    expect(items.filter(farm => farm.createdAt.getUTCMonth() === 10).map(farm => farm.id)).toEqual(['farm-recanto'])
  })

  it('filters by status, production type and accent-insensitive search', async () => {
    const { api } = createTestApi()
    expect((await api.farms.list({ status: 'pending' })).items.map(farm => farm.name)).toEqual(['Fazenda Ouro Verde'])
    expect((await api.farms.list({ productionType: 'grains' })).items.map(farm => farm.id)).toEqual(['farm-recanto'])
    expect((await api.farms.list({ search: 'sao sebastiao' })).items).toHaveLength(1)
  })

  it('paginates', async () => {
    const { api } = createTestApi()
    const page = await api.farms.list({ page: 2, pageSize: 3 })
    expect(page).toMatchObject({ total: 4, page: 2, pageSize: 3 })
    expect(page.items).toHaveLength(1)
  })

  it('returns 404 for an unknown farm', async () => {
    const { api } = createTestApi()
    expect((await rejection(api.farms.get('nope'))).status).toBe(404)
  })

  it('creates a draft without perimeter and owners', async () => {
    const { api } = createTestApi()
    const farm = await api.farms.create(draftFarm)
    expect(farm).toMatchObject({ status: 'draft', state: 'MG', fieldCount: 0 })
    expect((await api.farms.list()).total).toBe(5)
  })

  it('rejects an active farm without perimeter or with shares not closing 100%', async () => {
    const { api } = createTestApi()
    const error = await rejection(api.farms.create({
      ...draftFarm,
      status: 'active',
      owners: [{ ownerId: 'owner-bruno', role: 'owner', sharePercent: 70 }],
    }))
    expect(error.code).toBe('validation_failed')
    expect(error.details.map(detail => detail.field)).toEqual(['location.perimeter', 'owners'])
  })

  it('imports a supported perimeter file and refuses other formats', async () => {
    const { api } = createTestApi()
    const imported = await api.farms.importPerimeter(new File(['<kml/>'], 'fazenda.KML'))
    expect(imported.sourceFormat).toBe('kml')
    expect(imported.areaHa).toBeGreaterThan(0)
    expect((await rejection(api.farms.importPerimeter(new File(['x'], 'mapa.pdf')))).status).toBe(415)
  })

  it('does not leak internal records to callers', async () => {
    const { api } = createTestApi()
    const first = await api.farms.get('farm-recanto')
    ;(first as { name: string }).name = 'mutated'
    expect((await api.farms.get('farm-recanto')).name).toBe('Fazenda Recanto')
  })
})

describe('mock fields', () => {
  it('creates a field with area computed from the polygon', async () => {
    const { api } = createTestApi()
    const field = await api.fields.create('farm-recanto', {
      name: 'Talhão 3',
      block: null,
      cropCode: 'soybean',
      variety: null,
      season: '2026/27',
      plantingDate: null,
      rowSpacingM: 0.5,
      plantSpacingM: 0.2,
      polygon: { type: 'Polygon', coordinates: [[[-46, -21.4], [-45.99, -21.4], [-45.99, -21.39], [-46, -21.39], [-46, -21.4]]] },
    })
    expect(field.areaHa).toBeGreaterThan(100)
    expect(field.plantsPerHa).toBe(100000)
    expect(await api.fields.listByFarm('farm-recanto')).toHaveLength(3)
  })
})

describe('mock owners', () => {
  it('searches by name or by document digits', async () => {
    const { api } = createTestApi()
    expect((await api.owners.list({ search: 'joao' })).items.map(owner => owner.id)).toEqual(['owner-joao'])
    expect((await api.owners.list({ search: '12.345.678/0001-90' })).items.map(owner => owner.id)).toEqual(['owner-agro-lima'])
    expect((await api.owners.list({ personType: 'company' })).total).toBe(2)
  })

  it('normalizes the document on create and refuses duplicates with 409', async () => {
    const { api } = createTestApi()
    const owner = await api.owners.create(newOwner)
    expect(owner.document).toBe('11122233344')
    expect(owner.hasPendingDocuments).toBe(true)
    expect((await rejection(api.owners.create(newOwner))).status).toBe(409)
  })

  it('rejects a CPF with the wrong length', async () => {
    const { api } = createTestApi()
    const error = await rejection(api.owners.create({ ...newOwner, document: '123' }))
    expect(error.details).toEqual([{ field: 'document', message: 'CPF inválido' }])
  })

  it('replaces links and refuses a farm total above 100%', async () => {
    const { api } = createTestApi()
    const links = await api.owners.replaceLinks('owner-helena', [
      { farmId: 'farm-boa-esperanca', role: 'owner', sharePercent: 50 },
      { farmId: 'farm-recanto', role: 'tenant', sharePercent: null },
    ])
    expect(links.map(link => link.farmName)).toEqual(['Sítio Boa Esperança', 'Fazenda Recanto'])

    const error = await rejection(api.owners.replaceLinks('owner-helena', [
      { farmId: 'farm-santa-rita', role: 'owner', sharePercent: 10 },
    ]))
    expect(error.details[0]?.field).toBe('links[0].share_percent')
  })
})
