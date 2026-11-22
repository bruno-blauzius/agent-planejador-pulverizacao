/** Administration models of page 03 (ADR 0013): users, invitations, companies. */
import type { CompanyRef, FarmScope, UserRole } from './auth'
import type { PageRequest } from './pagination'

/** invited = convite pendente (a pessoa ainda não criou a senha). */
export type UserStatus = 'active' | 'invited' | 'inactive'

/** Row of the "Usuários" table: a user or a pending invitation. */
export interface UserAccount {
  readonly id: string
  readonly name: string
  readonly email: string
  readonly role: UserRole
  readonly company: CompanyRef
  readonly farmScope: FarmScope
  readonly status: UserStatus
  readonly lastAccessAt: Date | null
  /** Only for `status: 'invited'`. */
  readonly invitation: { readonly id: string, readonly expiresAt: Date } | null
}

export interface UserFilters extends PageRequest {
  readonly search?: string
  readonly role?: UserRole
  readonly status?: UserStatus
  /** Super admin only; ignored by the API for other roles. */
  readonly companyId?: string
}

export interface UsersSummary {
  readonly totalUsers: number
  readonly companyCount: number
  readonly pendingInvitations: number
  readonly byRole: Readonly<Record<UserRole, number>>
}

export interface UserUpdateInput {
  readonly role?: UserRole
  readonly companyId?: string
  readonly farmScope?: FarmScope
  readonly status?: 'active' | 'inactive'
}

export interface InvitationInput {
  readonly name: string
  readonly email: string
  readonly role: UserRole
  readonly companyId: string
  readonly farmScope: FarmScope
}

/** trial = em avaliação. */
export type CompanyPlan = 'basic' | 'pro' | 'trial'
export type CompanyStatus = 'active' | 'trial' | 'inactive'

export interface Company {
  readonly id: string
  readonly name: string
  /** CNPJ digits. */
  readonly document: string
  readonly city: string
  readonly state: string
  readonly plan: CompanyPlan
  readonly status: CompanyStatus
  readonly trialEndsAt: Date | null
  readonly farmCount: number
  readonly userCount: number
  readonly createdAt: Date
}

export interface CompanyFilters extends PageRequest {
  readonly search?: string
  readonly status?: CompanyStatus
}

export interface CompaniesSummary {
  /** Includes companies in trial. */
  readonly activeCompanies: number
  readonly trialCompanies: number
  readonly farmCount: number
  readonly monitoredAreaHa: number
  readonly userCount: number
  readonly pendingInvitations: number
  readonly applicationsThisMonth: number
  readonly applicationsPreviousMonth: number
}

export interface CompanyInput {
  readonly name: string
  readonly document: string
  readonly city: string
  readonly state: string
  readonly plan: CompanyPlan
}

export interface CompanyUpdateInput {
  readonly name?: string
  readonly city?: string
  readonly state?: string
  readonly plan?: CompanyPlan
  readonly status?: CompanyStatus
}
