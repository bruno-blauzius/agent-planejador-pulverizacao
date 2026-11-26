/**
 * In-memory stand-in of the field-condition endpoints of ADR 0022, phase 2:
 * rain gauges and their readings, soil inspections and the soil profile of a
 * field (`NUXT_PUBLIC_API_MOCK=true`). It applies what the contract states
 * (401, 403 by capability, 404 out of scope, 422 per field) and keeps the
 * records in a store that the trafficability mock reads, so a reading or an
 * inspection recorded here shows up in "Chuva e solo".
 *
 * Nothing here decides a soil state: it only stores what people recorded.
 */
import { ApiError, type FieldError } from '../errors'
import type { FieldService, RainGaugeService, SoilConditionsService } from '../ports'
import type { Permission } from '~/types/auth'
import type { GeoPoint } from '~/types/geo'
import type { PersonRef } from '~/types/planner'
import {
  INSPECTABLE_SOIL_STATES,
  SOIL_TEXTURES,
  type RainGauge,
  type RainReading,
  type SoilInspection,
  type SoilProfile,
  type SoilTexture,
} from '~/types/trafficability'
import { fail, failIfInvalid } from './support'
import {
  MOCK_TRAFFICABILITY_FIXTURES,
  NO_RAIN_SOURCE_FIXTURE,
  WATERLOGGED_CLAY_FIXTURE,
  type MockTrafficabilityFixture,
} from './trafficabilitySeed'

const HOUR_MS = 3_600_000
const DAY_MS = 24 * HOUR_MS

/** Defaults of the API configuration (`API_SOIL_INSPECTION_VALIDITY`, `API_SOIL_GAUGE_MAX_STALENESS`). */
export const MOCK_SOIL_INSPECTION_VALIDITY_MS = 12 * HOUR_MS
export const MOCK_GAUGE_MAX_STALENESS_MS = 36 * HOUR_MS

const GAUGE_NAME_MAX = 100
const NOTE_MAX = 500
const RAIN_MM_MAX = 500
const READINGS_DEFAULT_RANGE_MS = 7 * DAY_MS
const READINGS_MAX_RANGE_MS = 31 * DAY_MS

/** Seed ids, useful in tests and to try the screens. */
export const MOCK_SEDE_GAUGE_ID = 'gauge-str-sede'
export const MOCK_SOUTH_GAUGE_ID = 'gauge-str-sul'

interface ProfileRecord {
  readonly soilTexture: SoilTexture | null
  readonly rainGaugeId: string | null
}

/** What the trafficability mock needs to know about a field, as recorded by people. */
export interface FieldConditionsSnapshot {
  readonly texture: SoilTexture | null
  readonly gauge: { readonly id: string, readonly name: string } | null
  /** Readings of the linked gauge, any age. */
  readonly readings: readonly RainReading[]
  /** Most recent inspection still valid at `now`. */
  readonly validInspection: SoilInspection | null
}

export interface MockFieldConditionsStore {
  readonly gauges: RainGauge[]
  readonly readings: RainReading[]
  readonly inspections: SoilInspection[]
  readonly profiles: Map<string, ProfileRecord>
  readonly nextId: (prefix: string) => string
  /** Profile of a field: what was set, else what its fixture says (no gauge). */
  readonly profileOf: (fieldId: string) => ProfileRecord
  readonly snapshot: (fieldId: string, now: Date) => FieldConditionsSnapshot
}

const SEED_RECORDER: PersonRef = { id: 'user-marina', name: 'Marina Rezende' }

/**
 * Santa Rita has the "Pluviômetro da sede", linked to Talhão 4, with one reading
 * a day for the last 7 days (the measured rain of the `waterloggedClay` example),
 * and a second gauge with no reading yet.
 */
export function createFieldConditionsStore(options: {
  readonly now: Date
  readonly fixtures?: Readonly<Record<string, MockTrafficabilityFixture>>
  readonly fallback?: MockTrafficabilityFixture
}): MockFieldConditionsStore {
  const fixtures = options.fixtures ?? MOCK_TRAFFICABILITY_FIXTURES
  const fallback = options.fallback ?? NO_RAIN_SOURCE_FIXTURE
  const created = new Date(options.now.getTime() - 30 * DAY_MS)
  let sequence = 0

  const gauges: RainGauge[] = [
    { id: MOCK_SEDE_GAUGE_ID, farmId: 'farm-santa-rita', name: 'Pluviômetro da sede', location: null, createdAt: created, updatedAt: created, version: 1 },
    { id: MOCK_SOUTH_GAUGE_ID, farmId: 'farm-santa-rita', name: 'Pluviômetro Área Sul', location: null, createdAt: created, updatedAt: created, version: 1 },
  ]
  // One hour before "now", one per day: the 24/48/72 h and 7-day windows hold 1, 2, 3 and 7 readings.
  const measured = WATERLOGGED_CLAY_FIXTURE.rain?.measuredMm ?? []
  const readings: RainReading[] = measured.map((rainMm, index) => {
    const daysAgo = measured.length - 1 - index
    const observedAt = new Date(options.now.getTime() - daysAgo * DAY_MS - HOUR_MS)
    return { id: `reading-seed-${index + 1}`, rainGaugeId: MOCK_SEDE_GAUGE_ID, rainMm, observedAt, recordedBy: SEED_RECORDER, recordedAt: observedAt }
  })
  const profiles = new Map<string, ProfileRecord>([['field-str-4', { soilTexture: 'clay', rainGaugeId: MOCK_SEDE_GAUGE_ID }]])
  const inspections: SoilInspection[] = []

  function profileOf(fieldId: string): ProfileRecord {
    return profiles.get(fieldId) ?? { soilTexture: (fixtures[fieldId] ?? fallback).texture, rainGaugeId: null }
  }

  return {
    gauges,
    readings,
    inspections,
    profiles,
    nextId: prefix => `${prefix}-mock-${++sequence}`,
    profileOf,
    snapshot(fieldId, now) {
      const profile = profileOf(fieldId)
      const gauge = gauges.find(item => item.id === profile.rainGaugeId) ?? null
      const validInspection = inspections
        .filter(item => item.fieldId === fieldId && item.inspectedAt <= now && item.validUntil > now)
        .sort((a, b) => b.inspectedAt.getTime() - a.inspectedAt.getTime())[0] ?? null
      return {
        texture: profile.soilTexture,
        gauge: gauge ? { id: gauge.id, name: gauge.name } : null,
        readings: gauge ? readings.filter(item => item.rainGaugeId === gauge.id) : [],
        validInspection,
      }
    },
  }
}

export interface MockFieldConditionsOptions {
  /** Scope: the farm is visible when its fields can be listed. */
  readonly fields: Pick<FieldService, 'listByFarm'>
  readonly store: MockFieldConditionsStore
  /** Capability of whoever is calling; rejects with 401 without a session. */
  readonly can: (permission: Permission) => boolean
  /** Who is recorded as the author. */
  readonly actor: () => PersonRef
  readonly now?: () => Date
  /** Simulated latency of each call; 0 in tests. */
  readonly latencyMs?: number
}

function notFound(message: string): never {
  throw new ApiError({ code: 'not_found', message, status: 404 })
}

function isValidPoint(point: GeoPoint | null | undefined): boolean {
  if (point === null || point === undefined) return true
  return Number.isFinite(point.lat) && Number.isFinite(point.lng) && Math.abs(point.lat) <= 90 && Math.abs(point.lng) <= 180
}

function createContext(options: MockFieldConditionsOptions) {
  const now = options.now ?? (() => new Date())

  async function respond<T>(produce: () => T): Promise<T> {
    if (options.latencyMs) await new Promise(resolve => setTimeout(resolve, options.latencyMs))
    return structuredClone(produce())
  }

  /** Checked after the scope, like the API: a hidden farm is 404 even without the capability. */
  function requirePermission(permission: Permission): void {
    if (!options.can(permission)) fail('forbidden', 'você não tem permissão para esta ação')
  }

  /** Fields of a visible farm. A farm out of the operator's scope is hidden like another company's (contract: 404). */
  async function farmFields(farmId: string) {
    try {
      return await options.fields.listByFarm(farmId)
    }
    catch (caught) {
      if (caught instanceof ApiError && caught.code === 'out_of_scope') notFound('fazenda não encontrada')
      throw caught
    }
  }

  async function requireField(farmId: string, fieldId: string): Promise<void> {
    const fields = await farmFields(farmId)
    if (!fields.some(item => item.id === fieldId)) notFound('talhão não encontrado')
  }

  function gaugeOf(farmId: string, gaugeId: string): RainGauge {
    return options.store.gauges.find(item => item.id === gaugeId && item.farmId === farmId) ?? notFound('pluviômetro não encontrado')
  }

  return { now, respond, requirePermission, farmFields, requireField, gaugeOf }
}

function validateGaugeName(store: MockFieldConditionsStore, farmId: string, name: string, exceptId: string | null): FieldError[] {
  const trimmed = name.trim()
  if (!trimmed) return [{ field: 'name', message: 'informe o nome' }]
  if (trimmed.length > GAUGE_NAME_MAX) return [{ field: 'name', message: `use no máximo ${GAUGE_NAME_MAX} caracteres` }]
  const repeated = store.gauges.some(item => item.farmId === farmId && item.id !== exceptId && item.name.toLowerCase() === trimmed.toLowerCase())
  return repeated ? [{ field: 'name', message: 'já existe um pluviômetro com este nome na fazenda' }] : []
}

export function createMockRainGaugeService(options: MockFieldConditionsOptions): RainGaugeService {
  const { store } = options
  const context = createContext(options)

  return {
    async list(farmId) {
      await context.farmFields(farmId)
      return context.respond(() => store.gauges
        .filter(item => item.farmId === farmId)
        .sort((a, b) => a.name.localeCompare(b.name, 'pt-BR')))
    },
    async create(farmId, input) {
      await context.farmFields(farmId)
      context.requirePermission('registry.write')
      return context.respond(() => {
        const details = validateGaugeName(store, farmId, input.name, null)
        if (!isValidPoint(input.location)) details.push({ field: 'location', message: 'localização inválida' })
        failIfInvalid(details)
        const at = context.now()
        const gauge: RainGauge = { id: store.nextId('gauge'), farmId, name: input.name.trim(), location: input.location ?? null, createdAt: at, updatedAt: at, version: 1 }
        store.gauges.push(gauge)
        return gauge
      })
    },
    async update(farmId, gaugeId, patch) {
      await context.farmFields(farmId)
      context.requirePermission('registry.write')
      return context.respond(() => {
        const current = context.gaugeOf(farmId, gaugeId)
        const details = patch.name === undefined ? [] : validateGaugeName(store, farmId, patch.name, gaugeId)
        if (!isValidPoint(patch.location)) details.push({ field: 'location', message: 'localização inválida' })
        failIfInvalid(details)
        const next: RainGauge = {
          ...current,
          name: patch.name === undefined ? current.name : patch.name.trim(),
          location: patch.location === undefined ? current.location : patch.location,
          updatedAt: context.now(),
          version: current.version + 1,
        }
        store.gauges.splice(store.gauges.indexOf(current), 1, next)
        return next
      })
    },
    async listReadings(farmId, gaugeId, range = {}) {
      await context.farmFields(farmId)
      return context.respond(() => {
        context.gaugeOf(farmId, gaugeId)
        const to = range.to ?? context.now()
        const from = range.from ?? new Date(to.getTime() - READINGS_DEFAULT_RANGE_MS)
        if (from > to) fail('validation_failed', 'dados inválidos', [{ field: 'from', message: 'o início não pode ser depois do fim' }])
        if (to.getTime() - from.getTime() > READINGS_MAX_RANGE_MS) fail('validation_failed', 'dados inválidos', [{ field: 'from', message: 'use um intervalo de até 31 dias' }])
        return store.readings
          .filter(item => item.rainGaugeId === gaugeId && item.observedAt > from && item.observedAt <= to)
          .sort((a, b) => b.observedAt.getTime() - a.observedAt.getTime())
      })
    },
    async recordReading(farmId, gaugeId, input) {
      await context.farmFields(farmId)
      context.requirePermission('field_conditions.record')
      return context.respond(() => {
        context.gaugeOf(farmId, gaugeId)
        const at = context.now()
        const details: FieldError[] = []
        if (!Number.isFinite(input.rainMm) || input.rainMm < 0 || input.rainMm > RAIN_MM_MAX) {
          details.push({ field: 'rain_mm', message: `informe de 0 a ${RAIN_MM_MAX} mm` })
        }
        if (Number.isNaN(input.observedAt.getTime()) || input.observedAt > at) {
          details.push({ field: 'observed_at', message: 'o horário da leitura não pode estar no futuro' })
        }
        else if (store.readings.some(item => item.rainGaugeId === gaugeId && item.observedAt.getTime() === input.observedAt.getTime())) {
          details.push({ field: 'observed_at', message: 'já existe uma leitura deste pluviômetro neste horário' })
        }
        failIfInvalid(details)
        const reading: RainReading = { id: store.nextId('reading'), rainGaugeId: gaugeId, rainMm: input.rainMm, observedAt: input.observedAt, recordedBy: options.actor(), recordedAt: at }
        store.readings.push(reading)
        return reading
      })
    },
  }
}

export function createMockSoilConditionsService(options: MockFieldConditionsOptions): SoilConditionsService {
  const { store } = options
  const context = createContext(options)

  function profileView(farmId: string, fieldId: string): SoilProfile {
    const profile = store.profileOf(fieldId)
    const gauge = store.gauges.find(item => item.id === profile.rainGaugeId)
    return { fieldId, farmId, soilTexture: profile.soilTexture, rainGauge: gauge ? { id: gauge.id, name: gauge.name } : null }
  }

  return {
    async recordInspection(farmId, fieldId, input) {
      await context.requireField(farmId, fieldId)
      context.requirePermission('field_conditions.record')
      return context.respond(() => {
        const at = context.now()
        const inspectedAt = input.inspectedAt ?? at
        const note = input.note?.trim() || null
        const details: FieldError[] = []
        if (!(INSPECTABLE_SOIL_STATES as readonly string[]).includes(input.state)) details.push({ field: 'state', message: 'escolha o estado do solo' })
        if (Number.isNaN(inspectedAt.getTime()) || inspectedAt > at) details.push({ field: 'inspected_at', message: 'o horário da vistoria não pode estar no futuro' })
        if (note && note.length > NOTE_MAX) details.push({ field: 'note', message: `use no máximo ${NOTE_MAX} caracteres` })
        failIfInvalid(details)
        const inspection: SoilInspection = {
          id: store.nextId('inspection'),
          fieldId,
          farmId,
          state: input.state,
          inspectedAt,
          validUntil: new Date(inspectedAt.getTime() + MOCK_SOIL_INSPECTION_VALIDITY_MS),
          inspectedBy: options.actor(),
          note,
        }
        store.inspections.push(inspection)
        return inspection
      })
    },
    async getProfile(farmId, fieldId) {
      await context.requireField(farmId, fieldId)
      return context.respond(() => profileView(farmId, fieldId))
    },
    async updateProfile(farmId, fieldId, input) {
      await context.requireField(farmId, fieldId)
      context.requirePermission('registry.write')
      return context.respond(() => {
        const details: FieldError[] = []
        if (input.soilTexture !== null && !(SOIL_TEXTURES as readonly string[]).includes(input.soilTexture)) {
          details.push({ field: 'soil_texture', message: 'textura desconhecida' })
        }
        if (input.rainGaugeId !== null && !store.gauges.some(item => item.id === input.rainGaugeId && item.farmId === farmId)) {
          details.push({ field: 'rain_gauge_id', message: 'escolha um pluviômetro desta fazenda' })
        }
        failIfInvalid(details)
        store.profiles.set(fieldId, { soilTexture: input.soilTexture, rainGaugeId: input.rainGaugeId })
        return profileView(farmId, fieldId)
      })
    },
  }
}
