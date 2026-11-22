import { afterEach, describe, expect, it, vi } from 'vitest'
import { shallowRef } from 'vue'
import { useFlash } from '~/composables/useFlash'
import { useUserAccessForm } from '~/composables/useUserAccessForm'
import { useUsers } from '~/composables/useUsers'
import type { UserAccount } from '~/types/admin'
import { flushPromises } from '../../support/nuxtFakes'
import { SEED_LOGINS, signInAs } from '../../support/session'

afterEach(() => vi.unstubAllGlobals())

async function accountOf(userId: string): Promise<UserAccount> {
  const list = useUsers({ pageSize: 50 })
  await flushPromises()
  const found = list.users.value.find(item => item.id === userId)
  if (!found) throw new Error(`seed user not listed: ${userId}`)
  return found
}

describe('useUserAccessForm (invite as super admin)', () => {
  it('offers every role and the client companies', async () => {
    await signInAs(SEED_LOGINS.superAdmin)
    const form = useUserAccessForm()
    await flushPromises()

    expect(form.mode.value).toBe('invite')
    expect(form.texts.value).toMatchObject({ title: 'Convidar usuário', submitLabel: 'Enviar convite' })
    expect(form.roleOptions.value.map(option => [option.title, option.icon])).toEqual([
      ['Super admin', 'shield'],
      ['Administrador', 'users'],
      ['Operador de planejamento', 'sprout'],
    ])
    expect(form.companyOptions.value[0]).toEqual({ value: 'company-agentes-agro', label: 'Agentes Agro' })
    expect(form.companyOptions.value.map(option => option.label)).toContain('Sítio Boa Esperança')
    expect(form.companyLocked.value).toBe(false)
  })

  it('validates locally before calling the API', async () => {
    await signInAs(SEED_LOGINS.superAdmin)
    const form = useUserAccessForm()
    await flushPromises()
    Object.assign(form.values, { name: ' ', email: 'ana@', companyId: null })

    expect(await form.submit()).toBe(false)
    expect(form.fieldErrors.value).toEqual({
      name: 'Informe o nome.',
      email: 'Informe um e-mail válido.',
      role: 'Escolha o papel.',
      companyId: 'Escolha a empresa.',
    })
  })

  it('scopes an operator to farms of the chosen company and sends the invite', async () => {
    await signInAs(SEED_LOGINS.superAdmin)
    const form = useUserAccessForm()
    await flushPromises()

    Object.assign(form.values, { name: 'Lia Campos', email: 'lia@email.test', role: 'planning_operator' })
    await flushPromises()
    expect(form.scopeLocked.value).toBe(false)
    form.values.companyId = 'company-santa-rita'
    await flushPromises()
    expect(form.farmOptions.value).toEqual([{ value: 'farm-santa-rita', label: 'Fazenda Santa Rita' }])
    form.values.farmScopeMode = 'selected'
    await flushPromises()

    expect(await form.submit()).toBe(false)
    expect(form.fieldErrors.value.farmIds).toBe('Selecione ao menos uma fazenda.')

    form.values.farmIds = ['farm-santa-rita']
    expect(await form.submit()).toBe(true)
    expect(useFlash().consume()).toMatchObject({
      title: 'Convite enviado',
      description: 'Lia Campos vai receber um e-mail em lia@email.test para criar a senha.',
    })
    const created = useUsers({ search: 'lia', pageSize: 10 })
    await flushPromises()
    expect(created.users.value[0]).toMatchObject({ status: 'invited', farmScope: { mode: 'selected', farmIds: ['farm-santa-rita'] } })
  })

  it('locks the company of a new super admin and resets the farm scope', async () => {
    await signInAs(SEED_LOGINS.superAdmin)
    const form = useUserAccessForm()
    await flushPromises()
    Object.assign(form.values, { role: 'planning_operator', companyId: 'company-santa-rita', farmScopeMode: 'selected' })
    await flushPromises()
    form.values.farmIds = ['farm-santa-rita']
    form.values.role = 'super_admin'
    await flushPromises()

    expect(form.values.companyId).toBe('company-agentes-agro')
    expect(form.companyLocked.value).toBe(true)
    expect(form.scopeLocked.value).toBe(true)
    expect(form.values.farmScopeMode).toBe('all')
    expect(form.values.farmIds).toEqual([])
  })

  it('shows a taken e-mail on the field', async () => {
    await signInAs(SEED_LOGINS.superAdmin)
    const form = useUserAccessForm()
    await flushPromises()
    Object.assign(form.values, { name: 'Outra Marina', email: 'marina@fazendasantarita.test', role: 'admin', companyId: 'company-santa-rita' })

    expect(await form.submit()).toBe(false)
    expect(form.fieldErrors.value.email).toBe('Este e-mail já tem cadastro.')
    expect(form.formError.value).toBeNull()
  })
})

describe('useUserAccessForm (administrator)', () => {
  it('does not offer super admin nor other companies', async () => {
    await signInAs(SEED_LOGINS.admin)
    const form = useUserAccessForm()
    await flushPromises()

    expect(form.roleOptions.value.map(option => option.value)).toEqual(['admin', 'planning_operator'])
    expect(form.companyOptions.value).toEqual([{ value: 'company-santa-rita', label: 'Fazenda Santa Rita' }])
    expect(form.companyLocked.value).toBe(true)
    expect(form.values.companyId).toBe('company-santa-rita')
  })

  it('reports a 403 honestly when the API refuses', async () => {
    await signInAs(SEED_LOGINS.admin)
    const form = useUserAccessForm()
    await flushPromises()
    Object.assign(form.values, { name: 'Intrusa', email: 'intrusa@email.test', role: 'admin', companyId: 'company-ouro-verde' })

    expect(await form.submit()).toBe(false)
    expect(form.formError.value).toBe('Você não tem permissão para dar esse papel ou acesso a essa empresa.')
  })
})

describe('useUserAccessForm (edit access)', () => {
  it('fills the form from the account and sends only what changed', async () => {
    await signInAs(SEED_LOGINS.superAdmin)
    const carlos = await accountOf('user-carlos')
    const user = shallowRef<UserAccount | null>(carlos)
    const form = useUserAccessForm({ user })
    await flushPromises()

    expect(form.mode.value).toBe('edit')
    expect(form.identityReadonly.value).toBe(true)
    expect(form.texts.value).toMatchObject({ title: 'Editar acesso', submitLabel: 'Salvar acesso' })
    expect(form.values).toMatchObject({ role: 'planning_operator', companyId: 'company-santa-rita', farmScopeMode: 'selected', farmIds: ['farm-santa-rita'] })

    form.values.role = 'admin'
    expect(await form.submit()).toBe(true)
    expect(useFlash().consume()).toMatchObject({ title: 'Acesso atualizado', highlightId: 'user-carlos' })
    expect((await accountOf('user-carlos'))).toMatchObject({ role: 'admin', farmScope: { mode: 'all' } })
  })

  it('locks the own role', async () => {
    await signInAs(SEED_LOGINS.superAdmin)
    const self = await accountOf('user-bruno')
    const form = useUserAccessForm({ user: self })
    await flushPromises()
    expect(form.roleOptions.value.filter(option => option.unavailableHint).map(option => option.value)).toEqual(['admin', 'planning_operator'])
  })
})
