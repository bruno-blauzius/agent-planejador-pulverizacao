import { computed, type ComputedRef } from 'vue'
import type { CompanyRef, FarmScope, Permission, UserRole } from '~/types/auth'
import { hasPermission, homePathFor, isFarmInScope } from '~/utils/permissions'
import { useSession } from './useSession'

export interface Permissions {
  /** True when the session has the capability (false while signed out). */
  readonly can: (permission: Permission) => boolean
  readonly permissions: ComputedRef<readonly Permission[]>
  /** For labels only ("Administradora"); never branch on it, use `can`. */
  readonly role: ComputedRef<UserRole | null>
  readonly company: ComputedRef<CompanyRef | null>
  readonly farmScope: ComputedRef<FarmScope | null>
  /** "/" with `dashboard.view`, otherwise "/farms". */
  readonly homePath: ComputedRef<string>
  readonly isFarmInScope: (farmId: string) => boolean
}

/**
 * Single entry point for authorization in the UI (ADR 0013): menu items,
 * buttons, routes and messages ask `can()`. Capabilities come from the API
 * session (`GET /auth/me`); the API still enforces them with 403.
 */
export function usePermissions(): Permissions {
  const { user } = useSession()
  const permissions = computed<readonly Permission[]>(() => user.value?.permissions ?? [])

  return {
    can: permission => hasPermission(permissions.value, permission),
    permissions,
    role: computed(() => user.value?.role ?? null),
    company: computed(() => user.value?.company ?? null),
    farmScope: computed(() => user.value?.farmScope ?? null),
    homePath: computed(() => homePathFor(permissions.value)),
    isFarmInScope: (farmId) => {
      const scope = user.value?.farmScope
      return Boolean(scope && isFarmInScope(scope, farmId))
    },
  }
}
