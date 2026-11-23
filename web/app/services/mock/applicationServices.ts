/**
 * In-memory application history (screens 19 and 20, ADR 0016). The store is shared
 * with the planner mock: an application registered after the human confirmation
 * shows up in the list. Seed mirrors the Figma list (T3 and T7 planned, T5, T1 and
 * T8 completed, T4 cancelled) with dates relative to the mock clock. Totals and
 * labels are computed here the way the API will; nothing is computed in the front.
 */
import type { ApplicationService, WeatherService } from '../ports'
import type {
  ApplicationDetail,
  ApplicationFilters,
  ApplicationMonthSummary,
  ApplicationPage,
  ApplicationProduct,
  ApplicationStatus,
  ApplicationSummary,
  ApplicationConditionsSummary,
  ApplicationWindow,
  CurrentWeather,
  RecordedConditions,
} from '~/types/application'
import type { PersonRef } from '~/types/planner'
import { fieldShortLabel } from '~/utils/farmDetail'
import { addDaysIso, zonedIsoDate } from '~/utils/planner'
import { canSeeFarm, farmAccess } from './access'
import type { AdminMockContext } from './adminServices'
import type { PlannerFaults } from './plannerServices'
import type { FarmRecord } from './seed'
import { fail, paginate } from './support'

export interface ApplicationRecord {
  id: string
  farmId: string
  planId: string | null
  fieldId: string
  status: ApplicationStatus
  window: ApplicationWindow
  product: ApplicationProduct
  areaHa: number
  conditions: ApplicationConditionsSummary
  operator: PersonRef
  lot: string | null
  equipment: PersonRef
  registeredAt: Date
  confirmedBy: { person: PersonRef, at: Date } | null
  preHarvestDays: number | null
  cancellationReason: string | null
  recorded: RecordedConditions | null
  signed: boolean
}

export interface ApplicationStore {
  readonly records: ApplicationRecord[]
}

const DEFAULT_PERIOD_DAYS = 30
const TANK_CAPACITY_L = 2000
const SCENARIO_FARM_ID = 'farm-santa-rita'
const FARM_TIME_OFFSET = '-03:00'
const RAIN_HOURS = 8
const OBSERVED_CLOCK = '06:48'
const WEATHER = { temperatureC: 24, windKmh: 7 } as const
const CARLOS: PersonRef = { id: 'user-carlos', name: 'Carlos Mendes' }
const JOAO: PersonRef = { id: 'operator-joao', name: 'João Silveira' }
const MARINA: PersonRef = { id: 'user-marina', name: 'Marina Rezende' }
const TURBO: PersonRef = { id: 'equipment-turbo-2000', name: 'Turbo 2.000 L · bico cone vazio' }
const PREHARVEST_DAYS = 14
/** Applications of the month that are not among the listed seed rows (Figma: 12 in the month, 412 ha of 982 ha). */
const EARLIER_THIS_MONTH = { completed: 6, areaHa: 265.9 } as const
const PRODUCTIVE_AREA_HA = 982

const atFarmTime = (isoDate: string, time: string) => new Date(`${isoDate}T${time}:00${FARM_TIME_OFFSET}`)
const roundOneDecimal = (value: number) => Math.round(value * 10) / 10

function minutesBetween(start: string, end: string): number {
  const toMinutes = (clock: string) => Number(clock.slice(0, 2)) * 60 + Number(clock.slice(3, 5))
  return toMinutes(end) - toMinutes(start)
}

const product = (kind: ApplicationProduct['kind'], name: string, dose: number, mix: number): ApplicationProduct =>
  ({ kind, name, dose: { value: dose, unit: 'L/ha' }, mixRateLPerHa: mix })

function usageOf(record: ApplicationRecord) {
  const mixTotalL = Math.round(record.areaHa * record.product.mixRateLPerHa)
  return {
    lot: record.lot,
    doseTotalL: roundOneDecimal(record.areaHa * record.product.dose.value),
    mixTotalL,
    tankCount: Math.ceil(mixTotalL / TANK_CAPACITY_L),
    tankCapacityL: TANK_CAPACITY_L,
  }
}

/** Seed of the Figma list. `now` fixes day 0; offsets are in days from it. */
export function createApplicationStore(now: () => Date): ApplicationStore {
  const day0 = zonedIsoDate(now())
  const at = (offset: number) => addDaysIso(day0, offset)

  function completed(
    id: string, fieldId: string, offset: number, start: string, end: string,
    prod: ApplicationProduct, areaHa: number, deltaTC: number, windKmh: number, operator: PersonRef,
    extra: Partial<ApplicationRecord> = {},
  ): ApplicationRecord {
    const date = at(offset)
    return {
      id, farmId: SCENARIO_FARM_ID, planId: null, fieldId, status: 'completed',
      window: { date, startTime: start, endTime: end },
      product: prod, areaHa,
      conditions: { source: 'recorded', deltaTC, windKmh, rainMm: null },
      operator, lot: '2409-B', equipment: TURBO,
      registeredAt: atFarmTime(date, '09:52'),
      confirmedBy: { person: MARINA, at: atFarmTime(addDaysIso(date, -1), '17:40') },
      preHarvestDays: PREHARVEST_DAYS,
      cancellationReason: null,
      recorded: {
        headline: 'Dentro da janela', deltaTC, windKmh, rainMm: 0, rainHours: RAIN_HOURS,
        durationMinutes: minutesBetween(start, end),
      },
      signed: true,
      ...extra,
    }
  }

  function planned(id: string, fieldId: string, offset: number, areaHa: number, conditions: ApplicationConditionsSummary): ApplicationRecord {
    const date = at(offset)
    return {
      id, farmId: SCENARIO_FARM_ID, planId: null, fieldId, status: 'planned',
      window: { date, startTime: '06:00', endTime: '10:00' },
      product: product('fungicide', 'Fungicida', 0.5, 200), areaHa, conditions,
      operator: CARLOS, lot: null, equipment: TURBO,
      registeredAt: atFarmTime(at(0), '09:30'),
      confirmedBy: { person: MARINA, at: atFarmTime(at(0), '09:30') },
      preHarvestDays: PREHARVEST_DAYS,
      cancellationReason: null, recorded: null, signed: false,
    }
  }

  const cancelled: ApplicationRecord = {
    ...planned('application-seed-t4', 'field-str-4', -4, 44, { source: 'recorded', deltaTC: null, windKmh: null, rainMm: 18 }),
    status: 'cancelled',
    product: product('herbicide', 'Herbicida', 1.2, 150),
    registeredAt: atFarmTime(at(-5), '16:10'),
    confirmedBy: { person: MARINA, at: atFarmTime(at(-5), '16:10') },
    preHarvestDays: null,
    cancellationReason: 'Chuva de 18 mm prevista na janela',
  }

  return {
    records: [
      planned('application-seed-t3', 'field-str-3', 5, 38.6, { source: 'forecast', deltaTC: 3.1, windKmh: 5, rainMm: null }),
      planned('application-seed-t7', 'field-str-7', 7, 51.4, { source: 'live', deltaTC: null, windKmh: null, rainMm: null }),
      completed('application-seed-t5', 'field-str-5', 0, '06:12', '09:40', product('insecticide', 'Inseticida', 0.3, 180), 56.3, 4.2, 6, CARLOS),
      completed('application-seed-t1', 'field-str-1', -2, '07:05', '08:30', product('fungicide', 'Fungicida', 0.5, 200), 42.1, 6.8, 9, JOAO),
      cancelled,
      completed('application-seed-t8', 'field-str-8', -6, '05:50', '09:10', product('foliar', 'Foliar', 2, 300), 47.7, 9.1, 11, CARLOS, {
        lot: 'F-1187',
        recorded: { headline: 'Fora da faixa ideal', deltaTC: 9.1, windKmh: 11, rainMm: 0, rainHours: RAIN_HOURS, durationMinutes: 200 },
      }),
    ],
  }
}

export function createApplicationHistoryMockServices(
  ctx: AdminMockContext,
  store: ApplicationStore,
  faults: PlannerFaults = {},
): { applications: Pick<ApplicationService, 'list' | 'get'>, weather: WeatherService } {
  const { db, now, authorized, currentViewer } = ctx

  function farmOf(record: ApplicationRecord): FarmRecord | undefined {
    return db.farms.find(item => item.id === record.farmId)
  }

  /** Other companies are 404, farms outside the operator scope 403 (ADR 0013). */
  function requireFarm(farmId: string): FarmRecord {
    const record = db.farms.find(item => item.id === farmId)
    const access = record ? farmAccess(currentViewer(), record) : 'hidden'
    if (!record || access === 'hidden') fail('not_found', 'fazenda não encontrada')
    if (access === 'out_of_scope') fail('out_of_scope', 'fazenda fora do seu escopo')
    return record
  }

  function visibleRecords(): ApplicationRecord[] {
    return store.records.filter((record) => {
      const farm = farmOf(record)
      return farm !== undefined && canSeeFarm(currentViewer(), farm)
    })
  }

  function fieldRef(record: ApplicationRecord) {
    const field = db.fields.find(item => item.id === record.fieldId)
    const name = field?.name ?? ''
    return {
      id: record.fieldId,
      label: fieldShortLabel(name),
      name,
      variety: field?.variety ?? null,
      block: field?.block ?? null,
    }
  }

  function toSummary(record: ApplicationRecord): ApplicationSummary {
    return {
      id: record.id,
      status: record.status,
      farmId: record.farmId,
      window: { ...record.window },
      field: fieldRef(record),
      product: { ...record.product, dose: { ...record.product.dose } },
      areaHa: record.areaHa,
      conditions: { ...record.conditions },
      operator: { ...record.operator },
    }
  }

  function toDetail(record: ApplicationRecord): ApplicationDetail {
    const released = record.preHarvestDays !== null && record.status === 'completed'
      ? addDaysIso(record.window.date, record.preHarvestDays)
      : null
    return {
      ...toSummary(record),
      registeredAt: record.registeredAt,
      recordedConditions: record.recorded ? { ...record.recorded } : null,
      usage: usageOf(record),
      preHarvest: record.preHarvestDays === null ? null : { days: record.preHarvestDays, releasedFrom: released },
      equipment: { ...record.equipment },
      confirmedBy: record.confirmedBy ? { person: { ...record.confirmedBy.person }, at: record.confirmedBy.at } : null,
      signed: record.signed,
      cancellation: record.cancellationReason ? { reason: record.cancellationReason } : null,
    }
  }

  /** Month indicators, computed like the API will: month of today, scope only (no table filters). */
  function monthSummary(scoped: ApplicationRecord[]): ApplicationMonthSummary {
    const today = zonedIsoDate(now())
    const month = today.slice(0, 7)
    const inMonth = scoped.filter(record => record.window.date.startsWith(month))
    // The earlier applications belong to the Figma farm only.
    const earlier = scoped.some(record => record.farmId === SCENARIO_FARM_ID) ? EARLIER_THIS_MONTH : { completed: 0, areaHa: 0 }
    const productiveAreaHa = earlier.completed > 0 ? PRODUCTIVE_AREA_HA : 0
    const count = (status: ApplicationStatus) =>
      inMonth.filter(record => record.status === status).length + (status === 'completed' ? earlier.completed : 0)
    const total = inMonth.length + earlier.completed
    const outside = inMonth.filter(record => record.recorded && record.recorded.headline !== 'Dentro da janela').length
    const areaHa = roundOneDecimal(
      inMonth.filter(record => record.status === 'completed').reduce((sum, record) => sum + record.areaHa, 0) + earlier.areaHa,
    )
    const next = inMonth
      .filter(record => record.status === 'planned' && record.window.date >= today)
      .sort((a, b) => a.window.date.localeCompare(b.window.date) || a.window.startTime.localeCompare(b.window.startTime))[0]
    return {
      monthCount: { total, planned: count('planned'), inProgress: count('in_progress'), completed: count('completed'), cancelled: count('cancelled') },
      appliedArea: { areaHa, productiveAreaHa, percent: productiveAreaHa > 0 ? Math.round((areaHa / productiveAreaHa) * 100) : null },
      withinWindow: { percent: total === 0 ? null : Math.round(((total - outside) / total) * 100), within: total - outside, total },
      next: next
        ? {
            applicationId: next.id,
            date: next.window.date,
            startTime: next.window.startTime,
            endTime: next.window.endTime,
            fieldLabel: fieldRef(next).label,
            productName: next.product.name,
          }
        : null,
    }
  }

  const isOpen = (record: ApplicationRecord) => record.status === 'planned' || record.status === 'in_progress'

  /** Planned and in progress first (nearest first), then past ones from the newest to the oldest. */
  function byApiOrder(a: ApplicationRecord, b: ApplicationRecord): number {
    if (isOpen(a) !== isOpen(b)) return isOpen(a) ? -1 : 1
    const order = a.window.date.localeCompare(b.window.date) || a.window.startTime.localeCompare(b.window.startTime)
    return isOpen(a) ? order : -order
  }

  function matches(record: ApplicationRecord, filters: ApplicationFilters, since: string): boolean {
    return record.window.date >= since
      && (!filters.farmId || record.farmId === filters.farmId)
      && (!filters.fieldId || record.fieldId === filters.fieldId)
      && (!filters.productKind || record.product.kind === filters.productKind)
      && (!filters.operatorId || record.operator.id === filters.operatorId)
      && (!filters.status || record.status === filters.status)
  }

  const applications: Pick<ApplicationService, 'list' | 'get'> = {
    list(filters = {}) {
      return authorized((): ApplicationPage => {
        if (filters.farmId) requireFarm(filters.farmId)
        const scoped = visibleRecords()
        const since = addDaysIso(zonedIsoDate(now()), -(filters.periodDays ?? DEFAULT_PERIOD_DAYS))
        const rows = scoped.filter(record => matches(record, filters, since)).sort(byApiOrder)
        const page = paginate(rows.map(toSummary), filters)
        const fields = [...new Map(scoped.map(record => [record.fieldId, fieldRef(record)])).values()]
          .map(({ id, label, name }) => ({ id, label, name }))
        const operators = [...new Map(scoped.map(record => [record.operator.id, record.operator])).values()]
          .map(operator => ({ ...operator }))
        const inScope = filters.farmId ? scoped.filter(record => record.farmId === filters.farmId) : scoped
        return { ...page, facets: { fields, operators }, summary: monthSummary(inScope) }
      })
    },
    get(applicationId) {
      return authorized(() => {
        const record = store.records.find(item => item.id === applicationId)
        const farm = record ? farmOf(record) : undefined
        const access = farm ? farmAccess(currentViewer(), farm) : 'hidden'
        if (!record || access === 'hidden') fail('not_found', 'aplicação não encontrada')
        if (access === 'out_of_scope') fail('out_of_scope', 'aplicação fora do seu escopo')
        return toDetail(record)
      })
    },
  }

  const weather: WeatherService = {
    getCurrent(farmId) {
      return authorized((): CurrentWeather => {
        if (faults.weather) fail('weather_unavailable', 'a previsão do tempo está indisponível')
        const viewer = currentViewer()
        const visible = db.farms.filter(farm => farmAccess(viewer, farm) === 'visible')
        const farm = farmId
          ? requireFarm(farmId)
          : visible.find(item => item.id === ctx.currentUserRecord().primaryFarmId) ?? visible[0] ?? fail('not_found', 'nenhuma fazenda liberada')
        return {
          farmId: farm.id,
          city: farm.city,
          state: farm.state,
          ...WEATHER,
          observedAt: atFarmTime(zonedIsoDate(now()), OBSERVED_CLOCK),
        }
      })
    },
  }

  return { applications, weather }
}
