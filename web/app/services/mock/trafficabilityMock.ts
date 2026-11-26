/**
 * In-memory stand-in of `GET .../trafficability` (ADR 0010, 0022). It lists the
 * fields through a `FieldService`, so company/farm scope is whatever that
 * service enforces (404 for another company, 403 out of the operator's farms),
 * with the mock fields or with the real API's (`NUXT_PUBLIC_API_MOCK_PENDING`).
 *
 * It does NOT reimplement the soil rule: each field answers a hand-written
 * fixture (`trafficabilitySeed.ts`), only placed on today's calendar and cut to
 * the requested horizon. The real values come from `services/api`. With a
 * conditions store (phase 2) the gauge readings, the soil texture and a valid
 * inspection recorded in the mock are laid over the fixture (`withConditions`).
 */
import { ApiError } from '../errors'
import type { FieldService, TrafficabilityService } from '../ports'
import {
  DEFAULT_TRAFFICABILITY_DAYS,
  type CalendarDay,
  type DailyRainEntry,
  type FieldTrafficability,
  type MethodRelease,
  type RainAccumulation,
  type RainReading,
  type TrafficabilityDay,
  TRAFFICABILITY_DATA_ISSUES,
} from '~/types/trafficability'
import { DISPLAY_TIME_ZONE } from '~/utils/timeZone'
import { MOCK_GAUGE_MAX_STALENESS_MS, type FieldConditionsSnapshot, type MockFieldConditionsStore } from './fieldConditionsMock'
import { fail } from './support'
import {
  MOCK_TRAFFICABILITY_FIXTURES,
  MOCK_TRAFFICABILITY_MAX_DAYS,
  NO_RAIN_SOURCE_FIXTURE,
  type MockRainSeed,
  type MockTrafficabilityFixture,
} from './trafficabilitySeed'

export interface MockTrafficabilityOptions {
  readonly fields: Pick<FieldService, 'listByFarm'>
  readonly now?: () => Date
  /** Simulated latency of each call; 0 in tests. */
  readonly latencyMs?: number
  /** Fixture by field id; defaults to `MOCK_TRAFFICABILITY_FIXTURES`. */
  readonly fixtures?: Readonly<Record<string, MockTrafficabilityFixture>>
  /** Answer of the fields without a fixture; defaults to "no rain source". */
  readonly fallback?: MockTrafficabilityFixture
  /** Readings, inspections and profiles recorded in the mock (phase 2); without it, the fixtures alone. */
  readonly conditions?: Pick<MockFieldConditionsStore, 'snapshot'>
}

const DAY_MS = 86_400_000
const MEASURED_DAY_COUNT = 7

/** Today's calendar day in the display zone, `YYYY-MM-DD` (`en-CA` formats it that way). */
function calendarDayOf(instant: Date): CalendarDay {
  return new Intl.DateTimeFormat('en-CA', { timeZone: DISPLAY_TIME_ZONE, year: 'numeric', month: '2-digit', day: '2-digit' }).format(instant)
}

function shiftDay(day: CalendarDay, offset: number): CalendarDay {
  return new Date(Date.parse(`${day}T00:00:00Z`) + offset * DAY_MS).toISOString().slice(0, 10)
}

function validateDays(days: number): void {
  if (Number.isInteger(days) && days >= 1 && days <= MOCK_TRAFFICABILITY_MAX_DAYS) return
  fail('validation_failed', 'dados inválidos', [{ field: 'days', message: `informe de 1 a ${MOCK_TRAFFICABILITY_MAX_DAYS} dias` }])
}

function accumulationOf(rain: MockRainSeed, observedAt: Date): RainAccumulation {
  return {
    last24hMm: rain.last24hMm,
    last48hMm: rain.last48hMm,
    last72hMm: rain.last72hMm,
    last7dMm: rain.last7dMm,
    source: rain.source,
    sourceName: rain.sourceName,
    observedAt,
  }
}

function dailyRainOf(rain: MockRainSeed | null, today: CalendarDay, days: number): DailyRainEntry[] {
  if (!rain) return []
  const measured = rain.measuredMm.slice(-MEASURED_DAY_COUNT).map((rainMm, index, all): DailyRainEntry => ({
    date: shiftDay(today, index - (all.length - 1)),
    rainMm,
    kind: 'measured',
    source: rain.source,
  }))
  const forecast = rain.forecastMm.slice(0, days).map((rainMm, index): DailyRainEntry => ({
    date: shiftDay(today, index + 1),
    rainMm,
    kind: 'forecast',
    source: 'model',
  }))
  return [...measured, ...forecast]
}

function daysOf(fixture: MockTrafficabilityFixture, today: CalendarDay, days: number): TrafficabilityDay[] {
  return fixture.dayStates.slice(0, days + 1).map((soilState, offset) => ({
    date: shiftDay(today, offset),
    soilState,
    methods: fixture.methodsByState[soilState] ?? [],
  }))
}

function releasesOf(fixture: MockTrafficabilityFixture, today: CalendarDay, days: number): MethodRelease[] {
  return fixture.releases.map(release => ({
    method: release.method,
    requiredState: release.requiredState,
    // Beyond the requested horizon the API answers "no release foreseen".
    date: release.dayOffset !== null && release.dayOffset <= days ? shiftDay(today, release.dayOffset) : null,
  }))
}

function buildView(fixture: MockTrafficabilityFixture, ids: { farmId: string, fieldId: string }, now: Date, days: number): FieldTrafficability {
  const today = calendarDayOf(now)
  return {
    fieldId: ids.fieldId,
    farmId: ids.farmId,
    asOf: now,
    soil: {
      state: fixture.state,
      basis: fixture.basis,
      texture: fixture.texture,
      thresholdsSource: fixture.thresholdsSource,
      rain: fixture.rain ? accumulationOf(fixture.rain, now) : null,
      modelMoisture: fixture.modelMoisture,
      inspection: null,
    },
    rainDaily: dailyRainOf(fixture.rain, today, days),
    days: daysOf(fixture, today, days),
    releases: releasesOf(fixture, today, days),
    dataIssues: fixture.dataIssues,
  }
}

// ---- what people recorded (phase 2): numbers and the inspection, never a calculated state ----

const HOUR_MS = 3_600_000
const WINDOW_HOURS = { last24hMm: 24, last48hMm: 48, last72hMm: 72, last7dMm: 168 } as const

function sumSince(readings: readonly RainReading[], now: Date, hours: number): number {
  const from = now.getTime() - hours * HOUR_MS
  return readings
    .filter(item => item.observedAt.getTime() > from && item.observedAt <= now)
    .reduce((sum, item) => sum + item.rainMm, 0)
}

/** Readings of the linked gauge, when the latest one is recent enough for the API to use the gauge. */
function recentReadings(snapshot: FieldConditionsSnapshot, now: Date): readonly RainReading[] | null {
  const past = snapshot.readings.filter(item => item.observedAt <= now)
  const latest = Math.max(...past.map(item => item.observedAt.getTime()))
  return snapshot.gauge && past.length && now.getTime() - latest < MOCK_GAUGE_MAX_STALENESS_MS ? past : null
}

function gaugeAccumulation(snapshot: FieldConditionsSnapshot, readings: readonly RainReading[], now: Date): RainAccumulation {
  const latest = readings.reduce((last, item) => (item.observedAt > last ? item.observedAt : last), readings[0]!.observedAt)
  return {
    last24hMm: sumSince(readings, now, WINDOW_HOURS.last24hMm),
    last48hMm: sumSince(readings, now, WINDOW_HOURS.last48hMm),
    last72hMm: sumSince(readings, now, WINDOW_HOURS.last72hMm),
    last7dMm: sumSince(readings, now, WINDOW_HOURS.last7dMm),
    source: 'gauge',
    sourceName: snapshot.gauge?.name ?? null,
    observedAt: latest,
  }
}

function gaugeDailyRain(readings: readonly RainReading[], today: CalendarDay): DailyRainEntry[] {
  return Array.from({ length: MEASURED_DAY_COUNT }, (_, index): DailyRainEntry => {
    const date = shiftDay(today, index - (MEASURED_DAY_COUNT - 1))
    const rainMm = readings.filter(item => calendarDayOf(item.observedAt) === date).reduce((sum, item) => sum + item.rainMm, 0)
    return { date, rainMm, kind: 'measured', source: 'gauge' }
  })
}

/**
 * Applies what people recorded on top of the fixture. Allowed: the numbers of
 * the gauge readings, the texture of the profile and a valid inspection, which
 * replaces today's state (the API does the same). Not done: recalculating the
 * state from rain or texture, the projection or the releases; they stay as in
 * the fixture. Today's verdicts after an inspection are the fixture's seeded
 * verdicts for that state.
 */
function withConditions(view: FieldTrafficability, fixture: MockTrafficabilityFixture, snapshot: FieldConditionsSnapshot, now: Date): FieldTrafficability {
  const today = calendarDayOf(now)
  const readings = recentReadings(snapshot, now)
  const issues = new Set(view.dataIssues)
  if (readings) issues.delete('no_rain_source')
  if (snapshot.texture === null) issues.add('no_soil_texture')
  else issues.delete('no_soil_texture')

  const inspection = snapshot.validInspection
  const forecast = view.rainDaily.filter(entry => entry.kind === 'forecast')
  const days = inspection
    ? view.days.map((day, index) => (index === 0
        ? { ...day, soilState: inspection.state, methods: fixture.methodsByState[inspection.state] ?? day.methods }
        : day))
    : view.days

  return {
    ...view,
    soil: {
      ...view.soil,
      state: inspection?.state ?? view.soil.state,
      basis: inspection ? 'inspection' : view.soil.basis,
      texture: snapshot.texture,
      rain: readings ? gaugeAccumulation(snapshot, readings, now) : view.soil.rain,
      inspection: inspection
        ? { state: inspection.state, inspectedAt: inspection.inspectedAt, validUntil: inspection.validUntil, inspectedBy: inspection.inspectedBy }
        : null,
    },
    rainDaily: readings ? [...gaugeDailyRain(readings, today), ...forecast] : view.rainDaily,
    days,
    dataIssues: TRAFFICABILITY_DATA_ISSUES.filter(issue => issues.has(issue)),
  }
}

export function createMockTrafficabilityService(options: MockTrafficabilityOptions): TrafficabilityService {
  const now = options.now ?? (() => new Date())
  const fixtures = options.fixtures ?? MOCK_TRAFFICABILITY_FIXTURES
  const fallback = options.fallback ?? NO_RAIN_SOURCE_FIXTURE

  async function respond<T>(produce: () => T): Promise<T> {
    if (options.latencyMs) await new Promise(resolve => setTimeout(resolve, options.latencyMs))
    return structuredClone(produce())
  }

  return {
    async getFieldTrafficability(farmId, fieldId, days = DEFAULT_TRAFFICABILITY_DAYS) {
      // Scope (401/403/404) is the field service's; a field of another farm is simply not in the list.
      const fields = await options.fields.listByFarm(farmId)
      if (!fields.some(item => item.id === fieldId)) {
        throw new ApiError({ code: 'not_found', message: 'talhão não encontrado', status: 404 })
      }
      validateDays(days)
      return respond(() => {
        const fixture = fixtures[fieldId] ?? fallback
        const at = now()
        const view = buildView(fixture, { farmId, fieldId }, at, days)
        return options.conditions ? withConditions(view, fixture, options.conditions.snapshot(fieldId, at), at) : view
      })
    },
  }
}
