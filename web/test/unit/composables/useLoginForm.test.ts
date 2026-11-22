import { afterEach, describe, expect, it, vi } from 'vitest'
import { nextTick } from 'vue'
import { useLoginForm } from '~/composables/useLoginForm'
import { AUTH_TOKEN_COOKIE } from '~/composables/useAuthToken'
import { ApiError } from '~/services/errors'
import { createMockServices } from '~/services/mock/mockServices'
import type { ApiServices, AuthService } from '~/services/ports'
import { LOGIN_MESSAGES } from '~/utils/login'
import { SEED_USER_EMAIL, SEED_USER_PASSWORD, SEED_USER_TOKEN } from '../../support/mockApi'
import { installNuxtFakes, type NuxtFakes } from '../../support/nuxtFakes'

afterEach(() => vi.unstubAllGlobals())

function setup(auth?: AuthService): NuxtFakes {
  return installNuxtFakes((fakes) => {
    const api: ApiServices = createMockServices({
      latencyMs: 0,
      getAccessToken: () => fakes.cookie(AUTH_TOKEN_COOKIE).value,
    })
    return auth ? { ...api, auth } : api
  })
}

/** Auth service that always rejects with the given error. */
function failingAuth(error: ApiError): AuthService {
  return {
    login: async () => {
      throw error
    },
    getCurrentUser: async () => {
      throw error
    },
  }
}

describe('useLoginForm', () => {
  it('shows field errors only after touch or submit', async () => {
    const fakes = setup()
    const form = useLoginForm()
    expect(form.fieldErrors.value).toEqual({})

    form.touch('identifier')
    expect(form.fieldErrors.value).toEqual({ identifier: LOGIN_MESSAGES.identifierRequired })

    expect(await form.submit()).toBe(false)
    expect(form.fieldErrors.value).toEqual({
      identifier: LOGIN_MESSAGES.identifierRequired,
      password: LOGIN_MESSAGES.passwordRequired,
    })
    expect(fakes.navigations).toEqual([])
  })

  it('logs in and goes to the safe redirect', async () => {
    const fakes = setup()
    fakes.route.query = { redirect: '/owners?personType=company' }
    const form = useLoginForm()
    form.identifier.value = SEED_USER_EMAIL
    form.password.value = SEED_USER_PASSWORD

    expect(await form.submit()).toBe(true)

    expect(fakes.cookie(AUTH_TOKEN_COOKIE).value).toBe(SEED_USER_TOKEN)
    expect(fakes.navigations).toEqual(['/owners?personType=company'])
  })

  it('ignores a malicious redirect and falls back to the home page of the role (ADR 0013)', async () => {
    const fakes = setup()
    fakes.route.query = { redirect: '//evil.test' }
    const form = useLoginForm()
    form.identifier.value = '123.278.916-05'
    form.password.value = SEED_USER_PASSWORD

    await form.submit()

    // Super admin has dashboard.view: home is the dashboard.
    expect(fakes.navigations).toEqual(['/'])
  })

  it('sends a user without dashboard.view to /farms', async () => {
    const fakes = setup()
    const form = useLoginForm()
    form.identifier.value = 'carlos.mendes@email.test'
    form.password.value = SEED_USER_PASSWORD

    await form.submit()

    expect(fakes.navigations).toEqual(['/farms'])
  })

  it('maps 401 to the credentials message and hides it on edit', async () => {
    setup()
    const form = useLoginForm()
    form.identifier.value = SEED_USER_EMAIL
    form.password.value = 'wrong'

    expect(await form.submit()).toBe(false)
    expect(form.formError.value).toBe(LOGIN_MESSAGES.invalidCredentials)

    form.password.value = 'another'
    await nextTick()
    expect(form.formError.value).toBeNull()
  })

  it('maps 422 details to the fields', async () => {
    setup(failingAuth(new ApiError({
      code: 'validation_failed',
      message: 'dados inválidos',
      status: 422,
      details: [{ field: 'identifier', message: 'CPF inválido' }],
    })))
    const form = useLoginForm()
    form.identifier.value = '111.111.111-11'
    form.password.value = 'x'

    await form.submit()

    expect(form.fieldErrors.value).toEqual({ identifier: 'CPF inválido' })
    expect(form.formError.value).toBeNull()
  })

  it('reports a network failure honestly', async () => {
    setup(failingAuth(new ApiError({ code: 'network_error', message: 'offline', status: 0 })))
    const form = useLoginForm()
    form.identifier.value = SEED_USER_EMAIL
    form.password.value = 'x'

    await form.submit()

    expect(form.formError.value).toBe(LOGIN_MESSAGES.network)
    expect(form.pending.value).toBe(false)
  })

  it('sends remember me to the API (longer cookie expiry)', async () => {
    const fakes = setup()
    const form = useLoginForm()
    form.identifier.value = SEED_USER_EMAIL
    form.password.value = SEED_USER_PASSWORD
    form.rememberMe.value = true

    await form.submit()

    const days = ((fakes.cookieExpires(AUTH_TOKEN_COOKIE)?.getTime() ?? 0) - Date.now()) / 86_400_000
    expect(days).toBeGreaterThan(29)
  })
})
