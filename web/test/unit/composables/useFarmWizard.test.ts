import { afterEach, describe, expect, it, vi } from 'vitest'
import { nextTick } from 'vue'
import { useFarmWizard } from '~/composables/useFarmWizard'
import { useSession } from '~/composables/useSession'
import { useFlash } from '~/composables/useFlash'
import { ApiError } from '~/services/errors'
import type { ApiServices } from '~/services/ports'
import type { GeoJsonPolygon } from '~/types/geo'
import { FORBIDDEN_MESSAGES } from '~/utils/permissions'
import { createEmptyDraft, WIZARD_MESSAGES } from '~/utils/wizard'
import { serializeDraft, type WizardDraftStorage } from '~/utils/wizardDraft'
import { createTestApi } from '../../support/mockApi'
import { installNuxtFakes, type NuxtFakes } from '../../support/nuxtFakes'

afterEach(() => vi.unstubAllGlobals())

const NOW = () => new Date('2026-11-21T12:00:00Z')

const SQUARE: GeoJsonPolygon = {
  type: 'Polygon',
  coordinates: [[[-46.72, -21.31], [-46.70, -21.31], [-46.70, -21.29], [-46.72, -21.29], [-46.72, -21.31]]],
}

class MemoryStorage implements WizardDraftStorage {
  value: string | null = null
  load() {
    return this.value
  }

  save(serialized: string) {
    this.value = serialized
  }

  clear() {
    this.value = null
  }
}

interface Setup {
  fakes: NuxtFakes
  api: ApiServices
  storage: MemoryStorage
}

function setup(query: Record<string, unknown> = {}, overrides: Partial<ApiServices> = {}, stored: string | null = null): Setup {
  const api = { ...createTestApi().api, ...overrides }
  const fakes = installNuxtFakes(api)
  fakes.route.path = '/farms/new'
  fakes.route.fullPath = '/farms/new'
  fakes.route.query = query
  const storage = new MemoryStorage()
  storage.value = stored
  return { fakes, api, storage }
}

type Wizard = ReturnType<typeof useFarmWizard>

function fillIdentification(wizard: Wizard) {
  wizard.identification.value.name = 'Fazenda Nova Esperança'
  wizard.identification.value.productionType = 'grains'
}

async function fillAll(wizard: Wizard, api: ApiServices) {
  fillIdentification(wizard)
  Object.assign(wizard.location.value, { city: 'Guaxupé', state: 'MG' })
  wizard.setDrawnPerimeter(SQUARE)
  const { items } = await api.owners.list()
  const [first] = items
  if (!first) throw new Error('seed has no owners')
  wizard.addOwner(first)
  wizard.confirmed.value = true
}

describe('useFarmWizard — step 1 and shell', () => {
  it('starts at step 1 with the 4 steps and an empty preview', () => {
    const { storage } = setup()
    const wizard = useFarmWizard({ storage, now: NOW })

    expect(wizard.currentStep.value).toBe('identification')
    expect(wizard.stepNumber.value).toBe(1)
    expect(wizard.isFirstStep.value).toBe(true)
    expect(wizard.steps.value.map(step => step.status)).toEqual(['current', 'pending', 'pending', 'pending'])
    expect(wizard.preview.value.name).toBe('Nova fazenda')
    expect(wizard.fieldErrors.value).toEqual({})
  })

  it('shows errors after touch or a blocked "Continuar", and stays on the step', async () => {
    const { fakes, storage } = setup()
    const wizard = useFarmWizard({ storage, now: NOW })

    wizard.touch('name')
    expect(wizard.fieldErrors.value).toEqual({ name: WIZARD_MESSAGES.nameRequired })

    expect(await wizard.next()).toBe(false)
    expect(wizard.fieldErrors.value).toEqual({
      name: WIZARD_MESSAGES.nameRequired,
      productionType: WIZARD_MESSAGES.productionTypeRequired,
    })
    expect(fakes.navigations).toEqual([])
  })

  it('advances to step 2 through the query and marks step 1 complete', async () => {
    const { fakes, storage } = setup()
    const wizard = useFarmWizard({ storage, now: NOW })
    fillIdentification(wizard)

    expect(await wizard.next()).toBe(true)

    expect(fakes.route.query).toEqual({ step: 'location' })
    expect(wizard.currentStep.value).toBe('location')
    expect(wizard.steps.value.map(step => step.status)).toEqual(['complete', 'current', 'pending', 'pending'])
    expect(wizard.preview.value.rows.find(row => row.key === 'crop')?.value).toBe('Grãos')

    await wizard.back()
    expect(wizard.currentStep.value).toBe('identification')
  })

  it('does not open a step whose previous steps are invalid', async () => {
    const { fakes, storage } = setup({ step: 'review' })
    const wizard = useFarmWizard({ storage, now: NOW })

    expect(wizard.currentStep.value).toBe('identification')
    expect(fakes.navigations).toEqual([{ target: { query: { step: 'identification' } }, options: { replace: true } }])
    expect(await wizard.goTo('owners')).toBe(false)
  })

  it('opens step 2 in file mode for "Importar KML"', () => {
    const { storage } = setup({ step: 'location', source: 'file' })
    const wizard = useFarmWizard({ storage, now: NOW })

    expect(wizard.currentStep.value).toBe('location')
    expect(wizard.location.value.mode).toBe('file')
    expect(wizard.steps.value[0]?.status).toBe('pending')
  })

  it('persists edits locally and restores the draft on the next visit', async () => {
    const { storage } = setup()
    const wizard = useFarmWizard({ storage, now: NOW })
    wizard.identification.value.name = 'Fazenda Santa Rita'
    await nextTick()

    expect(storage.value).toContain('Fazenda Santa Rita')
    expect(wizard.draftSavedLabel.value).toBe('Rascunho salvo · agora')
    expect(wizard.draftSavedAt.value).toBe(NOW().getTime())

    const saved = storage.value
    vi.unstubAllGlobals()
    const next = setup({}, {}, saved)
    const restored = useFarmWizard({ storage: next.storage, now: () => new Date('2026-11-21T12:02:00Z') })
    expect(restored.identification.value.name).toBe('Fazenda Santa Rita')
    expect(restored.draftSavedLabel.value).toBe('Rascunho salvo · há 2 min')
  })

  it('"Salvar e sair" keeps the draft and returns to the list with a toast', async () => {
    const { fakes, storage } = setup()
    const wizard = useFarmWizard({ storage, now: NOW })
    fillIdentification(wizard)

    await wizard.saveAndExit()

    expect(storage.value).toContain('Fazenda Nova Esperança')
    expect(useFlash().message.value?.title).toBe(WIZARD_MESSAGES.draftSaved)
    expect(fakes.navigations).toEqual(['/farms'])
  })

  it('"Cancelar" discards the draft', async () => {
    const draft = createEmptyDraft()
    draft.identification.name = 'Velha'
    draft.savedAt = '2026-11-21T11:00:00Z'
    const { fakes, storage } = setup({}, {}, serializeDraft(draft))
    const wizard = useFarmWizard({ storage, now: NOW })
    expect(wizard.identification.value.name).toBe('Velha')

    await wizard.cancel()

    expect(storage.value).toBeNull()
    expect(wizard.identification.value.name).toBe('')
    expect(fakes.navigations).toEqual(['/farms'])
  })
})

describe('useFarmWizard — final save', () => {
  it('creates the farm, shows the toast on /farms and clears the draft', async () => {
    const { fakes, api, storage } = setup({ step: 'review' })
    const wizard = useFarmWizard({ storage, now: NOW })
    await fillAll(wizard, api)
    expect(wizard.totalShare.value).toBe(100)
    expect(wizard.shareTotalLabel.value).toBe('100%')

    const saved = await wizard.save()
    expect(wizard.saveError.value).toBeNull()
    expect(saved).toBe(true)

    const message = useFlash().message.value
    expect(message?.kind).toBe('success')
    expect(message?.title).toBe('Fazenda Nova Esperança criada')
    expect(fakes.navigations.at(-1)).toBe('/farms')
    expect(storage.value).toBeNull()
    const { items } = await api.farms.list({ search: 'Nova Esperança' })
    expect(items.map(farm => [farm.city, farm.state, farm.status])).toEqual([['Guaxupé', 'MG', 'active']])
  })

  it('goes to the first invalid step instead of calling the API', async () => {
    const create = vi.fn()
    const base = createTestApi().api
    const { fakes, storage } = setup({ step: 'location' }, { farms: { ...base.farms, create } })
    const wizard = useFarmWizard({ storage, now: NOW })
    wizard.location.value.city = 'Guaxupé'

    expect(await wizard.save()).toBe(false)

    expect(create).not.toHaveBeenCalled()
    expect(fakes.route.query).toEqual({ step: 'identification' })
    expect(wizard.fieldErrors.value.name).toBe(WIZARD_MESSAGES.nameRequired)
  })

  it('maps 422 details to the step that owns the field', async () => {
    const base = createTestApi().api
    const failing: ApiServices['farms'] = {
      ...base.farms,
      create: async () => {
        throw new ApiError({
          code: 'validation_failed',
          message: 'dados inválidos',
          status: 422,
          details: [{ field: 'state', message: 'UF desconhecida' }],
        })
      },
    }
    const { fakes, api, storage } = setup({ step: 'review' }, { farms: failing })
    const wizard = useFarmWizard({ storage, now: NOW })
    await fillAll(wizard, api)

    expect(await wizard.save()).toBe(false)

    // Already on the step when save() resolves: the UI focuses the error next.
    expect(fakes.route.query).toEqual({ step: 'location' })
    expect(wizard.fieldErrors.value).toEqual({ state: 'UF desconhecida' })
    expect(wizard.saveError.value).toBe(WIZARD_MESSAGES.saveInvalid)
    expect(useFlash().message.value).toBeNull()

    wizard.location.value.state = 'SP'
    await nextTick()
    expect(wizard.fieldErrors.value).toEqual({})
  })

  it('shows an honest message when the API refuses the save with 403', async () => {
    const base = createTestApi().api
    const refused: ApiServices['farms'] = {
      ...base.farms,
      create: async () => {
        throw new ApiError({ code: 'forbidden', message: 'você não tem permissão para esta ação', status: 403 })
      },
    }
    const { api, storage } = setup({ step: 'review' }, { farms: refused })
    const wizard = useFarmWizard({ storage, now: NOW })
    await fillAll(wizard, api)

    expect(await wizard.save()).toBe(false)
    expect(wizard.saveError.value).toBe(FORBIDDEN_MESSAGES.forbidden)
  })

  it('reports a network failure honestly and keeps the draft', async () => {
    const base = createTestApi().api
    const offline: ApiServices['farms'] = {
      ...base.farms,
      create: async () => {
        throw new ApiError({ code: 'network_error', message: 'Sem conexão.', status: 0 })
      },
    }
    const { api, storage } = setup({ step: 'review' }, { farms: offline })
    const wizard = useFarmWizard({ storage, now: NOW })
    await fillAll(wizard, api)
    await nextTick()

    expect(await wizard.save()).toBe(false)

    expect(wizard.saveError.value).toBe('Sem conexão.')
    expect(wizard.saving.value).toBe(false)
    expect(storage.value).toContain('Fazenda Nova Esperança')
  })
})

describe('useFarmWizard — draft restore and logout', () => {
  function storedDraft(name: string, userId: string | null = null): string {
    const draft = createEmptyDraft()
    draft.identification.name = name
    draft.savedAt = '2026-11-21T11:00:00Z'
    draft.userId = userId
    return serializeDraft(draft)
  }

  it('typing before the restore does not overwrite the stored draft', async () => {
    const { fakes, storage } = setup({}, {}, storedDraft('Fazenda Salva'))
    fakes.autoMount = false
    const wizard = useFarmWizard({ storage, now: NOW })

    wizard.identification.value.name = 'Digitado cedo'
    await nextTick()
    expect(storage.value).toContain('Fazenda Salva')

    fakes.mount()
    expect(wizard.identification.value.name).toBe('Fazenda Salva')
    expect(storage.value).toContain('Fazenda Salva')
  })

  it('saves early typing once mounted when there was no stored draft', async () => {
    const { fakes, storage } = setup()
    fakes.autoMount = false
    const wizard = useFarmWizard({ storage, now: NOW })
    wizard.identification.value.name = 'Primeira'
    await nextTick()
    expect(storage.value).toBeNull()

    fakes.mount()
    expect(storage.value).toContain('Primeira')
  })

  it('discards a draft saved by another user', () => {
    const { storage } = setup({}, {}, storedDraft('De outra pessoa', 'user-other'))
    useState('session:user', () => ({ id: 'user-bruno' }))
    const wizard = useFarmWizard({ storage, now: NOW })

    expect(wizard.identification.value.name).toBe('')
    expect(storage.value).toBeNull()
  })

  it('useSession().logout() discards the draft in memory and in local storage', async () => {
    const { storage } = setup()
    const wizard = useFarmWizard({ storage, now: NOW })
    fillIdentification(wizard)
    await nextTick()
    expect(storage.value).not.toBeNull()

    await useSession().logout()

    expect(storage.value).toBeNull()
    expect(wizard.identification.value.name).toBe('')
  })
})
