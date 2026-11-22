/**
 * Realistic seed for the in-memory adapter, mirroring the Figma screens
 * ("01 · Acesso e Cadastros"). Records are stored in the shape of the models,
 * without derived fields (field count, productive area, crops), which the mock
 * computes the same way the API will.
 */
import type { CompanyPlan, CompanyStatus, UserStatus } from '~/types/admin'
import type { FarmScope, UserRole } from '~/types/auth'
import type { Crop } from '~/types/crop'
import type { FarmLocation, FarmStatus, ProductionType } from '~/types/farm'
import type { Field } from '~/types/field'
import type { GeoJsonPolygon, GeoJsonPosition } from '~/types/geo'
import type { LinkRole, OwnerDocument, PersonType } from '~/types/owner'
import type { SprayOutlook } from '~/types/spray'

export interface FarmRecord {
  /** Owning company (tenant, ADR 0013). */
  companyId: string
  id: string
  name: string
  code: string | null
  cooperative: string | null
  city: string
  state: string
  status: FarmStatus
  productionType: ProductionType
  totalAreaHa: number
  location: FarmLocation
  sprayOutlook: SprayOutlook | null
  createdAt: Date
  updatedAt: Date
}

export interface OwnerRecord {
  /** Company that registered the owner (tenant, ADR 0013). */
  companyId: string
  id: string
  personType: PersonType
  name: string
  document: string
  birthDate: string | null
  phone: string | null
  email: string | null
  windowAlertsEnabled: boolean
  hasPendingDocuments: boolean
  documents: OwnerDocument[]
  createdAt: Date
  updatedAt: Date
}

export interface LinkRecord {
  ownerId: string
  farmId: string
  role: LinkRole
  sharePercent: number | null
}

export interface UserRecord {
  id: string
  name: string
  email: string
  /** CPF digits, so the mock accepts "E-mail ou CPF" like the API. */
  document: string
  /** Test credential of the mock only; never a real password. Null while the invitation is pending. */
  password: string | null
  role: UserRole
  companyId: string
  farmScope: FarmScope
  status: UserStatus
  lastAccessAt: Date | null
  invitation: { id: string, expiresAt: Date } | null
  primaryFarmId: string | null
  passwordChangedAt: Date | null
}

export interface CompanyRecord {
  id: string
  name: string
  /** CNPJ digits. */
  document: string
  city: string
  state: string
  plan: CompanyPlan
  status: CompanyStatus
  trialEndsAt: Date | null
  createdAt: Date
  /** The platform operator (super admins); not listed among client companies. */
  isPlatformOperator: boolean
  /** Applications are not modeled yet: monthly counts feed the summaries. */
  applicationsThisMonth: number
  applicationsPreviousMonth: number
}

export interface MockSeed {
  companies: CompanyRecord[]
  users: UserRecord[]
  crops: Crop[]
  farms: FarmRecord[]
  owners: OwnerRecord[]
  links: LinkRecord[]
  fields: Field[]
}

type LatLng = { lat: number, lng: number }

const METERS_PER_DEGREE = 111_320

/** Offsets in meters from `center` to a `[lng, lat]` position (6 decimals ≈ 0.1 m). */
function offset(center: LatLng, eastMeters: number, northMeters: number): GeoJsonPosition {
  const lng = center.lng + eastMeters / (METERS_PER_DEGREE * Math.cos((center.lat * Math.PI) / 180))
  const lat = center.lat + northMeters / METERS_PER_DEGREE
  return [Math.round(lng * 1e6) / 1e6, Math.round(lat * 1e6) / 1e6]
}

/**
 * Irregular perimeter whose planar area matches `areaHa`. `shape` holds one
 * radial factor per vertex (evenly spaced angles), giving each farm its own
 * outline while keeping the area coherent with `totalAreaHa`.
 */
function perimeterOf(center: LatLng, areaHa: number, shape: readonly number[]): GeoJsonPolygon {
  const step = (2 * Math.PI) / shape.length
  const shapeArea = 0.5 * Math.sin(step) * shape.reduce((sum, factor, index) => sum + factor * shape[(index + 1) % shape.length]!, 0)
  const radius = Math.sqrt((areaHa * 10_000) / shapeArea)
  const ring = shape.map((factor, index) =>
    offset(center, radius * factor * Math.cos(index * step), radius * factor * Math.sin(index * step)))
  return { type: 'Polygon', coordinates: [[...ring, ring[0]!]] }
}

/** Square field of `areaHa` centered `east`/`north` meters away from the farm center. */
function fieldSquare(center: LatLng, areaHa: number, east: number, north: number): GeoJsonPolygon {
  const half = Math.sqrt(areaHa * 10_000) / 2
  const corners: [number, number][] = [[-half, -half], [half, -half], [half, half], [-half, half], [-half, -half]]
  return { type: 'Polygon', coordinates: [corners.map(([x, y]) => offset(center, east + x, north + y))] }
}

const SEED_DATE = new Date('2026-10-01T12:00:00Z')

export const CROPS: Crop[] = [
  { code: 'coffee_arabica', name: 'Café arábica' },
  { code: 'coffee_conilon', name: 'Café conilon' },
  { code: 'soybean', name: 'Soja' },
  { code: 'corn', name: 'Milho' },
  { code: 'corn_second_season', name: 'Milho safrinha' },
  { code: 'wheat', name: 'Trigo' },
]

function crop(code: string): Crop {
  const found = CROPS.find(item => item.code === code)
  if (!found) throw new Error(`unknown seed crop: ${code}`)
  return found
}

function farm(
  record: Omit<FarmRecord, 'createdAt' | 'updatedAt' | 'location'> & {
    center: LatLng
    shape: readonly number[]
    postalCode: string
    carNumber: string | null
    createdAt?: Date
  },
): FarmRecord {
  const { center, shape, postalCode, carNumber, createdAt = SEED_DATE, ...rest } = record
  return {
    ...rest,
    location: {
      postalCode,
      carNumber,
      forecastPoint: center,
      perimeter: perimeterOf(center, rest.totalAreaHa, shape),
    },
    createdAt,
    updatedAt: createdAt,
  }
}

interface FieldSeed {
  id: string
  name: string
  block: string | null
  areaHa: number
  cropCode: string
  variety: string | null
  plantingDate: string | null
  sprayCondition: Field['sprayCondition']
  lastApplicationDaysAgo: number | null
}

/** Lays the fields out on a grid around the farm center, each sized by its area. */
function fieldsOf(farmId: string, center: LatLng, seeds: FieldSeed[]): Field[] {
  const columns = Math.ceil(Math.sqrt(seeds.length))
  const rows = Math.ceil(seeds.length / columns)
  const cell = Math.max(...seeds.map(seed => Math.sqrt(seed.areaHa * 10_000))) * 1.08
  return seeds.map((seed, index) => {
    const east = ((index % columns) - (columns - 1) / 2) * cell
    const north = ((rows - 1) / 2 - Math.floor(index / columns)) * cell
    const isCoffee = seed.cropCode.startsWith('coffee')
    return {
      id: seed.id,
      farmId,
      name: seed.name,
      block: seed.block,
      areaHa: seed.areaHa,
      crop: crop(seed.cropCode),
      variety: seed.variety,
      season: '2026/27',
      plantingDate: seed.plantingDate,
      rowSpacingM: isCoffee ? 3.5 : 0.5,
      plantSpacingM: isCoffee ? 0.7 : 0.2,
      plantsPerHa: isCoffee ? 4082 : 100000,
      polygon: fieldSquare(center, seed.areaHa, east, north),
      sprayCondition: seed.sprayCondition,
      lastApplicationAt: seed.lastApplicationDaysAgo === null
        ? null
        : new Date(SEED_DATE.getTime() - seed.lastApplicationDaysAgo * 86_400_000),
    }
  })
}

const SANTA_RITA = { lat: -21.3, lng: -46.71 }
const BOA_ESPERANCA = { lat: -21.37, lng: -46.53 }
const OURO_VERDE = { lat: -20.92, lng: -46.99 }
const RECANTO = { lat: -21.43, lng: -45.95 }

/** Builds a fresh seed; each mock instance owns its own copy. */
export function createSeed(): MockSeed {
  return {
    companies: [
      company('company-agentes-agro', 'Agentes Agro', '41234567000155', 'Guaxupé', 'MG', 'pro', { isPlatformOperator: true }),
      company('company-santa-rita', 'Fazenda Santa Rita', '12345678000190', 'Guaxupé', 'MG', 'pro', { applications: [14, 11] }),
      company('company-ouro-verde', 'Fazenda Ouro Verde', '98765432000110', 'São Sebastião do Paraíso', 'MG', 'pro', { applications: [9, 8] }),
      company('company-boa-esperanca', 'Sítio Boa Esperança', '11222333000144', 'Muzambinho', 'MG', 'basic', { applications: [5, 4] }),
      company('company-recanto', 'Fazenda Recanto', '55666777000188', 'Alfenas', 'MG', 'trial', { applications: [3, 3], trialEndsAt: new Date('2026-11-30T12:00:00Z') }),
    ],
    // Test credentials (mock only), all with TEST_PASSWORD: Bruno = super admin,
    // Marina = admin, Carlos = operator. Ana and Beatriz are pending invitations; Helio is inactive.
    users: [
      user('user-bruno', 'Bruno Blauzius', 'bruno@agentesagro.test', 'super_admin', 'company-agentes-agro', { document: '12327891605', primaryFarmId: 'farm-santa-rita', lastAccessHoursAgo: 0 }),
      user('user-marina', 'Marina Rezende', 'marina@fazendasantarita.test', 'admin', 'company-santa-rita', { document: '38544120771', primaryFarmId: 'farm-santa-rita', lastAccessHoursAgo: 2 }),
      user('user-carlos', 'Carlos Mendes', 'carlos.mendes@email.test', 'planning_operator', 'company-santa-rita', { document: '52998224725', primaryFarmId: 'farm-santa-rita', farmIds: ['farm-santa-rita'], lastAccessHoursAgo: 6 }),
      user('user-joao', 'João Silveira', 'joao@ouroverde.test', 'admin', 'company-ouro-verde', { primaryFarmId: 'farm-ouro-verde', lastAccessHoursAgo: 26 }),
      user('user-ana', 'Ana Lúcia Prado', 'ana.prado@email.test', 'planning_operator', 'company-boa-esperanca', { status: 'invited' }),
      user('user-helio', 'Helio Nascimento', 'helio@helioagro.test', 'planning_operator', 'company-recanto', { status: 'inactive', lastAccessHoursAgo: 120 }),
      user('user-paula', 'Paula Lima', 'paula@recanto.test', 'admin', 'company-recanto', { primaryFarmId: 'farm-recanto', lastAccessHoursAgo: 30 }),
      user('user-rafael', 'Rafael Costa', 'rafael@ouroverde.test', 'planning_operator', 'company-ouro-verde', { primaryFarmId: 'farm-ouro-verde', lastAccessHoursAgo: 50 }),
      user('user-beatriz', 'Beatriz Souza', 'beatriz.souza@email.test', 'planning_operator', 'company-santa-rita', { status: 'invited', farmIds: ['farm-santa-rita'] }),
    ],
    crops: CROPS.map(item => ({ ...item })),
    farms: [
      farm({
        companyId: 'company-santa-rita',
        id: 'farm-santa-rita',
        name: 'Fazenda Santa Rita',
        code: 'STR-01',
        cooperative: 'Cooxupé',
        city: 'Guaxupé',
        state: 'MG',
        status: 'active',
        productionType: 'coffee',
        totalAreaHa: 486.2,
        center: SANTA_RITA,
        shape: [1.1, 0.95, 1.2, 0.85, 1, 0.9],
        postalCode: '37800000',
        carNumber: 'MG-3128709-8F2A1B7C9D0E4F21A3B5C7D9E1F3C41E',
        sprayOutlook: { condition: 'suitable', headline: '2 janelas aptas hoje' },
      }),
      farm({
        companyId: 'company-boa-esperanca',
        id: 'farm-boa-esperanca',
        name: 'Sítio Boa Esperança',
        code: 'SBE-01',
        cooperative: 'Cooxupé',
        city: 'Muzambinho',
        state: 'MG',
        status: 'active',
        productionType: 'coffee',
        totalAreaHa: 212,
        center: BOA_ESPERANCA,
        shape: [1, 1.15, 0.8, 1.05, 0.9],
        postalCode: '37890000',
        carNumber: null,
        sprayOutlook: { condition: 'rain', headline: 'Chuva prevista às 14h' },
      }),
      farm({
        companyId: 'company-ouro-verde',
        id: 'farm-ouro-verde',
        name: 'Fazenda Ouro Verde',
        code: 'FOV-01',
        cooperative: null,
        city: 'São Sebastião do Paraíso',
        state: 'MG',
        status: 'pending',
        productionType: 'coffee',
        totalAreaHa: 398,
        center: OURO_VERDE,
        shape: [0.9, 1.2, 1, 0.8, 1.1, 1, 0.85],
        postalCode: '37950000',
        carNumber: null,
        sprayOutlook: { condition: 'attention', headline: 'Vento acima de 12 km/h' },
      }),
      farm({
        companyId: 'company-recanto',
        id: 'farm-recanto',
        name: 'Fazenda Recanto',
        code: 'FRC-01',
        cooperative: null,
        city: 'Alfenas',
        state: 'MG',
        status: 'active',
        productionType: 'grains',
        totalAreaHa: 188,
        center: RECANTO,
        shape: [1.25, 0.8, 1.1, 0.75],
        createdAt: new Date('2026-11-10T12:00:00Z'),
        postalCode: '37130000',
        carNumber: null,
        sprayOutlook: { condition: 'suitable', headline: '1 janela apta amanhã' },
      }),
    ],
    owners: [
      owner('owner-bruno', 'company-santa-rita', 'individual', 'Bruno Blauzius', '12327891605', '35998120001', 'bruno@agentesagro.test', false),
      owner('owner-marina', 'company-santa-rita', 'individual', 'Marina Rezende', '38544120771', '35998120002', 'marina@email.test', false),
      owner('owner-agro-lima', 'company-recanto', 'company', 'Agropecuária Lima Ltda.', '12345678000190', '3532140000', 'contato@agrolima.test', false),
      owner('owner-joao', 'company-ouro-verde', 'individual', 'João Silveira', '27490211583', '35998120004', null, false),
      owner('owner-helena', 'company-boa-esperanca', 'individual', 'Helena Prado', '71533088420', '35998124470', 'helena@email.test', true),
      owner('owner-vale-verde', 'company-recanto', 'company', 'Cooperativa Vale Verde', '98765432000110', '3532990000', 'cadastro@valeverde.test', false),
    ],
    links: [
      { ownerId: 'owner-bruno', farmId: 'farm-santa-rita', role: 'owner', sharePercent: 60 },
      { ownerId: 'owner-bruno', farmId: 'farm-ouro-verde', role: 'owner', sharePercent: 40 },
      { ownerId: 'owner-marina', farmId: 'farm-santa-rita', role: 'owner', sharePercent: 40 },
      { ownerId: 'owner-marina', farmId: 'farm-boa-esperanca', role: 'owner', sharePercent: 50 },
      { ownerId: 'owner-agro-lima', farmId: 'farm-recanto', role: 'owner', sharePercent: 100 },
      { ownerId: 'owner-agro-lima', farmId: 'farm-ouro-verde', role: 'owner', sharePercent: 35 },
      { ownerId: 'owner-joao', farmId: 'farm-ouro-verde', role: 'owner', sharePercent: 25 },
      { ownerId: 'owner-helena', farmId: 'farm-boa-esperanca', role: 'owner', sharePercent: 50 },
      { ownerId: 'owner-vale-verde', farmId: 'farm-recanto', role: 'tenant', sharePercent: null },
    ],
    fields: [
      ...fieldsOf('farm-santa-rita', SANTA_RITA, [
        { id: 'field-str-1', name: 'Talhão 1', block: 'Área Norte', areaHa: 42.1, cropCode: 'coffee_arabica', variety: 'Catuaí 144', plantingDate: '2019-03-15', sprayCondition: 'suitable', lastApplicationDaysAgo: 20 },
        { id: 'field-str-2', name: 'Talhão 2', block: 'Área Norte', areaHa: 51.8, cropCode: 'coffee_arabica', variety: 'Mundo Novo', plantingDate: '2017-11-10', sprayCondition: 'attention', lastApplicationDaysAgo: 8 },
        { id: 'field-str-3', name: 'Talhão 3', block: 'Área Norte', areaHa: 38.6, cropCode: 'coffee_arabica', variety: 'Catuaí 144', plantingDate: '2019-03-20', sprayCondition: 'suitable', lastApplicationDaysAgo: 12 },
        { id: 'field-str-8', name: 'Talhão 8', block: 'Área Norte', areaHa: 29.5, cropCode: 'coffee_arabica', variety: 'Arara', plantingDate: '2021-02-01', sprayCondition: 'suitable', lastApplicationDaysAgo: null },
        { id: 'field-str-4', name: 'Talhão 4', block: 'Área Centro', areaHa: 44, cropCode: 'coffee_arabica', variety: 'Arara', plantingDate: '2020-01-12', sprayCondition: 'unsuitable', lastApplicationDaysAgo: 3 },
        { id: 'field-str-5', name: 'Talhão 5', block: 'Área Centro', areaHa: 56.3, cropCode: 'coffee_arabica', variety: 'Catuaí 144', plantingDate: '2018-10-05', sprayCondition: 'suitable', lastApplicationDaysAgo: 30 },
        { id: 'field-str-6', name: 'Talhão 6', block: 'Área Centro', areaHa: 47.7, cropCode: 'corn_second_season', variety: null, plantingDate: '2026-02-20', sprayCondition: 'rain', lastApplicationDaysAgo: 15 },
        { id: 'field-str-7', name: 'Talhão 7', block: 'Área Sul', areaHa: 55, cropCode: 'coffee_arabica', variety: 'Catuaí 144', plantingDate: '2016-09-01', sprayCondition: 'suitable', lastApplicationDaysAgo: 40 },
        { id: 'field-str-9', name: 'Talhão 9', block: 'Área Sul', areaHa: 47, cropCode: 'coffee_arabica', variety: 'Mundo Novo', plantingDate: '2016-09-01', sprayCondition: 'suitable', lastApplicationDaysAgo: 40 },
      ]),
      ...fieldsOf('farm-boa-esperanca', BOA_ESPERANCA, [
        { id: 'field-sbe-1', name: 'Talhão 1', block: null, areaHa: 61.4, cropCode: 'coffee_arabica', variety: 'Catuaí 99', plantingDate: '2015-10-01', sprayCondition: 'rain', lastApplicationDaysAgo: 10 },
        { id: 'field-sbe-2', name: 'Talhão 2', block: null, areaHa: 58.2, cropCode: 'coffee_arabica', variety: 'Bourbon Amarelo', plantingDate: '2018-11-01', sprayCondition: 'rain', lastApplicationDaysAgo: 10 },
      ]),
      ...fieldsOf('farm-ouro-verde', OURO_VERDE, [
        { id: 'field-fov-1', name: 'Talhão 1', block: 'Sede', areaHa: 72.5, cropCode: 'coffee_arabica', variety: 'Catuaí 144', plantingDate: '2019-01-15', sprayCondition: 'attention', lastApplicationDaysAgo: 6 },
        { id: 'field-fov-2', name: 'Talhão 2', block: 'Sede', areaHa: 64, cropCode: 'coffee_arabica', variety: 'Topázio', plantingDate: '2020-01-15', sprayCondition: 'attention', lastApplicationDaysAgo: 6 },
        { id: 'field-fov-3', name: 'Talhão 3', block: 'Baixada', areaHa: 90.3, cropCode: 'corn', variety: 'AG 8088', plantingDate: '2026-10-20', sprayCondition: 'attention', lastApplicationDaysAgo: null },
      ]),
      ...fieldsOf('farm-recanto', RECANTO, [
        { id: 'field-frc-1', name: 'Talhão 1', block: null, areaHa: 95, cropCode: 'soybean', variety: 'BRS 1074', plantingDate: '2026-10-25', sprayCondition: 'suitable', lastApplicationDaysAgo: null },
        { id: 'field-frc-2', name: 'Talhão 2', block: null, areaHa: 78.6, cropCode: 'corn_second_season', variety: null, plantingDate: '2026-02-28', sprayCondition: 'suitable', lastApplicationDaysAgo: 25 },
      ]),
    ],
  }
}

function owner(
  id: string,
  companyId: string,
  personType: PersonType,
  name: string,
  document: string,
  phone: string,
  email: string | null,
  hasPendingDocuments: boolean,
): OwnerRecord {
  return {
    companyId,
    id,
    personType,
    name,
    document,
    birthDate: personType === 'individual' ? '1979-03-12' : null,
    phone,
    email,
    windowAlertsEnabled: true,
    hasPendingDocuments,
    documents: hasPendingDocuments
      ? []
      : [{ id: `${id}-doc-1`, kind: personType === 'individual' ? 'id_card' : 'articles_of_incorporation', fileName: 'documento.pdf', uploadedAt: SEED_DATE }],
    createdAt: SEED_DATE,
    updatedAt: SEED_DATE,
  }
}

/** Password of every seeded test user (mock only, never a real credential). */
export const TEST_PASSWORD = 'mock-password'
const NOW_REFERENCE = new Date('2026-11-21T12:00:00Z')

function company(
  id: string,
  name: string,
  document: string,
  city: string,
  state: string,
  plan: CompanyPlan,
  extra: { isPlatformOperator?: boolean, applications?: [number, number], trialEndsAt?: Date } = {},
): CompanyRecord {
  return {
    id,
    name,
    document,
    city,
    state,
    plan,
    status: plan === 'trial' ? 'trial' : 'active',
    trialEndsAt: extra.trialEndsAt ?? null,
    createdAt: SEED_DATE,
    isPlatformOperator: extra.isPlatformOperator ?? false,
    applicationsThisMonth: extra.applications?.[0] ?? 0,
    applicationsPreviousMonth: extra.applications?.[1] ?? 0,
  }
}

function user(
  id: string,
  name: string,
  email: string,
  role: UserRole,
  companyId: string,
  extra: {
    document?: string
    status?: UserStatus
    farmIds?: string[]
    primaryFarmId?: string
    lastAccessHoursAgo?: number
  } = {},
): UserRecord {
  const status = extra.status ?? 'active'
  const isInvited = status === 'invited'
  return {
    id,
    name,
    email,
    document: extra.document ?? '',
    password: isInvited ? null : TEST_PASSWORD,
    role,
    companyId,
    farmScope: extra.farmIds ? { mode: 'selected', farmIds: extra.farmIds } : { mode: 'all' },
    status,
    lastAccessAt: extra.lastAccessHoursAgo === undefined ? null : new Date(NOW_REFERENCE.getTime() - extra.lastAccessHoursAgo * 3_600_000),
    invitation: isInvited ? { id: `invitation-${id.replace('user-', '')}`, expiresAt: new Date('2026-11-28T12:00:00Z') } : null,
    primaryFarmId: extra.primaryFarmId ?? null,
    passwordChangedAt: isInvited ? null : new Date('2026-08-20T12:00:00Z'),
  }
}
