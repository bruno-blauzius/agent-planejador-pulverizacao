import { afterEach, describe, expect, it, vi } from 'vitest'
import { useApiServices } from '~/composables/useApiServices'
import { useCompanyListPage } from '~/composables/useCompanyListPage'
import { useFlash } from '~/composables/useFlash'
import { flushPromises } from '../../support/nuxtFakes'
import { SEED_LOGINS, SEED_NOW, signInAs } from '../../support/session'

afterEach(() => vi.unstubAllGlobals())

async function setup(login: string = SEED_LOGINS.superAdmin) {
  const fakes = await signInAs(login)
  fakes.route.path = '/companies'
  const page = useCompanyListPage({ debounceMs: 0, now: () => SEED_NOW })
  await flushPromises()
  return { page, fakes }
}

const rowOf = (page: Awaited<ReturnType<typeof setup>>['page'], name: string) => {
  const row = page.rows.value.find(item => item.name === name)
  if (!row) throw new Error(`row not found: ${name}`)
  return row
}

describe('useCompanyListPage row actions', () => {
  it('lists the menu entries per status and none without the capability', async () => {
    const { page } = await setup()
    expect(rowOf(page, 'Fazenda Santa Rita').actions.map(action => action.kind)).toEqual(['edit', 'deactivate'])

    const operator = await setup(SEED_LOGINS.operator)
    expect(operator.page.canCreate.value).toBe(false)
    expect(operator.page.modal.value).toBeNull()
  })

  it('deactivates after confirmation, then reactivates and deletes only when inactive', async () => {
    const { page } = await setup()
    const row = rowOf(page, 'Sítio Boa Esperança')

    await page.runAction(row, 'deactivate')
    expect(page.confirmation.value).toMatchObject({ kind: 'deactivate', companyName: 'Sítio Boa Esperança', pending: false, error: null })
    // Delete is not offered while active.
    await page.runAction(row, 'delete')
    expect(page.confirmation.value?.kind).toBe('deactivate')

    await page.confirmAction()
    await flushPromises()
    expect(page.confirmation.value).toBeNull()
    expect(page.notice.value).toMatchObject({ kind: 'success', title: 'Sítio Boa Esperança inativada' })
    expect(page.actionPending.value).toBeNull()
    const inactive = rowOf(page, 'Sítio Boa Esperança')
    expect(inactive.status.value).toBe('inactive')
    expect(inactive.highlighted).toBe(true)
    expect(inactive.actions.map(action => action.kind)).toEqual(['edit', 'reactivate', 'delete'])
    // The summary was refreshed too: one active company less.
    expect(page.statCards.value[0]?.value).toBe('3')

    await page.runAction(inactive, 'reactivate')
    await flushPromises()
    expect(page.notice.value).toMatchObject({ kind: 'success', title: 'Sítio Boa Esperança reativada' })
    expect(rowOf(page, 'Sítio Boa Esperança').status.value).toBe('active')
  })

  it('keeps the dialog open with a clear message on 409 company_has_active_users', async () => {
    const { page } = await setup()
    await page.runAction(rowOf(page, 'Fazenda Santa Rita'), 'deactivate')
    await page.confirmAction()
    await flushPromises()

    await page.runAction(rowOf(page, 'Fazenda Santa Rita'), 'delete')
    await page.confirmAction()
    await flushPromises()

    expect(page.confirmation.value?.error).toMatch(/usuários ativos ou convidados/)
    expect(page.notice.value).toBeNull()
    expect(page.rows.value.some(row => row.name === 'Fazenda Santa Rita')).toBe(true)

    page.cancelConfirmation()
    expect(page.confirmation.value).toBeNull()
  })

  it('deletes an inactive company without users and refreshes the list and summary', async () => {
    const { page } = await setup()
    const created = await useApiServices().companies.create({
      name: 'Vazia', document: '33444555000166', city: 'Alfenas', state: 'MG', plan: 'basic',
    })
    await page.refresh()
    await flushPromises()
    expect(page.rows.value).toHaveLength(5)

    await page.runAction(rowOf(page, 'Vazia'), 'deactivate')
    await page.confirmAction()
    await flushPromises()
    await page.runAction(rowOf(page, 'Vazia'), 'delete')
    await page.confirmAction()
    await flushPromises()

    expect(page.notice.value).toMatchObject({ kind: 'success', title: 'Vazia excluída' })
    expect(page.rows.value.some(row => row.id === created.id)).toBe(false)
    expect(page.rows.value.every(row => !row.highlighted)).toBe(true)
  })

  it('reports a company that vanished (404) and refreshes', async () => {
    const { page } = await setup()
    const { companies } = useApiServices()
    const gone = await companies.create({ name: 'Efêmera', document: '33444555000166', city: 'Alfenas', state: 'MG', plan: 'basic' })
    await page.refresh()
    await flushPromises()
    const row = rowOf(page, 'Efêmera')
    await companies.update(gone.id, { status: 'inactive' })
    await companies.remove(gone.id)

    await page.runAction({ id: row.id }, 'deactivate')
    await page.confirmAction()
    await flushPromises()

    expect(page.notice.value).toMatchObject({ kind: 'error', description: 'A empresa não existe mais.' })
    expect(page.rows.value.some(item => item.id === row.id)).toBe(false)
  })

  it('ignores a second action while one is in flight', async () => {
    const { page } = await setup()
    await page.runAction(rowOf(page, 'Fazenda Santa Rita'), 'deactivate')
    const first = page.confirmAction()
    expect(page.actionPending.value).not.toBeNull()
    expect(page.confirmation.value?.pending).toBe(true)
    page.cancelConfirmation()
    expect(page.confirmation.value).not.toBeNull()
    await first
  })
})

describe('useCompanyListPage modal and notices', () => {
  it('opens the create and edit modals through the query', async () => {
    const { page, fakes } = await setup()
    page.openCreate()
    await flushPromises()
    expect(fakes.route.query).toEqual({ new: '1' })
    expect(page.modal.value).toEqual({ mode: 'create' })

    await page.runAction(rowOf(page, 'Fazenda Santa Rita'), 'edit')
    await flushPromises()
    expect(fakes.route.query).toEqual({ company: expect.any(String) })
    expect(page.modal.value).toMatchObject({ mode: 'edit', company: { name: 'Fazenda Santa Rita' }, loading: false })

    page.closeModal()
    await flushPromises()
    expect(fakes.route.query).toEqual({})
    expect(page.modal.value).toBeNull()
  })

  it('announces and highlights a saved company, closing the modal', async () => {
    const { page, fakes } = await setup()
    page.openCreate()
    await flushPromises()
    const company = await useApiServices().companies.create({
      name: 'Nova', document: '33444555000166', city: 'Alfenas', state: 'MG', plan: 'trial',
    })

    page.handleSaved({ mode: 'create', company })
    await flushPromises()
    expect(fakes.route.query).toEqual({})
    expect(page.notice.value).toMatchObject({ kind: 'success', title: 'Nova cadastrada', description: 'A empresa começou em avaliação.' })
    expect(rowOf(page, 'Nova').highlighted).toBe(true)
    expect(page.rows.value).toHaveLength(5)

    page.dismissNotice()
    expect(page.notice.value).toBeNull()

    page.handleSaved({ mode: 'edit', company, changed: false })
    expect(page.notice.value).toBeNull()
  })

  it('takes a flash message left by another screen once', async () => {
    const { page } = await setup()
    useFlash().show({ kind: 'success', title: 'Feito', highlightId: 'x' })
    expect(page.takeFlash()).toMatchObject({ title: 'Feito' })
    expect(page.notice.value).toMatchObject({ title: 'Feito' })
    expect(page.takeFlash()).toBeNull()
  })
})
