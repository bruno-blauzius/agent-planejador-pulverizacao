/** Presentation rules of the companies screen (14): stat cards, rows and pagination labels. */
import type { CompaniesSummary, Company, CompanyPlan, CompanyStatus } from '~/types/admin'
import { formatCnpj, formatHectares, initials } from './format'
import { DISPLAY_TIME_ZONE } from './timeZone'

export interface CompanyStatCardView {
  readonly key: 'companies' | 'farms' | 'users' | 'applications'
  readonly label: string
  readonly value: string
  readonly caption: string | null
  readonly highlight: boolean
}

export type CompanyStatusTone = 'success' | 'warning' | 'neutral'

export type CompanyRowActionKind = 'edit' | 'deactivate' | 'reactivate' | 'delete'

export interface CompanyRowAction {
  readonly kind: CompanyRowActionKind
  readonly label: string
  readonly tone: 'default' | 'danger'
}

export interface CompanyRowView {
  readonly id: string
  readonly name: string
  readonly initials: string
  readonly locationLabel: string
  readonly documentLabel: string
  readonly plan: { readonly value: CompanyPlan, readonly label: string }
  readonly farmCountLabel: string
  readonly userCountLabel: string
  readonly status: { readonly value: CompanyStatus, readonly label: string, readonly tone: CompanyStatusTone }
  /** Row menu entries; empty without `companies.manage`. */
  readonly actions: readonly CompanyRowAction[]
  readonly highlighted: boolean
}

export interface CompanyPaginationView {
  readonly rangeLabel: string
  readonly page: number
  readonly pageCount: number
  readonly canPrevious: boolean
  readonly canNext: boolean
}

export const COMPANY_PLAN_LABELS: Readonly<Record<CompanyPlan, string>> = {
  pro: 'Pro',
  basic: 'Básico',
  trial: 'Avaliação',
}

const STATUS_LABELS: Readonly<Record<CompanyStatus, string>> = {
  active: 'Ativa',
  trial: 'Em avaliação',
  inactive: 'Inativa',
}

const STATUS_TONES: Readonly<Record<CompanyStatus, CompanyStatusTone>> = {
  active: 'success',
  trial: 'warning',
  inactive: 'neutral',
}

const PLACEHOLDER = '—'
const DAY_MS = 86_400_000
const integer = new Intl.NumberFormat('pt-BR')
// The month name is formatted from a UTC date already shifted to the display zone.
const monthName = new Intl.DateTimeFormat('pt-BR', { month: 'long', timeZone: 'UTC' })
const yearMonthInZone = new Intl.DateTimeFormat('en-CA', { timeZone: DISPLAY_TIME_ZONE, year: 'numeric', month: '2-digit' })

function plural(count: number, singular: string, pluralForm: string): string {
  return `${integer.format(count)} ${count === 1 ? singular : pluralForm}`
}

/** "+18% vs. outubro"; month = the one before `now`. */
export function applicationsTrendCaption(current: number, previous: number, now: Date): string {
  if (previous <= 0) return current > 0 ? 'Primeiro mês com aplicações' : 'Sem aplicações no mês anterior'
  const change = Math.round(((current - previous) / previous) * 100)
  // "Mês anterior" is relative to the month in the display time zone, not the machine's.
  const [year, month] = yearMonthInZone.format(now).split('-').map(Number)
  const previousMonth = monthName.format(new Date(Date.UTC(year ?? 1970, (month ?? 1) - 2, 1)))
  const sign = change > 0 ? '+' : change < 0 ? '−' : ''
  return `${sign}${Math.abs(change)}% vs. ${previousMonth}`
}

/** The 4 cards; `summary` null (loading or error) shows "—" without captions. */
export function buildCompanyStatCards(summary: CompaniesSummary | null, now: Date): CompanyStatCardView[] {
  const value = (count: number | undefined) => (summary && count !== undefined ? integer.format(count) : PLACEHOLDER)
  return [
    {
      key: 'companies',
      label: 'Empresas ativas',
      value: value(summary?.activeCompanies),
      caption: summary && summary.trialCompanies > 0 ? `${integer.format(summary.trialCompanies)} em avaliação` : null,
      highlight: false,
    },
    {
      key: 'farms',
      label: 'Fazendas no sistema',
      value: value(summary?.farmCount),
      caption: summary ? `${formatHectares(summary.monitoredAreaHa, 0)} monitorados` : null,
      highlight: false,
    },
    {
      key: 'users',
      label: 'Usuários',
      value: value(summary?.userCount),
      caption: summary && summary.pendingInvitations > 0
        ? plural(summary.pendingInvitations, 'convite pendente', 'convites pendentes')
        : null,
      highlight: false,
    },
    {
      key: 'applications',
      label: 'Aplicações no mês',
      value: value(summary?.applicationsThisMonth),
      caption: summary
        ? applicationsTrendCaption(summary.applicationsThisMonth, summary.applicationsPreviousMonth, now)
        : null,
      highlight: true,
    },
  ]
}

/** "Em avaliação · 9 dias" (days left, rounded up); "· último dia" at 0. */
function statusLabel(company: Company, now: Date): string {
  const label = STATUS_LABELS[company.status]
  if (company.status !== 'trial' || !company.trialEndsAt) return label
  const days = Math.max(0, Math.ceil((company.trialEndsAt.getTime() - now.getTime()) / DAY_MS))
  if (days === 0) return `${label} · último dia`
  return `${label} · ${days === 1 ? '1 dia' : `${days} dias`}`
}

/** "Fazenda Santa Rita" → "SR": the generic prefix would repeat on every row. */
export function companyInitials(name: string): string {
  const meaningful = name.trim().replace(/^(fazenda|sítio|sitio|chácara|chacara)\s+/i, '')
  const words = meaningful.split(/\s+/).filter(Boolean)
  if (words.length === 1) return (words[0] ?? '').slice(0, 2).toUpperCase()
  return initials(meaningful)
}

export function toCompanyRowView(
  company: Company,
  now: Date,
  options: { actions?: readonly CompanyRowAction[], highlightId?: string | null } = {},
): CompanyRowView {
  return {
    id: company.id,
    name: company.name,
    initials: companyInitials(company.name),
    locationLabel: [company.city, company.state].filter(Boolean).join(' · '),
    documentLabel: formatCnpj(company.document),
    plan: { value: company.plan, label: COMPANY_PLAN_LABELS[company.plan] },
    farmCountLabel: integer.format(company.farmCount),
    userCountLabel: integer.format(company.userCount),
    status: { value: company.status, label: statusLabel(company, now), tone: STATUS_TONES[company.status] },
    actions: options.actions ?? [],
    highlighted: options.highlightId === company.id,
  }
}

export function buildCompanyPagination(page: number, pageSize: number, total: number): CompanyPaginationView {
  const pageCount = Math.max(1, Math.ceil(total / pageSize))
  const first = total === 0 ? 0 : (page - 1) * pageSize + 1
  const last = Math.min(total, page * pageSize)
  return {
    rangeLabel: total === 0 ? 'Nenhuma empresa' : `Mostrando ${first}–${last} de ${integer.format(total)}`,
    page,
    pageCount,
    canPrevious: page > 1,
    canNext: page < pageCount,
  }
}
