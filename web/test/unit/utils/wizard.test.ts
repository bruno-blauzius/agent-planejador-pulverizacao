import { describe, expect, it } from 'vitest'
import type { GeoJsonPolygon } from '~/types/geo'
import type { FarmWizardDraft, WizardOwnerShare } from '~/types/wizard'
import {
  buildPreview,
  buildReview,
  buildStepViews,
  createEmptyDraft,
  formatDraftSavedLabel,
  isStepReachable,
  mapApiErrorsToSteps,
  ownerErrorKey,
  parseWizardStep,
  toFarmInput,
  validateIdentification,
  validateLocation,
  validateOwners,
  validateReview,
  WIZARD_MESSAGES,
} from '~/utils/wizard'

const SQUARE: GeoJsonPolygon = {
  type: 'Polygon',
  coordinates: [[[-46.72, -21.31], [-46.70, -21.31], [-46.70, -21.29], [-46.72, -21.29], [-46.72, -21.31]]],
}

function owner(overrides: Partial<WizardOwnerShare> = {}): WizardOwnerShare {
  return {
    ownerId: 'owner-1',
    name: 'Bruno Blauzius',
    personType: 'individual',
    documentLabel: '***.278.916-**',
    role: 'owner',
    sharePercent: 100,
    isNewlyRegistered: false,
    ...overrides,
  }
}

function filledDraft(): FarmWizardDraft {
  const draft = createEmptyDraft()
  draft.identification = { name: ' Fazenda Santa Rita ', code: 'STR-01', cooperative: '', productionType: 'coffee' }
  Object.assign(draft.location, {
    postalCode: '37800-000',
    city: 'Guaxupé',
    state: 'mg',
    perimeter: SQUARE,
    perimeterAreaHa: 486.2,
    forecastPoint: { lat: -21.3, lng: -46.71 },
  })
  draft.owners = [owner({ sharePercent: 60 }), owner({ ownerId: 'owner-2', name: 'Marina Rezende', sharePercent: 40 })]
  draft.confirmed = true
  return draft
}

describe('parseWizardStep', () => {
  it('accepts known steps only', () => {
    expect(parseWizardStep('location')).toBe('location')
    expect(parseWizardStep(['owners'])).toBe('owners')
    expect(parseWizardStep('payment')).toBeNull()
    expect(parseWizardStep(undefined)).toBeNull()
  })
})

describe('step validation', () => {
  it('requires name and production type in step 1', () => {
    expect(validateIdentification(createEmptyDraft())).toEqual({
      name: WIZARD_MESSAGES.nameRequired,
      productionType: WIZARD_MESSAGES.productionTypeRequired,
    })
    expect(validateIdentification(filledDraft())).toEqual({})
  })

  it('requires city, a 2-letter UF and the perimeter in step 2; CEP is optional but complete', () => {
    const draft = createEmptyDraft()
    draft.location.postalCode = '3780'
    draft.location.state = 'M'
    expect(validateLocation(draft)).toEqual({
      postalCode: WIZARD_MESSAGES.postalCodeInvalid,
      city: WIZARD_MESSAGES.cityRequired,
      state: WIZARD_MESSAGES.stateInvalid,
      perimeter: WIZARD_MESSAGES.perimeterRequired,
    })
    expect(validateLocation(filledDraft())).toEqual({})
  })

  it('requires an owner and shares closing 100% in step 3', () => {
    expect(validateOwners(createEmptyDraft())).toEqual({ owners: WIZARD_MESSAGES.ownersRequired })

    const draft = filledDraft()
    draft.owners = [owner({ sharePercent: 60 }), owner({ ownerId: 'owner-2', sharePercent: null })]
    expect(validateOwners(draft)).toEqual({
      [ownerErrorKey('owner-2')]: WIZARD_MESSAGES.shareInvalid,
      owners: WIZARD_MESSAGES.shareTotal,
    })

    draft.owners = [owner({ sharePercent: 100 }), owner({ ownerId: 'tenant', role: 'tenant', sharePercent: null })]
    expect(validateOwners(draft)).toEqual({})
  })

  it('requires the confirmation in step 4', () => {
    expect(validateReview(createEmptyDraft())).toEqual({ confirmed: WIZARD_MESSAGES.confirmRequired })
  })
})

describe('buildStepViews', () => {
  it('marks the current step and the valid visited ones as complete', () => {
    const draft = filledDraft()
    draft.furthestStep = 'owners'
    const views = buildStepViews(draft, 'owners')
    expect(views.map(view => [view.number, view.title, view.status])).toEqual([
      [1, 'Identificação', 'complete'],
      [2, 'Localização', 'complete'],
      [3, 'Proprietários', 'current'],
      [4, 'Revisão', 'pending'],
    ])
    expect(views.every(view => view.isReachable)).toBe(true)
  })

  it('blocks steps after an invalid one unless they were already reached', () => {
    const draft = createEmptyDraft()
    expect(isStepReachable(draft, 'location')).toBe(false)
    draft.furthestStep = 'location'
    expect(isStepReachable(draft, 'location')).toBe(true)
    expect(isStepReachable(draft, 'owners')).toBe(false)
    expect(buildStepViews(draft, 'location')[0]?.status).toBe('pending')
  })
})

describe('buildPreview', () => {
  it('shows placeholders while empty', () => {
    const preview = buildPreview(createEmptyDraft())
    expect(preview.name).toBe('Nova fazenda')
    expect(preview.rows.map(row => row.value)).toEqual(['—', '—', '—', '—'])
    expect(preview.hint).toContain('Defina o perímetro')
  })

  it('renders the Figma values with an API-computed area', () => {
    const preview = buildPreview(filledDraft())
    expect(preview.name).toBe('Fazenda Santa Rita')
    expect(preview.rows).toEqual([
      { key: 'area', label: 'Área', value: '486,2 ha' },
      { key: 'city', label: 'Município', value: 'Guaxupé · MG' },
      { key: 'crop', label: 'Cultura principal', value: 'Café' },
      { key: 'forecastPoint', label: 'Ponto de previsão', value: '-21,30 · -46,71' },
    ])
    expect(preview.hint).toContain('Com o perímetro definido')
  })

  it('labels the area of a drawn perimeter as approximate', () => {
    const draft = filledDraft()
    draft.location.perimeterAreaHa = null
    expect(buildPreview(draft).rows[0]?.value).toMatch(/^≈ \d[\d.,]* ha · aproximada$/)
  })

  it('validates the optional headquarters coordinates and an unfinished drawing', () => {
    const draft = filledDraft()
    draft.location.latitude = '-95'
    draft.location.perimeter = { type: 'Polygon', coordinates: [[[-46.7, -21.3], [-46.6, -21.3], [-46.7, -21.3]]] }
    expect(validateLocation(draft)).toEqual({
      perimeter: 'Feche o perímetro com pelo menos 3 vértices.',
      latitude: 'Latitude entre -90 e 90 (ex.: -21,3045).',
      longitude: 'Informe latitude e longitude.',
    })
  })
})

describe('buildReview', () => {
  it('lists the three sections of the review step', () => {
    const draft = filledDraft()
    draft.identification.cooperative = 'Cooxupé'
    draft.owners.push(owner({ ownerId: 'tenant', name: 'João Arrendatário', role: 'tenant', sharePercent: null }))
    expect(buildReview(draft)).toEqual([
      {
        step: 'identification',
        title: '1 · Identificação',
        rows: [
          { label: 'Nome', value: 'Fazenda Santa Rita', isEmpty: false },
          { label: 'Apelido', value: 'STR-01', isEmpty: false },
          { label: 'Produção', value: 'Café · Cooxupé', isEmpty: false },
        ],
        emptyMessage: null,
      },
      {
        step: 'location',
        title: '2 · Localização',
        rows: [
          { label: 'Município', value: 'Guaxupé · MG', isEmpty: false },
          { label: 'Área do perímetro', value: '486,2 ha', isEmpty: false },
          { label: 'Ponto de previsão', value: '-21,30 · -46,71', isEmpty: false },
        ],
        emptyMessage: null,
      },
      {
        step: 'owners',
        title: '3 · Proprietários',
        rows: [
          { label: 'Bruno Blauzius', value: '60%', isEmpty: false },
          { label: 'Marina Rezende', value: '40%', isEmpty: false },
          { label: 'João Arrendatário', value: 'Arrendatário', isEmpty: false },
        ],
        emptyMessage: null,
      },
    ])
  })
})

describe('buildReview — missing values', () => {
  it('spells out missing values and the empty owners section', () => {
    const [identification, location, owners] = buildReview(createEmptyDraft())
    expect(identification?.rows).toEqual([
      { label: 'Nome', value: WIZARD_MESSAGES.notInformed, isEmpty: true },
      { label: 'Apelido', value: WIZARD_MESSAGES.notInformed, isEmpty: true },
      { label: 'Produção', value: WIZARD_MESSAGES.notInformed, isEmpty: true },
    ])
    expect(location?.rows.every(row => row.isEmpty)).toBe(true)
    expect(owners).toEqual({ step: 'owners', title: '3 · Proprietários', rows: [], emptyMessage: 'Nenhum proprietário vinculado.' })
  })

  it('shows the cooperative alone when the production type is missing', () => {
    const draft = createEmptyDraft()
    draft.identification.cooperative = 'Cooxupé'
    expect(buildReview(draft)[0]?.rows[2]).toEqual({ label: 'Produção', value: 'Cooxupé', isEmpty: false })
  })
})

describe('toFarmInput', () => {
  it('maps the draft to the create payload', () => {
    expect(toFarmInput(filledDraft())).toEqual({
      name: 'Fazenda Santa Rita',
      code: 'STR-01',
      cooperative: null,
      productionType: 'coffee',
      city: 'Guaxupé',
      state: 'MG',
      status: 'active',
      location: {
        postalCode: '37800000',
        carNumber: null,
        forecastPoint: { lat: -21.3, lng: -46.71 },
        perimeter: SQUARE,
      },
      owners: [
        { ownerId: 'owner-1', role: 'owner', sharePercent: 60 },
        { ownerId: 'owner-2', role: 'owner', sharePercent: 40 },
      ],
    })
  })

  it('returns null without a production type', () => {
    expect(toFarmInput(createEmptyDraft())).toBeNull()
  })
})

describe('mapApiErrorsToSteps', () => {
  it('routes 422 details to the step and field', () => {
    const owners = filledDraft().owners
    const { errors, unmapped } = mapApiErrorsToSteps([
      { field: 'name', message: 'nome já usado' },
      { field: 'location.perimeter', message: 'perímetro inválido' },
      { field: 'location.forecast_point', message: 'ponto fora do Brasil' },
      { field: 'owners[1].owner_id', message: 'proprietário não encontrado' },
      { field: 'owners', message: 'a participação total precisa fechar 100%' },
      { field: 'status', message: 'inválido' },
    ], owners)
    expect(errors).toEqual({
      identification: { name: 'nome já usado' },
      location: { perimeter: 'perímetro inválido', latitude: 'ponto fora do Brasil' },
      owners: { [ownerErrorKey('owner-2')]: 'proprietário não encontrado', owners: 'a participação total precisa fechar 100%' },
      review: {},
    })
    expect(unmapped).toEqual([{ field: 'status', message: 'inválido' }])
  })
})

describe('formatDraftSavedLabel', () => {
  const now = new Date('2026-11-21T12:00:00Z')

  it('describes how long ago the draft was saved', () => {
    expect(formatDraftSavedLabel(null, now)).toBeNull()
    expect(formatDraftSavedLabel('2026-11-21T11:59:40Z', now)).toBe('Rascunho salvo · agora')
    expect(formatDraftSavedLabel('2026-11-21T11:58:00Z', now)).toBe('Rascunho salvo · há 2 min')
    expect(formatDraftSavedLabel('2026-11-21T09:00:00Z', now)).toBe('Rascunho salvo · há 3 h')
    expect(formatDraftSavedLabel('2026-11-20T09:00:00Z', now)).toBe('Rascunho salvo · há 1 dia')
  })
})
