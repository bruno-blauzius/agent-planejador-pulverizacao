import { afterEach, describe, expect, it, vi } from 'vitest'
import { useSession } from '~/composables/useSession'
import { AUTH_TOKEN_COOKIE } from '~/composables/useAuthToken'
import { createMockServices } from '~/services/mock/mockServices'
import { installNuxtFakes, type NuxtFakes } from '../../support/nuxtFakes'
import { SEED_USER_EMAIL, SEED_USER_PASSWORD, SEED_USER_TOKEN } from '../../support/mockApi'

function setup(): NuxtFakes {
  // The mock reads the same cookie the session writes, exactly as plugins/api.ts wires it.
  return installNuxtFakes(fakes => createMockServices({
    latencyMs: 0,
    getAccessToken: () => fakes.cookie(AUTH_TOKEN_COOKIE).value,
  }))
}

afterEach(() => vi.unstubAllGlobals())

describe('useSession', () => {
  it('stores token and user after a successful login', async () => {
    const fakes = setup()
    const session = useSession()

    const user = await session.login({ identifier: SEED_USER_EMAIL, password: SEED_USER_PASSWORD, rememberMe: false })

    expect(user?.name).toBe('Bruno Blauzius')
    expect(fakes.cookie(AUTH_TOKEN_COOKIE).value).toBe(SEED_USER_TOKEN)
    expect(fakes.cookieExpires(AUTH_TOKEN_COOKIE)?.getTime()).toBeGreaterThan(Date.now())
    expect(session.isAuthenticated.value).toBe(true)
    expect(session.error.value).toBeNull()
  })

  it('shares the user across composable instances', async () => {
    setup()
    await useSession().login({ identifier: SEED_USER_EMAIL, password: SEED_USER_PASSWORD, rememberMe: false })
    expect(useSession().user.value?.email).toBe(SEED_USER_EMAIL)
  })

  it('exposes the rejection instead of throwing', async () => {
    setup()
    const session = useSession()

    const user = await session.login({ identifier: SEED_USER_EMAIL, password: 'wrong', rememberMe: false })

    expect(user).toBeNull()
    expect(session.error.value).toMatchObject({ code: 'unauthorized', message: 'e-mail ou senha inválidos' })
    expect(session.isAuthenticated.value).toBe(false)
    expect(session.pending.value).toBe(false)
  })

  it('restores the user from an existing token', async () => {
    const fakes = setup()
    fakes.cookie(AUTH_TOKEN_COOKIE).value = SEED_USER_TOKEN
    const session = useSession()

    await session.fetchCurrentUser()

    expect(session.user.value?.id).toBe('user-bruno')
  })

  it('drops an invalid token and reports it', async () => {
    const fakes = setup()
    fakes.cookie(AUTH_TOKEN_COOKIE).value = 'mock-token:revoked'
    const session = useSession()

    expect(await session.fetchCurrentUser()).toBeNull()

    expect(fakes.cookie(AUTH_TOKEN_COOKIE).value).toBeNull()
    expect(session.error.value?.code).toBe('unauthorized')
  })

  it('drops cached page data when the session changes (no data of the previous user)', async () => {
    const fakes = setup()
    const session = useSession()
    await session.login({ identifier: SEED_USER_EMAIL, password: SEED_USER_PASSWORD, rememberMe: false })
    expect(fakes.clearedNuxtData).toBe(1)
    await session.logout()
    expect(fakes.clearedNuxtData).toBe(2)
  })

  it('logs out locally', async () => {
    const fakes = setup()
    const session = useSession()
    await session.login({ identifier: SEED_USER_EMAIL, password: SEED_USER_PASSWORD, rememberMe: false })

    expect(await session.logout()).toBe(true)

    expect(fakes.cookie(AUTH_TOKEN_COOKIE).value).toBeNull()
    expect(fakes.cookieExpires(AUTH_TOKEN_COOKIE)).toBeUndefined()
    expect(session.user.value).toBeNull()
  })
})
