/**
 * Fake of the real (SSO) auth port, for tests of the cookie-session flow
 * (ADR 0015): the "server" knows whether the session cookie is valid and who
 * owns it. The rest of the services stay on the in-memory mock.
 */
import { ApiError } from '~/services/errors'
import type { ApiServices, SsoAuthService } from '~/services/ports'
import type { User } from '~/types/auth'
import { createTestApi } from './mockApi'
import { installNuxtFakes, type NuxtFakes } from './nuxtFakes'

export const SSO_API_BASE = 'http://localhost:8002'

export const SSO_ADMIN: User = {
  id: 'kc-user-1',
  name: 'Marina Rezende',
  email: 'marina@fazendasantarita.test',
  role: 'admin',
  primaryFarm: null,
  company: { id: 'company-1', name: 'Fazenda Santa Rita' },
  farmScope: { mode: 'all' },
  permissions: ['dashboard.view', 'users.manage', 'registry.write', 'applications.register', 'applications.confirm', 'planner.use'],
}

export interface FakeSso {
  readonly auth: SsoAuthService
  /** Valid session cookie of this user (null = signed out). */
  session: User | null
  /** What `GET /auth/me` answers instead of the session (403 product denied, 5xx...). */
  meError: ApiError | null
  /** Makes `POST /auth/logout` fail. */
  logoutError: ApiError | null
  readonly calls: { me: number, logout: number }
}

export function apiError(code: ApiError['code'], status: number, message = code): ApiError {
  return new ApiError({ code, message, status })
}

export function createFakeSso(session: User | null = null): FakeSso {
  const fake: FakeSso = {
    session,
    meError: null,
    logoutError: null,
    calls: { me: 0, logout: 0 },
    auth: {
      kind: 'sso',
      loginUrl: returnTo => `${SSO_API_BASE}/auth/login?return_to=${encodeURIComponent(returnTo)}`,
      async getCurrentUser() {
        fake.calls.me += 1
        if (fake.meError) throw fake.meError
        if (!fake.session) throw apiError('unauthorized', 401, 'autenticação necessária')
        return fake.session
      },
      async logout() {
        fake.calls.logout += 1
        if (fake.logoutError) throw fake.logoutError
        fake.session = null
      },
    },
  }
  return fake
}

/** Nuxt fakes whose `$api` is the mock with the SSO auth port in place of its password one. */
export function installSsoFakes(sso: FakeSso, overrides: Partial<ApiServices> = {}): NuxtFakes {
  const { api } = createTestApi(null)
  return installNuxtFakes({ ...api, ...overrides, auth: sso.auth })
}
