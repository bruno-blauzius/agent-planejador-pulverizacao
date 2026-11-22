import { afterEach, describe, expect, it, vi } from 'vitest'
import { ref } from 'vue'
import { useOwner } from '~/composables/useOwner'
import { useOwnerActions } from '~/composables/useOwnerActions'
import { useOwners } from '~/composables/useOwners'
import type { OwnerFilters, OwnerInput } from '~/types/owner'
import { createTestApi } from '../../support/mockApi'
import { flushPromises, installNuxtFakes } from '../../support/nuxtFakes'

afterEach(() => vi.unstubAllGlobals())

const helena: OwnerInput = {
  personType: 'individual',
  name: 'Helena Prado',
  document: '715.330.884-20',
  birthDate: '1979-03-12',
  phone: '35998124470',
  email: 'helena@email.test',
  windowAlertsEnabled: true,
}

describe('useOwners', () => {
  it('lists the six Figma owners and follows the search', async () => {
    installNuxtFakes(createTestApi().api)
    const filters = ref<OwnerFilters>({})
    const { owners, total } = useOwners(filters)
    await flushPromises()
    expect(total.value).toBe(6)

    filters.value = { search: 'prado' }
    await flushPromises()

    expect(owners.value.map(owner => owner.name)).toEqual(['Helena Prado'])
    expect(owners.value[0]?.hasPendingDocuments).toBe(true)
  })
})

describe('useOwner', () => {
  it('loads owner and links together', async () => {
    installNuxtFakes(createTestApi().api)
    const { owner, links, pending } = useOwner('owner-bruno')
    await flushPromises()

    expect(pending.value).toBe(false)
    expect(owner.value?.windowAlertsEnabled).toBe(true)
    expect(links.value.map(link => [link.farmName, link.sharePercent])).toEqual([
      ['Fazenda Santa Rita', 60],
      ['Fazenda Ouro Verde', 40],
    ])
  })
})

describe('useOwnerActions', () => {
  it('reports a duplicated CPF from the API', async () => {
    installNuxtFakes(createTestApi().api)
    const { createOwner, error } = useOwnerActions()

    expect(await createOwner(helena)).toBeNull()

    expect(error.value).toMatchObject({ code: 'conflict', status: 409, message: 'CPF já cadastrado' })
  })

  it('creates an owner and then saves the links (Dados → Vínculos)', async () => {
    installNuxtFakes(createTestApi().api)
    const { createOwner, replaceLinks, error } = useOwnerActions()

    const owner = await createOwner({ ...helena, name: 'Ana Souza', document: '111.222.333-44' })
    const links = await replaceLinks(owner!.id, [{ farmId: 'farm-recanto', role: 'tenant', sharePercent: null }])

    expect(error.value).toBeNull()
    expect(links).toEqual([{ farmId: 'farm-recanto', farmName: 'Fazenda Recanto', role: 'tenant', sharePercent: null }])
  })
})
