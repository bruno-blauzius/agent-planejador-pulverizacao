/**
 * Presentation helpers over the capabilities the API returns in the session
 * (ADR 0013). Hiding something is not security: the API answers 403 anyway.
 * Never derive capabilities from the role name here.
 */
import type { ApiError } from '~/services/errors'
import type { FarmScope, Permission } from '~/types/auth'
import { DEFAULT_AUTHENTICATED_PATH } from './redirect'

export const DASHBOARD_PATH = '/'

export const PERMISSION_DENIED_FLASH = {
  kind: 'error',
  title: 'Acesso não permitido',
  description: 'Você não tem permissão para acessar essa página. Fale com o administrador da sua empresa.',
} as const

/** Honest messages for a write refused by the API (403). */
export const FORBIDDEN_MESSAGES = {
  forbidden: 'Seu perfil não permite esta ação. Fale com o administrador da sua empresa.',
  out_of_scope: 'Esta fazenda não está liberada para você. Fale com o administrador da sua empresa.',
} as const

/** pt-BR message for `forbidden`/`out_of_scope`; null for any other error. */
export function forbiddenMessageFor(error: Pick<ApiError, 'code'> | null | undefined): string | null {
  if (error?.code === 'forbidden' || error?.code === 'out_of_scope') return FORBIDDEN_MESSAGES[error.code]
  return null
}

export function hasPermission(permissions: readonly Permission[], permission: Permission): boolean {
  return permissions.includes(permission)
}

/** Landing page after login: the dashboard for who can see it, otherwise the farm list. */
export function homePathFor(permissions: readonly Permission[]): string {
  return hasPermission(permissions, 'dashboard.view') ? DASHBOARD_PATH : DEFAULT_AUTHENTICATED_PATH
}

export function isFarmInScope(scope: FarmScope, farmId: string): boolean {
  return scope.mode === 'all' || scope.farmIds.includes(farmId)
}
