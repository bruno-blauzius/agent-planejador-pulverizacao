/**
 * In-memory adapter of the ports (ADR 0010). It behaves like the planned
 * services/api: requires the Bearer token, paginates, returns contract errors
 * (401, 403, 404, 409, 413, 415, 422), applies company/farm scope and role
 * capabilities (ADR 0013, `access.ts`) and imitates server-side rules in a
 * simplified way so the UI gets plausible feedback.
 */
import type { FieldError } from '../errors'
import type {
  ApiServices,
  PasswordAuthService,
  CropService,
  FarmService,
  FieldService,
  OwnerService,
} from '../ports'
import type { Farm, FarmInput, FarmSummary } from '~/types/farm'
import type { Field, FieldInput } from '~/types/field'
import type {
  Owner,
  OwnerDocument,
  OwnerDocumentKind,
  OwnerFarmLink,
  OwnerFarmLinkInput,
  OwnerInput,
  OwnerSummary,
} from '~/types/owner'
import type { Permission, User } from '~/types/auth'
import { ROLE_PERMISSIONS, canSeeFarm, farmAccess, type Viewer } from './access'
import { createAdminMockServices } from './adminServices'
import { createApplicationHistoryMockServices, createApplicationStore } from './applicationServices'
import { createPlannerMockServices, type PlannerFaults } from './plannerServices'
import { approximateAreaHa, approximateCentroid } from './geometry'
import { createSeed, type FarmRecord, type MockSeed, type OwnerRecord, type UserRecord } from './seed'
import { createMockTerrainService } from './terrainMock'
import { MOCK_TERRAIN_CALCULATED_FARM_IDS, MOCK_TERRAIN_FIELD_SEEDS } from './terrainSeed'
import { createFieldConditionsStore, createMockRainGaugeService, createMockSoilConditionsService } from './fieldConditionsMock'
import { createMockTrafficabilityService } from './trafficabilityMock'
import { digitsOf, fail, failIfInvalid, normalize, paginate, roundOneDecimal } from './support'

export interface MockServicesOptions {
  /** Simulated network latency; 0 in tests. */
  readonly latencyMs?: number
  readonly getAccessToken: () => string | null | undefined
  /** Defaults to a fresh `createSeed()`. */
  readonly seed?: MockSeed
  readonly now?: () => Date
  /** Mutable switches to exercise the planner's honest-error states (agent/weather down, stale review). */
  readonly plannerFaults?: PlannerFaults
}

const MAX_PERIMETER_UPLOAD_BYTES = 20 * 1024 * 1024
const TOKEN_PREFIX = 'mock-token:'
const SESSION_HOURS = 8
const REMEMBER_ME_DAYS = 30

const PERIMETER_FORMATS = {
  kml: 'kml',
  kmz: 'kmz',
  geojson: 'geojson',
  json: 'geojson',
  zip: 'shapefile',
} as const

/** The mock is always the password flavour, so tests can call `api.auth.login` without narrowing. */
export type MockApiServices = Omit<ApiServices, 'auth'> & { readonly auth: PasswordAuthService }

/** Owner document rules of the contract (POST /owners/{ownerId}/documents). */
const DOCUMENT_KINDS: readonly OwnerDocumentKind[] = ['id_card', 'driver_license', 'articles_of_incorporation', 'other']
const DOCUMENT_MEDIA_TYPES: readonly string[] = ['application/pdf', 'image/jpeg', 'image/png']
const MAX_DOCUMENT_BYTES = 10 * 1024 * 1024

export function createMockServices(options: MockServicesOptions): MockApiServices {
  const db = options.seed ?? createSeed()
  const now = options.now ?? (() => new Date())
  let sequence = 0
  /** Object URLs of the files uploaded in this browser session, by document id. */
  const documentFiles = new Map<string, string>()

  const nextId = (prefix: string) => `${prefix}-mock-${++sequence}`

  /** Simulates the round trip and isolates callers from the internal records. */
  async function respond<T>(produce: () => T): Promise<T> {
    if (options.latencyMs) await new Promise(resolve => setTimeout(resolve, options.latencyMs))
    return structuredClone(produce())
  }

  function viewerOf(record: UserRecord): Viewer {
    return { id: record.id, role: record.role, companyId: record.companyId, farmScope: record.farmScope, permissions: ROLE_PERMISSIONS[record.role] }
  }

  function toUser(record: UserRecord): User {
    const primaryFarm = db.farms.find(item => item.id === record.primaryFarmId)
    return {
      id: record.id,
      name: record.name,
      email: record.email,
      role: record.role,
      primaryFarm: primaryFarm ? { id: primaryFarm.id, name: primaryFarm.name } : null,
      company: { id: record.companyId, name: db.companies.find(item => item.id === record.companyId)?.name ?? '' },
      farmScope: record.farmScope,
      permissions: ROLE_PERMISSIONS[record.role],
    }
  }

  function requireUserRecord(): UserRecord {
    const token = options.getAccessToken()
    const record = token?.startsWith(TOKEN_PREFIX)
      ? db.users.find(item => item.id === token.slice(TOKEN_PREFIX.length) && item.status === 'active')
      : undefined
    if (!record) fail('unauthorized', 'sessão expirada ou inválida')
    return record
  }

  /** Caller of the request being produced (set by `authorized`, which runs synchronously). */
  let viewer: Viewer | null = null
  function currentViewer(): Viewer {
    if (!viewer) fail('unauthorized', 'sessão expirada ou inválida')
    return viewer
  }

  /** Requires the token and, when given, a capability (403 `forbidden` otherwise). */
  function authorized<T>(produce: () => T, permission?: Permission): Promise<T> {
    return respond(() => {
      viewer = viewerOf(requireUserRecord())
      if (permission && !viewer.permissions.includes(permission)) fail('forbidden', 'você não tem permissão para esta ação')
      return produce()
    })
  }

  // ---- farms -------------------------------------------------------------

  /** Farm visible to the caller: other companies are 404, farms outside the operator scope 403. */
  function findFarm(farmId: string): FarmRecord {
    const record = db.farms.find(item => item.id === farmId)
    const access = record ? farmAccess(currentViewer(), record) : 'hidden'
    if (!record || access === 'hidden') fail('not_found', 'fazenda não encontrada')
    if (access === 'out_of_scope') fail('out_of_scope', 'fazenda fora do seu escopo')
    return record
  }

  function toFarmSummary(record: FarmRecord): FarmSummary {
    const fields = db.fields.filter(item => item.farmId === record.id)
    const crops = [...new Map(fields.map(item => [item.crop.code, item.crop])).values()]
    const owners = db.links
      .filter(link => link.farmId === record.id)
      .map(link => ({
        ownerId: link.ownerId,
        name: db.owners.find(item => item.id === link.ownerId)?.name ?? '',
        role: link.role,
        sharePercent: link.sharePercent,
      }))
    return {
      companyId: record.companyId,
      id: record.id,
      name: record.name,
      code: record.code,
      city: record.city,
      state: record.state,
      status: record.status,
      productionType: record.productionType,
      totalAreaHa: record.totalAreaHa,
      productiveAreaHa: roundOneDecimal(fields.reduce((sum, item) => sum + item.areaHa, 0)),
      fieldCount: fields.length,
      crops,
      owners,
      sprayOutlook: record.sprayOutlook,
      forecastPoint: record.location.forecastPoint,
      perimeter: record.location.perimeter,
      createdAt: record.createdAt,
    }
  }

  function toFarm(record: FarmRecord): Farm {
    return {
      ...toFarmSummary(record),
      cooperative: record.cooperative,
      location: record.location,
      updatedAt: record.updatedAt,
    }
  }

  function validateFarmInput(input: FarmInput): void {
    const details: FieldError[] = []
    if (!input.name.trim()) details.push({ field: 'name', message: 'informe o nome da fazenda' })
    if (input.state.trim().length !== 2) details.push({ field: 'state', message: 'UF deve ter 2 letras' })
    input.owners.forEach((share, index) => {
      if (!db.owners.some(item => item.id === share.ownerId)) {
        details.push({ field: `owners[${index}].owner_id`, message: 'proprietário não encontrado' })
      }
    })
    if (input.status === 'active') {
      if (!input.location.perimeter) {
        details.push({ field: 'location.perimeter', message: 'defina o perímetro da fazenda' })
      }
      const total = input.owners.reduce((sum, share) => sum + (share.role === 'owner' ? share.sharePercent ?? 0 : 0), 0)
      if (total !== 100) details.push({ field: 'owners', message: 'a participação total precisa fechar 100%' })
    }
    failIfInvalid(details)
  }

  function applyFarmInput(record: FarmRecord, input: FarmInput): void {
    const perimeter = input.location.perimeter
    Object.assign(record, {
      name: input.name.trim(),
      code: input.code,
      cooperative: input.cooperative,
      productionType: input.productionType,
      city: input.city,
      state: input.state.toUpperCase(),
      status: input.status,
      totalAreaHa: perimeter ? approximateAreaHa(perimeter) : 0,
      location: {
        ...input.location,
        forecastPoint: input.location.forecastPoint ?? (perimeter ? approximateCentroid(perimeter) : null),
      },
      updatedAt: now(),
    })
    db.links = db.links.filter(link => link.farmId !== record.id)
    db.links.push(...input.owners.map(share => ({ ...share, farmId: record.id })))
  }

  const farms: FarmService = {
    list(filters = {}) {
      return authorized(() => {
        const search = normalize(filters.search ?? '')
        const matches = db.farms
          .filter(item => canSeeFarm(currentViewer(), item))
          .filter(item => !filters.status || item.status === filters.status)
          .filter(item => !filters.productionType || item.productionType === filters.productionType)
          .filter(item => !search || [item.name, item.code ?? '', item.city].some(text => normalize(text).includes(search)))
          .map(toFarmSummary)
        return paginate(matches, filters)
      })
    },
    get(farmId) {
      return authorized(() => toFarm(findFarm(farmId)))
    },
    create(input) {
      return authorized(() => {
        validateFarmInput(input)
        const record: FarmRecord = {
          companyId: currentViewer().companyId,
          id: nextId('farm'),
          name: '',
          code: null,
          cooperative: null,
          city: '',
          state: '',
          status: 'draft',
          productionType: input.productionType,
          totalAreaHa: 0,
          location: { postalCode: null, carNumber: null, forecastPoint: null, perimeter: null },
          sprayOutlook: null,
          createdAt: now(),
          updatedAt: now(),
        }
        applyFarmInput(record, input)
        db.farms.push(record)
        return toFarm(record)
      }, 'registry.write')
    },
    update(farmId, input) {
      return authorized(() => {
        const record = findFarm(farmId)
        validateFarmInput(input)
        applyFarmInput(record, input)
        return toFarm(record)
      }, 'registry.write')
    },
    importPerimeter(file) {
      return authorized(() => {
        const extension = file.name.split('.').pop()?.toLowerCase() ?? ''
        const sourceFormat = PERIMETER_FORMATS[extension as keyof typeof PERIMETER_FORMATS]
        if (!sourceFormat) fail('unsupported_media_type', 'formato não suportado; use KML, KMZ, GeoJSON ou Shapefile (.zip)')
        if (file.size > MAX_PERIMETER_UPLOAD_BYTES) fail('payload_too_large', 'arquivo acima de 20 MB')
        // Real parsing belongs to the API; the mock always returns the same plausible shape.
        const perimeter = {
          type: 'Polygon' as const,
          coordinates: [[
            [-46.735, -21.31] as const,
            [-46.695, -21.318] as const,
            [-46.688, -21.29] as const,
            [-46.712, -21.279] as const,
            [-46.738, -21.29] as const,
            [-46.735, -21.31] as const,
          ]],
        }
        return {
          sourceFormat,
          perimeter,
          areaHa: approximateAreaHa(perimeter),
          centroid: approximateCentroid(perimeter),
          city: 'Guaxupé',
          state: 'MG',
          datum: 'SIRGAS 2000',
          fieldCount: 14,
        }
      }, 'registry.write')
    },
  }

  // ---- fields ------------------------------------------------------------

  function validateFieldInput(input: FieldInput): void {
    const details: FieldError[] = []
    if (!input.name.trim()) details.push({ field: 'name', message: 'informe o nome do talhão' })
    if (!db.crops.some(item => item.code === input.cropCode)) details.push({ field: 'crop_code', message: 'cultura desconhecida' })
    if ((input.polygon.coordinates[0]?.length ?? 0) < 4) details.push({ field: 'polygon', message: 'desenhe ao menos 3 vértices' })
    for (const key of ['rowSpacingM', 'plantSpacingM'] as const) {
      const value = input[key]
      if (value !== null && value <= 0) details.push({ field: key === 'rowSpacingM' ? 'row_spacing_m' : 'plant_spacing_m', message: 'deve ser maior que zero' })
    }
    failIfInvalid(details)
  }

  /** New field from a validated input; area and stand computed like the API would. */
  function buildField(farmId: string, input: FieldInput, id: string = nextId('field')): Field {
    const crop = db.crops.find(item => item.code === input.cropCode)!
    const hasSpacing = input.rowSpacingM !== null && input.plantSpacingM !== null
    return {
      id,
      farmId,
      name: input.name.trim(),
      block: input.block,
      areaHa: approximateAreaHa(input.polygon),
      crop,
      variety: input.variety,
      season: input.season,
      plantingDate: input.plantingDate,
      rowSpacingM: input.rowSpacingM,
      plantSpacingM: input.plantSpacingM,
      plantsPerHa: hasSpacing ? Math.round(10_000 / (input.rowSpacingM! * input.plantSpacingM!)) : null,
      polygon: input.polygon,
      sprayCondition: null,
      lastApplicationAt: null,
    }
  }

  const fields: FieldService = {
    listByFarm(farmId) {
      return authorized(() => {
        findFarm(farmId)
        return db.fields
          .filter(item => item.farmId === farmId)
          .sort((a, b) => (a.block ?? '').localeCompare(b.block ?? '', 'pt-BR') || a.name.localeCompare(b.name, 'pt-BR', { numeric: true }))
      })
    },
    create(farmId, input) {
      return authorized(() => {
        findFarm(farmId)
        validateFieldInput(input)
        const field = buildField(farmId, input)
        db.fields.push(field)
        return field
      }, 'registry.write')
    },
    update(farmId, fieldId, input) {
      return authorized(() => {
        findFarm(farmId)
        const index = db.fields.findIndex(item => item.id === fieldId && item.farmId === farmId)
        if (index < 0) fail('not_found', 'talhão não encontrado')
        validateFieldInput(input)
        const current = db.fields[index]!
        const field: Field = { ...buildField(farmId, input, current.id), sprayCondition: current.sprayCondition, lastApplicationAt: current.lastApplicationAt }
        db.fields[index] = field
        return field
      }, 'registry.write')
    },
  }

  const crops: CropService = {
    list() {
      return authorized(() => db.crops)
    },
  }

  // ---- owners ------------------------------------------------------------

  /**
   * Owners visible to the caller (ADR 0013): every owner for `users.manage_all`;
   * with a farm scope (operator), only owners linked to the released farms;
   * otherwise the company's owners plus those linked to its farms.
   */
  function canSeeOwner(record: OwnerRecord): boolean {
    const caller = currentViewer()
    if (caller.permissions.includes('users.manage_all')) return true
    const linkedToVisibleFarm = db.links.some(link => link.ownerId === record.id && isFarmVisible(link.farmId))
    if (caller.farmScope.mode === 'selected') return linkedToVisibleFarm
    return record.companyId === caller.companyId || linkedToVisibleFarm
  }

  function isFarmVisible(farmId: string): boolean {
    const farm = db.farms.find(item => item.id === farmId)
    return Boolean(farm && canSeeFarm(currentViewer(), farm))
  }

  function findOwner(ownerId: string): OwnerRecord {
    const record = db.owners.find(item => item.id === ownerId)
    if (!record || !canSeeOwner(record)) fail('not_found', 'proprietário não encontrado')
    return record
  }

  function linksOf(ownerId: string): OwnerFarmLink[] {
    return db.links
      .filter(link => link.ownerId === ownerId && isFarmVisible(link.farmId))
      .map(link => ({
        farmId: link.farmId,
        farmName: db.farms.find(item => item.id === link.farmId)?.name ?? '',
        role: link.role,
        sharePercent: link.sharePercent,
      }))
  }

  function toOwnerSummary(record: OwnerRecord): OwnerSummary {
    return {
      id: record.id,
      personType: record.personType,
      name: record.name,
      document: record.document,
      hasPendingDocuments: record.hasPendingDocuments,
      links: linksOf(record.id),
    }
  }

  function toOwner(record: OwnerRecord): Owner {
    return {
      ...toOwnerSummary(record),
      birthDate: record.birthDate,
      phone: record.phone,
      email: record.email,
      windowAlertsEnabled: record.windowAlertsEnabled,
      documents: record.documents,
      createdAt: record.createdAt,
      updatedAt: record.updatedAt,
    }
  }

  function validateOwnerInput(input: OwnerInput, ownerId: string | null): string {
    const details: FieldError[] = []
    const document = digitsOf(input.document)
    const expectedLength = input.personType === 'individual' ? 11 : 14
    if (!input.name.trim()) details.push({ field: 'name', message: 'informe o nome' })
    if (document.length !== expectedLength) {
      details.push({ field: 'document', message: input.personType === 'individual' ? 'CPF inválido' : 'CNPJ inválido' })
    }
    if (input.email && !/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(input.email)) details.push({ field: 'email', message: 'e-mail inválido' })
    failIfInvalid(details)
    if (db.owners.some(item => item.document === document && item.id !== ownerId)) {
      fail('conflict', input.personType === 'individual' ? 'CPF já cadastrado' : 'CNPJ já cadastrado')
    }
    return document
  }

  function validateLinks(ownerId: string, links: readonly OwnerFarmLinkInput[]): void {
    const details: FieldError[] = []
    links.forEach((link, index) => {
      if (!isFarmVisible(link.farmId)) {
        details.push({ field: `links[${index}].farm_id`, message: 'fazenda não encontrada' })
        return
      }
      const othersTotal = db.links
        .filter(item => item.farmId === link.farmId && item.ownerId !== ownerId && item.role === 'owner')
        .reduce((sum, item) => sum + (item.sharePercent ?? 0), 0)
      if (link.role === 'owner' && othersTotal + (link.sharePercent ?? 0) > 100) {
        details.push({ field: `links[${index}].share_percent`, message: 'participação total da fazenda passaria de 100%' })
      }
    })
    failIfInvalid(details)
  }

  const owners: OwnerService = {
    list(filters = {}) {
      return authorized(() => {
        const search = normalize(filters.search ?? '')
        const searchDigits = digitsOf(search)
        const matches = db.owners
          .filter(canSeeOwner)
          .filter(item => !filters.personType || item.personType === filters.personType)
          .filter(item => !search
            || normalize(item.name).includes(search)
            || (searchDigits.length > 0 && item.document.includes(searchDigits)))
          .map(toOwnerSummary)
        return paginate(matches, filters)
      })
    },
    get(ownerId) {
      return authorized(() => toOwner(findOwner(ownerId)))
    },
    create(input) {
      return authorized(() => {
        const document = validateOwnerInput(input, null)
        const record: OwnerRecord = {
          ...input,
          companyId: currentViewer().companyId,
          id: nextId('owner'),
          name: input.name.trim(),
          document,
          hasPendingDocuments: true,
          documents: [],
          createdAt: now(),
          updatedAt: now(),
        }
        db.owners.push(record)
        return toOwner(record)
      }, 'registry.write')
    },
    update(ownerId, input) {
      return authorized(() => {
        const record = findOwner(ownerId)
        const document = validateOwnerInput(input, ownerId)
        Object.assign(record, { ...input, name: input.name.trim(), document, updatedAt: now() })
        return toOwner(record)
      }, 'registry.write')
    },
    getLinks(ownerId) {
      return authorized(() => {
        findOwner(ownerId)
        return linksOf(ownerId)
      })
    },
    replaceLinks(ownerId, links) {
      return authorized(() => {
        findOwner(ownerId)
        validateLinks(ownerId, links)
        db.links = db.links.filter(link => link.ownerId !== ownerId)
        db.links.push(...links.map(link => ({ ...link, ownerId })))
        return linksOf(ownerId)
      }, 'registry.write')
    },
    uploadDocument(ownerId, upload) {
      return authorized(() => {
        const record = findOwner(ownerId)
        if (!DOCUMENT_KINDS.includes(upload.kind)) fail('validation_failed', 'dados inválidos', [{ field: 'kind', message: 'tipo de documento desconhecido' }])
        if (upload.file.size > MAX_DOCUMENT_BYTES) fail('payload_too_large', 'arquivo maior que 10 MB')
        if (!DOCUMENT_MEDIA_TYPES.includes(upload.file.type)) fail('unsupported_media_type', 'formato não suportado: envie PDF, JPEG ou PNG')
        const document: OwnerDocument = { id: nextId('doc'), kind: upload.kind, fileName: upload.fileName, uploadedAt: now() }
        record.documents.push(document)
        record.hasPendingDocuments = requiredDocumentMissing(record)
        // In the browser the file can be opened again from memory; on the server there is nothing to link.
        if (import.meta.client) documentFiles.set(document.id, URL.createObjectURL(upload.file))
        return document
      }, 'registry.write')
    },
    removeDocument(ownerId, documentId) {
      return authorized(() => {
        const record = findOwner(ownerId)
        const index = record.documents.findIndex(item => item.id === documentId)
        if (index < 0) fail('not_found', 'documento não encontrado')
        record.documents.splice(index, 1)
        record.hasPendingDocuments = requiredDocumentMissing(record)
      }, 'registry.write')
    },
    documentUrl(_ownerId, documentId) {
      return documentFiles.get(documentId) ?? '#'
    },
  }

  /** Same rule as the API (domain.RequiredDocumentsMissing): photo ID for a person, articles for a company. */
  function requiredDocumentMissing(record: OwnerRecord): boolean {
    const accepted: readonly OwnerDocumentKind[] = record.personType === 'individual'
      ? ['id_card', 'driver_license']
      : ['articles_of_incorporation']
    return !record.documents.some(item => accepted.includes(item.kind))
  }

  // ---- auth --------------------------------------------------------------

  const auth: PasswordAuthService = {
    kind: 'password',
    login(credentials) {
      return respond(() => {
        const identifier = normalize(credentials.identifier)
        const record = db.users.find(item => normalize(item.email) === identifier)
        if (!record?.password || record.password !== credentials.password) fail('unauthorized', 'e-mail ou senha inválidos')
        if (record.status !== 'active') fail('unauthorized', 'acesso desativado; fale com o administrador da sua empresa')
        record.lastAccessAt = now()
        const lifetimeMs = credentials.rememberMe ? REMEMBER_ME_DAYS * 86_400_000 : SESSION_HOURS * 3_600_000
        return {
          accessToken: `${TOKEN_PREFIX}${record.id}`,
          expiresAt: new Date(now().getTime() + lifetimeMs),
          user: toUser(record),
        }
      })
    },
    getCurrentUser() {
      return respond(() => toUser(requireUserRecord()))
    },
    // The token lives in the front's cookie; the session clears it. Nothing to revoke here.
    logout() {
      return respond(() => undefined)
    },
    changePassword(input) {
      return changePassword(input)
    },
  }

  const adminContext = {
    db,
    now,
    nextId,
    authorized,
    currentViewer,
    currentUserRecord: () => db.users.find(item => item.id === currentViewer().id) ?? fail('unauthorized', 'sessão expirada ou inválida'),
  }
  const { changePassword, ...admin } = createAdminMockServices(adminContext)
  // One store: what the planner registers after the human confirmation is what the list shows.
  const applicationStore = createApplicationStore(now)
  const planner = createPlannerMockServices(adminContext, applicationStore, options.plannerFaults)
  const history = createApplicationHistoryMockServices(adminContext, applicationStore, options.plannerFaults)

  // One store: a reading or an inspection recorded here is what "Chuva e solo" shows (ADR 0022, phase 2).
  const fieldConditions = createFieldConditionsStore({ now: now() })
  const fieldConditionsOptions = {
    fields,
    store: fieldConditions,
    now,
    latencyMs: options.latencyMs,
    can: (permission: Permission) => ROLE_PERMISSIONS[requireUserRecord().role].includes(permission),
    actor: () => {
      const { id, name } = requireUserRecord()
      return { id, name }
    },
  }

  return {
    auth,
    farms,
    fields,
    crops,
    owners,
    ...admin,
    planner: planner.planner,
    applications: { ...planner.applications, ...history.applications },
    today: planner.today,
    weather: history.weather,
    terrain: createMockTerrainService({
      fields,
      canWrite: () => ROLE_PERMISSIONS[requireUserRecord().role].includes('registry.write'),
      now,
      latencyMs: options.latencyMs,
      overrides: MOCK_TERRAIN_FIELD_SEEDS,
      initiallyCalculatedFarmIds: MOCK_TERRAIN_CALCULATED_FARM_IDS,
      actor: () => {
        const { id, name } = requireUserRecord()
        return { id, name }
      },
    }),
    trafficability: createMockTrafficabilityService({ fields, now, latencyMs: options.latencyMs, conditions: fieldConditions }),
    rainGauges: createMockRainGaugeService(fieldConditionsOptions),
    soilConditions: createMockSoilConditionsService(fieldConditionsOptions),
  }
}
