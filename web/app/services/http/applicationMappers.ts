/** DTO <-> model of the application history endpoints (ADR 0016). */
import type {
  ApplicationDetail,
  ApplicationPage,
  ApplicationSummary,
  CurrentWeather,
} from '~/types/application'
import type {
  ApplicationDetailDto,
  ApplicationPageDto,
  ApplicationSummaryDto,
  CurrentWeatherDto,
} from '~/types/applicationDto'

export function toApplicationSummary(dto: ApplicationSummaryDto): ApplicationSummary {
  return {
    id: dto.id,
    status: dto.status,
    farmId: dto.farm_id,
    window: { date: dto.window.date, startTime: dto.window.start_time, endTime: dto.window.end_time },
    field: { ...dto.field },
    product: {
      kind: dto.product.kind,
      name: dto.product.name,
      dose: { ...dto.product.dose },
      mixRateLPerHa: dto.product.mix_rate_l_per_ha,
    },
    areaHa: dto.area_ha,
    conditions: {
      source: dto.conditions.source,
      deltaTC: dto.conditions.delta_t_c,
      windKmh: dto.conditions.wind_kmh,
      rainMm: dto.conditions.rain_mm,
    },
    operator: { ...dto.operator },
  }
}

export function toApplicationPage(dto: ApplicationPageDto): ApplicationPage {
  return {
    items: dto.items.map(toApplicationSummary),
    total: dto.total,
    page: dto.page,
    pageSize: dto.page_size,
    facets: {
      fields: dto.facets.fields.map(field => ({ ...field })),
      operators: dto.facets.operators.map(operator => ({ ...operator })),
    },
    summary: {
      monthCount: {
        total: dto.summary.month_count.total,
        planned: dto.summary.month_count.planned,
        inProgress: dto.summary.month_count.in_progress,
        completed: dto.summary.month_count.completed,
        cancelled: dto.summary.month_count.cancelled,
      },
      appliedArea: {
        areaHa: dto.summary.applied_area.area_ha,
        productiveAreaHa: dto.summary.applied_area.productive_area_ha,
        percent: dto.summary.applied_area.percent,
      },
      withinWindow: { ...dto.summary.within_window },
      next: dto.summary.next
        ? {
            applicationId: dto.summary.next.application_id,
            date: dto.summary.next.date,
            startTime: dto.summary.next.start_time,
            endTime: dto.summary.next.end_time,
            fieldLabel: dto.summary.next.field_label,
            productName: dto.summary.next.product_name,
          }
        : null,
    },
  }
}

export function toApplicationDetail(dto: ApplicationDetailDto): ApplicationDetail {
  const recorded = dto.recorded_conditions
  return {
    ...toApplicationSummary(dto),
    registeredAt: new Date(dto.registered_at),
    recordedConditions: recorded
      ? {
          headline: recorded.headline,
          deltaTC: recorded.delta_t_c,
          windKmh: recorded.wind_kmh,
          rainMm: recorded.rain_mm,
          rainHours: recorded.rain_hours,
          durationMinutes: recorded.duration_minutes,
        }
      : null,
    usage: {
      lot: dto.usage.lot,
      doseTotalL: dto.usage.dose_total_l,
      mixTotalL: dto.usage.mix_total_l,
      tankCount: dto.usage.tank_count,
      tankCapacityL: dto.usage.tank_capacity_l,
    },
    preHarvest: dto.pre_harvest ? { days: dto.pre_harvest.days, releasedFrom: dto.pre_harvest.released_from } : null,
    equipment: { ...dto.equipment },
    confirmedBy: dto.confirmed_by ? { person: { ...dto.confirmed_by.person }, at: new Date(dto.confirmed_by.at) } : null,
    signed: dto.signed,
    cancellation: dto.cancellation ? { ...dto.cancellation } : null,
  }
}

export function toCurrentWeather(dto: CurrentWeatherDto): CurrentWeather {
  return {
    farmId: dto.farm_id,
    city: dto.city,
    state: dto.state,
    temperatureC: dto.temperature_c,
    windKmh: dto.wind_kmh,
    observedAt: new Date(dto.observed_at),
  }
}
