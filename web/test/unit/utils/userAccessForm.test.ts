import { describe, expect, it } from 'vitest'
import type { UserAccount } from '~/types/admin'
import { toFarmScope, toUserUpdateInput, userAccessErrorsFromApi, userRoleOptions } from '~/utils/userAccessForm'

const carlos: UserAccount = {
  id: 'user-carlos',
  name: 'Carlos Mendes',
  email: 'carlos@email.test',
  role: 'planning_operator',
  company: { id: 'company-1', name: 'Fazenda Santa Rita' },
  farmScope: { mode: 'selected', farmIds: ['farm-a', 'farm-b'] },
  status: 'active',
  lastAccessAt: null,
  invitation: null,
}

describe('userAccessForm utils', () => {
  it('offers super admin only to who manages every company', () => {
    expect(userRoleOptions(true).map(option => option.value)).toEqual(['super_admin', 'admin', 'planning_operator'])
    expect(userRoleOptions(false).map(option => option.value)).toEqual(['admin', 'planning_operator'])
  })

  it('keeps a farm scope only for the planning operator', () => {
    expect(toFarmScope({ role: 'admin', farmScopeMode: 'selected', farmIds: ['farm-a'] })).toEqual({ mode: 'all' })
    expect(toFarmScope({ role: 'planning_operator', farmScopeMode: 'selected', farmIds: ['farm-a'] })).toEqual({ mode: 'selected', farmIds: ['farm-a'] })
  })

  it('sends only the changed fields, ignoring the order of the farms', () => {
    const same = { name: carlos.name, email: carlos.email, role: carlos.role, companyId: 'company-1', farmScopeMode: 'selected' as const, farmIds: ['farm-b', 'farm-a'] }
    expect(toUserUpdateInput(carlos, same)).toEqual({})
    expect(toUserUpdateInput(carlos, { ...same, role: 'admin' })).toEqual({ role: 'admin', farmScope: { mode: 'all' } })
    expect(toUserUpdateInput(carlos, { ...same, companyId: 'company-2', farmIds: ['farm-c'] }))
      .toEqual({ companyId: 'company-2', farmScope: { mode: 'selected', farmIds: ['farm-c'] } })
  })

  it('maps API details to the form fields', () => {
    expect(userAccessErrorsFromApi([
      { field: 'company_id', message: 'empresa não encontrada' },
      { field: 'farm_scope.farm_ids', message: 'fazenda não pertence à empresa' },
      { field: 'other', message: 'x' },
    ])).toEqual({ companyId: 'empresa não encontrada', farmIds: 'fazenda não pertence à empresa' })
  })
})
