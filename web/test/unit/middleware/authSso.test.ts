import { afterEach, describe, expect, it, vi } from 'vitest'
import { AUTH_TOKEN_COOKIE } from '~/composables/useAuthToken'
import { useFlash } from '~/composables/useFlash'
import {
  installNuxtFakes,
  type FakeRouteMiddleware,
  type FakeRouteTarget,
  type NuxtFakes,
} from '../../support/nuxtFakes'
import { apiError, createFakeSso, installSsoFakes, SSO_ADMIN, type FakeSso } from '../../support/sso'

afterEach(() => {
  vi.unstubAllGlobals()
  vi.resetModules()
})

async function setup(sso: FakeSso): Promise<{ fakes: NuxtFakes, middleware: FakeRouteMiddleware }> {
  const fakes = installSsoFakes(sso)
  // Imported after the fakes: `defineNuxtRouteMiddleware` runs at module load.
  await import('~/middleware/auth.global')
  if (!fakes.routeMiddleware) throw new Error('middleware was not registered')
  return { fakes, middleware: fakes.routeMiddleware }
}

const privateRoute: FakeRouteTarget = { path: '/farms', fullPath: '/farms?status=pending', meta: {}, query: {} }
const loginRoute: FakeRouteTarget = { path: '/login', fullPath: '/login', meta: { public: true }, query: {} }

describe('auth.global middleware with the SSO cookie session (ADR 0015)', () => {
  it('asks GET /auth/me (the cookie is invisible to JS) and lets a signed-in user through', async () => {
    const sso = createFakeSso(SSO_ADMIN)
    const { fakes, middleware } = await setup(sso)

    expect(await middleware(privateRoute)).toBeUndefined()
    expect(sso.calls.me).toBe(1)
    // The real path never creates the mock's token cookie.
    expect(fakes.cookie(AUTH_TOKEN_COOKIE).value).toBeNull()
  })

  it('does not ask again once the user is known', async () => {
    const sso = createFakeSso(SSO_ADMIN)
    const { middleware } = await setup(sso)

    await middleware(privateRoute)
    await middleware({ ...privateRoute, path: '/owners', fullPath: '/owners' })

    expect(sso.calls.me).toBe(1)
  })

  it('sends a guest (401) to /login keeping the destination, with no error banner', async () => {
    const { middleware } = await setup(createFakeSso(null))
    expect(await middleware(privateRoute)).toEqual({ path: '/login', query: { redirect: '/farms?status=pending' } })
  })

  it.each([
    'user_not_provisioned',
    'no_access_to_product',
    'ambiguous_role',
    'forbidden',
  ] as const)('reports a 403 %s on /login instead of looping', async (code) => {
    const sso = createFakeSso(SSO_ADMIN)
    sso.meError = apiError(code, 403)
    const { middleware } = await setup(sso)

    expect(await middleware(privateRoute)).toEqual({ path: '/login', query: { error: code, redirect: '/farms?status=pending' } })
  })

  it('does not ask /auth/me again on the /login page that shows the error (no redirect loop)', async () => {
    const sso = createFakeSso(SSO_ADMIN)
    sso.meError = apiError('user_not_provisioned', 403)
    const { middleware } = await setup(sso)

    const result = await middleware({ ...loginRoute, query: { error: 'user_not_provisioned' } })

    expect(result).toBeUndefined()
    expect(sso.calls.me).toBe(0)
  })

  it('reports an outage as unavailable, not as a signed-out user', async () => {
    const sso = createFakeSso(SSO_ADMIN)
    sso.meError = apiError('network_error', 0)
    const { middleware } = await setup(sso)

    expect(await middleware(privateRoute)).toEqual({ path: '/login', query: { error: 'unavailable', redirect: '/farms?status=pending' } })
  })

  it('moves a signed-in user out of /login to the safe destination', async () => {
    const { middleware } = await setup(createFakeSso(SSO_ADMIN))
    expect(await middleware({ ...loginRoute, query: { redirect: '/owners' } })).toEqual({ path: '/owners' })
  })

  it('keeps /login open for guests', async () => {
    const { middleware } = await setup(createFakeSso(null))
    expect(await middleware(loginRoute)).toBeUndefined()
  })

  it('still applies the capability of the page after restoring the session', async () => {
    const operator = { ...SSO_ADMIN, role: 'planning_operator' as const, permissions: ['planner.use' as const] }
    const { middleware } = await setup(createFakeSso(operator))
    const usersPage = { path: '/users', fullPath: '/users', meta: { permission: 'users.manage' }, query: {} }

    expect(await middleware(usersPage)).toBe('/farms')
    expect(useFlash().consume()).toMatchObject({ title: 'Acesso não permitido' })
  })
})

describe('mock flavour', () => {
  it('does not call the API for a guest without a token cookie', async () => {
    const { createMockServices } = await import('~/services/mock/mockServices')
    const fakes = installNuxtFakes(nuxt => createMockServices({ latencyMs: 0, getAccessToken: () => nuxt.cookie(AUTH_TOKEN_COOKIE).value }))
    await import('~/middleware/auth.global')
    expect(await fakes.routeMiddleware?.(privateRoute)).toEqual({ path: '/login', query: { redirect: '/farms?status=pending' } })
  })
})
