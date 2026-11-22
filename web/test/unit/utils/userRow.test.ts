import { describe, expect, it } from 'vitest'
import type { UserAccount } from '~/types/admin'
import {
  filterToUserFilters,
  formatLastAccess,
  toRoleCards,
  toUserRowView,
  userFilterCount,
  userRangeLabel,
  usersEyebrow,
} from '~/utils/userRow'

const NOW = new Date('2026-11-21T12:00:00Z') // 09:00 in São Paulo
const hoursAgo = (hours: number) => new Date(NOW.getTime() - hours * 3_600_000)
const SUMMARY = { totalUsers: 9, companyCount: 5, pendingInvitations: 2, byRole: { super_admin: 1, admin: 3, planning_operator: 5 } }

function account(overrides: Partial<UserAccount> = {}): UserAccount {
  return {
    id: 'user-ana',
    name: 'Ana Lúcia Prado',
    email: 'ana@email.test',
    role: 'planning_operator',
    company: { id: 'company-1', name: 'Sítio Boa Esperança' },
    farmScope: { mode: 'all' },
    status: 'active',
    lastAccessAt: null,
    invitation: null,
    ...overrides,
  }
}

describe('userRow utils', () => {
  it('formats the last access in São Paulo time', () => {
    expect(formatLastAccess(null, NOW)).toBe('—')
    expect(formatLastAccess(new Date(NOW.getTime() - 20_000), NOW)).toBe('agora')
    expect(formatLastAccess(new Date(NOW.getTime() - 12 * 60_000), NOW)).toBe('há 12 min')
    expect(formatLastAccess(hoursAgo(2), NOW)).toBe('há 2 h')
    expect(formatLastAccess(hoursAgo(6), NOW)).toBe('hoje, 03:00')
    // 10:00 UTC of the 21st is 07:00 local; 11:00 UTC of the 20th is "ontem".
    expect(formatLastAccess(hoursAgo(25), NOW)).toBe('ontem')
    // 01:00 UTC of the 21st is still the 20th in São Paulo (22:00).
    expect(formatLastAccess(hoursAgo(11), NOW)).toBe('ontem')
    expect(formatLastAccess(hoursAgo(120), NOW)).toBe('há 5 dias')
    expect(formatLastAccess(new Date('2026-09-01T15:00:00Z'), NOW)).toBe('01/09/2026')
  })

  it('builds the row with labels, status and the menu by status', () => {
    const active = toUserRowView(account({ lastAccessAt: hoursAgo(2) }), { now: NOW, selfId: null, highlightId: 'user-ana' })
    expect(active).toMatchObject({
      initials: 'AP',
      roleLabel: 'Operador de planejamento',
      roleTone: 'neutral',
      companyName: 'Sítio Boa Esperança',
      lastAccessLabel: 'há 2 h',
      status: { value: 'active', label: 'Ativo', tone: 'suitable' },
      highlighted: true,
    })
    expect(active.actions.map(item => item.kind)).toEqual(['edit_access', 'deactivate'])
    expect(active.actions[1]?.confirm?.title).toBe('Inativar Ana Lúcia Prado?')

    const invited = toUserRowView(account({ status: 'invited', invitation: { id: 'inv-1', expiresAt: NOW } }), { now: NOW, selfId: null, highlightId: null })
    expect(invited.status).toEqual({ value: 'invited', label: 'Convite pendente', tone: 'attention' })
    expect(invited.actions.map(item => [item.kind, item.danger])).toEqual([['resend_invitation', false], ['cancel_invitation', true]])

    const inactive = toUserRowView(account({ status: 'inactive' }), { now: NOW, selfId: null, highlightId: null })
    expect(inactive.actions.map(item => item.label)).toEqual(['Editar papel e acesso', 'Reativar'])

    const self = toUserRowView(account(), { now: NOW, selfId: 'user-ana', highlightId: null })
    expect(self.isSelf).toBe(true)
    expect(self.actions.find(item => item.kind === 'deactivate')?.disabledHint).toBe('Você não pode alterar a própria conta.')
  })

  it('builds role cards, eyebrow, filters and the range label', () => {
    expect(toRoleCards(['super_admin', 'admin'], SUMMARY).map(card => [card.label, card.tone, card.countLabel])).toEqual([
      ['Super admin', 'cafe', '1 usuário'],
      ['Administrador', 'green', '3 usuários'],
    ])
    expect(toRoleCards(['admin'], null)[0]?.countLabel).toBe('—')
    expect(usersEyebrow(SUMMARY, true, null)).toBe('Administração · 9 usuários em 5 empresas')
    expect(usersEyebrow({ ...SUMMARY, totalUsers: 4, companyCount: 1 }, false, 'Fazenda Santa Rita')).toBe('Administração · 4 usuários · Fazenda Santa Rita')
    expect(filterToUserFilters('invited')).toEqual({ status: 'invited' })
    expect(filterToUserFilters('admin')).toEqual({ role: 'admin' })
    expect(filterToUserFilters('all')).toEqual({})
    expect(userFilterCount('invited', SUMMARY)).toBe(2)
    expect(userFilterCount('admin', null)).toBeNull()
    expect(userRangeLabel(1, 10, 6, 9)).toBe('Mostrando 1–6 de 9')
    expect(userRangeLabel(2, 10, 3, 13)).toBe('Mostrando 11–13 de 13')
    expect(userRangeLabel(1, 10, 0, 0)).toBe('Mostrando 0 de 0')
  })
})
