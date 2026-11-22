/**
 * Presentation rules of the invite / access modal (screen 13, Figma 52:758).
 * Which roles and companies are offered follows the session capabilities;
 * the API enforces the same rules with 403.
 */
import type { FieldError } from '~/services/errors'
import type { InvitationInput, UserAccount, UserUpdateInput } from '~/types/admin'
import type { FarmScope, UserRole } from '~/types/auth'
import { USER_ROLE_LABELS } from './labels'

export type FarmScopeMode = 'all' | 'selected'
export type UserAccessField = 'name' | 'email' | 'role' | 'companyId' | 'farmIds'
export type UserAccessErrors = Partial<Record<UserAccessField, string>>

export const USER_ACCESS_FIELDS: readonly UserAccessField[] = ['name', 'email', 'role', 'companyId', 'farmIds']

export interface UserAccessValues {
  name: string
  email: string
  role: UserRole | null
  companyId: string | null
  farmScopeMode: FarmScopeMode
  farmIds: string[]
}

/** Structurally compatible with `OptionCardItem` (components/OptionCardGroup.vue). */
export interface RoleOptionView {
  readonly value: UserRole
  readonly title: string
  readonly description: string
  readonly icon: 'shield' | 'users' | 'sprout'
  readonly unavailableHint?: string
}

const ROLE_OPTION_COPY: Readonly<Record<UserRole, { description: string, icon: RoleOptionView['icon'] }>> = {
  super_admin: { description: 'Gestão do sistema: empresas, usuários e planos.', icon: 'shield' },
  admin: { description: 'Gerencia cadastros e usuários da própria empresa.', icon: 'users' },
  planning_operator: { description: 'Usa o Planejador e registra aplicações nas fazendas liberadas.', icon: 'sprout' },
}

export const FARM_SCOPE_LABELS: Readonly<Record<FarmScopeMode, string>> = {
  all: 'Todas da empresa',
  selected: 'Fazendas selecionadas',
}

export const USER_ACCESS_MESSAGES = {
  name: 'Informe o nome.',
  email: 'Informe um e-mail válido.',
  role: 'Escolha o papel.',
  companyId: 'Escolha a empresa.',
  farmIds: 'Selecione ao menos uma fazenda.',
  emailTaken: 'Este e-mail já tem cadastro.',
  forbidden: 'Você não tem permissão para dar esse papel ou acesso a essa empresa.',
  network: 'Não foi possível conectar ao servidor. Verifique sua conexão e tente novamente.',
  notFound: 'O usuário não existe mais.',
  generic: 'Não foi possível salvar. Tente novamente.',
} as const

const EMAIL_PATTERN = /^[^\s@]+@[^\s@]+\.[^\s@]+$/

/** Only the planning operator has a farm scope (ADR 0013); other roles see every farm of the company. */
export function roleHasFarmScope(role: UserRole | null): boolean {
  return role === 'planning_operator'
}

export function userRoleOptions(canManageAll: boolean, lockedRole: UserRole | null = null): RoleOptionView[] {
  const roles: UserRole[] = canManageAll ? ['super_admin', 'admin', 'planning_operator'] : ['admin', 'planning_operator']
  return roles.map(role => ({
    value: role,
    title: USER_ROLE_LABELS[role],
    ...ROLE_OPTION_COPY[role],
    ...(lockedRole && role !== lockedRole ? { unavailableHint: 'Você não pode alterar o próprio papel.' } : {}),
  }))
}

export function emptyUserAccessValues(companyId: string | null): UserAccessValues {
  return { name: '', email: '', role: null, companyId, farmScopeMode: 'all', farmIds: [] }
}

export function accountToValues(account: UserAccount): UserAccessValues {
  return {
    name: account.name,
    email: account.email,
    role: account.role,
    companyId: account.company.id,
    farmScopeMode: account.farmScope.mode,
    farmIds: account.farmScope.mode === 'selected' ? [...account.farmScope.farmIds] : [],
  }
}

export function toFarmScope(values: Pick<UserAccessValues, 'role' | 'farmScopeMode' | 'farmIds'>): FarmScope {
  return roleHasFarmScope(values.role) && values.farmScopeMode === 'selected'
    ? { mode: 'selected', farmIds: [...values.farmIds] }
    : { mode: 'all' }
}

export function validateUserAccess(values: UserAccessValues): UserAccessErrors {
  const errors: UserAccessErrors = {}
  if (!values.name.trim()) errors.name = USER_ACCESS_MESSAGES.name
  if (!EMAIL_PATTERN.test(values.email.trim())) errors.email = USER_ACCESS_MESSAGES.email
  if (!values.role) errors.role = USER_ACCESS_MESSAGES.role
  if (!values.companyId) errors.companyId = USER_ACCESS_MESSAGES.companyId
  if (roleHasFarmScope(values.role) && values.farmScopeMode === 'selected' && values.farmIds.length === 0) {
    errors.farmIds = USER_ACCESS_MESSAGES.farmIds
  }
  return errors
}

/** Call only after `validateUserAccess` returned no errors. */
export function toInvitationInput(values: UserAccessValues): InvitationInput {
  return {
    name: values.name.trim(),
    email: values.email.trim(),
    role: values.role ?? 'planning_operator',
    companyId: values.companyId ?? '',
    farmScope: toFarmScope(values),
  }
}

function sameScope(a: FarmScope, b: FarmScope): boolean {
  if (a.mode !== b.mode) return false
  if (a.mode === 'all' || b.mode === 'all') return true
  return [...a.farmIds].sort().join(',') === [...b.farmIds].sort().join(',')
}

/** PATCH body with only what changed; empty object when nothing changed. */
export function toUserUpdateInput(account: UserAccount, values: UserAccessValues): UserUpdateInput {
  const scope = toFarmScope(values)
  return {
    ...(values.role && values.role !== account.role ? { role: values.role } : {}),
    ...(values.companyId && values.companyId !== account.company.id ? { companyId: values.companyId } : {}),
    ...(!sameScope(scope, account.farmScope) ? { farmScope: scope } : {}),
  }
}

const API_FIELD_MAP: Readonly<Record<string, UserAccessField>> = {
  'name': 'name',
  'email': 'email',
  'role': 'role',
  'company_id': 'companyId',
  'farm_scope': 'farmIds',
  'farm_scope.farm_ids': 'farmIds',
}

export function userAccessErrorsFromApi(details: readonly FieldError[]): UserAccessErrors {
  const errors: UserAccessErrors = {}
  for (const detail of details) {
    const field = API_FIELD_MAP[detail.field]
    if (field && !errors[field]) errors[field] = detail.message
  }
  return errors
}
