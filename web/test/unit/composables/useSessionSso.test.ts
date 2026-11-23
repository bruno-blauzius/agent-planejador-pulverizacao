import { afterEach, describe, expect, it, vi } from 'vitest'
import { useLoginForm } from '~/composables/useLoginForm'
import { useProfilePage } from '~/composables/useProfilePage'
import { useSession } from '~/composables/useSession'
import { useSessionExpiry } from '~/composables/useSessionExpiry'
import { AUTH_TOKEN_COOKIE } from '~/composables/useAuthToken'
import { DEFAULT_ACCOUNT_URL } from '~/utils/profile'
import { flushPromises } from '../../support/nuxtFakes'
import { apiError, createFakeSso, installSsoFakes, SSO_ADMIN, SSO_API_BASE } from '../../support/sso'

afterEach(() => vi.unstubAllGlobals())

describe('useSession with the SSO cookie session', () => {
  it('restores the user from GET /auth/me and exposes the SSO flavour', async () => {
    const sso = createFakeSso(SSO_ADMIN)
    const fakes = installSsoFakes(sso)
    const session = useSession()

    expect(session.authKind).toBe('sso')
    expect(session.canRestore.value).toBe(true)
    await session.fetchCurrentUser()

    expect(session.user.value?.id).toBe('kc-user-1')
    expect(session.isAuthenticated.value).toBe(true)
    expect(fakes.cookie(AUTH_TOKEN_COOKIE).value).toBeNull()
  })

  it('drops the user on 401 and on a denied product, keeps it on an outage', async () => {
    const sso = createFakeSso(SSO_ADMIN)
    installSsoFakes(sso)
    const session = useSession()
    await session.fetchCurrentUser()

    sso.meError = apiError('network_error', 0)
    expect(await session.fetchCurrentUser()).toBeNull()
    expect(session.user.value?.id).toBe('kc-user-1')
    expect(session.error.value?.code).toBe('network_error')

    sso.meError = apiError('no_access_to_product', 403)
    await session.fetchCurrentUser()
    expect(session.user.value).toBeNull()

    sso.meError = null
    sso.session = null
    await session.fetchCurrentUser()
    expect(session.error.value?.code).toBe('unauthorized')
  })

  it('builds the SSO login URL and refuses a password login', async () => {
    installSsoFakes(createFakeSso())
    const session = useSession()

    expect(session.signInUrl('/farms')).toBe(`${SSO_API_BASE}/auth/login?return_to=%2Ffarms`)
    expect(await session.login({ identifier: 'a@x.test', password: 'x', rememberMe: false })).toBeNull()
    expect(session.error.value?.code).toBe('unknown_error')
  })

  it('logs out on the server first, then clears the session and the cached data', async () => {
    const sso = createFakeSso(SSO_ADMIN)
    const fakes = installSsoFakes(sso)
    const session = useSession()
    await session.fetchCurrentUser()

    expect(await session.logout()).toBe(true)

    expect(sso.calls.logout).toBe(1)
    expect(session.user.value).toBeNull()
    expect(fakes.clearedNuxtData).toBe(1)
  })

  it('keeps the session when the server refuses to log out (it is still valid there)', async () => {
    const sso = createFakeSso(SSO_ADMIN)
    sso.logoutError = apiError('forbidden', 403, 'origem não permitida para esta ação')
    const fakes = installSsoFakes(sso)
    const session = useSession()
    await session.fetchCurrentUser()

    expect(await session.logout()).toBeNull()

    expect(session.user.value?.id).toBe('kc-user-1')
    expect(session.error.value?.code).toBe('forbidden')
    expect(fakes.clearedNuxtData).toBe(0)
  })
})

describe('useSessionExpiry (401 on a page call)', () => {
  async function signedIn() {
    const sso = createFakeSso(SSO_ADMIN)
    const fakes = installSsoFakes(sso)
    const session = useSession()
    await session.fetchCurrentUser()
    return { fakes, session }
  }

  it('forgets the user and sends them to /login keeping the page, once', async () => {
    const { fakes, session } = await signedIn()
    fakes.route.fullPath = '/users?page=2'
    const expiry = useSessionExpiry()

    expiry.handle('/users')
    expiry.handle('/users/summary')

    expect(session.user.value).toBeNull()
    expect(fakes.navigations).toEqual([{ path: '/login', query: { redirect: '/users?page=2' } }])
  })

  it('ignores the auth calls themselves', async () => {
    const { fakes, session } = await signedIn()
    useSessionExpiry().handle('/auth/me')
    expect(session.user.value?.id).toBe('kc-user-1')
    expect(fakes.navigations).toEqual([])
  })

  it('does not navigate away from a public page', async () => {
    const { fakes, session } = await signedIn()
    fakes.route.meta = { public: true }
    useSessionExpiry().handle('/users')
    expect(session.user.value).toBeNull()
    expect(fakes.navigations).toEqual([])
  })
})

describe('useLoginForm with the SSO', () => {
  it('links to the API login with the safe redirect', () => {
    const fakes = installSsoFakes(createFakeSso())
    fakes.route.query = { redirect: '/owners?q=ana' }
    const form = useLoginForm()

    expect(form.kind).toBe('sso')
    expect(form.ssoUrl.value).toBe(`${SSO_API_BASE}/auth/login?return_to=%2Fowners%3Fq%3Dana`)
  })

  it('falls back to "/" for an open redirect attempt', () => {
    const fakes = installSsoFakes(createFakeSso())
    fakes.route.query = { redirect: '//evil.test' }
    expect(useLoginForm().ssoUrl.value).toBe(`${SSO_API_BASE}/auth/login?return_to=%2F`)
  })

  it.each([
    ['user_not_provisioned', 'ainda não foi convidada'],
    ['no_access_to_product', 'não tem acesso'],
    ['ambiguous_role', 'mais de um papel'],
    ['forbidden', 'inativo'],
    ['identity_provider_unavailable', 'indisponível'],
    ['unavailable', 'indisponível'],
  ])('explains ?error=%s in pt-BR', (code, fragment) => {
    const fakes = installSsoFakes(createFakeSso())
    fakes.route.query = { error: code }
    const error = useLoginForm().accessError.value
    expect(error?.title).toBe('Não foi possível entrar')
    expect(error?.message).toContain(fragment)
  })

  it('never echoes an unknown error code', () => {
    const fakes = installSsoFakes(createFakeSso())
    fakes.route.query = { error: '<script>alert(1)</script>' }
    expect(useLoginForm().accessError.value).toBeNull()
  })
})

describe('useProfilePage with the SSO', () => {
  async function profilePage() {
    const sso = createFakeSso(SSO_ADMIN)
    const fakes = installSsoFakes(sso)
    await useSession().fetchCurrentUser()
    return fakes
  }

  it('points the password to the Keycloak account page and never marks the form dirty for it', async () => {
    const fakes = await profilePage()
    fakes.publicConfig.accountUrl = 'https://sso.inceres.test/realms/inceres/account'
    const page = useProfilePage()
    await flushPromises()

    expect(page.passwordManagement).toEqual({ kind: 'sso', accountUrl: 'https://sso.inceres.test/realms/inceres/account' })
    // Typing into the (hidden) password fields can never make the page dirty.
    page.currentPassword.value = 'abc'
    expect(page.isDirty.value).toBe(false)
  })

  it('falls back to the default account URL when the config is empty', async () => {
    const fakes = await profilePage()
    fakes.publicConfig.accountUrl = ''
    expect(useProfilePage().passwordManagement).toEqual({ kind: 'sso', accountUrl: DEFAULT_ACCOUNT_URL })
  })
})
