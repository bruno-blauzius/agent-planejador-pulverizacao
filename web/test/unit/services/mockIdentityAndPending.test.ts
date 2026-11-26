import { describe, expect, it } from 'vitest'
import { ApiError } from '~/services/errors'
import { createMockServices } from '~/services/mock/mockServices'
import { createPendingMockServices, MOCK_USER_BY_ROLE, PENDING_SERVICE_KEYS } from '~/services/mock/pendingServices'
import { createSsoRealServices } from '../../support/pending'
import type { UserRole } from '~/types/auth'

function worldAs(userId: string) {
  const api = createMockServices({ latencyMs: 0, getAccessToken: () => `mock-token:${userId}`, now: () => new Date('2026-11-21T12:00:00Z') })
  return api
}

async function rejection(promise: Promise<unknown>): Promise<ApiError> {
  const error = await promise.then(() => null, (caught: unknown) => caught)
  expect(error).toBeInstanceOf(ApiError)
  return error as ApiError
}

describe('mock users: get, remove and rules of the implemented contract', () => {
  it('gets a user and 404s one of another company', async () => {
    const admin = worldAs('user-marina')
    expect((await admin.users.get('user-carlos')).name).toBe('Carlos Mendes')
    expect((await rejection(admin.users.get('user-joao'))).status).toBe(404)
  })

  it('renames through update and validates the name', async () => {
    const admin = worldAs('user-marina')
    expect((await admin.users.update('user-carlos', { name: ' Carlos M. ' })).name).toBe('Carlos M.')
    expect((await rejection(admin.users.update('user-carlos', { name: ' ' }))).details[0]?.field).toBe('name')
  })

  it('removes a user, but not yourself or a pending invitation', async () => {
    const admin = worldAs('user-marina')
    expect((await rejection(admin.users.remove('user-marina'))).code).toBe('forbidden')
    expect((await rejection(admin.users.remove('user-beatriz'))).code).toBe('invitation_pending')
    await admin.users.remove('user-carlos')
    expect((await rejection(admin.users.get('user-carlos'))).status).toBe(404)
  })

  it('refuses an e-mail already registered with its stable code', async () => {
    const admin = worldAs('user-marina')
    expect((await rejection(admin.users.invite({
      name: 'X', email: 'carlos.mendes@email.test', role: 'planning_operator', companyId: 'company-santa-rita', farmScope: { mode: 'all' },
    }))).code).toBe('email_already_registered')
  })
})

describe('mock companies: get and remove', () => {
  it('gets a client company and 404s the platform operator', async () => {
    const superAdmin = worldAs('user-bruno')
    expect((await superAdmin.companies.get('company-recanto')).plan).toBe('trial')
    expect((await rejection(superAdmin.companies.get('company-agentes-agro'))).status).toBe(404)
  })

  it('refuses to delete a company with active or invited users, allows it once empty', async () => {
    const superAdmin = worldAs('user-bruno')
    expect((await rejection(superAdmin.companies.remove('company-santa-rita'))).code).toBe('company_has_active_users')
    await superAdmin.users.remove('user-ana').catch(() => undefined)
    await superAdmin.companies.create({ name: 'Vazia', document: '33444555000166', city: 'Alfenas', state: 'MG', plan: 'basic' })
    const empty = (await superAdmin.companies.list({ search: 'Vazia' })).items[0]!
    await superAdmin.companies.remove(empty.id)
    expect((await rejection(superAdmin.companies.get(empty.id))).status).toBe(404)
  })
})

describe('createPendingMockServices (real API + mock for the endpoints still planned)', () => {
  it('keeps the real services for session, users, companies, profile, farms, fields, crops and owners', () => {
    const real = createSsoRealServices()
    const merged = createPendingMockServices(real, { latencyMs: 0, getRole: () => 'admin' })

    for (const key of ['auth', 'users', 'companies', 'profile', 'farms', 'fields', 'crops', 'owners'] as const) expect(merged[key]).toBe(real[key])
    for (const key of PENDING_SERVICE_KEYS) expect(merged[key]).not.toBe(real[key])
  })

  it('serves the relief from the real API (ADR 0021): terrain is no longer a pending service', () => {
    const real = createSsoRealServices()
    const merged = createPendingMockServices(real, { latencyMs: 0, getRole: () => 'admin' })

    expect(PENDING_SERVICE_KEYS).not.toContain('terrain')
    expect(merged.terrain).toBe(real.terrain)
  })

  it.each(Object.entries(MOCK_USER_BY_ROLE) as [UserRole, string][])('acts as the seed user of the %s role', async (role, userId) => {
    const merged = createPendingMockServices(createSsoRealServices(), { latencyMs: 0, getRole: () => role })
    const applications = await merged.applications.list()
    const direct = await worldAs(userId).applications.list()
    expect(applications.items.map(item => item.id)).toEqual(direct.items.map(item => item.id))
  })

  it('is unauthenticated for the mock while there is no signed-in user', async () => {
    const merged = createPendingMockServices(createSsoRealServices(), { latencyMs: 0, getRole: () => null })
    expect((await rejection(merged.applications.list())).code).toBe('unauthorized')
  })
})
