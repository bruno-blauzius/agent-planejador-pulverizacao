import { afterEach, describe, expect, it, vi } from 'vitest'
import { useFlash } from '~/composables/useFlash'
import { useUserListPage } from '~/composables/useUserListPage'
import { flushPromises } from '../../support/nuxtFakes'
import { SEED_LOGINS, SEED_NOW, signInAs } from '../../support/session'

afterEach(() => vi.unstubAllGlobals())

const now = () => SEED_NOW

describe('useUserListPage (super admin)', () => {
  it('shows every company with role cards, chips and rows ready to render', async () => {
    const fakes = await signInAs(SEED_LOGINS.superAdmin)
    fakes.route.path = '/users'
    const page = useUserListPage({ now })
    expect(page.roleCards.value.map(card => card.countLabel)).toEqual(['—', '—', '—'])
    await flushPromises()

    expect(page.eyebrow.value).toBe('Administração · 9 usuários em 5 empresas')
    expect(page.roleCards.value.map(card => [card.label, card.countLabel])).toEqual([
      ['Super admin', '1 usuário'],
      ['Administrador', '3 usuários'],
      ['Operador de planejamento', '5 usuários'],
    ])
    expect(page.filterChips.value.map(chip => `${chip.label}:${chip.count}`)).toEqual([
      'Todos:9', 'Super admin:1', 'Administrador:3', 'Operador:5', 'Convites pendentes:2',
    ])
    expect(page.rows.value.slice(0, 6).map(row => [row.name, row.lastAccessLabel, row.status.label])).toEqual([
      ['Bruno Blauzius', 'agora', 'Ativo'],
      ['Marina Rezende', 'há 2 h', 'Ativo'],
      ['Carlos Mendes', 'hoje, 03:00', 'Ativo'],
      ['João Silveira', 'ontem', 'Ativo'],
      ['Ana Lúcia Prado', '—', 'Convite pendente'],
      ['Helio Nascimento', 'há 5 dias', 'Inativo'],
    ])
    expect(page.rows.value[0]?.isSelf).toBe(true)
    expect(page.pagination.value).toEqual({ rangeLabel: 'Mostrando 1–9 de 9', page: 1, pageCount: 1 })
    expect(page.canInvite.value).toBe(true)
    expect(page.exportAction).toEqual({ disabled: true, hint: 'Em breve' })
  })

  it('filters through the query and paginates with the page size', async () => {
    const fakes = await signInAs(SEED_LOGINS.superAdmin)
    const page = useUserListPage({ now })
    await flushPromises()

    page.filter.value = 'invited'
    await flushPromises()
    expect(fakes.route.query).toEqual({ filter: 'invited' })
    expect(page.rows.value.map(row => row.name)).toEqual(['Ana Lúcia Prado', 'Beatriz Souza'])

    page.filter.value = 'all'
    page.pageSize.value = 20
    await flushPromises()
    expect(fakes.route.query).toEqual({ size: '20' })

    fakes.route.query = { size: '7', page: '1' }
    expect(page.pageSize.value).toBe(10)
  })

  it('searches after the debounce', async () => {
    vi.useFakeTimers()
    try {
      const fakes = await signInAs(SEED_LOGINS.superAdmin)
      const page = useUserListPage({ now, debounceMs: 300 })
      page.search.value = ' ouro '
      await vi.advanceTimersByTimeAsync(299)
      expect(fakes.route.query).toEqual({})
      await vi.advanceTimersByTimeAsync(1)
      expect(fakes.route.query).toEqual({ q: 'ouro' })
    }
    finally {
      vi.useRealTimers()
    }
  })

  it('runs the menu actions and reports the outcome', async () => {
    const fakes = await signInAs(SEED_LOGINS.superAdmin)
    const page = useUserListPage({ now })
    await flushPromises()

    const helio = page.rows.value.find(row => row.name === 'Helio Nascimento')!
    await page.runAction(helio, 'activate')
    await flushPromises()
    expect(page.notice.value).toMatchObject({ kind: 'success', title: 'Helio Nascimento reativado' })
    expect(page.rows.value.find(row => row.id === helio.id)).toMatchObject({ status: { label: 'Ativo' }, highlighted: true })

    const ana = page.rows.value.find(row => row.name === 'Ana Lúcia Prado')!
    await page.runAction(ana, 'resend_invitation')
    expect(page.notice.value).toMatchObject({ title: 'Convite reenviado' })
    await page.runAction(ana, 'cancel_invitation')
    await flushPromises()
    expect(page.notice.value).toMatchObject({ title: 'Convite cancelado' })
    expect(page.rows.value.some(row => row.id === ana.id)).toBe(false)
    expect(page.eyebrow.value).toBe('Administração · 8 usuários em 4 empresas')

    const marina = page.rows.value.find(row => row.name === 'Marina Rezende')!
    await page.runAction(marina, 'edit_access')
    expect(fakes.navigations.at(-1)).toEqual({ query: { user: marina.id } })
    fakes.route.query = { user: marina.id }
    expect(page.modal.value).toMatchObject({ mode: 'edit', user: { id: marina.id } })
    page.openInvite()
    expect(fakes.navigations.at(-1)).toEqual({ query: { invite: '1' } })
  })

  it('does not run a disabled action on the own row', async () => {
    await signInAs(SEED_LOGINS.superAdmin)
    const page = useUserListPage({ now })
    await flushPromises()
    const self = page.rows.value.find(row => row.isSelf)!
    await page.runAction(self, 'deactivate')
    expect(page.notice.value).toBeNull()
    expect(page.rows.value.find(row => row.isSelf)?.status.label).toBe('Ativo')
  })

  it('takes the flash of the modal, highlighting the saved row', async () => {
    await signInAs(SEED_LOGINS.superAdmin)
    const page = useUserListPage({ now })
    await flushPromises()
    useFlash().show({ kind: 'success', title: 'Acesso atualizado', highlightId: 'user-carlos' })
    expect(page.takeFlash()?.title).toBe('Acesso atualizado')
    await flushPromises()
    expect(page.rows.value.find(row => row.highlighted)?.name).toBe('Carlos Mendes')
    expect(page.takeFlash()).toBeNull()
  })
})

describe('useUserListPage (administrator)', () => {
  it('shows only the own company, without super admin chip and card', async () => {
    await signInAs(SEED_LOGINS.admin)
    const page = useUserListPage({ now })
    await flushPromises()

    expect(page.eyebrow.value).toBe('Administração · 3 usuários · Fazenda Santa Rita')
    expect(page.roleCards.value.map(card => card.role)).toEqual(['admin', 'planning_operator'])
    expect(page.filterChips.value.map(chip => chip.value)).toEqual(['all', 'admin', 'planning_operator', 'invited'])
    expect(new Set(page.rows.value.map(row => row.companyName))).toEqual(new Set(['Fazenda Santa Rita']))
  })

  it('ignores a super admin filter in the query', async () => {
    const fakes = await signInAs(SEED_LOGINS.admin)
    fakes.route.query = { filter: 'super_admin' }
    const page = useUserListPage({ now })
    expect(page.filter.value).toBe('all')
  })
})

describe('useUserListPage (forbidden)', () => {
  it('exposes the 403 of an operator honestly', async () => {
    await signInAs(SEED_LOGINS.operator)
    const page = useUserListPage({ now })
    await flushPromises()
    expect(page.error.value?.code).toBe('forbidden')
    expect(page.canInvite.value).toBe(false)
    expect(page.eyebrow.value).toBeNull()
  })
})
