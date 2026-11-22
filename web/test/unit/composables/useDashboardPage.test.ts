import { afterEach, describe, expect, it, vi } from 'vitest'
import { AUTH_TOKEN_COOKIE } from '~/composables/useAuthToken'
import { useDashboardPage } from '~/composables/useDashboardPage'
import { useSession } from '~/composables/useSession'
import { createTestApi } from '../../support/mockApi'
import { flushPromises, installNuxtFakes, type NuxtFakes } from '../../support/nuxtFakes'

afterEach(() => vi.unstubAllGlobals())

const NOW = () => new Date('2026-11-21T12:00:00Z')

async function setup(token = 'mock-token:user-marina', query: Record<string, unknown> = {}) {
  const { api } = createTestApi(token)
  const fakes: NuxtFakes = installNuxtFakes(api)
  fakes.cookie(AUTH_TOKEN_COOKIE).value = token
  fakes.route.path = '/'
  fakes.route.query = query
  await useSession().fetchCurrentUser()
  const page = useDashboardPage({ now: NOW })
  await flushPromises()
  return { page, fakes }
}

describe('useDashboardPage', () => {
  it('renders header, selector and the 4 indicators', async () => {
    const { page } = await setup()

    expect(page.greeting.value).toBe('Bom dia, Marina')
    expect(page.dateLine.value).toBe('Sábado, 21 de novembro · Fazenda Santa Rita')
    expect(page.farmOptions.value[0]?.label).toMatch(/^Todas \(\d+\)$/)
    expect(page.canOpenPlanner.value).toBe(true)
    expect(page.statCards.value.map(card => [card.key, card.label, card.highlight])).toEqual([
      ['windows', 'Janelas aptas hoje', true],
      ['applications', 'Aplicações no mês', false],
      ['area', 'Área pulverizada', false],
      ['pending', 'Pendências', false],
    ])
    expect(page.statCards.value[3]).toMatchObject({ value: '2', caption: '1 carência vence amanhã' })
  })

  it('builds the chart, upcoming windows, alerts and activity', async () => {
    const { page } = await setup()

    expect(page.chart.value.title).toBe('Janelas aptas × aplicações · últimos 14 dias')
    expect(page.chart.value.days).toHaveLength(14)
    expect(page.chart.value.days.filter(day => day.showLabel).map(day => day.label)).toEqual(['8/11', '14/11', '21/11'])
    expect(page.upcomingWindows.value[0]?.whenLabel).toMatch(/^Hoje · /)
    expect(page.alerts.value.map(alert => alert.icon)).toEqual(['drop', 'rain', 'info'])
    expect(page.activity.value[0]).toMatchObject({ initials: 'CM', timeLabel: 'hoje, 07:00' })
    expect(page.activity.value[3]).toMatchObject({ initials: 'IA', isAgent: true })
  })

  it('dismisses an alert for this session', async () => {
    const { page } = await setup()
    page.dismissAlert('alert-rain')
    expect(page.alerts.value.map(alert => alert.id)).toEqual(['alert-phi', 'alert-forecast'])
  })

  it('filters by farm through the query', async () => {
    const { page, fakes } = await setup()
    page.selectedFarmId.value = 'farm-santa-rita'
    await flushPromises()

    expect(fakes.route.query).toEqual({ farm: 'farm-santa-rita' })
    expect(page.selectedFarmId.value).toBe('farm-santa-rita')
    expect(page.dateLine.value).toMatch(/· Fazenda Santa Rita$/)

    page.selectedFarmId.value = null
    expect(fakes.route.query).toEqual({})
  })

  it('reports a farm outside the scope (other company: 404, out of scope: 403)', async () => {
    const { page } = await setup('mock-token:user-marina', { farm: 'farm-ouro-verde' })
    expect(page.isOutOfScope.value).toBe(true)
    expect(page.statCards.value.every(card => card.value === '—')).toBe(true)
  })

  it('shows the forbidden state without dashboard.view', async () => {
    const { page } = await setup('mock-token:user-carlos')
    expect(page.isForbidden.value).toBe(true)
  })
})
