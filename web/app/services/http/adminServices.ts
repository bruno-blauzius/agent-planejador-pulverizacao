/** HTTP adapters of page 03 (ADR 0013, 0015) against services/api. */
import type { CompanyService, DashboardService, ProfileService, UserService } from '../ports'
import type { InvitationEmailStatus } from '~/types/admin'
import type {
  CompaniesSummaryDto,
  CompanyDto,
  DashboardDto,
  PageDto,
  ProfileDto,
  UserAccountDto,
  UsersSummaryDto,
} from '~/types/dto'
import type { HttpClient, ResponseHeaders } from './client'
import {
  toCompaniesSummary,
  toCompany,
  toCompanyInputDto,
  toCompanyUpdateDto,
  toDashboard,
  toInvitationInputDto,
  toProfile,
  toUserAccount,
  toUsersSummary,
  toUserUpdateDto,
} from './adminMappers'
import { toPage } from './mappers'

const segment = encodeURIComponent

export const INVITATION_EMAIL_HEADER = 'X-Invitation-Email'

/**
 * `X-Invitation-Email` (sent | not_sent | not_needed). A missing or unknown
 * value is `unknown`, never `sent`: cross-origin the header is only readable
 * when the API lists it in `Access-Control-Expose-Headers`, and claiming a
 * delivery nobody confirmed would be dishonest.
 */
export function readInvitationEmailStatus(headers: ResponseHeaders): InvitationEmailStatus {
  const value = headers.get(INVITATION_EMAIL_HEADER)?.trim().toLowerCase()
  return value === 'sent' || value === 'not_sent' || value === 'not_needed' ? value : 'unknown'
}

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
    async get(userId) {
      return toUserAccount(await http<UserAccountDto>(`/users/${segment(userId)}`))
    },
    async update(userId, input) {
      const dto = await http<UserAccountDto>(`/users/${segment(userId)}`, { method: 'PATCH', body: toUserUpdateDto(input) })
      return toUserAccount(dto)
    },
    async remove(userId) {
      await http<unknown>(`/users/${segment(userId)}`, { method: 'DELETE' })
    },
    async invite(input) {
      const { data, headers } = await http.withHeaders<UserAccountDto>('/invitations', {
        method: 'POST',
        body: toInvitationInputDto(input),
      })
      return { user: toUserAccount(data), emailStatus: readInvitationEmailStatus(headers) }
    },
    async resendInvitation(invitationId) {
      const { headers } = await http.withHeaders<unknown>(`/invitations/${segment(invitationId)}/resend`, { method: 'POST' })
      return readInvitationEmailStatus(headers)
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
    async get(companyId) {
      return toCompany(await http<CompanyDto>(`/companies/${segment(companyId)}`))
    },
    async create(input) {
      return toCompany(await http<CompanyDto>('/companies', { method: 'POST', body: toCompanyInputDto(input) }))
    },
    async update(companyId, input) {
      const dto = await http<CompanyDto>(`/companies/${segment(companyId)}`, { method: 'PATCH', body: toCompanyUpdateDto(input) })
      return toCompany(dto)
    },
    async remove(companyId) {
      await http<unknown>(`/companies/${segment(companyId)}`, { method: 'DELETE' })
    },
  }
}

export function createProfileHttpService(http: HttpClient): ProfileService {
  return {
    async get() {
      return toProfile(await http<ProfileDto>('/me/profile'))
    },
    async update(input) {
      // Only the name: the API ignores e-mail, role and company in this PATCH.
      return toProfile(await http<ProfileDto>('/me/profile', { method: 'PATCH', body: { name: input.name.trim() } }))
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
