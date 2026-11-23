/**
 * In-memory adapters of the spray planner (ADR 0016). Deterministic, no LLM:
 * replies come from rules over a seed scenario that mirrors the Figma page 02
 * (T3 and T7, fungicide, 90 ha, 12 ha/h, window 06h-10h, rain on the next day).
 * Dates are relative to the mock clock; the time of day is fixed (data refreshed
 * 05:40, "now" 06:48) so tests and screens are reproducible.
 *
 * The suitability below stands in for the spray-planner: the real API computes it.
 */
import type { ApplicationService, PlannerService, TodayService } from '../ports'
import type { SprayCondition } from '~/types/spray'
import type {
  ApplicationReview,
  DayConditions,
  HourlyConditions,
  PlanEntry,
  PlannerReply,
  RegisteredApplication,
  ReviewCheck,
  SprayPlan,
  SprayWindowMap,
  SprayWindowSlot,
  TodayBrief,
  TodayTask,
  ProductKind,
  WindowConditionSummary,
} from '~/types/planner'
import type { Field } from '~/types/field'
import {
  addDaysIso,
  capitalize,
  formatDayMonth,
  formatDecimal,
  formatDurationMinutes,
  onWeekday,
  warningCodesOf,
  weekdayLong,
  zonedIsoDate,
} from '~/utils/planner'
import { fieldShortLabel } from '~/utils/farmDetail'
import { farmAccess } from './access'
import type { AdminMockContext } from './adminServices'
import type { ApplicationStore } from './applicationServices'
import type { FarmRecord } from './seed'
import type { FieldError } from '../errors'
import { fail, failIfInvalid, normalize } from './support'

/** Switches a test or a dev session can flip to exercise the honest-error states. */
export interface PlannerFaults {
  agent?: boolean
  weather?: boolean
  /** Makes registration answer `conditions_changed`, as if the forecast moved after the review. */
  conditionsChanged?: boolean
  /** Turns one check of every review into `blocked`. */
  blockedCheck?: ReviewCheck['code']
  /** Answers 403 `forbidden` on review, registration, "Hoje" and starting a task. */
  forbidden?: boolean
  /** Makes the registration fail with a generic 500. */
  registrationFailure?: boolean
}

const DAY_START_HOUR = 6
const DAY_END_HOUR = 20
const FORECAST_DAYS = 5
const CAPACITY_HA_PER_HOUR = 12
const TANK_CAPACITY_L = 2000
const MIX_RATE_L_PER_HA = 200
const ROUND_MINUTES = 5
const REVIEW_TTL_MS = 15 * 60_000
const FORECAST_UPDATED = '05:40'
const NOW_CLOCK = '06:48'
const RAIN_FREE_LABEL = '8h livres'
const PRE_HARVEST_DAYS = 30
const TODAY_TASK_ID = 'task-mock-t3'
const SCENARIO_FARM_ID = 'farm-santa-rita'
const OPERATOR_ROLE = 'planning_operator'
const IDEAL_RANGES = { deltaTC: { min: 2, max: 8 }, windKmh: { min: 3, max: 10 } } as const
/** The plan applies 51.4 ha at T7 (Figma); the registered field area is 55 ha. */
const PLANNED_AREA_HA: Readonly<Record<string, number>> = { 'field-str-7': 51.4 }

const PRODUCTS: Readonly<Record<ProductKind, { name: string, doseLPerHa: number }>> = {
  fungicide: { name: 'Fungicida', doseLPerHa: 0.5 },
  insecticide: { name: 'Inseticida', doseLPerHa: 0.3 },
  herbicide: { name: 'Herbicida', doseLPerHa: 1.5 },
  foliar: { name: 'Foliar', doseLPerHa: 2 },
  other: { name: 'Produto', doseLPerHa: 0.5 },
}

const SOURCES = [
  { kind: 'weather', title: 'Previsão do tempo', detail: 'Open-Meteo · 5 dias, hora a hora' },
  { kind: 'conditions', title: 'Condições de aplicação', detail: 'Delta T, vento e chuva por hora' },
  { kind: 'product_rules', title: 'Regras do produto', detail: 'Carência e intervalo · Consultor de Bulas' },
  { kind: 'operational_capacity', title: 'Rendimento operacional', detail: 'Pulverizador 2.000 L · 12 ha/h' },
] as const

// ---- seed forecast ------------------------------------------------------------------

const CODE_TO_CONDITION: Record<string, SprayCondition> = { S: 'suitable', A: 'attention', U: 'unsuitable', R: 'rain' }

/** One letter per hour 06..20 (S apta, A atenção, U inapta, R chuva), by day offset. */
const DAY_PROFILES = [
  'SSSSAAUUUAASSSS',
  'RRRRRRRRRRRRRRR',
  'SSSSAAUUUAAAAAA',
  'ASSSSAAUUUUUUUU',
  'AAAAAAUUUUUUUUU',
] as const

/** Day 0 numbers, hours 05..20 (Figma: window 06-10h → Delta T 2,6-6,2, wind 6, gust 11). */
const DAY0_DELTA_T = [2.1, 2.6, 4.2, 5.1, 6.2, 7.4, 8.6, 10.1, 11.2, 11.8, 9.4, 8.9, 7.2, 5.8, 4.4, 3.6]
const DAY0_WIND = [3, 4, 6, 7, 8, 11, 12, 14, 16, 15, 12, 11, 7, 5, 4, 3]
const GUST_EXTRA = 3
/** Spread over the 16 hourly rows (05h to 20h) of the rainy day. */
const DAY1_RAIN_TOTAL_MM = 18

const GENERATED_BY_CONDITION: Record<SprayCondition, { deltaT: number, wind: number }> = {
  suitable: { deltaT: 4, wind: 6 },
  attention: { deltaT: 8.8, wind: 11 },
  unsuitable: { deltaT: 11.5, wind: 16 },
  rain: { deltaT: 1.2, wind: 12 },
}

function conditionAt(offset: number, hour: number): SprayCondition {
  const profile = DAY_PROFILES[offset] ?? DAY_PROFILES[DAY_PROFILES.length - 1]!
  const index = Math.min(Math.max(hour, DAY_START_HOUR), DAY_END_HOUR) - DAY_START_HOUR
  return CODE_TO_CONDITION[profile.charAt(index)] ?? 'unsuitable'
}

function hourlyOf(offset: number): HourlyConditions[] {
  const rows: HourlyConditions[] = []
  for (let hour = DAY_START_HOUR - 1; hour <= DAY_END_HOUR; hour++) {
    const condition = hour < DAY_START_HOUR && offset === 0 ? 'attention' : conditionAt(offset, hour)
    const index = hour - (DAY_START_HOUR - 1)
    const base = GENERATED_BY_CONDITION[condition]
    const deltaTC = offset === 0 ? DAY0_DELTA_T[index]! : base.deltaT + (index % 3) * 0.4
    const windKmh = offset === 0 ? DAY0_WIND[index]! : base.wind
    rows.push({
      hour,
      deltaTC,
      windKmh,
      gustKmh: windKmh + GUST_EXTRA,
      rainMm: condition === 'rain' ? DAY1_RAIN_TOTAL_MM / (DAY_END_HOUR - DAY_START_HOUR + 2) : 0,
      condition,
    })
  }
  return rows
}

function slotsOf(hours: readonly { hour: number, condition: SprayCondition }[]): { start: number, end: number }[] {
  const runs: { start: number, end: number }[] = []
  for (const { hour, condition } of hours) {
    const last = runs[runs.length - 1]
    if (condition !== 'suitable') continue
    if (last && last.end === hour) last.end = hour + 1
    else runs.push({ start: hour, end: hour + 1 })
  }
  return runs
}

const clock = (hour: number, minute = 0) => `${String(hour).padStart(2, '0')}:${String(minute).padStart(2, '0')}`
const atFarmTime = (isoDate: string, time: string) => new Date(`${isoDate}T${time}:00-03:00`)
const averageOf = (values: readonly number[]) => values.reduce((sum, value) => sum + value, 0) / values.length

export function createPlannerMockServices(ctx: AdminMockContext, store: ApplicationStore, faults: PlannerFaults = {}): {
  planner: PlannerService
  applications: Pick<ApplicationService, 'review' | 'register'>
  today: TodayService
} {
  const { db, now, nextId, authorized, currentViewer } = ctx
  const plans = new Map<string, SprayPlan>()
  const reviews = new Map<string, ApplicationReview>()
  const startedTasks = new Set<string>()

  const day0 = () => zonedIsoDate(now())
  const dayAt = (offset: number) => addDaysIso(day0(), offset)

  function requireAllowed(): void {
    if (faults.forbidden) fail('forbidden', 'você não tem permissão para esta ação')
  }

  function requireWeather(): void {
    if (faults.weather) fail('weather_unavailable', 'a previsão do tempo está indisponível')
  }

  /** Other companies are 404, farms outside the operator scope 403 (ADR 0013). */
  function requireFarm(farmId: string): FarmRecord {
    const record = db.farms.find(item => item.id === farmId)
    const access = record ? farmAccess(currentViewer(), record) : 'hidden'
    if (!record || access === 'hidden') fail('not_found', 'fazenda não encontrada')
    if (access === 'out_of_scope') fail('out_of_scope', 'fazenda fora do seu escopo')
    return record
  }

  function fieldsOf(farmId: string, fieldIds: readonly string[] | undefined, field: string): Field[] {
    const all = db.fields.filter(item => item.farmId === farmId)
    if (!fieldIds?.length) return all
    return fieldIds.map(id => all.find(item => item.id === id) ?? fail('validation_failed', 'talhão não pertence à fazenda', [{ field, message: `talhão ${id} não encontrado` }]))
  }

  // ---- window map and conditions ----------------------------------------------------

  function buildWindowMap(farmId: string, fields: readonly Field[], days: number): SprayWindowMap {
    const offsets = Array.from({ length: days }, (_, offset) => offset)
    const rows = fields.map(field => ({
      fieldId: field.id,
      fieldLabel: fieldShortLabel(field.name),
      days: offsets.map(offset => ({
        date: dayAt(offset),
        hours: Array.from({ length: DAY_END_HOUR - DAY_START_HOUR + 1 }, (_, index) => ({
          hour: DAY_START_HOUR + index,
          condition: conditionAt(offset, DAY_START_HOUR + index),
        })),
      })),
    }))
    const windows: SprayWindowSlot[] = rows.flatMap(row => row.days.flatMap(day =>
      slotsOf(day.hours).map(slot => ({
        fieldId: row.fieldId,
        fieldLabel: row.fieldLabel,
        date: day.date,
        startTime: clock(slot.start),
        endTime: clock(slot.end),
        durationMinutes: (slot.end - slot.start) * 60,
        condition: 'suitable' as const,
      }))))
    return {
      farmId,
      generatedAt: atFarmTime(day0(), FORECAST_UPDATED),
      forecastUpdatedAt: atFarmTime(day0(), FORECAST_UPDATED),
      dayStartHour: DAY_START_HOUR,
      dayEndHour: DAY_END_HOUR,
      fields: rows,
      windows,
    }
  }

  function summarize(offset: number, hours: readonly HourlyConditions[], start: number, end: number): WindowConditionSummary {
    const inside = hours.filter(hour => hour.hour >= start && hour.hour < end)
    return {
      startTime: clock(start),
      endTime: clock(end),
      deltaTMinC: Math.min(...inside.map(hour => hour.deltaTC)),
      deltaTMaxC: Math.max(...inside.map(hour => hour.deltaTC)),
      windAvgKmh: averageOf(inside.map(hour => hour.windKmh)),
      gustMaxKmh: Math.max(...inside.map(hour => hour.gustKmh)),
      rainMm: hours.filter(hour => hour.hour < 18).reduce((sum, hour) => sum + hour.rainMm, 0),
      rainUntilHour: 18,
      nextRainAt: offset < 1 ? atFarmTime(dayAt(1), '03:00') : null,
    }
  }

  function offsetOf(isoDate: string): number {
    const offset = Array.from({ length: FORECAST_DAYS }, (_, index) => index).find(index => dayAt(index) === isoDate)
    if (offset === undefined) fail('validation_failed', 'data fora da previsão', [{ field: 'date', message: 'use uma data dos próximos 5 dias' }])
    return offset
  }

  function dayConditions(farmId: string, date: string, windowStart?: string, windowEnd?: string): DayConditions {
    const offset = offsetOf(date)
    const hours = hourlyOf(offset)
    const best = slotsOf(hours.filter(hour => hour.hour >= DAY_START_HOUR))
      .reduce<{ start: number, end: number } | null>((acc, slot) => (!acc || slot.end - slot.start > acc.end - acc.start ? slot : acc), null)
    const start = windowStart ? Number(windowStart.split(':')[0]) : best?.start
    const end = windowEnd ? Number(windowEnd.split(':')[0]) : best?.end
    return {
      farmId,
      date,
      idealRanges: { deltaTC: { ...IDEAL_RANGES.deltaTC }, windKmh: { ...IDEAL_RANGES.windKmh } },
      hours,
      windowSummary: start !== undefined && end !== undefined ? summarize(offset, hours, start, end) : null,
    }
  }

  // ---- plan -----------------------------------------------------------------------------

  /** Earliest longest apt slot of each day that has one: where the agent proposes to work. */
  function bestSlots(): { date: string, start: number, end: number }[] {
    return Array.from({ length: FORECAST_DAYS }, (_, offset) => offset).flatMap((offset) => {
      const best = slotsOf(hourlyOf(offset).filter(hour => hour.hour >= DAY_START_HOUR))
        .reduce<{ start: number, end: number } | null>((acc, slot) => (!acc || slot.end - slot.start > acc.end - acc.start ? slot : acc), null)
      return best ? [{ date: dayAt(offset), ...best }] : []
    })
  }

  function planEntryOf(field: Field, slot: { date: string, start: number, end: number }): PlanEntry {
    const areaHa = PLANNED_AREA_HA[field.id] ?? field.areaHa
    const durationMinutes = Math.ceil((areaHa / CAPACITY_HA_PER_HOUR) * 60 / ROUND_MINUTES) * ROUND_MINUTES
    const availableMinutes = (slot.end - slot.start) * 60
    const label = fieldShortLabel(field.name)
    const shortfall = durationMinutes - availableMinutes
    const leftoverHa = Math.round(areaHa - (CAPACITY_HA_PER_HOUR * availableMinutes) / 60)
    const earlier = new Date(atFarmTime(slot.date, clock(slot.start)).getTime() - shortfall * 60_000)
    const earlierClock = earlier.toLocaleTimeString('pt-BR', { timeZone: 'America/Sao_Paulo', hour: '2-digit', minute: '2-digit', hourCycle: 'h23' })
    return {
      fieldId: field.id,
      fieldLabel: label,
      areaHa,
      window: { date: slot.date, startTime: clock(slot.start), endTime: clock(slot.end), condition: 'suitable' },
      durationMinutes,
      alert: shortfall > 0
        ? {
            kind: 'window_too_short',
            message: `${label} precisa de ${formatDurationMinutes(durationMinutes)} e a janela tem ${formatDurationMinutes(availableMinutes)}. O agente sugere começar ${earlierClock} ou deixar ${leftoverHa} ha para ${weekdayLong(dayAt(3))}.`,
            requiredMinutes: durationMinutes,
            availableMinutes,
          }
        : null,
    }
  }

  function buildPlan(farmId: string, fields: readonly Field[], kind: ProductKind): SprayPlan {
    const slots = bestSlots()
    const entries = fields.map((field, index) => planEntryOf(field, slots[index % slots.length]!))
    const total = Math.round(entries.reduce((sum, entry) => sum + entry.areaHa, 0) * 10) / 10
    const plan: SprayPlan = {
      id: nextId('plan'),
      farmId,
      productKind: kind,
      headline: `${entries.length} ${entries.length === 1 ? 'janela apta cobre' : 'janelas aptas cobrem'} os ${formatDecimal(total)} ha`,
      totalAreaHa: total,
      capacityHaPerHour: CAPACITY_HA_PER_HOUR,
      entries,
    }
    plans.set(plan.id, plan)
    return plan
  }

  function bestPlanText(plan: SprayPlan): string {
    const first = plan.entries[0]!
    const days = plan.entries.map(entry => `${entry.fieldLabel} ${onWeekday(entry.window.date)}`)
    const workMinutes = Math.round((plan.totalAreaHa / CAPACITY_HA_PER_HOUR) * 60 / 30) * 30
    const windowMinutes = (Number(first.window.endTime.slice(0, 2)) - Number(first.window.startTime.slice(0, 2))) * 60
    const subject = plan.entries.length === 1
      ? `O talhão ${first.fieldLabel} soma ${formatDecimal(plan.totalAreaHa)} ha`
      : `Os ${plan.entries.length === 2 ? 'dois' : plan.entries.length} talhões somam ${formatDecimal(plan.totalAreaHa)} ha`
    const advice = plan.entries.length === 1
      ? `A janela de ${formatDurationMinutes(windowMinutes)} comporta a aplicação.`
      : `Como a janela tem ${formatDurationMinutes(windowMinutes)}, sugiro dividir: ${days.join(' e ')}, no mesmo horário.`
    return [
      `A melhor janela é ${weekdayLong(first.window.date)} (${formatDayMonth(first.window.date)}), das ${first.window.startTime.slice(0, 2)}h às ${first.window.endTime.slice(0, 2)}h:`,
      `${subject} e o pulverizador faz ${CAPACITY_HA_PER_HOUR} ha/h, ou seja, cerca de ${formatDurationMinutes(workMinutes)} de trabalho. ${advice}`,
      `${capitalize(weekdayLong(dayAt(1)))} tem ${DAY1_RAIN_TOTAL_MM} mm de chuva prevista. Evite: o produto precisa de 2h sem chuva para não lavar.`,
    ].join('\n\n')
  }

  function suggestionsExcept(used: string): string[] {
    return [`E se chover ${onWeekday(dayAt(0))}?`, `Comparar com ${weekdayLong(dayAt(3))}`, 'Só o talhão 3']
      .filter(text => normalize(text) !== normalize(used))
  }

  /** Rule-based "agent": the same text and plan for the same message and context. */
  function answer(text: string, farmId: string, fields: readonly Field[], kind: ProductKind): { text: string, plan: SprayPlan | null } {
    const asked = normalize(text)
    const d0 = dayAt(0)
    const d2 = dayAt(2)
    const d3 = dayAt(3)
    if (asked.includes('chover') || asked.includes('chuva')) {
      return {
        plan: null,
        text: `Se chover ${onWeekday(d0)}, a próxima janela apta é ${weekdayLong(d2)} (${formatDayMonth(d2)}), das 06h às 10h, e a alternativa seguinte é ${weekdayLong(d3)}, das 07h às 11h.\n\nO produto precisa de 2h sem chuva depois da aplicação para não lavar.`,
      }
    }
    if (asked.includes('compar') || asked.includes('sabado')) {
      return {
        plan: null,
        text: `Comparando ${weekdayLong(d0)} e ${weekdayLong(d3)}: ${weekdayLong(d0)} tem 4h aptas, das 06h às 10h, com Delta T entre 2,6 e 6,2 °C. ${capitalize(weekdayLong(d3))} também tem 4h, mas das 07h às 11h, com vento maior no fim da janela.\n\n${capitalize(weekdayLong(d0))} é a melhor opção.`,
      }
    }
    if (asked.includes('so o talhao') || asked.includes('apenas o talhao')) {
      const only = fields.find(field => asked.includes(fieldShortLabel(field.name).toLowerCase()) || asked.includes(normalize(field.name))) ?? fields[0]!
      const plan = buildPlan(farmId, [only], kind)
      const entry = plan.entries[0]!
      return {
        plan,
        text: `Só o ${entry.fieldLabel} (${formatDecimal(entry.areaHa)} ha): ${formatDurationMinutes(entry.durationMinutes)} de trabalho na janela de ${weekdayLong(entry.window.date)}, das ${entry.window.startTime.slice(0, 2)}h às ${entry.window.endTime.slice(0, 2)}h.`,
      }
    }
    const plan = buildPlan(farmId, fields, kind)
    return { plan, text: bestPlanText(plan) }
  }

  // ---- services ------------------------------------------------------------------------------

  const planner: PlannerService = {
    sendMessage(input) {
      return authorized((): PlannerReply => {
        if (faults.agent) fail('agent_unavailable', 'o agente de pulverização está indisponível')
        requireWeather()
        const details: FieldError[] = []
        if (!input.text.trim()) details.push({ field: 'text', message: 'escreva uma mensagem' })
        if (input.context.fieldIds.length === 0) details.push({ field: 'context.field_ids', message: 'selecione ao menos um talhão' })
        failIfInvalid(details)
        const farm = requireFarm(input.context.farmId)
        const fields = fieldsOf(farm.id, input.context.fieldIds, 'context.field_ids')
        const kind = input.context.productKind ?? 'fungicide'
        const { text, plan } = answer(input.text, farm.id, fields, kind)
        return {
          conversationId: input.conversationId ?? nextId('conversation'),
          message: { id: nextId('message'), text, createdAt: atFarmTime(day0(), NOW_CLOCK) },
          consulted: SOURCES.map(source => ({ ...source })),
          plan,
          suggestions: suggestionsExcept(input.text),
        }
      }, 'planner.use')
    },
    getWindowMap(query) {
      return authorized(() => {
        requireWeather()
        const farm = requireFarm(query.farmId)
        const days = Math.min(Math.max(query.days ?? FORECAST_DAYS, 1), FORECAST_DAYS)
        return buildWindowMap(farm.id, fieldsOf(farm.id, query.fieldIds, 'field_ids'), days)
      }, 'planner.use')
    },
    getDayConditions(query) {
      return authorized(() => {
        requireWeather()
        const farm = requireFarm(query.farmId)
        return dayConditions(farm.id, query.date, query.windowStart, query.windowEnd)
      }, 'planner.use')
    },
  }

  function operatorOf(farm: FarmRecord) {
    const operator = db.users.find(user => user.role === OPERATOR_ROLE && user.status === 'active' && user.companyId === farm.companyId
      && (user.farmScope.mode === 'all' || user.farmScope.farmIds.includes(farm.id)))
    const person = operator ?? db.users.find(user => user.id === currentViewer().id)
    return { id: person?.id ?? '', name: person?.name ?? '' }
  }

  function checksOf(entry: PlanEntry, farm: FarmRecord): ReviewCheck[] {
    const offset = offsetOf(entry.window.date)
    const hours = hourlyOf(offset)
    const start = Number(entry.window.startTime.slice(0, 2))
    const end = Number(entry.window.endTime.slice(0, 2))
    const summary = summarize(offset, hours, start, end)
    const inside = hours.filter(hour => hour.hour >= start && hour.hour < end)
    const winds = inside.map(hour => hour.windKmh)
    const operator = operatorOf(farm)
    const checks: ReviewCheck[] = [
      { code: 'delta_t', status: 'ok', title: 'Delta T dentro da faixa', detail: `${formatDecimal(summary.deltaTMinC)} a ${formatDecimal(summary.deltaTMaxC)} °C (ideal ${IDEAL_RANGES.deltaTC.min} a ${IDEAL_RANGES.deltaTC.max} °C)` },
      { code: 'wind', status: 'ok', title: 'Vento adequado', detail: `${Math.min(...winds)} a ${Math.max(...winds)} km/h, rajada máxima ${summary.gustMaxKmh} km/h` },
      { code: 'rain_after', status: 'ok', title: 'Sem chuva após a aplicação', detail: `${RAIN_FREE_LABEL}; próxima chuva ${weekdayLong(dayAt(1))} 03h` },
      { code: 'interval_since_last', status: 'ok', title: 'Intervalo desde a última aplicação', detail: '21 dias (mínimo do produto: 14 dias)' },
      { code: 'pre_harvest_interval', status: 'ok', title: 'Carência até a colheita', detail: 'Colheita prevista mai/2027 · carência de 30 dias respeitada' },
      { code: 'operator_equipment', status: 'ok', title: 'Operador e equipamento', detail: `${operator.name} · Pulverizador 2.000 L` },
    ]
    if (entry.alert) {
      checks.push({ code: 'window_fits', status: 'warning', title: 'Janela menor que o tempo de aplicação', detail: entry.alert.message })
    }
    return checks.map(check => (check.code === faults.blockedCheck ? { ...check, status: 'blocked' as const } : check))
  }

  const applications: Pick<ApplicationService, 'review' | 'register'> = {
    review(planId, fieldId) {
      return authorized((): ApplicationReview => {
        requireAllowed()
        requireWeather()
        const plan = plans.get(planId) ?? fail('not_found', 'plano não encontrado')
        const entry = plan.entries.find(item => item.fieldId === fieldId) ?? fail('not_found', 'talhão não está no plano')
        const farm = requireFarm(plan.farmId)
        const field = db.fields.find(item => item.id === fieldId)
        const product = PRODUCTS[plan.productKind]
        const mixVolumeL = Math.round(entry.areaHa * MIX_RATE_L_PER_HA)
        const checks = checksOf(entry, farm)
        const operator = operatorOf(farm)
        const review: ApplicationReview = {
          id: nextId('review'),
          planId,
          fieldId,
          fieldLabel: entry.fieldLabel,
          variety: field?.variety ?? null,
          areaHa: entry.areaHa,
          window: entry.window,
          product: {
            kind: plan.productKind,
            name: product.name,
            dose: { value: product.doseLPerHa, unit: 'L/ha' },
            mixRateLPerHa: MIX_RATE_L_PER_HA,
          },
          mixVolumeL,
          tankCount: Math.ceil(mixVolumeL / TANK_CAPACITY_L),
          tankCapacityL: TANK_CAPACITY_L,
          checks,
          operator,
          equipment: { id: 'equipment-mock-2000', name: 'Pulverizador 2.000 L' },
          canConfirm: checks.every(check => check.status !== 'blocked'),
          expiresAt: new Date(now().getTime() + REVIEW_TTL_MS),
        }
        reviews.set(review.id, review)
        return review
      }, 'applications.register')
    },
    register(input) {
      return authorized((): RegisteredApplication => {
        requireAllowed()
        if (faults.registrationFailure) fail('internal_error', 'erro interno')
        if (input.confirmation.confirmed !== true) {
          fail('validation_failed', 'dados inválidos', [{ field: 'confirmation.confirmed', message: 'confirme a aplicação' }])
        }
        const review = reviews.get(input.reviewId)
        if (!review || review.planId !== input.planId || review.fieldId !== input.fieldId) fail('not_found', 'revisão não encontrada')
        if (now() > review.expiresAt || faults.conditionsChanged) fail('conditions_changed', 'as condições mudaram; revise novamente')
        if (!review.canConfirm) fail('validation_failed', 'dados inválidos', [{ field: 'review_id', message: 'há checagem bloqueada' }])
        const missing = warningCodesOf(review.checks).filter(code => !input.confirmation.acknowledgedWarnings.includes(code))
        if (missing.length) {
          fail('validation_failed', 'dados inválidos', [{ field: 'confirmation.acknowledged_warnings', message: `reconheça os avisos: ${missing.join(', ')}` }])
        }
        if (store.records.some(record => record.planId === input.planId && record.fieldId === input.fieldId && record.status !== 'cancelled')) {
          fail('conflict', 'aplicação já registrada para este talhão')
        }
        const viewer = db.users.find(user => user.id === currentViewer().id)
        const application: RegisteredApplication = {
          id: nextId('application'),
          status: 'planned',
          fieldId: review.fieldId,
          fieldLabel: review.fieldLabel,
          areaHa: review.areaHa,
          window: review.window,
          registeredAt: now(),
          registeredBy: { id: currentViewer().id, name: viewer?.name ?? '' },
        }
        const plan = plans.get(input.planId)
        store.records.push({
          id: application.id,
          farmId: plan?.farmId ?? '',
          planId: input.planId,
          fieldId: review.fieldId,
          status: 'planned',
          window: { date: review.window.date, startTime: review.window.startTime, endTime: review.window.endTime },
          product: { kind: review.product.kind, name: review.product.name, dose: { ...review.product.dose }, mixRateLPerHa: review.product.mixRateLPerHa },
          areaHa: review.areaHa,
          conditions: { source: 'live', deltaTC: null, windKmh: null, rainMm: null },
          operator: { ...review.operator },
          lot: null,
          equipment: { ...review.equipment },
          registeredAt: application.registeredAt,
          confirmedBy: { person: application.registeredBy, at: application.registeredAt },
          preHarvestDays: PRE_HARVEST_DAYS,
          cancellationReason: null,
          recorded: null,
          signed: false,
        })
        return application
      }, 'applications.register')
    },
  }

  function todayFarm(): FarmRecord {
    const viewer = currentViewer()
    const primary = ctx.currentUserRecord().primaryFarmId
    const visible = db.farms.filter(farm => farmAccess(viewer, farm) === 'visible')
    return visible.find(farm => farm.id === primary) ?? visible[0] ?? fail('not_found', 'nenhuma fazenda liberada')
  }

  function todayTask(farm: FarmRecord): TodayTask | null {
    const field = db.fields.find(item => item.id === 'field-str-3')
    if (farm.id !== SCENARIO_FARM_ID || !field) return null
    return {
      id: TODAY_TASK_ID,
      fieldId: field.id,
      fieldLabel: fieldShortLabel(field.name),
      fieldName: field.name,
      operation: PRODUCTS.fungicide.name,
      areaHa: field.areaHa,
      dose: { value: PRODUCTS.fungicide.doseLPerHa, unit: 'L/ha' },
      tankCount: Math.ceil((field.areaHa * MIX_RATE_L_PER_HA) / TANK_CAPACITY_L),
      tankCapacityL: TANK_CAPACITY_L,
      status: startedTasks.has(TODAY_TASK_ID) ? 'in_progress' : 'planned',
    }
  }

  const today: TodayService = {
    getBrief() {
      return authorized((): TodayBrief => {
        requireAllowed()
        requireWeather()
        const farm = todayFarm()
        const windowEnd = 10
        const nowMinutes = Number(NOW_CLOCK.slice(0, 2)) * 60 + Number(NOW_CLOCK.slice(3))
        return {
          farm: { id: farm.id, name: farm.name },
          asOf: atFarmTime(day0(), NOW_CLOCK),
          status: {
            condition: 'suitable',
            headline: 'Pode pulverizar',
            untilTime: clock(windowEnd),
            remainingMinutes: windowEnd * 60 - nowMinutes,
            windowStart: clock(DAY_START_HOUR),
            windowEnd: clock(windowEnd),
          },
          metrics: [
            { kind: 'delta_t', label: 'Delta T', value: 4.2, unit: '°C', condition: 'suitable' },
            { kind: 'wind', label: 'Vento', value: 6, unit: 'km/h', condition: 'suitable' },
            { kind: 'rain', label: 'Chuva 4h', value: 0, unit: 'mm', condition: 'suitable' },
          ],
          hours: Array.from({ length: 13 }, (_, index) => ({
            hour: DAY_START_HOUR + index,
            condition: conditionAt(0, DAY_START_HOUR + index),
            isCurrent: DAY_START_HOUR + index === DAY_START_HOUR,
          })),
          task: todayTask(farm),
        }
      }, 'planner.use')
    },
    startTask(taskId) {
      return authorized((): TodayTask => {
        requireAllowed()
        const farm = todayFarm()
        const task = taskId === TODAY_TASK_ID ? todayTask(farm) : null
        if (!task) fail('not_found', 'tarefa não encontrada')
        if (startedTasks.has(task.id)) fail('conflict', 'a aplicação já foi iniciada')
        startedTasks.add(task.id)
        return { ...task, status: 'in_progress' }
      }, 'applications.register')
    },
  }

  return { planner, applications, today }
}
