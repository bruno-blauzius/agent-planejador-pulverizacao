import { afterEach, describe, expect, it, vi } from 'vitest'
import { AUTH_TOKEN_COOKIE } from '~/composables/useAuthToken'
import { useCompanyListPage } from '~/composables/useCompanyListPage'
import { useSession } from '~/composables/useSession'
import { useUserListPage } from '~/composables/useUserListPage'
import { ApiError } from '~/services/errors'
import { createMockServices } from '~/services/mock/mockServices'
import { TEST_PASSWORD } from '~/services/mock/seed'
import type { ApiServices } from '~/services/ports'
import { flushPromises, installNuxtFakes } from '../../support/nuxtFakes'
import { SEED_LOGINS, SEED_NOW } from '../../support/session'

afterEach(() => vi.unstubAllGlobals())

type Overrides = (api: ApiServices) => ApiServices

/** Signed-in super admin whose services can be wrapped (to count or fail `get`). */
async function signIn(route: { path: string, query: Record<string, unknown> }, override: Overrides = api => api) {
  const fakes = installNuxtFakes(nuxt => override(createMockServices({
    latencyMs: 0,
    getAccessToken: () => nuxt.cookie(AUTH_TOKEN_COOKIE).value,
    now: () => SEED_NOW,
  })))
  await useSession().login({ identifier: SEED_LOGINS.superAdmin, password: TEST_PASSWORD, rememberMe: false })
  fakes.route.path = route.path
  fakes.route.query = route.query
  return fakes
}

const serverError = () => new ApiError({ code: 'internal_error', message: 'boom', status: 500 })

describe('?company= deep link outside the listed page', () => {
  const query = { q: 'Recanto', company: 'company-ouro-verde' }
  const options = { debounceMs: 0, now: () => SEED_NOW }

  it('fetches the company by id, loading until it arrives', async () => {
    await signIn({ path: '/companies', query })
    const page = useCompanyListPage(options)
    expect(page.modal.value).toMatchObject({ mode: 'edit', company: null, loading: true, loadError: null })
    await flushPromises()
    expect(page.rows.value.map(row => row.name)).toEqual(['Fazenda Recanto'])
    expect(page.modal.value).toMatchObject({
      mode: 'edit', company: { id: 'company-ouro-verde' }, loading: false, loadError: null,
    })
  })

  it('does not fetch when the company is already listed', async () => {
    const get = vi.fn()
    await signIn({ path: '/companies', query: { company: 'company-recanto' } }, (api) => {
      get.mockImplementation((id: string) => api.companies.get(id))
      return { ...api, companies: { ...api.companies, get } }
    })
    const page = useCompanyListPage(options)
    await flushPromises()
    expect(page.modal.value).toMatchObject({ company: { id: 'company-recanto' }, loading: false })
    expect(get).not.toHaveBeenCalled()
  })

  it('treats 404 as not found (no error)', async () => {
    await signIn({ path: '/companies', query: { company: 'company-inexistente' } })
    const page = useCompanyListPage(options)
    await flushPromises()
    expect(page.modal.value).toEqual({
      mode: 'edit', companyId: 'company-inexistente', company: null, loading: false, loadError: null,
    })
  })

  it('reports other failures as a load error, not as not found', async () => {
    await signIn({ path: '/companies', query }, api => ({
      ...api,
      companies: { ...api.companies, get: () => Promise.reject(serverError()) },
    }))
    const page = useCompanyListPage(options)
    await flushPromises()
    expect(page.modal.value).toMatchObject({ mode: 'edit', company: null, loading: false })
    expect(page.modal.value).toMatchObject({ loadError: { code: 'internal_error' } })
  })

  it('fetches once per link and again after the link changes', async () => {
    const get = vi.fn()
    const fakes = await signIn({ path: '/companies', query }, (api) => {
      get.mockImplementation((id: string) => api.companies.get(id))
      return { ...api, companies: { ...api.companies, get } }
    })
    const page = useCompanyListPage(options)
    await flushPromises()
    expect(page.modal.value).toMatchObject({ company: { id: 'company-ouro-verde' } })
    expect(get).toHaveBeenCalledTimes(1)

    fakes.route.query = { q: 'Recanto', company: 'company-santa-rita' }
    await flushPromises()
    expect(page.modal.value).toMatchObject({ company: { id: 'company-santa-rita' }, loading: false })
    expect(get).toHaveBeenCalledTimes(2)
  })
})

describe('?user= deep link outside the listed page', () => {
  const options = { now: () => SEED_NOW }

  it('fetches the user by id when the filter hides them', async () => {
    await signIn({ path: '/users', query: { q: 'Marina', user: 'user-carlos' } })
    const page = useUserListPage(options)
    await flushPromises()
    expect(page.rows.value.map(row => row.name)).toEqual(['Marina Rezende'])
    expect(page.modal.value).toMatchObject({ mode: 'edit', user: { id: 'user-carlos' }, loading: false, loadError: null })
  })

  it('treats 404 as not found and other failures as a load error', async () => {
    const fakes = await signIn({ path: '/users', query: { user: 'user-inexistente' } }, api => ({
      ...api,
      users: {
        ...api.users,
        get: (id: string) => id === 'user-quebrado' ? Promise.reject(serverError()) : api.users.get(id),
      },
    }))
    const page = useUserListPage(options)
    await flushPromises()
    expect(page.modal.value).toEqual({ mode: 'edit', user: null, loading: false, loadError: null })

    fakes.route.query = { user: 'user-quebrado' }
    await flushPromises()
    expect(page.modal.value).toMatchObject({ user: null, loading: false, loadError: { code: 'internal_error' } })
  })
})
