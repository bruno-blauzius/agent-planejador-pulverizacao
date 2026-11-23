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

/**
 * How the user signs in. `sso`: the real API (redirect to the Keycloak of the
 * Inceres SSO, session in an HttpOnly cookie; ADR 0015). `password`: the dev
 * mock (in-memory form and Bearer token; ADR 0010).
 */
export type AuthKind = 'sso' | 'password'

/** 403 codes of `GET /auth/me` and `GET /auth/callback` that deny access to the product. */
export type ProductAccessCode = 'user_not_provisioned' | 'no_access_to_product' | 'ambiguous_role' | 'forbidden'

/** Dev mock only: the real login has no password in the front. */
export interface Credentials {
  /** E-mail. */
  readonly identifier: string
  readonly password: string
  /** "Manter conectado": the mock answers with a longer `expiresAt`. */
  readonly rememberMe: boolean
}

/** Dev mock only: the real session is the `agro_session` cookie, invisible to JavaScript. */
export interface AuthSession {
  readonly accessToken: string
  readonly expiresAt: Date
  readonly user: User
}
