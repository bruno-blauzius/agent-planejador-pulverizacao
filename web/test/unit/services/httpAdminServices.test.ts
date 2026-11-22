import { describe, expect, it } from 'vitest'
import { createHttpClient, type RawFetchOptions } from '~/services/http/client'
import { createHttpServices } from '~/services/http/services'
import type { DashboardDto, UserAccountDto } from '~/types/dto'

interface RecordedCall {
  path: string
  options: RawFetchOptions
}

function servicesWith(answer: unknown) {
  const calls: RecordedCall[] = []
  const http = createHttpClient({ baseURL: 'http://api.test', getAccessToken: () => 't' }, async (path, options) => {
    calls.push({ path, options })
    if (answer instanceof Error) throw answer
    return answer
  })
  return { api: createHttpServices(http), calls }
}

const invitedDto: UserAccountDto = {
  id: 'u-9',
  name: 'Ana Lúcia Prado',
  email: 'ana.prado@email.test',
  role: 'planning_operator',
  company: { id: 'c-2', name: 'Sítio Boa Esperança' },
  farm_scope: { mode: 'all' },
  status: 'invited',
  last_access_at: null,
  invitation: { id: 'inv-1', expires_at: '2026-11-28T12:00:00Z' },
}

describe('http user service', () => {
  it('lists with snake_case filters and maps invitations', async () => {
    const { api, calls } = servicesWith({ items: [invitedDto], total: 1, page: 1, page_size: 10 })
    const page = await api.users.list({ search: 'ana', role: 'planning_operator', status: 'invited', companyId: 'c-2', pageSize: 10 })

    expect(calls[0]).toMatchObject({
      path: '/users',
      options: { query: { search: 'ana', role: 'planning_operator', status: 'invited', company_id: 'c-2', page_size: 10 } },
    })
    expect(page.items[0]).toMatchObject({
      farmScope: { mode: 'all' },
      lastAccessAt: null,
      invitation: { id: 'inv-1', expiresAt: new Date('2026-11-28T12:00:00Z') },
    })
  })

  it('sends only the changed keys in PATCH /users/{id}', async () => {
    const { api, calls } = servicesWith({ ...invitedDto, status: 'active', invitation: null })
    await api.users.update('u-9', { farmScope: { mode: 'selected', farmIds: ['f-1'] } })

    expect(calls[0]).toMatchObject({ path: '/users/u-9', options: { method: 'PATCH', body: { farm_scope: { mode: 'selected', farm_ids: ['f-1'] } } } })
    expect(Object.keys(calls[0]!.options.body ?? {})).toEqual(['farm_scope'])
  })

  it('posts and deletes invitations', async () => {
    const { api, calls } = servicesWith(null)
    await api.users.resendInvitation('inv 1')
    await api.users.cancelInvitation('inv-1')
    expect(calls.map(call => [call.options.method, call.path])).toEqual([
      ['POST', '/invitations/inv%201/resend'],
      ['DELETE', '/invitations/inv-1'],
    ])
  })
})

describe('http profile and dashboard services', () => {
  it('changes the password with snake_case keys', async () => {
    const { api, calls } = servicesWith(null)
    await api.profile.changePassword({ currentPassword: 'a', newPassword: 'b' })
    expect(calls[0]).toMatchObject({ path: '/me/password', options: { method: 'POST', body: { current_password: 'a', new_password: 'b' } } })
  })

  it('maps the dashboard', async () => {
    const dto: DashboardDto = {
      generated_at: '2026-11-21T12:00:00Z',
      farm_id: null,
      farm_count: 4,
      indicators: {
        suitable_windows_today: { count: 3, field_labels: ['T3', 'T5', 'T7'], next_start_at: '2026-11-21T09:00:00Z' },
        applications_this_month: { count: 12, confirmed_this_week: 4 },
        sprayed_area: { area_ha: 412, productive_area_ha: 982 },
        pending_items: { count: 2, headline: '1 carência vence amanhã' },
      },
      windows_vs_applications: { days: [{ date: '2026-11-08', suitable_hours: 6, applications: 2 }], summary: 'texto' },
      upcoming_windows: [{ field_id: 'f', field_label: 'T3', farm_id: 'farm', date: '2026-11-21', start_time: '06:00', end_time: '09:30', crop_variety: 'Catuaí 144', operation: 'fungicida', condition: 'suitable' }],
      agent_alerts: [{ id: 'a', kind: 'rain', tone: 'highlight', title: 'Chuva', description: 'd' }],
      recent_activity: [{ id: 'r', actor: { kind: 'agent', name: 'Agente' }, description: 'sugeriu', occurred_at: '2026-11-20T14:58:00Z' }],
    }
    const { api, calls } = servicesWith(dto)
    const dashboard = await api.dashboard.get({ farmId: 'farm-1', periodDays: 14 })

    expect(calls[0]?.options.query).toEqual({ farm_id: 'farm-1', period_days: 14 })
    expect(dashboard).toMatchObject({
      farmCount: 4,
      indicators: { suitableWindowsToday: { fieldLabels: ['T3', 'T5', 'T7'] }, sprayedArea: { areaHa: 412, productiveAreaHa: 982 } },
      windowsVsApplications: { days: [{ suitableHours: 6, applications: 2 }] },
      upcomingWindows: [{ fieldLabel: 'T3', startTime: '06:00', cropVariety: 'Catuaí 144' }],
      recentActivity: [{ occurredAt: new Date('2026-11-20T14:58:00Z') }],
    })
  })
})
