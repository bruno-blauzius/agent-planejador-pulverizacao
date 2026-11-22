/** HTTP adapters of page 03 (ADR 0013) against services/api. */
import type { CompanyService, DashboardService, ProfileService, UserService } from '../ports'
import type {
  CompaniesSummaryDto,
  CompanyDto,
  DashboardDto,
  PageDto,
  ProfileDto,
  UserAccountDto,
  UsersSummaryDto,
} from '~/types/dto'
import type { HttpClient } from './client'
import {
  toCompaniesSummary,
  toCompany,
  toCompanyInputDto,
  toCompanyUpdateDto,
  toDashboard,
  toInvitationInputDto,
  toPasswordChangeDto,
  toProfile,
  toUserAccount,
  toUsersSummary,
  toUserUpdateDto,
} from './adminMappers'
import { toPage } from './mappers'

const segment = encodeURIComponent

export function createUserHttpService(http: HttpClient): UserService {
  return {
    async list(filters = {}) {
      const dto = await http<PageDto<UserAccountDto>>('/users', {
        query: {
          search: filters.search,
          role: filters.role,
          status: filters.status,
          company_id: filters.companyId,
          page: filters.page,
          page_size: filters.pageSize,
        },
      })
      return toPage(dto, toUserAccount)
    },
    async summary() {
      return toUsersSummary(await http<UsersSummaryDto>('/users/summary'))
    },
    async update(userId, input) {
      const dto = await http<UserAccountDto>(`/users/${segment(userId)}`, { method: 'PATCH', body: toUserUpdateDto(input) })
      return toUserAccount(dto)
    },
    async invite(input) {
      return toUserAccount(await http<UserAccountDto>('/invitations', { method: 'POST', body: toInvitationInputDto(input) }))
    },
    async resendInvitation(invitationId) {
      await http<unknown>(`/invitations/${segment(invitationId)}/resend`, { method: 'POST' })
    },
    async cancelInvitation(invitationId) {
      await http<unknown>(`/invitations/${segment(invitationId)}`, { method: 'DELETE' })
    },
  }
}

export function createCompanyHttpService(http: HttpClient): CompanyService {
  return {
    async list(filters = {}) {
      const dto = await http<PageDto<CompanyDto>>('/companies', {
        query: { search: filters.search, status: filters.status, page: filters.page, page_size: filters.pageSize },
      })
      return toPage(dto, toCompany)
    },
    async summary() {
      return toCompaniesSummary(await http<CompaniesSummaryDto>('/companies/summary'))
    },
    async create(input) {
      return toCompany(await http<CompanyDto>('/companies', { method: 'POST', body: toCompanyInputDto(input) }))
    },
    async update(companyId, input) {
      const dto = await http<CompanyDto>(`/companies/${segment(companyId)}`, { method: 'PATCH', body: toCompanyUpdateDto(input) })
      return toCompany(dto)
    },
  }
}

export function createProfileHttpService(http: HttpClient): ProfileService {
  return {
    async get() {
      return toProfile(await http<ProfileDto>('/me/profile'))
    },
    async update(input) {
      return toProfile(await http<ProfileDto>('/me/profile', { method: 'PATCH', body: { name: input.name.trim() } }))
    },
    async changePassword(input) {
      await http<unknown>('/me/password', { method: 'POST', body: toPasswordChangeDto(input) })
    },
  }
}

export function createDashboardHttpService(http: HttpClient): DashboardService {
  return {
    async get(filters = {}) {
      const dto = await http<DashboardDto>('/dashboard', {
        query: { farm_id: filters.farmId, period_days: filters.periodDays },
      })
      return toDashboard(dto)
    },
  }
}
