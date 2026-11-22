import { afterEach, describe, expect, it, vi } from 'vitest'
import { AUTH_TOKEN_COOKIE } from '~/composables/useAuthToken'
import { useFlash } from '~/composables/useFlash'
import { SEED_USER_TOKEN } from '../../support/mockApi'
import {
  installNuxtFakes,
  type FakeRouteMiddleware,
  type FakeRouteTarget,
  type NuxtFakes,
} from '../../support/nuxtFakes'

afterEach(() => {
  vi.unstubAllGlobals()
  vi.resetModules()
})

async function setup(token: string | null): Promise<{ fakes: NuxtFakes, middleware: FakeRouteMiddleware }> {
  // Loaded per test (after resetModules) so the mock and the middleware share
  // the same `ApiError` class instance.
  const { createMockServices } = await import('~/services/mock/mockServices')
  const fakes = installNuxtFakes(nuxt => createMockServices({
    latencyMs: 0,
    getAccessToken: () => nuxt.cookie(AUTH_TOKEN_COOKIE).value,
  }))
  fakes.cookie(AUTH_TOKEN_COOKIE).value = token
  // Imported after the fakes, since `defineNuxtRouteMiddleware` runs at module load.
  await import('~/middleware/auth.global')
  if (!fakes.routeMiddleware) throw new Error('middleware was not registered')
  return { fakes, middleware: fakes.routeMiddleware }
}

const privateRoute: FakeRouteTarget = { path: '/', fullPath: '/', meta: {}, query: {} }
const loginRoute: FakeRouteTarget = { path: '/login', fullPath: '/login', meta: { public: true }, query: {} }

describe('auth.global middleware', () => {
  it('sends a guest to /login with the original destination', async () => {
    const { middleware } = await setup(null)
    expect(await middleware(privateRoute)).toEqual({ path: '/login', query: { redirect: '/' } })
  })

  it('restores the user from the cookie and lets the navigation through', async () => {
    const { middleware } = await setup(SEED_USER_TOKEN)
    expect(await middleware({ ...privateRoute, path: '/farms', fullPath: '/farms' })).toBeUndefined()
  })

  it('drops an invalid token and redirects to /login', async () => {
    const { fakes, middleware } = await setup('mock-token:revoked')
    expect(await middleware(privateRoute)).toEqual({ path: '/login', query: { redirect: '/' } })
    expect(fakes.cookie(AUTH_TOKEN_COOKIE).value).toBeNull()
  })

  it('keeps /login open for guests and moves signed-in users out of it', async () => {
    const guest = await setup(null)
    expect(await guest.middleware(loginRoute)).toBeUndefined()

    vi.unstubAllGlobals()
    vi.resetModules()
    const signedIn = await setup(SEED_USER_TOKEN)
    expect(await signedIn.middleware({ ...loginRoute, query: { redirect: '/owners' } })).toEqual({ path: '/owners' })
  })

  it('sends a user without the page permission to their home page with an honest message', async () => {
    const { fakes, middleware } = await setup('mock-token:user-carlos')
    const usersPage = { path: '/users', fullPath: '/users', meta: { permission: 'users.manage' }, query: {} }

    expect(await middleware(usersPage)).toBe('/farms')
    expect(fakes.navigations.at(-1)).toEqual({ target: '/farms', options: { replace: true } })
    expect(useFlash().consume()).toMatchObject({ kind: 'error', title: 'Acesso não permitido' })
  })

  it('moves an operator from the dashboard root to /farms without a message', async () => {
    const { middleware } = await setup('mock-token:user-carlos')
    expect(await middleware({ ...privateRoute, meta: { permission: 'dashboard.view' } })).toBe('/farms')
    expect(useFlash().consume()).toBeNull()
  })

  it('lets an administrator open pages of their capabilities', async () => {
    const { middleware } = await setup('mock-token:user-marina')
    expect(await middleware({ path: '/users', fullPath: '/users', meta: { permission: 'users.manage' }, query: {} })).toBeUndefined()
    expect(await middleware({ ...loginRoute })).toEqual({ path: '/' })
  })
})
