import type { FarmRef } from './farm'

/** Roles of ADR 0013. The UI never branches on them: it checks `permissions`. */
export type UserRole = 'super_admin' | 'admin' | 'planning_operator'

/** Capabilities returned by the API in the session (table in api/openapi.yaml, `Permission`). */
export type Permission =
  | 'dashboard.view'
  | 'companies.manage'
  | 'users.manage'
  | 'users.manage_all'
  | 'registry.write'
  | 'applications.register'
  | 'applications.confirm'
  | 'planner.use'

export interface CompanyRef {
  readonly id: string
  readonly name: string
}

/** Farms the user sees: every farm of the company, or only `farmIds`. */
export type FarmScope =
  | { readonly mode: 'all' }
  | { readonly mode: 'selected', readonly farmIds: readonly string[] }

export interface User {
  readonly id: string
  readonly name: string
  readonly email: string
  readonly role: UserRole
  readonly primaryFarm: FarmRef | null
  readonly company: CompanyRef
  readonly farmScope: FarmScope
  readonly permissions: readonly Permission[]
}

export interface Credentials {
  /** E-mail or CPF (masked or not). */
  readonly identifier: string
  readonly password: string
  /** "Manter conectado": the API answers with a longer `expiresAt`. */
  readonly rememberMe: boolean
}

export interface AuthSession {
  readonly accessToken: string
  readonly expiresAt: Date
  readonly user: User
}
