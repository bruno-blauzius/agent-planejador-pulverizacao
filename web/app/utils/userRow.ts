/**
 * View models of the "Usuários" screen (Figma 52:289). Presentation only: what
 * a role may do is decided by the API (ADR 0013); the copy of the role cards is
 * descriptive text from the design, never used to authorize anything.
 */
import type { UserAccount, UserFilters, UserStatus, UsersSummary } from '~/types/admin'
import type { UserRole } from '~/types/auth'
import { USER_ROLE_LABELS } from './labels'
import { DISPLAY_TIME_ZONE } from './timeZone'

export type UserListFilter = 'all' | UserRole | 'invited'
export type UserRowActionKind = 'edit_access' | 'deactivate' | 'activate' | 'resend_invitation' | 'cancel_invitation'
export type RoleTone = 'cafe' | 'green' | 'neutral'
export type UserStatusTone = 'suitable' | 'attention' | 'muted'

export const USER_ROLES: readonly UserRole[] = ['super_admin', 'admin', 'planning_operator']
export const USER_LIST_FILTERS: readonly UserListFilter[] = ['all', 'super_admin', 'admin', 'planning_operator', 'invited']

export const USER_LIST_FILTER_LABELS: Readonly<Record<UserListFilter, string>> = {
  all: 'Todos',
  super_admin: 'Super admin',
  admin: 'Administrador',
  planning_operator: 'Operador',
  invited: 'Convites pendentes',
}

export const ROLE_TONES: Readonly<Record<UserRole, RoleTone>> = {
  super_admin: 'cafe',
  admin: 'green',
  planning_operator: 'neutral',
}

export const USER_STATUS_VIEW: Readonly<Record<UserStatus, { label: string, tone: UserStatusTone }>> = {
  active: { label: 'Ativo', tone: 'suitable' },
  invited: { label: 'Convite pendente', tone: 'attention' },
  inactive: { label: 'Inativo', tone: 'muted' },
}

/** Copy of the role cards (Figma 52:317). */
const ROLE_CARD_COPY: Readonly<Record<UserRole, { description: string, capabilities: readonly string[] }>> = {
  super_admin: {
    description: 'Gestão do sistema: empresas, usuários e planos. Vê todas as empresas.',
    capabilities: ['Empresas', 'Usuários de todas as empresas', 'Configurações do sistema'],
  },
  admin: {
    description: 'Gerencia a própria empresa: fazendas, talhões, proprietários e usuários dela.',
    capabilities: ['Cadastros da empresa', 'Usuários da empresa', 'Confirma aplicações'],
  },
  planning_operator: {
    description: 'Usa o Planejador e registra aplicações nas fazendas liberadas para ele.',
    capabilities: ['Planejador e agente', 'Registro de aplicações', 'Somente leitura nos cadastros'],
  },
}

export interface UserRowAction {
  readonly kind: UserRowActionKind
  readonly label: string
  readonly danger: boolean
  readonly disabledHint: string | null
  readonly confirm: { readonly title: string, readonly description: string, readonly confirmLabel: string } | null
}

export interface UserRowView {
  readonly id: string
  readonly name: string
  readonly email: string
  readonly initials: string
  readonly roleLabel: string
  readonly roleTone: RoleTone
  readonly companyName: string
  readonly lastAccessLabel: string
  readonly status: { readonly value: UserStatus, readonly label: string, readonly tone: UserStatusTone }
  readonly isSelf: boolean
  readonly highlighted: boolean
  readonly actions: readonly UserRowAction[]
}

export interface RoleCardView {
  readonly role: UserRole
  readonly label: string
  readonly tone: RoleTone
  readonly countLabel: string
  readonly description: string
  readonly capabilities: readonly string[]
}

/** First and last name, as in the Figma avatars: "Ana Lúcia Prado" → "AP". */
export function userInitials(name: string): string {
  const words = name.trim().split(/\s+/).filter(Boolean)
  const letters = words.length > 1 ? [words[0]!, words[words.length - 1]!] : words
  return letters.map(word => word.charAt(0)).join('').toUpperCase()
}

export const SELF_ACTION_HINT = 'Você não pode alterar a própria conta.'
const PLACEHOLDER = '—'

export function usersCountLabel(count: number): string {
  return count === 1 ? '1 usuário' : `${count} usuários`
}

export function toRoleCards(roles: readonly UserRole[], summary: UsersSummary | null): RoleCardView[] {
  return roles.map(role => ({
    role,
    label: USER_ROLE_LABELS[role],
    tone: ROLE_TONES[role],
    countLabel: summary ? usersCountLabel(summary.byRole[role]) : PLACEHOLDER,
    ...ROLE_CARD_COPY[role],
  }))
}

/** "Administração · 9 usuários em 5 empresas" (all companies) or "… · 4 usuários · Fazenda Santa Rita". */
export function usersEyebrow(summary: UsersSummary, allCompanies: boolean, companyName: string | null): string {
  const users = usersCountLabel(summary.totalUsers)
  if (allCompanies) {
    const companies = summary.companyCount === 1 ? '1 empresa' : `${summary.companyCount} empresas`
    return `Administração · ${users} em ${companies}`
  }
  return companyName ? `Administração · ${users} · ${companyName}` : `Administração · ${users}`
}

export function filterToUserFilters(filter: UserListFilter): Pick<UserFilters, 'role' | 'status'> {
  if (filter === 'all') return {}
  if (filter === 'invited') return { status: 'invited' }
  return { role: filter }
}

export function userFilterCount(filter: UserListFilter, summary: UsersSummary | null): number | null {
  if (!summary) return null
  if (filter === 'all') return summary.totalUsers
  if (filter === 'invited') return summary.pendingInvitations
  return summary.byRole[filter]
}

// ---- last access ----------------------------------------------------------

const MINUTE_MS = 60_000
const HOUR_MS = 60 * MINUTE_MS
const RECENT_HOURS = 3
const RECENT_DAYS = 30

const dayKey = new Intl.DateTimeFormat('en-CA', { timeZone: DISPLAY_TIME_ZONE, year: 'numeric', month: '2-digit', day: '2-digit' })
const timeOfDay = new Intl.DateTimeFormat('pt-BR', { timeZone: DISPLAY_TIME_ZONE, hour: '2-digit', minute: '2-digit' })
const shortDate = new Intl.DateTimeFormat('pt-BR', { timeZone: DISPLAY_TIME_ZONE, day: '2-digit', month: '2-digit', year: 'numeric' })

function calendarDaysBetween(earlier: Date, later: Date): number {
  const toUtcMidnight = (date: Date) => Date.parse(`${dayKey.format(date)}T00:00:00Z`)
  return Math.round((toUtcMidnight(later) - toUtcMidnight(earlier)) / (24 * HOUR_MS))
}

/** "agora", "há 12 min", "há 2 h", "hoje, 06:48", "ontem", "há 5 dias", "03/10/2026" or "—". */
export function formatLastAccess(date: Date | null, now: Date): string {
  if (!date) return PLACEHOLDER
  const elapsed = Math.max(0, now.getTime() - date.getTime())
  if (elapsed < MINUTE_MS) return 'agora'
  if (elapsed < HOUR_MS) return `há ${Math.floor(elapsed / MINUTE_MS)} min`
  if (elapsed < RECENT_HOURS * HOUR_MS) return `há ${Math.floor(elapsed / HOUR_MS)} h`
  const days = calendarDaysBetween(date, now)
  if (days <= 0) return `hoje, ${timeOfDay.format(date)}`
  if (days === 1) return 'ontem'
  if (days < RECENT_DAYS) return `há ${days} dias`
  return shortDate.format(date)
}

// ---- rows -------------------------------------------------------------------

function action(kind: UserRowActionKind, label: string, extra: Partial<UserRowAction> = {}): UserRowAction {
  return { kind, label, danger: false, disabledHint: null, confirm: null, ...extra }
}

/** Menu "…" by status. Self-protection mirrors the API rule so the user is not offered a sure 403. */
export function userRowActions(account: UserAccount, isSelf: boolean): UserRowAction[] {
  if (account.status === 'invited') {
    return [
      action('resend_invitation', 'Reenviar convite'),
      action('cancel_invitation', 'Cancelar convite', {
        danger: true,
        confirm: {
          title: 'Cancelar convite?',
          description: `${account.name} não poderá mais criar a senha com o link enviado para ${account.email}.`,
          confirmLabel: 'Cancelar convite',
        },
      }),
    ]
  }
  const edit = action('edit_access', 'Editar papel e acesso')
  if (account.status === 'inactive') return [edit, action('activate', 'Reativar')]
  return [
    edit,
    action('deactivate', 'Inativar', {
      danger: true,
      disabledHint: isSelf ? SELF_ACTION_HINT : null,
      confirm: {
        title: `Inativar ${account.name}?`,
        description: 'A pessoa perde o acesso ao sistema até ser reativada. Os registros dela são mantidos.',
        confirmLabel: 'Inativar',
      },
    }),
  ]
}

export function toUserRowView(account: UserAccount, context: { now: Date, selfId: string | null, highlightId: string | null }): UserRowView {
  const isSelf = account.id === context.selfId
  return {
    id: account.id,
    name: account.name,
    email: account.email,
    initials: userInitials(account.name),
    roleLabel: USER_ROLE_LABELS[account.role],
    roleTone: ROLE_TONES[account.role],
    companyName: account.company.name,
    lastAccessLabel: formatLastAccess(account.lastAccessAt, context.now),
    status: { value: account.status, ...USER_STATUS_VIEW[account.status] },
    isSelf,
    highlighted: account.id === context.highlightId,
    actions: userRowActions(account, isSelf),
  }
}

/** "Mostrando 1–6 de 9"; "Mostrando 0 de 0" when empty. */
export function userRangeLabel(page: number, pageSize: number, shown: number, total: number): string {
  if (total === 0 || shown === 0) return `Mostrando 0 de ${total}`
  const first = (page - 1) * pageSize + 1
  return `Mostrando ${first}–${first + shown - 1} de ${total}`
}
