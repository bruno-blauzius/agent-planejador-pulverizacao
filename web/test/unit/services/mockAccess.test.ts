import { describe, expect, it } from 'vitest'
import { ApiError } from '~/services/errors'
import { createMockServices, type MockApiServices } from '~/services/mock/mockServices'
import { TEST_PASSWORD } from '~/services/mock/seed'
import type { FarmInput } from '~/types/farm'

/** Mock services sharing one database, with a switchable signed-in user. */
function createWorld() {
  let token: string | null = null
  const api = createMockServices({ latencyMs: 0, getAccessToken: () => token, now: () => new Date('2026-11-21T12:00:00Z') })
  return {
    api,
    as(userId: string): MockApiServices {
      token = `mock-token:${userId}`
      return api
    },
  }
}

async function rejection(promise: Promise<unknown>): Promise<ApiError> {
  const error = await promise.then(() => null, (caught: unknown) => caught)
  expect(error).toBeInstanceOf(ApiError)
  return error as ApiError
}

const newFarm: FarmInput = {
  name: 'Fazenda Nova',
  code: null,
  cooperative: null,
  productionType: 'coffee',
  city: 'Guaxupé',
  state: 'MG',
  status: 'draft',
  location: { postalCode: null, carNumber: null, forecastPoint: null, perimeter: null },
  owners: [],
}

describe('mock session (ADR 0013)', () => {
  it.each([
    ['bruno@agentesagro.test', 'super_admin', 'Agentes Agro', 8],
    ['marina@fazendasantarita.test', 'admin', 'Fazenda Santa Rita', 6],
    ['carlos.mendes@email.test', 'planning_operator', 'Fazenda Santa Rita', 2],
  ])('%s signs in as %s with the capabilities of the role', async (identifier, role, companyName, permissionCount) => {
    const { api } = createWorld()
    const session = await api.auth.login({ identifier, password: TEST_PASSWORD, rememberMe: false })
    expect(session.user).toMatchObject({ role, company: { name: companyName } })
    expect(session.user.permissions).toHaveLength(permissionCount)
  })

  it('refuses invited and inactive users', async () => {
    const { api } = createWorld()
    expect((await rejection(api.auth.login({ identifier: 'ana.prado@email.test', password: TEST_PASSWORD, rememberMe: false }))).code).toBe('unauthorized')
    const inactive = await rejection(api.auth.login({ identifier: 'helio@helioagro.test', password: TEST_PASSWORD, rememberMe: false }))
    expect(inactive.message).toContain('desativado')
  })

  it('exposes the operator farm scope in the session', async () => {
    const world = createWorld()
    expect(await world.as('user-carlos').auth.getCurrentUser()).toMatchObject({
      farmScope: { mode: 'selected', farmIds: ['farm-santa-rita'] },
      permissions: ['applications.register', 'planner.use'],
    })
  })
})

describe('mock company and farm scope', () => {
  it('shows each user only the farms of their company and scope', async () => {
    const world = createWorld()
    expect((await world.as('user-bruno').farms.list()).total).toBe(4)
    expect((await world.as('user-marina').farms.list()).items.map(farm => farm.id)).toEqual(['farm-santa-rita'])
    expect((await world.as('user-joao').farms.list()).items.map(farm => farm.companyId)).toEqual(['company-ouro-verde'])
  })

  it('hides other tenants with 404 and blocks farms outside the operator scope with 403 out_of_scope', async () => {
    const world = createWorld()
    expect((await rejection(world.as('user-marina').farms.get('farm-ouro-verde'))).status).toBe(404)

    const created = await world.as('user-marina').farms.create(newFarm)
    expect(created.companyId).toBe('company-santa-rita')
    const outOfScope = await rejection(world.as('user-carlos').farms.get(created.id))
    expect(outOfScope).toMatchObject({ code: 'out_of_scope', status: 403 })
    expect((await rejection(world.as('user-carlos').fields.listByFarm(created.id))).code).toBe('out_of_scope')
  })

  it('answers 403 forbidden to registry writes without registry.write', async () => {
    const world = createWorld()
    const operator = world.as('user-carlos')
    expect(await rejection(operator.farms.create(newFarm))).toMatchObject({ code: 'forbidden', status: 403 })
    expect((await rejection(operator.owners.update('owner-marina', {
      personType: 'individual',
      name: 'Marina',
      document: '38544120771',
      birthDate: null,
      phone: null,
      email: null,
      windowAlertsEnabled: false,
    }))).code).toBe('forbidden')
    expect((await operator.farms.get('farm-santa-rita')).name).toBe('Fazenda Santa Rita')
  })

  it('shows an operator only owners linked to the released farms (never other companies)', async () => {
    const world = createWorld()
    const created = await world.as('user-marina').owners.create({
      personType: 'individual',
      name: 'Sem Vínculo',
      document: '11144477735',
      birthDate: null,
      phone: null,
      email: null,
      windowAlertsEnabled: false,
    })
    expect((await world.as('user-marina').owners.list()).items.map(owner => owner.id)).toContain(created.id)

    const operator = world.as('user-carlos')
    expect((await operator.owners.list()).items.map(owner => owner.name)).toEqual(['Bruno Blauzius', 'Marina Rezende'])
    expect((await rejection(operator.owners.get('owner-agro-lima'))).status).toBe(404)
    expect((await rejection(operator.owners.get('owner-helena'))).status).toBe(404)
    expect((await rejection(operator.owners.get(created.id))).status).toBe(404)
  })

  it('limits owners to the company or to farms in scope', async () => {
    const world = createWorld()
    const admin = world.as('user-marina')
    const ids = (await admin.owners.list()).items.map(owner => owner.id)
    expect(ids).toEqual(expect.arrayContaining(['owner-bruno', 'owner-marina']))
    expect(ids).not.toContain('owner-agro-lima')
    expect((await rejection(admin.owners.get('owner-agro-lima'))).status).toBe(404)
    expect((await admin.owners.getLinks('owner-bruno')).map(link => link.farmId)).toEqual(['farm-santa-rita'])
  })
})

describe('mock users and invitations', () => {
  it('lists every user for the super admin and only the company for an admin', async () => {
    const world = createWorld()
    expect((await world.as('user-bruno').users.list()).total).toBe(9)
    expect(await world.as('user-bruno').users.summary()).toEqual({
      totalUsers: 9,
      companyCount: 5,
      pendingInvitations: 2,
      byRole: { super_admin: 1, admin: 3, planning_operator: 5 },
    })
    expect((await world.as('user-marina').users.list()).items.map(user => user.name)).toEqual(['Marina Rezende', 'Carlos Mendes', 'Beatriz Souza'])
    expect((await rejection(world.as('user-carlos').users.list())).code).toBe('forbidden')
  })

  it('filters by status and searches by company name', async () => {
    const world = createWorld()
    const superAdmin = world.as('user-bruno')
    expect((await superAdmin.users.list({ status: 'invited' })).items.map(user => user.invitation?.id)).toEqual(['invitation-ana', 'invitation-beatriz'])
    expect((await superAdmin.users.list({ search: 'ouro verde' })).total).toBe(2)
  })

  it('keeps admins inside their company and away from the super admin role', async () => {
    const world = createWorld()
    const admin = world.as('user-marina')
    const base = { name: 'Nova Pessoa', email: 'nova@email.test', farmScope: { mode: 'all' } as const }
    expect((await rejection(admin.users.invite({ ...base, role: 'super_admin', companyId: 'company-santa-rita' }))).code).toBe('forbidden')
    expect((await rejection(admin.users.invite({ ...base, role: 'admin', companyId: 'company-recanto' }))).code).toBe('forbidden')
    expect((await rejection(admin.users.update('user-joao', { role: 'admin' }))).status).toBe(404)
    expect((await rejection(admin.users.update('user-marina', { status: 'inactive' }))).code).toBe('forbidden')
  })

  it('invites, resends and cancels', async () => {
    const world = createWorld()
    const admin = world.as('user-marina')
    const invited = await admin.users.invite({
      name: 'Nova Operadora',
      email: 'nova@email.test',
      role: 'planning_operator',
      companyId: 'company-santa-rita',
      farmScope: { mode: 'selected', farmIds: ['farm-santa-rita'] },
    })
    expect(invited.emailStatus).toBe('sent')
    expect(invited.user).toMatchObject({ status: 'invited', company: { name: 'Fazenda Santa Rita' }, lastAccessAt: null })
    expect((await rejection(admin.users.invite({ ...invited.user, companyId: 'company-santa-rita' }))).code).toBe('email_already_registered')

    expect(await admin.users.resendInvitation(invited.user.invitation!.id)).toBe('sent')
    await admin.users.cancelInvitation(invited.user.invitation!.id)
    expect((await admin.users.list()).total).toBe(3)
  })

  it('validates farms of the scope against the company', async () => {
    const world = createWorld()
    const error = await rejection(world.as('user-bruno').users.update('user-carlos', { farmScope: { mode: 'selected', farmIds: ['farm-recanto'] } }))
    expect(error.details).toEqual([{ field: 'farm_scope.farm_ids', message: 'fazenda não pertence à empresa' }])
  })
})

describe('mock companies', () => {
  it('lists client companies with counts for the super admin only', async () => {
    const world = createWorld()
    const page = await world.as('user-bruno').companies.list()
    expect(page.items.map(company => [company.name, company.plan, company.farmCount, company.userCount])).toEqual([
      ['Fazenda Santa Rita', 'pro', 1, 3],
      ['Fazenda Ouro Verde', 'pro', 1, 2],
      ['Sítio Boa Esperança', 'basic', 1, 1],
      ['Fazenda Recanto', 'trial', 1, 2],
    ])
    expect((await rejection(world.as('user-marina').companies.list())).code).toBe('forbidden')
  })

  it('summarizes the system and refuses a duplicated CNPJ', async () => {
    const world = createWorld()
    const superAdmin = world.as('user-bruno')
    expect(await superAdmin.companies.summary()).toEqual({
      activeCompanies: 4,
      trialCompanies: 1,
      farmCount: 4,
      monitoredAreaHa: 1284.2,
      userCount: 9,
      pendingInvitations: 2,
      applicationsThisMonth: 31,
      applicationsPreviousMonth: 26,
    })
    const duplicate = { name: 'Outra', document: '12.345.678/0001-90', city: 'Alfenas', state: 'MG', plan: 'basic' } as const
    expect((await rejection(superAdmin.companies.create(duplicate))).code).toBe('conflict')
    const trial = await superAdmin.companies.create({ ...duplicate, document: '33444555000166', plan: 'trial' })
    expect(trial).toMatchObject({ status: 'trial', farmCount: 0 })
    expect(trial.trialEndsAt).not.toBeNull()
  })
})

describe('mock profile', () => {
  it('updates the name and changes the password with the current one', async () => {
    const world = createWorld()
    const operator = world.as('user-carlos')
    expect((await operator.profile.update({ name: 'Carlos M. Mendes' })).name).toBe('Carlos M. Mendes')

    const wrong = await rejection(operator.auth.changePassword({ currentPassword: 'nope', newPassword: 'curta' }))
    expect(wrong.details.map(detail => detail.field)).toEqual(['current_password', 'new_password'])

    await operator.auth.changePassword({ currentPassword: TEST_PASSWORD, newPassword: 'nova-senha-123' })
    expect((await operator.profile.get()).passwordChangedAt).toEqual(new Date('2026-11-21T12:00:00Z'))
    const session = await world.api.auth.login({ identifier: 'carlos.mendes@email.test', password: 'nova-senha-123', rememberMe: false })
    expect(session.user.id).toBe('user-carlos')
  })
})

describe('mock dashboard', () => {
  it('serves the dashboard in the caller scope', async () => {
    const world = createWorld()
    const admin = await world.as('user-marina').dashboard.get()
    expect(admin.farmCount).toBe(1)
    expect(admin.windowsVsApplications.days).toHaveLength(14)
    expect(admin.indicators.suitableWindowsToday.fieldLabels).toEqual(['T1', 'T3', 'T8'])
    expect(admin.upcomingWindows.every(window => window.farmId === 'farm-santa-rita')).toBe(true)
    expect(admin.agentAlerts.map(alert => alert.tone)).toEqual(['attention', 'highlight', 'info'])

    expect((await world.as('user-bruno').dashboard.get({ farmId: 'farm-recanto', periodDays: 7 })).windowsVsApplications.days).toHaveLength(7)
  })

  it('refuses users without dashboard.view and farms of other tenants', async () => {
    const world = createWorld()
    expect((await rejection(world.as('user-carlos').dashboard.get())).code).toBe('forbidden')
    expect((await rejection(world.as('user-marina').dashboard.get({ farmId: 'farm-ouro-verde' }))).status).toBe(404)
  })
})
