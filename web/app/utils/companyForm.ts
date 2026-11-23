/**
 * Presentation rules of the companies screen actions (14): the "Nova empresa" /
 * "Editar empresa" form, the row menu, confirmations and notices. The API
 * decides everything (CNPJ check digits included); these checks only spare a
 * round trip and put errors on the right field.
 */
import { hasCuratedMessage, type ApiError } from '~/services/errors'
import type { Company, CompanyInput, CompanyPlan, CompanyStatus, CompanyUpdateInput } from '~/types/admin'
import type { FlashMessage } from '~/types/flash'
import { COMPANY_PLAN_LABELS, type CompanyRowAction, type CompanyRowActionKind } from './company'
import { formatCnpj } from './format'
import { isValidCnpj } from './ownerForm'
import { forbiddenMessageFor } from './permissions'

export type CompanyFormField = 'name' | 'document' | 'city' | 'state' | 'plan'
export type CompanyFormErrors = Partial<Record<CompanyFormField, string>>

export const COMPANY_FORM_FIELDS: readonly CompanyFormField[] = ['name', 'document', 'city', 'state', 'plan']

export interface CompanyFormValues {
  name: string
  document: string
  city: string
  state: string
  plan: CompanyPlan
}

export const COMPANY_FORM_MESSAGES = {
  nameRequired: 'Informe o nome da empresa.',
  documentRequired: 'Informe o CNPJ.',
  documentInvalid: 'CNPJ inválido.',
  documentTaken: 'Já existe uma empresa com este CNPJ.',
  cityRequired: 'Informe a cidade.',
  stateInvalid: 'Informe a UF com 2 letras.',
  generic: 'Não foi possível salvar. Tente novamente.',
  network: 'Não foi possível conectar ao servidor. Verifique sua conexão e tente novamente.',
  notFound: 'A empresa não existe mais.',
} as const

export const COMPANY_PLAN_VALUES: readonly CompanyPlan[] = ['basic', 'pro', 'trial']

export const COMPANY_PLAN_HINTS: Readonly<Record<CompanyPlan, string>> = {
  basic: 'A empresa nasce ativa no plano Básico.',
  pro: 'A empresa nasce ativa no plano Pro.',
  trial: 'A empresa nasce em avaliação; a plataforma define a duração do período.',
}

export const COMPANY_PLAN_EDIT_HINTS: Readonly<Record<CompanyPlan, string>> = {
  basic: 'Muda o plano da empresa para Básico.',
  pro: 'Muda o plano da empresa para Pro.',
  trial: 'Passar para Avaliação inicia um novo período de avaliação.',
}

export const COMPANY_PLAN_OPTIONS: readonly { readonly value: CompanyPlan, readonly title: string, readonly hint: string }[] =
  COMPANY_PLAN_VALUES.map(value => ({ value, title: COMPANY_PLAN_LABELS[value], hint: COMPANY_PLAN_HINTS[value] }))

/** Maps the `field` of 422 details to form fields (names coincide with the contract). */
const API_FIELDS: Readonly<Record<string, CompanyFormField>> = {
  name: 'name',
  document: 'document',
  city: 'city',
  state: 'state',
  plan: 'plan',
}

export function emptyCompanyForm(): CompanyFormValues {
  return { name: '', document: '', city: '', state: '', plan: 'basic' }
}

export function companyToFormValues(company: Company): CompanyFormValues {
  return {
    name: company.name,
    document: formatCnpj(company.document),
    city: company.city,
    state: company.state,
    plan: company.plan,
  }
}

/** Progressive CNPJ mask: "12.345.678/0001-90". */
export function maskCnpjInput(value: string): string {
  const digits = value.replace(/\D/g, '').slice(0, 14)
  const mask = '00.000.000/0000-00'
  let result = ''
  let index = 0
  for (const char of mask) {
    if (index >= digits.length) break
    result += char === '0' ? digits[index++] : char
  }
  return result
}

/** UF input: letters only, upper case, at most 2. */
export function maskStateInput(value: string): string {
  return value.replace(/[^a-zA-Z]/g, '').slice(0, 2).toUpperCase()
}

export function validateCompanyForm(values: CompanyFormValues, options: { skipDocument?: boolean } = {}): CompanyFormErrors {
  const errors: CompanyFormErrors = {}
  if (!values.name.trim()) errors.name = COMPANY_FORM_MESSAGES.nameRequired
  if (!options.skipDocument) {
    if (!values.document.replace(/\D/g, '')) errors.document = COMPANY_FORM_MESSAGES.documentRequired
    else if (!isValidCnpj(values.document)) errors.document = COMPANY_FORM_MESSAGES.documentInvalid
  }
  if (!values.city.trim()) errors.city = COMPANY_FORM_MESSAGES.cityRequired
  if (values.state.trim().length !== 2) errors.state = COMPANY_FORM_MESSAGES.stateInvalid
  return errors
}

/** Field errors carried by a 409 (`conflict` = duplicated CNPJ) or 422 (`details`). */
export function companyApiFieldErrors(error: ApiError | null): CompanyFormErrors {
  if (!error) return {}
  if (error.code === 'conflict') return { document: COMPANY_FORM_MESSAGES.documentTaken }
  const errors: CompanyFormErrors = {}
  for (const detail of error.details) {
    const field = API_FIELDS[detail.field]
    if (field) errors[field] = detail.message
  }
  return errors
}

/** Message for the form footer: only what no field explains. */
export function companyFormErrorMessage(error: ApiError | null): string | null {
  if (!error) return null
  const forbidden = forbiddenMessageFor(error)
  if (forbidden) return forbidden
  if (Object.keys(companyApiFieldErrors(error)).length > 0) return null
  if (error.code === 'network_error') return COMPANY_FORM_MESSAGES.network
  if (error.code === 'not_found') return COMPANY_FORM_MESSAGES.notFound
  if (hasCuratedMessage(error.code)) return error.message
  return COMPANY_FORM_MESSAGES.generic
}

export function companyCreateInput(values: CompanyFormValues): CompanyInput {
  return {
    name: values.name.trim(),
    document: values.document.replace(/\D/g, ''),
    city: values.city.trim(),
    state: values.state.trim().toUpperCase(),
    plan: values.plan,
  }
}

/** Only what changed (the PATCH needs at least one property); empty when nothing did. */
export function companyUpdateInput(values: CompanyFormValues, original: Company): CompanyUpdateInput {
  const next = companyCreateInput(values)
  return {
    ...(next.name !== original.name ? { name: next.name } : {}),
    ...(next.city !== original.city ? { city: next.city } : {}),
    ...(next.state !== original.state ? { state: next.state } : {}),
    ...(next.plan !== original.plan ? { plan: next.plan } : {}),
  }
}

// ---- row menu and confirmations ---------------------------------------------

/** Delete is offered only for inactive companies (UI flow); the API still answers 409 with active users. */
export function companyRowActions(status: CompanyStatus): readonly CompanyRowAction[] {
  if (status === 'inactive') {
    return [
      { kind: 'edit', label: 'Editar', tone: 'default' },
      { kind: 'reactivate', label: 'Reativar', tone: 'default' },
      { kind: 'delete', label: 'Excluir', tone: 'danger' },
    ]
  }
  return [
    { kind: 'edit', label: 'Editar', tone: 'default' },
    { kind: 'deactivate', label: 'Inativar', tone: 'danger' },
  ]
}

export interface CompanyConfirmationCopy {
  readonly title: string
  readonly description: string
  readonly confirmLabel: string
  readonly cancelLabel: string
  readonly tone: 'default' | 'danger'
}

export function companyConfirmationCopy(kind: 'deactivate' | 'delete', company: Company): CompanyConfirmationCopy {
  if (kind === 'deactivate') {
    return {
      title: `Inativar ${company.name}?`,
      description: 'Todos os usuários da empresa perdem o acesso na hora e as sessões abertas são encerradas. Nenhum dado é apagado e você pode reativar a empresa depois.',
      confirmLabel: 'Inativar empresa',
      cancelLabel: 'Cancelar',
      tone: 'danger',
    }
  }
  return {
    title: `Excluir ${company.name}?`,
    description: 'A empresa deixa de aparecer no sistema e não pode ser restaurada por aqui. O CNPJ continua reservado e não poderá ser cadastrado de novo.',
    confirmLabel: 'Excluir empresa',
    cancelLabel: 'Cancelar',
    tone: 'danger',
  }
}

// ---- notices ----------------------------------------------------------------

const ACTION_ERROR_TITLE = 'Não foi possível concluir'

export const COMPANY_ACTION_MESSAGES = {
  hasActiveUsers: 'A empresa ainda tem usuários ativos ou convidados. Inative ou remova esses usuários na tela Usuários e tente excluir de novo.',
  sessionsPending: 'A empresa foi inativada, mas o SSO Inceres não respondeu ao encerrar as sessões abertas. Reative e inative a empresa de novo para concluir.',
  generic: 'Tente novamente em instantes.',
} as const

export type CompanyActionKind = Exclude<CompanyRowActionKind, 'edit'>

/** Text for a failed deactivate/reactivate/delete, curated per action. */
export function companyActionErrorMessage(error: ApiError | null, kind: CompanyActionKind): string {
  if (!error) return COMPANY_ACTION_MESSAGES.generic
  const forbidden = forbiddenMessageFor(error)
  if (forbidden) return forbidden
  if (error.code === 'company_has_active_users') return COMPANY_ACTION_MESSAGES.hasActiveUsers
  if (error.code === 'identity_provider_unavailable' && kind === 'deactivate') return COMPANY_ACTION_MESSAGES.sessionsPending
  if (error.code === 'network_error') return COMPANY_FORM_MESSAGES.network
  if (error.code === 'not_found') return COMPANY_FORM_MESSAGES.notFound
  if (hasCuratedMessage(error.code)) return error.message
  return COMPANY_ACTION_MESSAGES.generic
}

/**
 * The deactivation is written before the SSO call, so a 502 still leaves the
 * company inactive: the list must refresh even though the call "failed".
 */
export function companyActionChangedDespiteError(error: ApiError | null, kind: CompanyActionKind): boolean {
  return kind === 'deactivate' && error?.code === 'identity_provider_unavailable'
}

export function companyActionErrorNotice(error: ApiError | null, kind: CompanyActionKind): FlashMessage {
  const description = companyActionErrorMessage(error, kind)
  if (companyActionChangedDespiteError(error, kind)) return { kind: 'attention', title: 'Empresa inativada com ressalva', description }
  return { kind: 'error', title: ACTION_ERROR_TITLE, description }
}

export function companyActionSuccessNotice(kind: CompanyActionKind, company: Company): FlashMessage {
  switch (kind) {
    case 'deactivate':
      return { kind: 'success', title: `${company.name} inativada`, description: 'Os usuários da empresa perderam o acesso.', highlightId: company.id }
    case 'reactivate':
      return {
        kind: 'success',
        title: `${company.name} reativada`,
        description: 'A empresa voltou a funcionar. Usuários inativos continuam inativos até serem reativados um a um.',
        highlightId: company.id,
      }
    case 'delete':
      return { kind: 'success', title: `${company.name} excluída`, description: 'A empresa deixou de aparecer na lista.' }
  }
}

export function companySavedNotice(company: Company, mode: 'created' | 'updated'): FlashMessage {
  return mode === 'created'
    ? {
        kind: 'success',
        title: `${company.name} cadastrada`,
        description: company.status === 'trial' ? 'A empresa começou em avaliação.' : 'A empresa já está ativa.',
        highlightId: company.id,
      }
    : { kind: 'success', title: `${company.name} atualizada`, highlightId: company.id }
}

/** Status sent on reactivation: back to the plan it had (trial keeps its evaluation period). */
export function reactivationStatus(company: Company): CompanyStatus {
  return company.plan === 'trial' && company.trialEndsAt ? 'trial' : 'active'
}
