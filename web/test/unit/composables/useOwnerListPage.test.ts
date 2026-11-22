import { afterEach, describe, expect, it, vi } from 'vitest'
import { useFlash } from '~/composables/useFlash'
import { useOwnerListPage } from '~/composables/useOwnerListPage'
import type { OwnerSummary } from '~/types/owner'
import { linkShareLabel, ownerSavedFlash, shortFarmName, toOwnerRowView } from '~/utils/ownerRow'
import { useSession } from '~/composables/useSession'
import { createTestApi, SEED_USER_EMAIL, SEED_USER_PASSWORD } from '../../support/mockApi'
import { installSignedIn, TEST_USERS } from '../../support/session'
import { flushPromises, installNuxtFakes, type NuxtFakes } from '../../support/nuxtFakes'

afterEach(() => {
  vi.unstubAllGlobals()
  vi.useRealTimers()
})

function setup(token?: string | null): NuxtFakes {
  const fakes = installNuxtFakes(createTestApi(token).api)
  fakes.route.path = '/owners'
  fakes.route.fullPath = '/owners'
  return fakes
}

describe('useOwnerListPage', () => {
  it('lists the Figma owners with formatted rows and footer labels', async () => {
    setup()
    const page = useOwnerListPage()
    await flushPromises()

    expect(page.eyebrow.value).toBe('6 pessoas e empresas')
    expect(page.pagination.value).toEqual({ showingLabel: 'Mostrando 6 de 6', pageLabel: 'Página 1 de 1', page: 1, pageCount: 1 })
    expect(page.rows.value.map(row => [row.name, row.shareLabel, row.shareTone])).toEqual([
      ['Bruno Blauzius', 'Varia', 'muted'],
      ['Marina Rezende', 'Varia', 'muted'],
      ['Agropecuária Lima Ltda.', 'Varia', 'muted'],
      ['João Silveira', '25%', 'default'],
      ['Helena Prado', '50%', 'default'],
      ['Cooperativa Vale Verde', 'Arrend.', 'role'],
    ])
    expect(page.rows.value[0]).toMatchObject({
      initials: 'BB',
      documentLabel: 'PF · ***.278.916-**',
      hasPendingDocuments: false,
      farms: [
        { id: 'farm-santa-rita', label: 'Santa Rita', shareLabel: '60%', text: 'Santa Rita · 60%' },
        { id: 'farm-ouro-verde', label: 'Ouro Verde', shareLabel: '40%', text: 'Ouro Verde · 40%' },
      ],
      isHighlighted: false,
    })
    expect(page.rows.value.find(row => row.name === 'Helena Prado')?.hasPendingDocuments).toBe(true)
  })

  it('filters by person type through the query, resetting the page', async () => {
    const fakes = setup()
    fakes.route.query = { page: '2' }
    const page = useOwnerListPage()

    page.personType.value = 'company'
    await flushPromises()

    expect(fakes.route.query).toEqual({ type: 'company' })
    expect(page.personType.value).toBe('company')
    expect(page.rows.value.map(row => row.documentLabel)).toEqual(['PJ · 12.345.678/0001-90', 'PJ · 98.765.432/0001-10'])
  })

  it('debounces the search before querying the server', async () => {
    vi.useFakeTimers()
    const fakes = setup()
    const page = useOwnerListPage({ debounceMs: 300 })

    page.search.value = 'hel'
    await vi.advanceTimersByTimeAsync(100)
    page.search.value = 'helena'
    await vi.advanceTimersByTimeAsync(299)
    expect(fakes.route.query).toEqual({})

    await vi.advanceTimersByTimeAsync(1)
    expect(fakes.route.query).toEqual({ q: 'helena' })
    await vi.runAllTimersAsync()
    expect(page.rows.value.map(row => row.name)).toEqual(['Helena Prado'])
  })

  it('tells an empty registry apart from a search without results', async () => {
    const fakes = setup()
    fakes.route.query = { q: 'ninguém' }
    const page = useOwnerListPage()
    await flushPromises()

    expect(page.isFilterEmpty.value).toBe(true)
    expect(page.isEmpty.value).toBe(false)
    expect(page.eyebrow.value).toBe('0 pessoas e empresas')
  })

  it('moves a page beyond the end to the last valid page instead of showing "empty"', async () => {
    const fakes = setup()
    fakes.route.query = { page: '2', type: 'individual' }
    const page = useOwnerListPage()
    await flushPromises()

    expect(fakes.route.query).toEqual({ type: 'individual' })
    expect(fakes.navigations.at(-1)).toEqual({ target: { query: { type: 'individual', page: undefined } }, options: { replace: true } })
    await flushPromises()
    expect(page.isEmpty.value).toBe(false)
    expect(page.rows.value).toHaveLength(4)
  })

  it('hides eyebrow and pagination during the first load and on error', async () => {
    setup()
    const loading = useOwnerListPage()
    expect(loading.eyebrow.value).toBeNull()
    expect(loading.pagination.value).toBeNull()
    vi.unstubAllGlobals()

    setup(null)
    const failed = useOwnerListPage()
    await flushPromises()
    expect(failed.eyebrow.value).toBeNull()
    expect(failed.pagination.value).toBeNull()
  })

  it('reports load errors without empty states', async () => {
    setup(null)
    const page = useOwnerListPage()
    await flushPromises()

    expect(page.error.value?.code).toBe('unauthorized')
    expect(page.isEmpty.value || page.isFilterEmpty.value).toBe(false)
  })

  it('opens and closes the owner modal through the query', async () => {
    const fakes = setup()
    await useSession().login({ identifier: SEED_USER_EMAIL, password: SEED_USER_PASSWORD, rememberMe: false })
    fakes.route.query = { type: 'individual' }
    const page = useOwnerListPage()
    expect(page.canWrite.value).toBe(true)

    page.openLinks('owner-helena')
    expect(page.modalOwnerId.value).toBe('owner-helena')
    expect(page.modalStep.value).toBe('links')

    page.openNew()
    expect(fakes.route.query).toEqual({ type: 'individual', owner: 'new' })
    expect(page.modalStep.value).toBe('data')

    page.closeModal()
    expect(fakes.route.query).toEqual({ type: 'individual' })
    expect(page.modalOwnerId.value).toBeNull()
  })

  it('highlights only the row of the saved owner from the flash', async () => {
    setup()
    const page = useOwnerListPage()
    await flushPromises()
    expect(page.takeFlash()).toBeNull()

    useFlash().show(ownerSavedFlash({ id: 'owner-helena', name: 'Helena Prado' }, [
      { farmId: 'farm-boa-esperanca', farmName: 'Sítio Boa Esperança', role: 'owner', sharePercent: 50 },
    ], 'created'))
    const message = page.takeFlash()
    await flushPromises()

    expect(message?.title).toBe('Cadastro de Helena Prado salvo')
    expect(page.rows.value.filter(row => row.isHighlighted).map(row => row.id)).toEqual(['owner-helena'])
  })
})

describe('owner row helpers', () => {
  it.each([
    ['Fazenda Santa Rita', 'Santa Rita'],
    ['Sítio Boa Esperança', 'Boa Esperança'],
    ['sitio Novo', 'Novo'],
    ['Chácara Primavera', 'Primavera'],
    ['Recanto', 'Recanto'],
    ['Fazenda', 'Fazenda'],
  ])('shortFarmName(%j) = %j', (name, expected) => {
    expect(shortFarmName(name)).toBe(expected)
  })

  it('labels shares by role', () => {
    expect(linkShareLabel({ role: 'owner', sharePercent: 60 })).toBe('60%')
    expect(linkShareLabel({ role: 'owner', sharePercent: null })).toBe('—')
    expect(linkShareLabel({ role: 'tenant', sharePercent: null })).toBe('Arrend.')
    expect(linkShareLabel({ role: 'legal_representative', sharePercent: null })).toBe('Resp. legal')
  })

  it('shows a single share when equal in every farm and "—" without farms', () => {
    const base: OwnerSummary = { id: 'o', personType: 'individual', name: 'Ana', document: '11122233344', hasPendingDocuments: false, links: [] }
    expect(toOwnerRowView(base)).toMatchObject({ shareLabel: '—', shareTone: 'muted' })
    expect(toOwnerRowView({
      ...base,
      links: [
        { farmId: 'a', farmName: 'Fazenda A', role: 'owner', sharePercent: 50 },
        { farmId: 'b', farmName: 'Fazenda B', role: 'owner', sharePercent: 50 },
      ],
    }).shareLabel).toBe('50%')
  })

  it('describes the saved links in neutral text', () => {
    expect(ownerSavedFlash({ id: 'o', name: 'Helena Prado' }, [
      { farmId: 'f', farmName: 'Sítio Boa Esperança', role: 'owner', sharePercent: 50 },
    ], 'created')).toEqual({
      kind: 'success',
      title: 'Cadastro de Helena Prado salvo',
      description: 'Vínculo: Sítio Boa Esperança (50% de participação).',
      highlightId: 'o',
    })
    const updated = ownerSavedFlash({ id: 'o', name: 'Cooperativa Vale Verde' }, [
      { farmId: 'r', farmName: 'Fazenda Recanto', role: 'tenant', sharePercent: null },
      { farmId: 's', farmName: 'Fazenda Santa Rita', role: 'owner', sharePercent: 10 },
    ], 'updated')
    expect(updated.title).toBe('Dados de Cooperativa Vale Verde atualizados')
    expect(updated.description).toBe('Vínculos: Fazenda Recanto (arrendatário), Fazenda Santa Rita (10% de participação).')
    expect(ownerSavedFlash({ id: 'o', name: 'Ana' }, [], 'created').description).toBe('Sem vínculos com fazendas.')
  })
})

describe('useOwnerListPage · permissions (ADR 0013)', () => {
  it('keeps the modal closed for a user without registry.write and strips a pasted ?owner=', async () => {
    const fakes = await installSignedIn(TEST_USERS.operator)
    fakes.route.query = { owner: 'new', type: 'individual' }
    const page = useOwnerListPage()

    expect(page.canWrite.value).toBe(false)
    expect(page.modalOwnerId.value).toBeNull()
    expect(fakes.route.query).toEqual({ type: 'individual' })

    page.openNew()
    page.openEdit('owner-marina')
    expect(fakes.route.query).toEqual({ type: 'individual' })
  })
})
