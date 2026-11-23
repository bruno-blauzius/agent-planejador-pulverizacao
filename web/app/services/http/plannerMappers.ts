/** DTO <-> model of the planner endpoints (ADR 0016). */
import type {
  ApplicationRegistrationInput,
  ApplicationReview,
  DayConditions,
  PlanWindow,
  PlannerMessageInput,
  PlannerReply,
  RegisteredApplication,
  SprayPlan,
  SprayWindowMap,
  TodayBrief,
  TodayTask,
} from '~/types/planner'
import type {
  ApplicationRegistrationDto,
  ApplicationReviewDto,
  DayConditionsDto,
  PlanWindowDto,
  PlannerMessageRequestDto,
  PlannerReplyDto,
  RegisteredApplicationDto,
  SprayPlanDto,
  SprayWindowMapDto,
  TodayBriefDto,
  TodayTaskDto,
} from '~/types/plannerDto'

const dateOrNull = (value: string | null) => (value ? new Date(value) : null)

export function toPlannerMessageRequestDto(input: PlannerMessageInput): PlannerMessageRequestDto {
  return {
    conversation_id: input.conversationId,
    text: input.text,
    context: {
      farm_id: input.context.farmId,
      field_ids: [...input.context.fieldIds],
      product_kind: input.context.productKind,
    },
  }
}

function toPlanWindow(dto: PlanWindowDto): PlanWindow {
  return { date: dto.date, startTime: dto.start_time, endTime: dto.end_time, condition: dto.condition }
}

export function toSprayPlan(dto: SprayPlanDto): SprayPlan {
  return {
    id: dto.id,
    farmId: dto.farm_id,
    productKind: dto.product_kind,
    headline: dto.headline,
    totalAreaHa: dto.total_area_ha,
    capacityHaPerHour: dto.capacity_ha_per_hour,
    entries: dto.entries.map(entry => ({
      fieldId: entry.field_id,
      fieldLabel: entry.field_label,
      areaHa: entry.area_ha,
      window: toPlanWindow(entry.window),
      durationMinutes: entry.duration_minutes,
      alert: entry.alert
        ? {
            kind: entry.alert.kind,
            message: entry.alert.message,
            requiredMinutes: entry.alert.required_minutes,
            availableMinutes: entry.alert.available_minutes,
          }
        : null,
    })),
  }
}

export function toPlannerReply(dto: PlannerReplyDto): PlannerReply {
  return {
    conversationId: dto.conversation_id,
    message: { id: dto.message.id, text: dto.message.text, createdAt: new Date(dto.message.created_at) },
    consulted: dto.consulted.map(source => ({ ...source })),
    plan: dto.plan ? toSprayPlan(dto.plan) : null,
    suggestions: [...dto.suggestions],
  }
}

export function toSprayWindowMap(dto: SprayWindowMapDto): SprayWindowMap {
  return {
    farmId: dto.farm_id,
    generatedAt: new Date(dto.generated_at),
    forecastUpdatedAt: new Date(dto.forecast_updated_at),
    dayStartHour: dto.day_start_hour,
    dayEndHour: dto.day_end_hour,
    fields: dto.fields.map(row => ({
      fieldId: row.field_id,
      fieldLabel: row.field_label,
      days: row.days.map(day => ({ date: day.date, hours: day.hours.map(hour => ({ ...hour })) })),
    })),
    windows: dto.windows.map(slot => ({
      fieldId: slot.field_id,
      fieldLabel: slot.field_label,
      date: slot.date,
      startTime: slot.start_time,
      endTime: slot.end_time,
      durationMinutes: slot.duration_minutes,
      condition: slot.condition,
    })),
  }
}

export function toDayConditions(dto: DayConditionsDto): DayConditions {
  const summary = dto.window_summary
  return {
    farmId: dto.farm_id,
    date: dto.date,
    idealRanges: {
      deltaTC: { ...dto.ideal_ranges.delta_t_c },
      windKmh: { ...dto.ideal_ranges.wind_kmh },
    },
    hours: dto.hours.map(hour => ({
      hour: hour.hour,
      deltaTC: hour.delta_t_c,
      windKmh: hour.wind_kmh,
      gustKmh: hour.gust_kmh,
      rainMm: hour.rain_mm,
      condition: hour.condition,
    })),
    windowSummary: summary
      ? {
          startTime: summary.start_time,
          endTime: summary.end_time,
          deltaTMinC: summary.delta_t_min_c,
          deltaTMaxC: summary.delta_t_max_c,
          windAvgKmh: summary.wind_avg_kmh,
          gustMaxKmh: summary.gust_max_kmh,
          rainMm: summary.rain_mm,
          rainUntilHour: summary.rain_until_hour,
          nextRainAt: dateOrNull(summary.next_rain_at),
        }
      : null,
  }
}

export function toApplicationReview(dto: ApplicationReviewDto): ApplicationReview {
  return {
    id: dto.id,
    planId: dto.plan_id,
    fieldId: dto.field_id,
    fieldLabel: dto.field_label,
    variety: dto.variety,
    areaHa: dto.area_ha,
    window: toPlanWindow(dto.window),
    product: {
      kind: dto.product.kind,
      name: dto.product.name,
      dose: { ...dto.product.dose },
      mixRateLPerHa: dto.product.mix_rate_l_per_ha,
    },
    mixVolumeL: dto.mix_volume_l,
    tankCount: dto.tank_count,
    tankCapacityL: dto.tank_capacity_l,
    checks: dto.checks.map(check => ({ ...check })),
    operator: { ...dto.operator },
    equipment: { ...dto.equipment },
    canConfirm: dto.can_confirm,
    expiresAt: new Date(dto.expires_at),
  }
}

export function toApplicationRegistrationDto(input: ApplicationRegistrationInput): ApplicationRegistrationDto {
  return {
    plan_id: input.planId,
    field_id: input.fieldId,
    review_id: input.reviewId,
    confirmation: {
      confirmed: input.confirmation.confirmed,
      acknowledged_warnings: [...input.confirmation.acknowledgedWarnings],
    },
  }
}

export function toRegisteredApplication(dto: RegisteredApplicationDto): RegisteredApplication {
  return {
    id: dto.id,
    status: dto.status,
    fieldId: dto.field_id,
    fieldLabel: dto.field_label,
    areaHa: dto.area_ha,
    window: toPlanWindow(dto.window),
    registeredAt: new Date(dto.registered_at),
    registeredBy: { ...dto.registered_by },
  }
}

export function toTodayTask(dto: TodayTaskDto): TodayTask {
  return {
    id: dto.id,
    fieldId: dto.field_id,
    fieldLabel: dto.field_label,
    fieldName: dto.field_name,
    operation: dto.operation,
    areaHa: dto.area_ha,
    dose: { ...dto.dose },
    tankCount: dto.tank_count,
    tankCapacityL: dto.tank_capacity_l,
    status: dto.status,
  }
}

export function toTodayBrief(dto: TodayBriefDto): TodayBrief {
  return {
    farm: { ...dto.farm },
    asOf: new Date(dto.as_of),
    status: {
      condition: dto.status.condition,
      headline: dto.status.headline,
      untilTime: dto.status.until_time,
      remainingMinutes: dto.status.remaining_minutes,
      windowStart: dto.status.window_start,
      windowEnd: dto.status.window_end,
    },
    metrics: dto.metrics.map(metric => ({ ...metric })),
    hours: dto.hours.map(hour => ({ hour: hour.hour, condition: hour.condition, isCurrent: hour.is_current })),
    task: dto.task ? toTodayTask(dto.task) : null,
  }
}
