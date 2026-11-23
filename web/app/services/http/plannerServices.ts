/** HTTP adapters of the planner (ADR 0016) against services/api. */
import type { ApplicationService, PlannerService, TodayService, WeatherService } from '../ports'
import type { ApplicationDetailDto, ApplicationPageDto, CurrentWeatherDto } from '~/types/applicationDto'
import type {
  ApplicationReviewDto,
  DayConditionsDto,
  PlannerReplyDto,
  RegisteredApplicationDto,
  SprayWindowMapDto,
  TodayBriefDto,
  TodayTaskDto,
} from '~/types/plannerDto'
import type { HttpClient } from './client'
import { toApplicationDetail, toApplicationPage, toCurrentWeather } from './applicationMappers'
import {
  toApplicationRegistrationDto,
  toApplicationReview,
  toDayConditions,
  toPlannerMessageRequestDto,
  toPlannerReply,
  toRegisteredApplication,
  toSprayWindowMap,
  toTodayBrief,
  toTodayTask,
} from './plannerMappers'

const segment = encodeURIComponent

export function createPlannerHttpService(http: HttpClient): PlannerService {
  return {
    async sendMessage(input) {
      const dto = await http<PlannerReplyDto>('/planner/messages', {
        method: 'POST',
        body: toPlannerMessageRequestDto(input),
      })
      return toPlannerReply(dto)
    },
    async getWindowMap(query) {
      const dto = await http<SprayWindowMapDto>('/planner/window-map', {
        query: { farm_id: query.farmId, field_ids: query.fieldIds?.join(','), days: query.days },
      })
      return toSprayWindowMap(dto)
    },
    async getDayConditions(query) {
      const dto = await http<DayConditionsDto>('/planner/conditions', {
        query: {
          farm_id: query.farmId,
          date: query.date,
          field_id: query.fieldId,
          window_start: query.windowStart,
          window_end: query.windowEnd,
        },
      })
      return toDayConditions(dto)
    },
  }
}

export function createApplicationHttpService(http: HttpClient): ApplicationService {
  return {
    async list(filters = {}) {
      const dto = await http<ApplicationPageDto>('/applications', {
        query: {
          period_days: filters.periodDays,
          farm_id: filters.farmId,
          field_id: filters.fieldId,
          product_kind: filters.productKind,
          operator_id: filters.operatorId,
          status: filters.status,
          page: filters.page,
          page_size: filters.pageSize,
        },
      })
      return toApplicationPage(dto)
    },
    async get(applicationId) {
      return toApplicationDetail(await http<ApplicationDetailDto>(`/applications/${segment(applicationId)}`))
    },
    async review(planId, fieldId) {
      const dto = await http<ApplicationReviewDto>(
        `/planner/plans/${segment(planId)}/entries/${segment(fieldId)}/review`,
        { method: 'POST' },
      )
      return toApplicationReview(dto)
    },
    async register(input) {
      const dto = await http<RegisteredApplicationDto>('/applications', {
        method: 'POST',
        body: toApplicationRegistrationDto(input),
      })
      return toRegisteredApplication(dto)
    },
  }
}

export function createTodayHttpService(http: HttpClient): TodayService {
  return {
    async getBrief() {
      return toTodayBrief(await http<TodayBriefDto>('/planner/today'))
    },
    async startTask(taskId) {
      const dto = await http<TodayTaskDto>(`/planner/today/tasks/${segment(taskId)}/start`, { method: 'POST' })
      return toTodayTask(dto)
    },
  }
}

export function createWeatherHttpService(http: HttpClient): WeatherService {
  return {
    async getCurrent(farmId) {
      return toCurrentWeather(await http<CurrentWeatherDto>('/weather/current', { query: { farm_id: farmId } }))
    },
  }
}
