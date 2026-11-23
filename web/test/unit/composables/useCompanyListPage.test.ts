import { afterEach, describe, expect, it, vi } from 'vitest'
import { useCompanyListPage } from '~/composables/useCompanyListPage'
import { AUTH_TOKEN_COOKIE } from '~/composables/useAuthToken'
import { useSession } from '~/composables/useSession'
import { createTestApi } from '../../support/mockApi'
import { flushPromises, installNuxtFakes, type NuxtFakes } from '../../support/nuxtFakes'

afterEach(() => vi.unstubAllGlobals())

const NOW = () => new Date('2026-11-21T12:00:00Z')

async function setup(token = 'mock-token:user-bruno', query: Record<string, unknown> = {}) {
  const { api } = createTestApi(token)
  const fakes: NuxtFakes = installNuxtFakes(api)
  fakes.cookie(AUTH_TOKEN_COOKIE).value = token
  fakes.route.path = '/companies'
  fakes.route.query = query
  await useSession().fetchCurrentUser()
  const page = useCompanyListPage({ debounceMs: 0, now: NOW })
  await flushPromises()
  return { page, fakes }
}

describe('useCompanyListPage', () => {
  it('builds the 4 summary cards', async () => {
    const { page } = await setup()
    expect(page.statCards.value.map(card => [card.label, card.value, card.caption, card.highlight])).toEqual([
      ['Empresas ativas', '4', '1 em avaliação', false],
      ['Fazendas no sistema', expect.any(String), expect.stringMatching(/ha monitorados$/), false],
      ['Usuários', '9', '2 convites pendentes', false],
      ['Aplicações no mês', '31', '+19% vs. outubro', true],
    ])
  })

  it('lists client companies with ready-to-render rows', async () => {
    const { page } = await setup()
    expect(page.rows.value.map(row => row.name)).toEqual([
      'Fazenda Santa Rita', 'Fazenda Ouro Verde', 'Sítio Boa Esperança', 'Fazenda Recanto',
    ])
    expect(page.rows.value[0]).toMatchObject({
      initials: 'SR',
      locationLabel: 'Guaxupé · MG',
      documentLabel: '12.345.678/0001-90',
      plan: { value: 'pro', label: 'Pro' },
      status: { label: 'Ativa', tone: 'success' },
    })
    expect(page.rows.value[3]?.status).toEqual({ value: 'trial', label: 'Em avaliação · 9 dias', tone: 'warning' })
    expect(page.pagination.value).toEqual({ rangeLabel: 'Mostrando 1–4 de 4', page: 1, pageCount: 1, canPrevious: false, canNext: false })
    expect(page.canCreate.value).toBe(true)
  })

  it('searches through the query and tells an empty search apart', async () => {
    const { page, fakes } = await setup()
    page.search.value = 'zzz'
    await flushPromises()

    expect(fakes.route.query).toEqual({ q: 'zzz' })
    expect(page.isEmptySearch.value).toBe(true)
    expect(page.isEmpty.value).toBe(false)

    const before = fakes.navigations.length
    page.clearSearch()
    await flushPromises()
    expect(page.rows.value).toHaveLength(4)
    expect(fakes.route.query).toEqual({})
    // A single query replace, not one from the watcher plus one from clearSearch.
    expect(fakes.navigations.length - before).toBe(1)
  })

  it('clears the search at once even with a debounce', async () => {
    const { api } = createTestApi('mock-token:user-bruno')
    const fakes = installNuxtFakes(api)
    fakes.cookie(AUTH_TOKEN_COOKIE).value = 'mock-token:user-bruno'
    fakes.route.path = '/companies'
    fakes.route.query = { q: 'santa' }
    await useSession().fetchCurrentUser()
    const page = useCompanyListPage({ debounceMs: 300, now: NOW })
    await flushPromises()

    page.clearSearch()
    await flushPromises()
    expect(fakes.route.query).toEqual({})
  })

  it('changes the page size and the page through the query', async () => {
    const { page, fakes } = await setup()
    page.pageSize.value = 20
    expect(fakes.route.query).toEqual({ size: '20' })
    expect(page.pageSizeOptions.map(option => option.label)).toEqual(['10 por página', '20 por página', '50 por página'])

    page.page.value = 2
    await flushPromises()
    // Only one page exists: the page goes back to the last valid one.
    expect(fakes.route.query.page).toBeUndefined()
  })

  it('shows the forbidden state for an administrator', async () => {
    const { page } = await setup('mock-token:user-marina')
    expect(page.isForbidden.value).toBe(true)
    expect(page.canCreate.value).toBe(false)
    expect(page.statCards.value.every(card => card.value === '—')).toBe(true)
  })
})
