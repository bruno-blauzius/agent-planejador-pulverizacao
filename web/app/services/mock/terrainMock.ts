/**
 * In-memory stand-in of the Terrain endpoints (ADR 0010, 0021). It lists the
 * fields through a `FieldService`, so it works with the mock fields and with
 * the real API's fields (`NUXT_PUBLIC_API_MOCK_PENDING`).
 *
 * Everything here imitates the server so the screen gets plausible data: the
 * class, the method fit and the "outdated adjustment" flag below are a double
 * of what the API computes. They are NOT rules of the front; the real values
 * come from `services/api` (ADR 0021) and the front only presents them.
 *
 * Values are deterministic: the same field id always gets the same relief.
 * A requested calculation is asynchronous: `calculate` answers with progress and
 * each `getFarm` finishes one or two more fields, until `progress` is null.
 */
import { ApiError, type FieldError } from '../errors'
import type { FieldService, TerrainService } from '../ports'
import type { Field } from '~/types/field'
import type { PersonRef } from '~/types/planner'
import {
  SLOPE_CLASSES,
  TERRAIN_ASPECTS,
  type ElevationProfilePoint,
  type FarmTerrain,
  type FieldTerrain,
  type FieldTerrainAdjustmentInput,
  type FieldTerrainSummary,
  type MethodFit,
  type MethodFitStatus,
  type SlopeClass,
  type SlopeClassArea,
  type SprayMethod,
  type TerrainAdjustment,
  type TerrainAspect,
  type TerrainCalculation,
  type TerrainCalculationScope,
  type TerrainMeasurement,
  type TerrainPosition,
  type TerrainStatus,
} from '~/types/terrain'
import { SPRAY_METHODS } from '~/utils/terrain'
import { fail, failIfInvalid, roundOneDecimal } from './support'

/** Optional hand-made values for a field of the mock (anything missing is generated). */
export interface MockTerrainFieldSeed {
  readonly slopeMeanPct?: number
  readonly slopeMaxPct?: number
  readonly position?: TerrainPosition
  readonly aspect?: TerrainAspect
  readonly elevationMinM?: number
  readonly elevationMaxM?: number
  /** Share of the area (%) per class, in `SLOPE_CLASSES` order. */
  readonly classShares?: readonly number[]
  /** The calculation of this field fails with this reason (never a made-up value). */
  readonly failureReason?: string
  /** A manual adjustment that already exists when the farm is first read. */
  readonly adjustment?: {
    readonly origin: TerrainAdjustment['origin']
    readonly slopeMeanPct: number
    readonly slopeMaxPct: number
    readonly position: TerrainPosition
    readonly justification: string
    readonly adjustedBy: PersonRef
  }
}

export interface MockTerrainOptions {
  readonly fields: Pick<FieldService, 'listByFarm'>
  /** `registry.write` of whoever is calling; read at each write. */
  readonly canWrite: () => boolean
  readonly now?: () => Date
  /** Simulated latency of each call; 0 in tests. */
  readonly latencyMs?: number
  /** Hand-made values by field id. */
  readonly overrides?: Readonly<Record<string, MockTerrainFieldSeed>>
  /** Farms that already have relief calculated when first read; every other farm starts "not calculated". */
  readonly initiallyCalculatedFarmIds?: readonly string[]
  /** Who is recorded in `adjusted_by`. */
  readonly actor?: () => PersonRef
}

// ---- double of the API's computations (defaults of openapi.yaml) --------------------

/** Lower bound (%) of each class, in `SLOPE_CLASSES` order; the last class goes to 100 %. */
const CLASS_LOWER_BOUNDS_PCT: readonly number[] = [0, 3, 8, 20, 45]
/** Slope limit (%) per method; `null` = not limited by slope. */
const METHOD_SLOPE_LIMITS_PCT: Readonly<Record<SprayMethod, number | null>> = {
  knapsack: null,
  tractor_boom: 20,
  self_propelled: 12,
  airblast: 25,
  aircraft: null,
  drone: null,
}
/** Share of the area above the limit from which the method is `unsuitable`. */
const UNSUITABLE_AREA_SHARE_PCT = 80
/** Gap between calculated and adjusted mean slope that flags the adjustment as outdated. */
const OUTDATED_ADJUSTMENT_GAP_PCT = 5
const SEED_CALCULATION_AGE_DAYS = 9
const DAY_MS = 86_400_000
const PROFILE_POINT_COUNT = 7

const ELEVATION_MODEL = { source: 'Copernicus DEM GLO-30', resolutionM: 30, version: '2023_1' } as const
const MOCK_ACTOR: PersonRef = { id: 'mock-user', name: 'Usuário (simulado)' }

function classOf(meanPct: number): SlopeClass {
  const index = CLASS_LOWER_BOUNDS_PCT.reduce((found, bound, current) => (meanPct >= bound ? current : found), 0)
  return SLOPE_CLASSES[index]!
}

/** FNV-1a: same text, same number. */
function hashOf(text: string): number {
  let hash = 2166136261
  for (let index = 0; index < text.length; index++) {
    hash ^= text.charCodeAt(index)
    hash = Math.imul(hash, 16777619) >>> 0
  }
  return hash
}

/** Deterministic value in [0, 1) for a field and a purpose. */
function unit(fieldId: string, purpose: string): number {
  return (hashOf(`${fieldId}:${purpose}`) % 10_000) / 10_000
}

function defaultShares(meanPct: number): number[] {
  const center = SLOPE_CLASSES.indexOf(classOf(meanPct))
  const weights: number[] = SLOPE_CLASSES.map((_, index) => (index === center ? 60 : Math.abs(index - center) === 1 ? 20 : 0))
  const total = weights.reduce((sum, weight) => sum + weight, 0)
  const shares = weights.map(weight => Math.round((weight / total) * 100))
  shares[center] = (shares[center] ?? 0) + (100 - shares.reduce((sum, share) => sum + share, 0))
  return shares
}

function buildProfile(maxM: number, reliefM: number, lengthM: number, bend: number): ElevationProfilePoint[] {
  return Array.from({ length: PROFILE_POINT_COUNT }, (_, index) => {
    const t = index / (PROFILE_POINT_COUNT - 1)
    return { distanceM: Math.round(lengthM * t), elevationM: Math.round(maxM - reliefM * Math.pow(t, bend)) }
  })
}

function buildCalculation(field: Field, seed: MockTerrainFieldSeed | undefined, calculatedAt: Date): TerrainCalculation {
  const random = (purpose: string) => unit(field.id, purpose)
  const mean = seed?.slopeMeanPct ?? roundOneDecimal(1 + random('mean') * random('mean') * 32)
  const max = seed?.slopeMaxPct ?? roundOneDecimal(Math.min(100, mean * (1.4 + random('max') * 0.8) + 2))
  const slopeClass = classOf(mean)
  const aspect = seed?.aspect ?? (slopeClass === 'flat' ? 'flat' : TERRAIN_ASPECTS[Math.floor(random('aspect') * 8)]!)
  const draw = random('position')
  const position: TerrainPosition = seed?.position ?? (draw < 0.25 ? 'hilltop' : draw < 0.8 ? 'slope' : 'lowland')
  const elevationMinM = seed?.elevationMinM ?? Math.round(700 + random('elevation') * 400)
  const elevationMaxM = seed?.elevationMaxM ?? elevationMinM + Math.round(mean * 6 + 12 + random('relief') * 20)
  const reliefM = elevationMaxM - elevationMinM
  const shares = seed?.classShares ?? defaultShares(mean)
  const classAreas: SlopeClassArea[] = SLOPE_CLASSES.map((key, index) => ({
    slopeClass: key,
    sharePct: shares[index] ?? 0,
    areaHa: roundOneDecimal((field.areaHa * (shares[index] ?? 0)) / 100),
  }))
  return {
    measurement: {
      slopeMeanPct: mean,
      slopeP90Pct: roundOneDecimal(mean + (max - mean) * 0.55),
      slopeMaxPct: max,
      slopeClass,
      position,
      aspect,
    },
    elevationMinM,
    elevationMaxM,
    classAreas,
    profile: buildProfile(elevationMaxM, reliefM, Math.round(150 + random('length') * 400), 1.2 + random('bend') * 0.6),
    elevationModel: { ...ELEVATION_MODEL },
    calculatedAt,
  }
}

function areaAboveLimit(classAreas: readonly SlopeClassArea[], limitPct: number): number {
  const total = classAreas.reduce((sum, item) => {
    const index = SLOPE_CLASSES.indexOf(item.slopeClass)
    const low = CLASS_LOWER_BOUNDS_PCT[index] ?? 0
    const high = CLASS_LOWER_BOUNDS_PCT[index + 1] ?? 100
    const overlap = Math.min(1, Math.max(0, (high - Math.max(low, limitPct)) / (high - low)))
    return sum + item.sharePct * overlap
  }, 0)
  return Math.round(total)
}

function buildFit(method: SprayMethod, measurement: TerrainMeasurement, classAreas: readonly SlopeClassArea[] | null): MethodFit {
  const limit = METHOD_SLOPE_LIMITS_PCT[method]
  if (limit === null) return { method, slopeLimitPct: null, limitSource: 'reference', status: 'compatible', areaAboveLimitPct: null }
  let status: MethodFitStatus = 'compatible'
  let above: number | null = null
  if (classAreas) {
    above = areaAboveLimit(classAreas, limit)
    status = above >= UNSUITABLE_AREA_SHARE_PCT ? 'unsuitable' : above > 0 ? 'restricted' : 'compatible'
  }
  else if (measurement.slopeMeanPct > limit) status = 'unsuitable'
  else if (measurement.slopeMaxPct > limit) status = 'restricted'
  return { method, slopeLimitPct: limit, limitSource: 'reference', status, areaAboveLimitPct: above }
}

// ---- state ----------------------------------------------------------------------------

interface FieldState {
  status: TerrainStatus
  failureReason: string | null
  calculation: TerrainCalculation | null
  adjustment: TerrainAdjustment | null
}

interface Run {
  readonly queue: string[]
  readonly total: number
  done: number
}

const notCalculated = (): FieldState => ({ status: 'not_calculated', failureReason: null, calculation: null, adjustment: null })

function effectiveOf(state: FieldState): { measurement: TerrainMeasurement, source: 'calculated' | 'manual' } | null {
  if (state.adjustment) return { measurement: state.adjustment.measurement, source: 'manual' }
  return state.calculation ? { measurement: state.calculation.measurement, source: 'calculated' } : null
}

function fitsOf(state: FieldState): MethodFit[] {
  const effective = effectiveOf(state)
  if (!effective) return []
  const distribution = effective.source === 'calculated' ? (state.calculation?.classAreas ?? null) : null
  return SPRAY_METHODS.map(method => buildFit(method, effective.measurement, distribution))
}

function validateAdjustment(input: FieldTerrainAdjustmentInput): void {
  const details: FieldError[] = []
  if (input.origin !== 'topographic_survey' && input.origin !== 'field_assessment') {
    details.push({ field: 'origin', message: 'origem desconhecida' })
  }
  const inRange = (value: number) => Number.isFinite(value) && value >= 0 && value <= 100
  if (!inRange(input.slopeMeanPct)) details.push({ field: 'slope_mean_pct', message: 'informe um valor entre 0 e 100' })
  if (!inRange(input.slopeMaxPct)) details.push({ field: 'slope_max_pct', message: 'informe um valor entre 0 e 100' })
  else if (inRange(input.slopeMeanPct) && input.slopeMaxPct < input.slopeMeanPct) {
    details.push({ field: 'slope_max_pct', message: 'a declividade máxima não pode ser menor que a média' })
  }
  if (!['hilltop', 'slope', 'lowland'].includes(input.position)) details.push({ field: 'position', message: 'posição desconhecida' })
  const justification = input.justification.trim()
  if (!justification) details.push({ field: 'justification', message: 'informe a justificativa' })
  else if (justification.length > 1000) details.push({ field: 'justification', message: 'use no máximo 1000 caracteres' })
  failIfInvalid(details)
}

export function createMockTerrainService(options: MockTerrainOptions): TerrainService {
  const now = options.now ?? (() => new Date())
  const overrides = options.overrides ?? {}
  const calculatedFarmIds = new Set(options.initiallyCalculatedFarmIds ?? [])
  const actor = options.actor ?? (() => MOCK_ACTOR)
  const states = new Map<string, FieldState>()
  const initializedFarms = new Set<string>()
  const runs = new Map<string, Run>()
  let advanceCount = 0

  const stateOf = (fieldId: string): FieldState => {
    let state = states.get(fieldId)
    if (!state) {
      state = notCalculated()
      states.set(fieldId, state)
    }
    return state
  }

  function applyCalculation(field: Field, calculatedAt: Date): void {
    const state = stateOf(field.id)
    const seed = overrides[field.id]
    if (seed?.failureReason) {
      Object.assign(state, { status: 'failed', failureReason: seed.failureReason, calculation: null })
      return
    }
    Object.assign(state, { status: 'ready', failureReason: null, calculation: buildCalculation(field, seed, calculatedAt) })
  }

  /** The first read of a farm decides what it starts with; fields created later start "not calculated". */
  function ensureFarm(farmId: string, fields: readonly Field[]): void {
    if (initializedFarms.has(farmId)) return
    initializedFarms.add(farmId)
    const calculatedAt = new Date(now().getTime() - SEED_CALCULATION_AGE_DAYS * DAY_MS)
    for (const field of fields) {
      if (calculatedFarmIds.has(farmId)) applyCalculation(field, calculatedAt)
      const adjustment = overrides[field.id]?.adjustment
      if (calculatedFarmIds.has(farmId) && adjustment) {
        stateOf(field.id).adjustment = {
          origin: adjustment.origin,
          measurement: manualMeasurement(adjustment),
          justification: adjustment.justification,
          adjustedBy: adjustment.adjustedBy,
          adjustedAt: new Date(now().getTime() - 2 * DAY_MS),
        }
      }
    }
  }

  function manualMeasurement(input: Pick<FieldTerrainAdjustmentInput, 'slopeMeanPct' | 'slopeMaxPct' | 'position'>): TerrainMeasurement {
    return {
      slopeMeanPct: input.slopeMeanPct,
      slopeP90Pct: null,
      slopeMaxPct: input.slopeMaxPct,
      slopeClass: classOf(input.slopeMeanPct),
      position: input.position,
      aspect: null,
    }
  }

  /** Finishes one or two queued fields per read, as a background job would. */
  function advance(farmId: string, fields: readonly Field[]): void {
    const run = runs.get(farmId)
    if (!run) return
    const step = 1 + (advanceCount++ % 2)
    for (let taken = 0; taken < step; taken++) {
      const id = run.queue.shift()
      if (id === undefined) break
      const field = fields.find(item => item.id === id)
      if (field) applyCalculation(field, now())
      run.done += 1
    }
    const next = run.queue[0]
    if (next === undefined) runs.delete(farmId)
    else stateOf(next).status = 'calculating'
  }

  function summaryOf(field: Field): FieldTerrainSummary {
    const state = stateOf(field.id)
    const effective = effectiveOf(state)
    return {
      fieldId: field.id,
      fieldName: field.name,
      block: field.block,
      areaHa: field.areaHa,
      status: state.status,
      failureReason: state.failureReason,
      source: effective?.source ?? null,
      measurement: effective?.measurement ?? null,
      elevationMinM: state.calculation?.elevationMinM ?? null,
      elevationMaxM: state.calculation?.elevationMaxM ?? null,
      hasRestrictedMethod: fitsOf(state).some(fit => fit.status !== 'compatible'),
    }
  }

  function farmView(farmId: string, fields: readonly Field[]): FarmTerrain {
    const items = fields.map(summaryOf)
    const withRelief = items.filter(item => item.measurement !== null)
    const area = withRelief.reduce((sum, item) => sum + item.areaHa, 0)
    const withElevation = items.filter(item => item.elevationMinM !== null && item.elevationMaxM !== null)
    const dates = fields
      .map(field => states.get(field.id)?.calculation?.calculatedAt.getTime())
      .filter((time): time is number => time !== undefined)
    const run = runs.get(farmId)
    return {
      farmId,
      summary: {
        fieldCount: fields.length,
        calculatedFieldCount: withRelief.length,
        slopeMeanPct: withRelief.length && area > 0
          ? roundOneDecimal(withRelief.reduce((sum, item) => sum + (item.measurement?.slopeMeanPct ?? 0) * item.areaHa, 0) / area)
          : null,
        restrictedFieldCount: items.filter(item => item.hasRestrictedMethod).length,
        elevationMinM: withElevation.length ? Math.min(...withElevation.map(item => item.elevationMinM ?? 0)) : null,
        elevationMaxM: withElevation.length ? Math.max(...withElevation.map(item => item.elevationMaxM ?? 0)) : null,
        calculatedAt: dates.length ? new Date(Math.max(...dates)) : null,
      },
      progress: run ? { total: run.total, done: run.done } : null,
      items,
    }
  }

  function fieldView(farmId: string, field: Field): FieldTerrain {
    const state = stateOf(field.id)
    const effective = effectiveOf(state)
    const gap = state.adjustment && state.calculation
      ? Math.abs(state.adjustment.measurement.slopeMeanPct - state.calculation.measurement.slopeMeanPct)
      : 0
    return {
      fieldId: field.id,
      farmId,
      status: state.status,
      failureReason: state.failureReason,
      source: effective?.source ?? null,
      effective: effective?.measurement ?? null,
      calculated: state.calculation,
      adjustment: state.adjustment,
      adjustmentOutdated: gap > OUTDATED_ADJUSTMENT_GAP_PCT,
      methodFits: fitsOf(state),
    }
  }

  async function respond<T>(produce: () => T): Promise<T> {
    if (options.latencyMs) await new Promise(resolve => setTimeout(resolve, options.latencyMs))
    return structuredClone(produce())
  }

  function requireWrite(): void {
    if (!options.canWrite()) fail('forbidden', 'você não tem permissão para esta ação')
  }

  async function findField(farmId: string, fieldId: string): Promise<{ fields: readonly Field[], field: Field }> {
    const fields = await options.fields.listByFarm(farmId)
    const field = fields.find(item => item.id === fieldId)
    if (!field) throw new ApiError({ code: 'not_found', message: 'talhão não encontrado', status: 404 })
    ensureFarm(farmId, fields)
    return { fields, field }
  }

  return {
    // On purpose: reading the farm advances a running calculation, standing in for the server finishing it in the background.
    async getFarm(farmId) {
      const fields = await options.fields.listByFarm(farmId)
      ensureFarm(farmId, fields)
      return respond(() => {
        advance(farmId, fields)
        return farmView(farmId, fields)
      })
    },
    async calculate(farmId, scope: TerrainCalculationScope) {
      requireWrite()
      const fields = await options.fields.listByFarm(farmId)
      ensureFarm(farmId, fields)
      return respond(() => {
        // A repeated request while running does not duplicate the work.
        if (!runs.has(farmId)) {
          const targets = fields.filter(field => scope === 'all' || ['not_calculated', 'pending', 'failed'].includes(stateOf(field.id).status))
          if (targets.length) {
            for (const field of targets) Object.assign(stateOf(field.id), { status: 'pending', failureReason: null })
            runs.set(farmId, { queue: targets.map(field => field.id), total: targets.length, done: 0 })
            stateOf(targets[0]!.id).status = 'calculating'
          }
        }
        return farmView(farmId, fields)
      })
    },
    async getField(farmId, fieldId) {
      const { field } = await findField(farmId, fieldId)
      return respond(() => fieldView(farmId, field))
    },
    async adjust(farmId, fieldId, input) {
      requireWrite()
      const { field } = await findField(farmId, fieldId)
      return respond(() => {
        validateAdjustment(input)
        stateOf(field.id).adjustment = {
          origin: input.origin,
          measurement: manualMeasurement(input),
          justification: input.justification.trim(),
          adjustedBy: actor(),
          adjustedAt: now(),
        }
        return fieldView(farmId, field)
      })
    },
    async removeAdjustment(farmId, fieldId) {
      requireWrite()
      const { field } = await findField(farmId, fieldId)
      return respond(() => {
        const state = stateOf(field.id)
        if (!state.adjustment) fail('not_found', 'o talhão não tem ajuste manual')
        state.adjustment = null
        return fieldView(farmId, field)
      })
    },
  }
}
