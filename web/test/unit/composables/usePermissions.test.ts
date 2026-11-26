import { afterEach, describe, expect, it, vi } from 'vitest'
import { AUTH_TOKEN_COOKIE } from '~/composables/useAuthToken'
import { usePermissions } from '~/composables/usePermissions'
import { useSession } from '~/composables/useSession'
import { createMockServices } from '~/services/mock/mockServices'
import { TEST_PASSWORD } from '~/services/mock/seed'
import { homePathFor, isFarmInScope } from '~/utils/permissions'
import { resolveAuthRedirect, resolvePermissionRedirect } from '~/utils/redirect'
import { installNuxtFakes } from '../../support/nuxtFakes'

afterEach(() => vi.unstubAllGlobals())

async function signIn(identifier: string) {
  installNuxtFakes(fakes => createMockServices({ latencyMs: 0, getAccessToken: () => fakes.cookie(AUTH_TOKEN_COOKIE).value }))
  await useSession().login({ identifier, password: TEST_PASSWORD, rememberMe: false })
  return usePermissions()
}

describe('usePermissions', () => {
  it('answers false for everything while signed out', () => {
    installNuxtFakes(createMockServices({ latencyMs: 0, getAccessToken: () => null }))
    const permissions = usePermissions()
    expect(permissions.can('dashboard.view')).toBe(false)
    expect(permissions.role.value).toBeNull()
    expect(permissions.homePath.value).toBe('/farms')
    expect(permissions.isFarmInScope('farm-santa-rita')).toBe(false)
  })

  it('reads the capabilities of an administrator from the session', async () => {
    const permissions = await signIn('marina@fazendasantarita.test')
    expect(permissions.can('users.manage')).toBe(true)
    expect(permissions.can('registry.write')).toBe(true)
    expect(permissions.can('users.manage_all')).toBe(false)
    expect(permissions.can('companies.manage')).toBe(false)
    expect(permissions.company.value).toEqual({ id: 'company-santa-rita', name: 'Fazenda Santa Rita' })
    expect(permissions.homePath.value).toBe('/')
  })

  it('limits an operator to planning and registering, inside the released farms', async () => {
    const permissions = await signIn('carlos.mendes@email.test')
    expect(permissions.permissions.value).toEqual(['applications.register', 'planner.use', 'field_conditions.record'])
    expect(permissions.can('registry.write')).toBe(false)
    expect(permissions.homePath.value).toBe('/farms')
    expect(permissions.isFarmInScope('farm-santa-rita')).toBe(true)
    expect(permissions.isFarmInScope('farm-recanto')).toBe(false)
  })
})

describe('permission helpers', () => {
  it('picks the home page by dashboard.view', () => {
    expect(homePathFor(['dashboard.view'])).toBe('/')
    expect(homePathFor(['planner.use'])).toBe('/farms')
  })

  it('checks the farm scope', () => {
    expect(isFarmInScope({ mode: 'all' }, 'any')).toBe(true)
    expect(isFarmInScope({ mode: 'selected', farmIds: ['a'] }, 'b')).toBe(false)
  })

  it('sends a signed-in user out of /login to their home page', () => {
    const login = { path: '/login', fullPath: '/login', isPublic: true, redirect: undefined, isAuthenticated: true }
    expect(resolveAuthRedirect({ ...login, homePath: '/' })).toEqual({ path: '/' })
    expect(resolveAuthRedirect(login)).toEqual({ path: '/farms' })
  })

  it('redirects denied pages, silently for the dashboard root', () => {
    const base = { granted: ['planner.use'], homePath: '/farms' }
    expect(resolvePermissionRedirect({ ...base, path: '/farms', required: undefined })).toBeNull()
    expect(resolvePermissionRedirect({ ...base, path: '/planner', required: 'planner.use' })).toBeNull()
    expect(resolvePermissionRedirect({ ...base, path: '/users', required: 'users.manage' })).toEqual({ path: '/farms', notify: true })
    expect(resolvePermissionRedirect({ ...base, path: '/', required: 'dashboard.view' })).toEqual({ path: '/farms', notify: false })
  })
})
