/**
 * The invitation flows of the "Usuários" screen with an API that cannot send the
 * e-mail (`X-Invitation-Email: not_sent`) or that refuses the operation with a
 * stable 409/502 code (ADR 0015, D5 and D7).
 */
import { afterEach, describe, expect, it, vi } from 'vitest'
import { AUTH_TOKEN_COOKIE } from '~/composables/useAuthToken'
import { useFlash } from '~/composables/useFlash'
import { useSession } from '~/composables/useSession'
import { useUserAccessForm } from '~/composables/useUserAccessForm'
import { useUsers } from '~/composables/useUsers'
import { useUserListPage } from '~/composables/useUserListPage'
import { ApiError } from '~/services/errors'
import type { ApiServices } from '~/services/ports'
import type { InvitationEmailStatus } from '~/types/admin'
import { createTestApi } from '../../support/mockApi'
import { flushPromises, installNuxtFakes, type NuxtFakes } from '../../support/nuxtFakes'
import { SEED_NOW } from '../../support/session'

afterEach(() => vi.unstubAllGlobals())

const SUPER_ADMIN_TOKEN = 'mock-token:user-bruno'

async function setup(wrapUsers: (users: ApiServices['users']) => ApiServices['users']): Promise<NuxtFakes> {
  const { api } = createTestApi(SUPER_ADMIN_TOKEN)
  const fakes = installNuxtFakes({ ...api, users: wrapUsers(api.users) })
  fakes.cookie(AUTH_TOKEN_COOKIE).value = SUPER_ADMIN_TOKEN
  await useSession().fetchCurrentUser()
  return fakes
}

const withEmail = (emailStatus: InvitationEmailStatus) => (users: ApiServices['users']): ApiServices['users'] => ({
  ...users,
  invite: async input => ({ ...(await users.invite(input)), emailStatus }),
  resendInvitation: async (invitationId) => {
    await users.resendInvitation(invitationId)
    return emailStatus
  },
})

const failing = (error: ApiError) => (users: ApiServices['users']): ApiServices['users'] => ({
  ...users,
  invite: async () => {
    throw error
  },
  resendInvitation: async () => {
    throw error
  },
  cancelInvitation: async () => {
    throw error
  },
  update: async () => {
    throw error
  },
})

async function submitInvite(email = 'nova@email.test') {
  const form = useUserAccessForm()
  await flushPromises()
  Object.assign(form.values, { name: 'Nova Operadora', email, role: 'planning_operator', companyId: 'company-santa-rita' })
  const saved = await form.submit()
  return { form, saved }
}

describe('invite modal', () => {
  it('flashes a success when the e-mail left', async () => {
    await setup(withEmail('sent'))
    const { saved } = await submitInvite()
    expect(saved).toBe(true)
    expect(useFlash().consume()).toMatchObject({ kind: 'success', title: 'Convite enviado' })
  })

  it('flashes an honest warning when the invitation was created but the e-mail was not sent', async () => {
    await setup(withEmail('not_sent'))
    const { saved } = await submitInvite()
    expect(saved).toBe(true)
    expect(useFlash().consume()).toMatchObject({
      kind: 'attention',
      title: 'Convite criado, mas o e-mail não foi enviado',
      highlightId: expect.any(String),
    })
  })

  it('flashes a warning, not a success, when the header could not be read', async () => {
    await setup(withEmail('unknown'))
    await submitInvite()
    expect(useFlash().consume()).toMatchObject({ kind: 'attention', title: 'Convite criado' })
  })

  it('puts email_already_registered on the e-mail field', async () => {
    await setup(failing(new ApiError({ code: 'email_already_registered', message: 'x', status: 409 })))
    const { form, saved } = await submitInvite()
    expect(saved).toBe(false)
    expect(form.fieldErrors.value.email).toBe('Este e-mail já tem cadastro.')
    expect(form.formError.value).toBeNull()
  })

  it.each([
    ['email_not_verified', 409, 'ainda não foi verificado'],
    ['company_inactive', 409, 'empresa está inativa'],
    ['identity_provider_unavailable', 502, 'Nada foi alterado'],
  ] as const)('shows the curated message of %s as the form error', async (code, status, fragment) => {
    await setup(failing(new ApiError({ code, message: 'texto do servidor', status })))
    const { form, saved } = await submitInvite()
    expect(saved).toBe(false)
    expect(form.formError.value).toContain(fragment)
  })
})

describe('users list actions', () => {
  async function pageWith(wrapUsers: (users: ApiServices['users']) => ApiServices['users']) {
    await setup(wrapUsers)
    const page = useUserListPage({ now: () => SEED_NOW })
    await flushPromises()
    return { page, ana: page.rows.value.find(row => row.name === 'Ana Lúcia Prado')! }
  }

  it('warns when the resent invitation did not get an e-mail', async () => {
    const { page, ana } = await pageWith(withEmail('not_sent'))
    await page.runAction(ana, 'resend_invitation')
    expect(page.notice.value).toMatchObject({ kind: 'attention', title: 'Convite renovado, mas o e-mail não foi enviado' })
  })

  it('names the reason when the invitation is already closed', async () => {
    const { page, ana } = await pageWith(failing(new ApiError({ code: 'invitation_closed', message: 'x', status: 409 })))
    await page.runAction(ana, 'cancel_invitation')
    expect(page.notice.value).toMatchObject({ kind: 'error', description: 'O convite já foi aceito ou cancelado.' })
  })

  it('names the reason when the last super admin would be lost', async () => {
    const { page } = await pageWith(failing(new ApiError({ code: 'last_super_admin', message: 'x', status: 409 })))
    const marina = page.rows.value.find(row => row.name === 'Marina Rezende')!
    await page.runAction(marina, 'deactivate')
    expect(page.notice.value).toMatchObject({ kind: 'error', description: expect.stringContaining('ao menos um super admin') })
  })

  it('reports the identity provider outage without claiming the change happened', async () => {
    const { page, ana } = await pageWith(failing(new ApiError({ code: 'identity_provider_unavailable', message: 'x', status: 502 })))
    await page.runAction(ana, 'resend_invitation')
    expect(page.notice.value).toMatchObject({ kind: 'error', description: expect.stringContaining('Nada foi alterado') })
  })
})

describe('flash handed over by the invite modal', () => {
  it('shows the honest "e-mail not sent" warning on the users page', async () => {
    await setup(withEmail('not_sent'))
    await submitInvite()
    const page = useUserListPage({ now: () => SEED_NOW })
    await flushPromises()

    page.takeFlash()
    expect(page.notice.value).toMatchObject({ kind: 'attention', title: 'Convite criado, mas o e-mail não foi enviado' })
    expect(useFlash().consume()).toBeNull()
  })

  it('shows "Acesso atualizado" after editing an account', async () => {
    await setup(users => users)
    const page = useUserListPage({ now: () => SEED_NOW })
    await flushPromises()
    const listed = useUsers({ pageSize: 50 })
    await flushPromises()
    const account = listed.users.value.find(item => item.status === 'active' && item.role !== 'admin' && item.id !== 'user-bruno')!

    const form = useUserAccessForm({ user: () => account })
    await flushPromises()
    form.values.role = 'admin'
    expect(await form.submit()).toBe(true)

    page.takeFlash()
    expect(page.notice.value).toMatchObject({ kind: 'success', title: 'Acesso atualizado' })
  })
})

describe('invite modal without the farm list', () => {
  async function withFarmsDown() {
    const { api } = createTestApi(SUPER_ADMIN_TOKEN)
    const fakes = installNuxtFakes({
      ...api,
      farms: { ...api.farms, list: async () => { throw new ApiError({ code: 'not_found', message: 'x', status: 404 }) } },
    })
    fakes.cookie(AUTH_TOKEN_COOKIE).value = SUPER_ADMIN_TOKEN
    await useSession().fetchCurrentUser()
  }

  it('keeps companies loaded, flags farms as unavailable and still invites', async () => {
    await withFarmsDown()
    const form = useUserAccessForm()
    await flushPromises()

    expect(form.loadError.value).toBeNull()
    expect(form.farmsUnavailable.value).toBe(true)
    expect(form.companyOptions.value.length).toBeGreaterThan(1)
    expect(form.farmOptions.value).toEqual([])
    expect(form.scopeOptions.value.find(item => item.value === 'selected')?.disabled).toBe(true)

    Object.assign(form.values, { name: 'Nova Operadora', email: 'nova@email.test', role: 'planning_operator', companyId: 'company-santa-rita' })
    expect(await form.submit()).toBe(true)
  })
})
