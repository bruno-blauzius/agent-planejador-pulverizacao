import { afterEach, describe, expect, it, vi } from 'vitest'
import { nextTick } from 'vue'
import { useFarmWizard } from '~/composables/useFarmWizard'
import type { GeoJsonPolygon } from '~/types/geo'
import type { Owner } from '~/types/owner'
import { ownerErrorKey, WIZARD_MESSAGES } from '~/utils/wizard'
import { createTestApi } from '../../support/mockApi'
import { flushPromises, installNuxtFakes } from '../../support/nuxtFakes'

afterEach(() => {
  vi.unstubAllGlobals()
  vi.useRealTimers()
})

const NOW = () => new Date('2026-11-21T12:00:00Z')

const SQUARE: GeoJsonPolygon = {
  type: 'Polygon',
  coordinates: [[[-46.72, -21.31], [-46.70, -21.31], [-46.70, -21.29], [-46.72, -21.29], [-46.72, -21.31]]],
}

function wizardAtOwners(searchDebounceMs = 0) {
  const { api } = createTestApi()
  const fakes = installNuxtFakes(api)
  fakes.route.query = { step: 'owners' }
  const wizard = useFarmWizard({ storage: null, now: NOW, searchDebounceMs })
  // Steps 1 and 2 valid, so ?step=owners is reachable.
  Object.assign(wizard.identification.value, { name: 'Fazenda Nova', productionType: 'coffee' })
  Object.assign(wizard.location.value, { city: 'Guaxupé', state: 'MG' })
  wizard.setDrawnPerimeter(SQUARE)
  // The mount sent the empty draft back to step 1; return to step 3.
  fakes.route.query = { step: 'owners' }
  return { wizard, api }
}

async function ownerById(api: ReturnType<typeof createTestApi>['api'], id: string): Promise<Owner> {
  return api.owners.get(id)
}

describe('useFarmWizard — step 3 (owners)', () => {
  it('searches owners from 2 characters, hiding the ones already linked', async () => {
    const { wizard, api } = wizardAtOwners()
    expect(wizard.ownerSuggestions.value).toEqual([])

    wizard.ownerSearch.value = 'H'
    await flushPromises()
    expect(wizard.isOwnerSearchActive.value).toBe(false)

    wizard.ownerSearch.value = 'Hel'
    await flushPromises()
    expect(wizard.ownerSuggestions.value.map(item => item.name)).toEqual(['Helena Prado'])
    expect(wizard.newOwnerSuggestion.value).toBe('Não achou? Cadastrar “Hel…” como novo proprietário')
    expect(wizard.newOwnerPrefillName.value).toBe('Hel')

    wizard.addOwner(await ownerById(api, 'owner-helena'))
    expect(wizard.ownerSearch.value).toBe('')
    wizard.ownerSearch.value = 'Hel'
    await flushPromises()
    expect(wizard.ownerSuggestions.value).toEqual([])
  })

  it('debounces the search while typing', async () => {
    vi.useFakeTimers()
    const { wizard } = wizardAtOwners(300)

    wizard.ownerSearch.value = 'Mar'
    await nextTick()
    expect(wizard.isOwnerSearchActive.value).toBe(false)

    vi.advanceTimersByTime(300)
    expect(wizard.isOwnerSearchActive.value).toBe(true)
  })

  it('links owners with the remaining share and shows the total', async () => {
    const { wizard, api } = wizardAtOwners()
    wizard.addOwner(await ownerById(api, 'owner-bruno'))
    wizard.setOwnerShare('owner-bruno', '60')
    wizard.addOwner(await ownerById(api, 'owner-marina'))

    expect(wizard.ownerRows.value.map(row => [row.name, row.shareInput, row.detail])).toEqual([
      ['Bruno Blauzius', '60', 'PF · ***.278.916-**'],
      ['Marina Rezende', '40', 'PF · ***.441.207-**'],
    ])
    expect(wizard.shareTotalLabel.value).toBe('100%')
    expect(wizard.shareProgress.value).toBe(100)
    expect(wizard.isShareComplete.value).toBe(true)
    expect(wizard.ownerFormPreset.value.currentShares).toEqual([
      { name: 'Bruno Blauzius', sharePercent: 60 },
      { name: 'Marina Rezende', sharePercent: 40 },
    ])
  })

  it('shows per-owner and total errors after "Continuar"', async () => {
    const { wizard, api } = wizardAtOwners()
    wizard.addOwner(await ownerById(api, 'owner-bruno'))
    wizard.setOwnerShare('owner-bruno', 'abc')

    expect(await wizard.next()).toBe(false)

    expect(wizard.ownerRows.value[0]?.error).toBe(WIZARD_MESSAGES.shareInvalid)
    expect(wizard.fieldErrors.value).toEqual({
      [ownerErrorKey('owner-bruno')]: WIZARD_MESSAGES.shareInvalid,
      owners: WIZARD_MESSAGES.shareTotal,
    })

    wizard.removeOwner('owner-bruno')
    expect(wizard.fieldErrors.value).toEqual({ owners: WIZARD_MESSAGES.ownersRequired })
  })

  it('links the owner registered in the wizard form without touching the others', async () => {
    const { wizard, api } = wizardAtOwners()
    wizard.addOwner(await ownerById(api, 'owner-bruno'))
    wizard.setOwnerShare('owner-bruno', 60)
    wizard.addOwner(await ownerById(api, 'owner-marina'))

    wizard.linkRegisteredOwner({ owner: await ownerById(api, 'owner-helena'), sharePercent: 20 })

    expect(wizard.ownerRows.value.map(row => [row.name, row.shareInput, row.isHighlighted])).toEqual([
      ['Bruno Blauzius', '60', false],
      ['Marina Rezende', '40', false],
      ['Helena Prado', '20', true],
    ])
    expect(wizard.ownerRows.value[2]?.detail).toContain('nova')
    expect(wizard.shareTotalLabel.value).toBe('120%')
    expect(wizard.ownerNotice.value).toEqual({
      title: 'Helena Prado cadastrada e vinculada',
      description: 'Participação total: 120%. Ajuste para fechar 100%.',
    })

    // 120% blocks "Continuar" until the user adjusts.
    expect(await wizard.next()).toBe(false)
    expect(wizard.fieldErrors.value.owners).toBe(WIZARD_MESSAGES.shareTotal)
    wizard.setOwnerShare('owner-bruno', '40')
    expect(await wizard.next()).toBe(true)

    wizard.dismissOwnerNotice()
    expect(wizard.ownerNotice.value).toBeNull()
  })

  it('clears the toast when an owner is removed or when leaving step 3', async () => {
    const { wizard, api } = wizardAtOwners()
    wizard.linkRegisteredOwner({ owner: await ownerById(api, 'owner-helena'), sharePercent: 100 })
    expect(wizard.ownerNotice.value).not.toBeNull()
    wizard.removeOwner('owner-helena')
    expect(wizard.ownerNotice.value).toBeNull()

    wizard.linkRegisteredOwner({ owner: await ownerById(api, 'owner-helena'), sharePercent: 100 })
    await wizard.back()
    await nextTick()
    expect(wizard.currentStep.value).toBe('location')
    expect(wizard.ownerNotice.value).toBeNull()
  })

  it('lets the alerts banner be dismissed', () => {
    const { wizard } = wizardAtOwners()
    expect(wizard.alertsBannerVisible.value).toBe(true)
    wizard.dismissAlertsBanner()
    expect(wizard.alertsBannerVisible.value).toBe(false)
  })

  it('saves after removing an owner (no reactive proxies in the payload)', async () => {
    const { wizard, api } = wizardAtOwners()
    wizard.addOwner(await ownerById(api, 'owner-bruno'))
    wizard.addOwner(await ownerById(api, 'owner-marina'))
    wizard.removeOwner('owner-bruno')
    wizard.setOwnerShare('owner-marina', '100')
    wizard.confirmed.value = true

    expect(await wizard.save()).toBe(true)
    expect(wizard.saveError.value).toBeNull()
  })

  it('saves after linking an owner registered in the wizard', async () => {
    const { wizard, api } = wizardAtOwners()
    wizard.addOwner(await ownerById(api, 'owner-bruno'))
    wizard.setOwnerShare('owner-bruno', '80')
    wizard.linkRegisteredOwner({ owner: await ownerById(api, 'owner-helena'), sharePercent: 20 })
    wizard.confirmed.value = true

    expect(await wizard.save()).toBe(true)
    const { items } = await api.farms.list({ search: 'Fazenda Nova' })
    expect(items[0]?.owners.map(owner => [owner.name, owner.sharePercent])).toEqual([
      ['Bruno Blauzius', 80],
      ['Helena Prado', 20],
    ])
  })

  it('reports an unexpected failure honestly instead of throwing', async () => {
    const { wizard, api } = wizardAtOwners()
    wizard.addOwner(await ownerById(api, 'owner-bruno'))
    wizard.confirmed.value = true
    vi.spyOn(console, 'error').mockImplementation(() => undefined)
    vi.spyOn(JSON, 'stringify').mockImplementationOnce(() => {
      throw new Error('boom')
    })

    expect(await wizard.save()).toBe(false)
    expect(wizard.saveError.value).toBe(WIZARD_MESSAGES.saveFailed)
  })
})
