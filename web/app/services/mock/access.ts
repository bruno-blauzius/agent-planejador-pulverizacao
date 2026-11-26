/**
 * Authorization of the in-memory adapter (ADR 0013). It plays the role of the
 * API: derives capabilities from the role with the same table documented in
 * api/openapi.yaml (`Permission`) and applies company/farm scope. The front
 * never imports this file: it only reads `User.permissions`.
 */
import type { FarmScope, Permission, UserRole } from '~/types/auth'

export const ROLE_PERMISSIONS: Readonly<Record<UserRole, readonly Permission[]>> = {
  super_admin: [
    'dashboard.view',
    'companies.manage',
    'users.manage',
    'users.manage_all',
    'registry.write',
    'applications.register',
    'applications.confirm',
    'planner.use',
    'field_conditions.record',
  ],
  admin: [
    'dashboard.view',
    'users.manage',
    'registry.write',
    'applications.register',
    'applications.confirm',
    'planner.use',
    'field_conditions.record',
  ],
  planning_operator: ['applications.register', 'planner.use', 'field_conditions.record'],
}

/** Who is calling, as the API would read it from the token. */
export interface Viewer {
  readonly id: string
  readonly role: UserRole
  readonly companyId: string
  readonly farmScope: FarmScope
  readonly permissions: readonly Permission[]
}

export type FarmAccess = 'visible' | 'out_of_scope' | 'hidden'

/**
 * `hidden`: another company (the API answers 404, never revealing it exists).
 * `out_of_scope`: same company, but not among the operator's released farms (403).
 */
export function farmAccess(viewer: Viewer, farm: { id: string, companyId: string }): FarmAccess {
  if (viewer.permissions.includes('users.manage_all')) return 'visible'
  if (farm.companyId !== viewer.companyId) return 'hidden'
  if (viewer.farmScope.mode === 'selected' && !viewer.farmScope.farmIds.includes(farm.id)) return 'out_of_scope'
  return 'visible'
}

export function canSeeFarm(viewer: Viewer, farm: { id: string, companyId: string }): boolean {
  return farmAccess(viewer, farm) === 'visible'
}
