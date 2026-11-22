/**
 * Presentation rules of the new-farm wizard: steps, form checks, preview and
 * review view models, and the draft → `FarmInput` mapping. The API is the
 * authority (area, share total, perimeter); these checks only avoid a request
 * that would certainly fail and guide the user step by step.
 */
import type { FieldError } from '~/services/errors'
import type { FarmInput } from '~/types/farm'
import type {
  FarmWizardDraft,
  FarmWizardPreview,
  WizardErrors,
  WizardFieldErrors,
  WizardOwnerShare,
  WizardReviewRow,
  WizardReviewSection,
  WizardStepKey,
  WizardStepView,
} from '~/types/wizard'
import { formatCoordinates, formatPercent } from './format'
import { LINK_ROLE_LABELS, PRODUCTION_TYPE_LABELS } from './labels'
import { isIncompletePerimeter, LOCATION_MESSAGES, perimeterAreaLabel, validateCoordinateInputs } from './wizardLocation'

export const WIZARD_STEPS: readonly WizardStepKey[] = ['identification', 'location', 'owners', 'review']

export const WIZARD_STEP_META: Readonly<Record<WizardStepKey, { title: string, description: string }>> = {
  identification: { title: 'Identificação', description: 'Nome, apelido e tipo' },
  location: { title: 'Localização', description: 'Município e perímetro' },
  owners: { title: 'Proprietários', description: 'Responsáveis legais' },
  review: { title: 'Revisão', description: 'Confira e salve' },
}

export const WIZARD_MESSAGES = {
  nameRequired: 'Informe o nome da fazenda.',
  productionTypeRequired: 'Escolha o tipo de produção.',
  cityRequired: 'Informe o município.',
  stateInvalid: 'Informe a UF com 2 letras.',
  postalCodeInvalid: 'O CEP deve ter 8 dígitos.',
  perimeterRequired: 'Defina o perímetro da fazenda.',
  ownersRequired: 'Vincule ao menos um proprietário.',
  shareInvalid: 'Informe a participação entre 1% e 100%.',
  shareTotal: 'A participação total precisa fechar 100%.',
  confirmRequired: 'Confirme que os dados estão corretos.',
  saveFailed: 'Não foi possível salvar a fazenda. Tente novamente.',
  saveInvalid: 'Revise os campos destacados.',
  draftSaved: 'Rascunho salvo',
  notInformed: 'Não informado',
  noOwners: 'Nenhum proprietário vinculado.',
} as const

const EMPTY = '—'
const UF_PATTERN = /^[A-Za-z]{2}$/
const SHARE_TOTAL = 100

export function createEmptyDraft(): FarmWizardDraft {
  return {
    version: 1,
    identification: { name: '', code: '', cooperative: '', productionType: null },
    location: {
      mode: 'address',
      perimeterMethod: 'draw',
      postalCode: '',
      city: '',
      state: '',
      carNumber: '',
      perimeter: null,
      perimeterAreaHa: null,
      forecastPoint: null,
      latitude: '',
      longitude: '',
      importedFileName: null,
      importedFileSize: null,
      importedSourceFormat: null,
      importedCity: null,
      importedState: null,
      importedDatum: null,
      importedFieldCount: null,
    },
    owners: [],
    confirmed: false,
    furthestStep: 'identification',
    savedAt: null,
    userId: null,
  }
}

/** `?step=` → step key; null for missing or unknown values. */
export function parseWizardStep(value: unknown): WizardStepKey | null {
  const raw = Array.isArray(value) ? value[0] : value
  return WIZARD_STEPS.find(step => step === raw) ?? null
}

export function stepIndex(step: WizardStepKey): number {
  return WIZARD_STEPS.indexOf(step)
}

function onlyDigits(value: string): string {
  return value.replace(/\D/g, '')
}

function blankToNull(value: string): string | null {
  const trimmed = value.trim()
  return trimmed ? trimmed : null
}

export function validateIdentification(draft: FarmWizardDraft): WizardFieldErrors {
  const errors: WizardFieldErrors = {}
  if (!draft.identification.name.trim()) errors.name = WIZARD_MESSAGES.nameRequired
  if (!draft.identification.productionType) errors.productionType = WIZARD_MESSAGES.productionTypeRequired
  return errors
}

export function validateLocation(draft: FarmWizardDraft): WizardFieldErrors {
  const { location } = draft
  const errors: WizardFieldErrors = {}
  const postalDigits = onlyDigits(location.postalCode)
  if (postalDigits && postalDigits.length !== 8) errors.postalCode = WIZARD_MESSAGES.postalCodeInvalid
  if (!location.city.trim()) errors.city = WIZARD_MESSAGES.cityRequired
  if (!UF_PATTERN.test(location.state.trim())) errors.state = WIZARD_MESSAGES.stateInvalid
  if (!location.perimeter) errors.perimeter = WIZARD_MESSAGES.perimeterRequired
  else if (isIncompletePerimeter(location.perimeter)) errors.perimeter = LOCATION_MESSAGES.perimeterIncomplete
  return { ...errors, ...validateCoordinateInputs(location.latitude, location.longitude) }
}

/** Error key of one owner row in step 3. */
export function ownerErrorKey(ownerId: string): string {
  return `owner:${ownerId}`
}

function isValidShare(share: number | null): share is number {
  return share !== null && Number.isFinite(share) && share > 0 && share <= SHARE_TOTAL
}

/** Single rule for "the shares close 100%" (tolerant to decimal noise like 33,3 × 3). */
export function isShareTotalComplete(total: number): boolean {
  return Math.abs(total - SHARE_TOTAL) < 0.05
}

/** Display-only sum, same rule as `totalSharePercent`: tenants have no share. */
export function wizardShareTotal(owners: readonly WizardOwnerShare[]): number {
  const total = owners.reduce((sum, owner) => sum + (owner.role === 'owner' ? owner.sharePercent ?? 0 : 0), 0)
  return Math.round(total * 10) / 10
}

export function validateOwners(draft: FarmWizardDraft): WizardFieldErrors {
  const errors: WizardFieldErrors = {}
  const shareholders = draft.owners.filter(owner => owner.role === 'owner')
  if (shareholders.length === 0) {
    errors.owners = WIZARD_MESSAGES.ownersRequired
    return errors
  }
  for (const owner of shareholders) {
    if (!isValidShare(owner.sharePercent)) errors[ownerErrorKey(owner.ownerId)] = WIZARD_MESSAGES.shareInvalid
  }
  if (!isShareTotalComplete(wizardShareTotal(draft.owners))) errors.owners = WIZARD_MESSAGES.shareTotal
  return errors
}

export function validateReview(draft: FarmWizardDraft): WizardFieldErrors {
  return draft.confirmed ? {} : { confirmed: WIZARD_MESSAGES.confirmRequired }
}

const VALIDATORS: Readonly<Record<WizardStepKey, (draft: FarmWizardDraft) => WizardFieldErrors>> = {
  identification: validateIdentification,
  location: validateLocation,
  owners: validateOwners,
  review: validateReview,
}

export function validateStep(draft: FarmWizardDraft, step: WizardStepKey): WizardFieldErrors {
  return VALIDATORS[step](draft)
}

export function isStepValid(draft: FarmWizardDraft, step: WizardStepKey): boolean {
  return Object.keys(validateStep(draft, step)).length === 0
}

/** First invalid step before `limit` (exclusive), or null when all are valid. */
export function firstInvalidStep(draft: FarmWizardDraft, limit: WizardStepKey = 'review'): WizardStepKey | null {
  const end = stepIndex(limit)
  return WIZARD_STEPS.slice(0, end).find(step => !isStepValid(draft, step)) ?? null
}

/**
 * A step can be opened when it was already reached (e.g. "Importar KML" enters
 * at step 2) or every step before it is valid. Saving still checks all steps.
 */
export function isStepReachable(draft: FarmWizardDraft, step: WizardStepKey): boolean {
  return stepIndex(step) <= stepIndex(draft.furthestStep) || firstInvalidStep(draft, step) === null
}

export function buildStepViews(draft: FarmWizardDraft, current: WizardStepKey): WizardStepView[] {
  const furthest = stepIndex(draft.furthestStep)
  return WIZARD_STEPS.map((key, index) => {
    const isComplete = key !== current && key !== 'review' && index <= furthest && isStepValid(draft, key)
    return {
      key,
      number: index + 1,
      ...WIZARD_STEP_META[key],
      status: key === current ? 'current' : isComplete ? 'complete' : 'pending',
      isReachable: isStepReachable(draft, key),
    }
  })
}

function cityLabel(draft: FarmWizardDraft): string {
  const city = draft.location.city.trim()
  const state = draft.location.state.trim().toUpperCase()
  if (!city) return EMPTY
  return state ? `${city} · ${state}` : city
}

/** API area (import) or the approximate preview of a drawn perimeter. */
function areaLabel(draft: FarmWizardDraft): string {
  return perimeterAreaLabel(draft.location) ?? EMPTY
}

function productionLabel(draft: FarmWizardDraft): string {
  const type = draft.identification.productionType
  return type ? PRODUCTION_TYPE_LABELS[type] : EMPTY
}

export function buildPreview(draft: FarmWizardDraft): FarmWizardPreview {
  const { location } = draft
  return {
    name: draft.identification.name.trim() || 'Nova fazenda',
    rows: [
      { key: 'area', label: 'Área', value: areaLabel(draft) },
      { key: 'city', label: 'Município', value: cityLabel(draft) },
      // The contract has no main crop yet: the production type stands in for it.
      { key: 'crop', label: 'Cultura principal', value: productionLabel(draft) },
      { key: 'forecastPoint', label: 'Ponto de previsão', value: formatCoordinates(location.forecastPoint) },
    ],
    perimeter: location.perimeter,
    forecastPoint: location.forecastPoint,
    hint: location.perimeter
      ? 'Com o perímetro definido, o agente já consegue buscar a previsão do tempo da fazenda.'
      : 'Defina o perímetro no passo 2 para o agente buscar a previsão do tempo da fazenda.',
  }
}

function ownerShareLabel(owner: WizardOwnerShare): string {
  return owner.role === 'owner' ? formatPercent(owner.sharePercent) : LINK_ROLE_LABELS[owner.role]
}

/** Review row with the final text: missing values (EMPTY) read "Não informado". */
function reviewRow(label: string, value: string): WizardReviewRow {
  const isEmpty = !value.trim() || value === EMPTY
  return { label, value: isEmpty ? WIZARD_MESSAGES.notInformed : value, isEmpty }
}

export function buildReview(draft: FarmWizardDraft): WizardReviewSection[] {
  const { identification } = draft
  // "Café · Cooxupé": production type and cooperative, whichever were filled.
  const productionValue = [productionLabel(draft), identification.cooperative.trim()]
    .filter(part => part && part !== EMPTY)
    .join(' · ')
  return [
    {
      step: 'identification',
      title: `1 · ${WIZARD_STEP_META.identification.title}`,
      rows: [
        reviewRow('Nome', identification.name.trim()),
        reviewRow('Apelido', identification.code.trim()),
        reviewRow('Produção', productionValue),
      ],
      emptyMessage: null,
    },
    {
      step: 'location',
      title: `2 · ${WIZARD_STEP_META.location.title}`,
      rows: [
        reviewRow('Município', cityLabel(draft)),
        reviewRow('Área do perímetro', areaLabel(draft)),
        reviewRow('Ponto de previsão', formatCoordinates(draft.location.forecastPoint)),
      ],
      emptyMessage: null,
    },
    {
      step: 'owners',
      title: `3 · ${WIZARD_STEP_META.owners.title}`,
      rows: draft.owners.map(owner => reviewRow(owner.name, ownerShareLabel(owner))),
      emptyMessage: draft.owners.length === 0 ? WIZARD_MESSAGES.noOwners : null,
    },
  ]
}

/** Draft → payload of `POST /farms`. Null while the production type is missing. */
export function toFarmInput(draft: FarmWizardDraft, status: FarmInput['status'] = 'active'): FarmInput | null {
  const { identification, location } = draft
  if (!identification.productionType) return null
  return {
    name: identification.name.trim(),
    code: blankToNull(identification.code),
    cooperative: blankToNull(identification.cooperative),
    productionType: identification.productionType,
    city: location.city.trim(),
    state: location.state.trim().toUpperCase(),
    status,
    location: {
      postalCode: blankToNull(onlyDigits(location.postalCode)),
      carNumber: blankToNull(location.carNumber),
      forecastPoint: location.forecastPoint,
      perimeter: location.perimeter,
    },
    owners: draft.owners.map(owner => ({
      ownerId: owner.ownerId,
      role: owner.role,
      sharePercent: owner.role === 'owner' ? owner.sharePercent : null,
    })),
  }
}

const API_FIELD_TO_STEP: Readonly<Record<string, [WizardStepKey, string]>> = {
  'name': ['identification', 'name'],
  'code': ['identification', 'code'],
  'cooperative': ['identification', 'cooperative'],
  'production_type': ['identification', 'productionType'],
  'city': ['location', 'city'],
  'state': ['location', 'state'],
  'location.postal_code': ['location', 'postalCode'],
  'location.car_number': ['location', 'carNumber'],
  'location.perimeter': ['location', 'perimeter'],
  'location.forecast_point': ['location', 'latitude'],
  'owners': ['owners', 'owners'],
}

const OWNER_ITEM_FIELD = /^owners\[(\d+)\]/

export function emptyWizardErrors(): WizardErrors {
  return { identification: {}, location: {}, owners: {}, review: {} }
}

function locateApiField(field: string, owners: readonly WizardOwnerShare[]): [WizardStepKey, string] | null {
  const direct = API_FIELD_TO_STEP[field]
  if (direct) return direct
  const ownerMatch = OWNER_ITEM_FIELD.exec(field)
  const owner = ownerMatch ? owners[Number(ownerMatch[1])] : undefined
  return owner ? ['owners', ownerErrorKey(owner.ownerId)] : null
}

/** 422 `details` (snake_case paths of the contract) → per-step field errors. */
export function mapApiErrorsToSteps(
  details: readonly FieldError[],
  owners: readonly WizardOwnerShare[],
): { errors: WizardErrors, unmapped: FieldError[] } {
  const errors = emptyWizardErrors()
  const unmapped: FieldError[] = []
  for (const detail of details) {
    const target = locateApiField(detail.field, owners)
    if (!target) {
      unmapped.push(detail)
      continue
    }
    const [step, field] = target
    errors[step][field] ??= detail.message
  }
  return { errors, unmapped }
}

/** "Rascunho salvo · há 2 min"; null before the first save. */
export function formatDraftSavedLabel(savedAt: string | null, now: Date): string | null {
  if (!savedAt) return null
  const saved = new Date(savedAt)
  if (Number.isNaN(saved.getTime())) return null
  const minutes = Math.max(0, Math.floor((now.getTime() - saved.getTime()) / 60_000))
  if (minutes < 1) return `${WIZARD_MESSAGES.draftSaved} · agora`
  if (minutes < 60) return `${WIZARD_MESSAGES.draftSaved} · há ${minutes} min`
  const hours = Math.floor(minutes / 60)
  if (hours < 24) return `${WIZARD_MESSAGES.draftSaved} · há ${hours} h`
  const days = Math.floor(hours / 24)
  return `${WIZARD_MESSAGES.draftSaved} · há ${days} ${days === 1 ? 'dia' : 'dias'}`
}
