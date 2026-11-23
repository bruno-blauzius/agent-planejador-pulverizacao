/** Users, invitations, companies and profile against the implemented contract (ADR 0013, 0015). */
import { describe, expect, it } from 'vitest'
import { ApiError } from '~/services/errors'
import { createHttpClient, type RawFetchOptions, type ResponseHeaders } from '~/services/http/client'
import { createHttpServices } from '~/services/http/services'
import { readInvitationEmailStatus } from '~/services/http/adminServices'
import type { CompanyDto, ProfileDto, UserAccountDto } from '~/types/dto'

interface RecordedCall {
  path: string
  options: RawFetchOptions
}

/** Fake transport: records requests, exposes `headers` of the response and answers or throws. */
function servicesWith(answer: unknown, headers: Record<string, string> = {}) {
  const calls: RecordedCall[] = []
  const http = createHttpClient({ baseURL: 'http://api.test', credentials: 'include' }, async (path, options) => {
    calls.push({ path, options })
    if (answer instanceof Error) throw answer
    options.onHeaders?.({ get: name => headers[name] ?? null })
    return answer
  })
  return { api: createHttpServices(http, 'http://localhost:8002'), calls }
}

function httpFailure(status: number, code: string, message = code, details: unknown[] = []) {
  return Object.assign(new Error(`[HTTP] ${status}`), { name: 'FetchError', statusCode: status, data: { error: { code, message, details } } })
}

const invitedDto: UserAccountDto = {
  id: 'u-9',
  name: 'Ana Lúcia Prado',
  email: 'ana.prado@email.test',
  role: 'planning_operator',
  company: { id: 'c-2', name: 'Sítio Boa Esperança' },
  farm_scope: { mode: 'all' },
  status: 'invited',
  last_access_at: null,
  invitation: { id: 'inv-1', expires_at: '2026-11-28T12:00:00Z' },
}

const companyDto: CompanyDto = {
  id: 'c-2',
  name: 'Sítio Boa Esperança',
  document: '11222333000144',
  city: 'Muzambinho',
  state: 'MG',
  plan: 'trial',
  status: 'trial',
  trial_ends_at: '2026-12-05T12:00:00Z',
  farm_count: 1,
  user_count: 2,
  created_at: '2026-10-01T12:00:00Z',
}

const invitation = {
  name: ' Ana Lúcia Prado ',
  email: ' ana.prado@email.test ',
  role: 'planning_operator',
  companyId: 'c-2',
  farmScope: { mode: 'all' },
} as const

describe('readInvitationEmailStatus (X-Invitation-Email)', () => {
  const headers = (value: string | null): ResponseHeaders => ({ get: () => value })

  it.each([
    ['sent', 'sent'],
    ['not_sent', 'not_sent'],
    ['not_needed', 'not_needed'],
    [' Not_Sent ', 'not_sent'],
  ] as const)('reads %j', (raw, expected) => {
    expect(readInvitationEmailStatus(headers(raw))).toBe(expected)
  })

  it('never claims a delivery it cannot see (header absent, unreadable cross-origin or unknown)', () => {
    expect(readInvitationEmailStatus(headers(null))).toBe('unknown')
    expect(readInvitationEmailStatus(headers('delivered'))).toBe('unknown')
  })
})

describe('http user service (implemented endpoints)', () => {
  it('invites with a trimmed snake_case body and reports the e-mail status of the header', async () => {
    const { api, calls } = servicesWith(invitedDto, { 'X-Invitation-Email': 'not_sent' })
    const result = await api.users.invite(invitation)

    expect(calls[0]).toMatchObject({
      path: '/invitations',
      options: {
        method: 'POST',
        credentials: 'include',
        body: { name: 'Ana Lúcia Prado', email: 'ana.prado@email.test', role: 'planning_operator', company_id: 'c-2', farm_scope: { mode: 'all' } },
      },
    })
    expect(result.emailStatus).toBe('not_sent')
    expect(result.user).toMatchObject({ id: 'u-9', status: 'invited', invitation: { id: 'inv-1', expiresAt: new Date('2026-11-28T12:00:00Z') } })
  })

  it('reports "unknown" when the header is not exposed to the browser', async () => {
    const { api } = servicesWith(invitedDto)
    expect((await api.users.invite(invitation)).emailStatus).toBe('unknown')
  })

  it('resends and returns the e-mail status of the 204', async () => {
    const { api, calls } = servicesWith(undefined, { 'X-Invitation-Email': 'sent' })
    expect(await api.users.resendInvitation('inv 1')).toBe('sent')
    expect(calls[0]).toMatchObject({ path: '/invitations/inv%201/resend', options: { method: 'POST' } })
  })

  it('gets, renames, and deletes a user', async () => {
    const { api, calls } = servicesWith({ ...invitedDto, status: 'active', invitation: null })
    expect((await api.users.get('u/9')).id).toBe('u-9')
    await api.users.update('u-9', { name: '  Ana P.  ', status: 'inactive' })
    await api.users.remove('u-9')

    expect(calls.map(call => [call.options.method, call.path])).toEqual([
      ['GET', '/users/u%2F9'],
      ['PATCH', '/users/u-9'],
      ['DELETE', '/users/u-9'],
    ])
    expect(calls[1]?.options.body).toEqual({ name: 'Ana P.', status: 'inactive' })
  })

  it.each([
    [409, 'email_already_registered', 'Este e-mail já tem cadastro'],
    [409, 'email_not_verified', 'ainda não foi verificado'],
    [409, 'company_inactive', 'empresa está inativa'],
    [409, 'invitation_closed', 'já foi aceito ou cancelado'],
    [409, 'invitation_pending', 'Cancele o convite'],
    [409, 'last_super_admin', 'ao menos um super admin'],
    [502, 'identity_provider_unavailable', 'Nada foi alterado'],
  ])('maps %i %s to a stable code and a pt-BR message', async (status, code, fragment) => {
    const { api } = servicesWith(httpFailure(status, code, 'texto técnico do servidor'))
    const error = await api.users.invite(invitation).catch((caught: unknown) => caught)

    expect(error).toBeInstanceOf(ApiError)
    expect(error).toMatchObject({ code, status })
    expect((error as ApiError).message).toContain(fragment)
    expect((error as ApiError).message).not.toContain('técnico')
  })

  it('keeps the server text and details of a 422 (field errors are mapped per form)', async () => {
    const { api } = servicesWith(httpFailure(422, 'validation_failed', 'dados inválidos', [{ field: 'farm_scope', message: 'escopo inválido' }]))
    const error = await api.users.invite(invitation).catch((caught: unknown) => caught)
    expect(error).toMatchObject({ code: 'validation_failed', status: 422, details: [{ field: 'farm_scope', message: 'escopo inválido' }] })
  })

  it('maps a 404 and a 403 to their codes', async () => {
    expect(await servicesWith(httpFailure(404, 'not_found')).api.users.get('x').catch((e: unknown) => e)).toMatchObject({ code: 'not_found', status: 404 })
    expect(await servicesWith(httpFailure(403, 'forbidden')).api.users.remove('x').catch((e: unknown) => e)).toMatchObject({ code: 'forbidden', status: 403 })
  })
})

describe('http company service (implemented endpoints)', () => {
  it('gets, creates, patches only the changed keys and deletes', async () => {
    const { api, calls } = servicesWith(companyDto)
    expect((await api.companies.get('c-2')).trialEndsAt).toEqual(new Date('2026-12-05T12:00:00Z'))
    await api.companies.create({ name: 'Sítio', document: '11.222.333/0001-44', city: 'Muzambinho', state: 'MG', plan: 'trial' })
    await api.companies.update('c-2', { status: 'inactive' })
    await api.companies.remove('c-2')

    expect(calls.map(call => [call.options.method, call.path])).toEqual([
      ['GET', '/companies/c-2'],
      ['POST', '/companies'],
      ['PATCH', '/companies/c-2'],
      ['DELETE', '/companies/c-2'],
    ])
    expect(calls[2]?.options.body).toEqual({ status: 'inactive' })
  })

  it('lists with snake_case paging', async () => {
    const { api, calls } = servicesWith({ items: [companyDto], total: 1, page: 2, page_size: 10 })
    const page = await api.companies.list({ search: 'sítio', status: 'trial', page: 2, pageSize: 10 })
    expect(calls[0]?.options.query).toEqual({ search: 'sítio', status: 'trial', page: 2, page_size: 10 })
    expect(page).toMatchObject({ total: 1, page: 2, pageSize: 10 })
  })

  it('maps CNPJ conflicts and companies with users', async () => {
    const duplicate = await servicesWith(httpFailure(409, 'conflict', 'CNPJ já cadastrado')).api.companies
      .create({ name: 'x', document: '1', city: 'a', state: 'MG', plan: 'basic' }).catch((e: unknown) => e)
    expect(duplicate).toMatchObject({ code: 'conflict', status: 409, message: 'CNPJ já cadastrado' })

    const withUsers = await servicesWith(httpFailure(409, 'company_has_active_users')).api.companies.remove('c-2').catch((e: unknown) => e)
    expect(withUsers).toMatchObject({ code: 'company_has_active_users', status: 409 })
    expect((withUsers as ApiError).message).toContain('usuários ativos ou convidados')
  })

  it('maps a retryable identity provider failure on deactivation', async () => {
    const error = await servicesWith(httpFailure(502, 'identity_provider_unavailable')).api.companies
      .update('c-2', { status: 'inactive' }).catch((e: unknown) => e)
    expect(error).toMatchObject({ code: 'identity_provider_unavailable', status: 502 })
  })
})

describe('http profile service', () => {
  const profileDto: ProfileDto = {
    id: 'u-1',
    name: 'Bruno',
    email: 'b@x.test',
    role: 'admin',
    company: { id: 'c-1', name: 'Fazenda Santa Rita' },
    farm_scope: { mode: 'all' },
    password_changed_at: null,
  }

  it('reads the profile; the password date is null because the password is Keycloak\'s', async () => {
    const { api, calls } = servicesWith(profileDto)
    const profile = await api.profile.get()
    expect(calls[0]).toMatchObject({ path: '/me/profile', options: { method: 'GET' } })
    expect(profile.passwordChangedAt).toBeNull()
  })

  it('sends only the name in PATCH /me/profile (e-mail, role and company are ignored by the API)', async () => {
    const { api, calls } = servicesWith({ ...profileDto, name: 'Bruno B.' })
    await api.profile.update({ name: '  Bruno B.  ' })
    expect(calls[0]).toMatchObject({ path: '/me/profile', options: { method: 'PATCH', body: { name: 'Bruno B.' } } })
  })

  it('has no password endpoint: POST /me/password is obsolete', () => {
    const { api } = servicesWith(profileDto)
    expect('changePassword' in api.profile).toBe(false)
    expect('changePassword' in api.auth).toBe(false)
  })
})
