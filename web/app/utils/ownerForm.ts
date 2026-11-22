/**
 * Input masks and presentation validation of the owner form (screen 4).
 * The API remains the authority on documents (422/409); these checks give
 * immediate feedback only.
 */
import type { PersonType } from '~/types/owner'

/** `windowAlerts` is the alerts toggle of the "Vínculos" step. */
export type OwnerFormField = 'name' | 'document' | 'birthDate' | 'phone' | 'email' | 'wizardShare' | 'windowAlerts'
export type OwnerFormErrors = Partial<Record<OwnerFormField, string>>

export const OWNER_FORM_MESSAGES = {
  nameRequired: { individual: 'Informe o nome completo.', company: 'Informe a razão social.' },
  documentRequired: { individual: 'Informe o CPF.', company: 'Informe o CNPJ.' },
  documentInvalid: { individual: 'CPF inválido.', company: 'CNPJ inválido.' },
  birthDateInvalid: 'Data inválida.',
  birthDateFuture: 'A data não pode estar no futuro.',
  phoneInvalid: 'Telefone inválido.',
  phoneRequiredForAlerts: 'Informe o WhatsApp no passo Dados ou desative os alertas.',
  emailInvalid: 'E-mail inválido.',
  shareRequired: 'Informe a participação.',
  shareOutOfRange: 'Use um valor entre 0 e 100.',
  farmTotalOver: 'A participação total da fazenda passaria de 100%.',
} as const

function digitsOf(value: string): string {
  return value.replace(/\D/g, '')
}

/** Applies a mask like "000.000.000-00" to the digits typed so far. */
function applyMask(digits: string, mask: string): string {
  let result = ''
  let index = 0
  for (const char of mask) {
    if (index >= digits.length) break
    if (char === '0') result += digits[index++]
    else result += char
  }
  return result
}

/** Progressive CPF/CNPJ mask while typing. */
export function maskDocumentInput(value: string, personType: PersonType): string {
  return personType === 'individual'
    ? applyMask(digitsOf(value).slice(0, 11), '000.000.000-00')
    : applyMask(digitsOf(value).slice(0, 14), '00.000.000/0000-00')
}

/** Progressive phone mask: "(35) 99812-4470" (mobile) or "(35) 3214-0000" (landline). */
export function maskPhoneInput(value: string): string {
  const digits = digitsOf(value).slice(0, 11)
  return applyMask(digits, digits.length > 10 ? '(00) 00000-0000' : '(00) 0000-00000')
}

/** Progressive "dd/mm/aaaa" mask. */
export function maskDateInput(value: string): string {
  return applyMask(digitsOf(value).slice(0, 8), '00/00/0000')
}

/** "12/03/1979" → "1979-03-12"; null when incomplete or not a real date. */
export function parseBrDate(value: string): string | null {
  const match = /^(\d{2})\/(\d{2})\/(\d{4})$/.exec(value.trim())
  if (!match) return null
  const [, day, month, year] = match
  const date = new Date(Date.UTC(Number(year), Number(month) - 1, Number(day)))
  const isReal = date.getUTCFullYear() === Number(year) && date.getUTCMonth() === Number(month) - 1 && date.getUTCDate() === Number(day)
  return isReal ? `${year}-${month}-${day}` : null
}

/** "1979-03-12" → "12/03/1979". */
export function isoToBrDate(value: string | null): string {
  if (!value) return ''
  const [year, month, day] = value.split('-')
  return year && month && day ? `${day}/${month}/${year}` : ''
}

function checkDigit(digits: string, weights: readonly number[]): number {
  const sum = weights.reduce((total, weight, index) => total + Number(digits[index]) * weight, 0)
  const rest = sum % 11
  return rest < 2 ? 0 : 11 - rest
}

export function isValidCpf(value: string): boolean {
  const digits = digitsOf(value)
  if (digits.length !== 11 || /^(\d)\1{10}$/.test(digits)) return false
  const first = checkDigit(digits, [10, 9, 8, 7, 6, 5, 4, 3, 2])
  const second = checkDigit(digits, [11, 10, 9, 8, 7, 6, 5, 4, 3, 2])
  return first === Number(digits[9]) && second === Number(digits[10])
}

export function isValidCnpj(value: string): boolean {
  const digits = digitsOf(value)
  if (digits.length !== 14 || /^(\d)\1{13}$/.test(digits)) return false
  const first = checkDigit(digits, [5, 4, 3, 2, 9, 8, 7, 6, 5, 4, 3, 2])
  const second = checkDigit(digits, [6, 5, 4, 3, 2, 9, 8, 7, 6, 5, 4, 3, 2])
  return first === Number(digits[12]) && second === Number(digits[13])
}

/** Parses a share typed as "50", "12,5" or "12.5"; null when empty or not a number. */
export function parseShare(value: string): number | null {
  const normalized = value.trim().replace(',', '.')
  if (!normalized) return null
  const number = Number(normalized)
  return Number.isFinite(number) ? number : null
}

export function validateShare(value: string): string | null {
  const share = parseShare(value)
  if (share === null) return value.trim() ? OWNER_FORM_MESSAGES.shareOutOfRange : OWNER_FORM_MESSAGES.shareRequired
  return share < 0 || share > 100 ? OWNER_FORM_MESSAGES.shareOutOfRange : null
}

export interface OwnerDataValues {
  readonly personType: PersonType
  readonly name: string
  readonly document: string
  readonly birthDate: string
  readonly phone: string
  readonly email: string
  readonly windowAlertsEnabled: boolean
}

const EMAIL_PATTERN = /^[^\s@]+@[^\s@]+\.[^\s@]+$/

/** Validates the "Dados" step. `skipDocument` in edit mode (document is locked). */
export function validateOwnerData(values: OwnerDataValues, options: { skipDocument?: boolean, today?: Date } = {}): OwnerFormErrors {
  const errors: OwnerFormErrors = {}
  const type = values.personType
  if (!values.name.trim()) errors.name = OWNER_FORM_MESSAGES.nameRequired[type]

  if (!options.skipDocument) {
    const isValid = type === 'individual' ? isValidCpf(values.document) : isValidCnpj(values.document)
    if (!digitsOf(values.document)) errors.document = OWNER_FORM_MESSAGES.documentRequired[type]
    else if (!isValid) errors.document = OWNER_FORM_MESSAGES.documentInvalid[type]
  }

  if (type === 'individual' && values.birthDate.trim()) {
    const iso = parseBrDate(values.birthDate)
    const today = (options.today ?? new Date()).toISOString().slice(0, 10)
    if (!iso) errors.birthDate = OWNER_FORM_MESSAGES.birthDateInvalid
    else if (iso > today) errors.birthDate = OWNER_FORM_MESSAGES.birthDateFuture
  }

  const phoneDigits = digitsOf(values.phone)
  if (phoneDigits && phoneDigits.length < 10) errors.phone = OWNER_FORM_MESSAGES.phoneInvalid

  if (values.email.trim() && !EMAIL_PATTERN.test(values.email.trim())) errors.email = OWNER_FORM_MESSAGES.emailInvalid
  return errors
}

/**
 * Alerts need a WhatsApp. Checked on the "Vínculos" step (where the toggle
 * lives) and on submit, never while filling "Dados".
 */
export function validateAlertsPhone(values: Pick<OwnerDataValues, 'phone' | 'windowAlertsEnabled'>): string | null {
  return values.windowAlertsEnabled && !digitsOf(values.phone) ? OWNER_FORM_MESSAGES.phoneRequiredForAlerts : null
}

/** "Hoje: Bruno 60% · Marina 40%" */
export function currentSharesLabel(shares: readonly { name: string, sharePercent: number }[]): string {
  if (shares.length === 0) return 'Hoje: nenhum proprietário vinculado'
  return `Hoje: ${shares.map(share => `${share.name.split(/\s+/)[0]} ${formatShare(share.sharePercent)}`).join(' · ')}`
}

function formatShare(value: number): string {
  return `${new Intl.NumberFormat('pt-BR', { maximumFractionDigits: 1 }).format(value)}%`
}

/** "Com 20% para Helena, ajuste os demais para fechar 100%." — null when the total closes 100%. */
export function shareAdjustmentHint(
  shares: readonly { sharePercent: number }[],
  newShare: number | null,
  newOwnerName: string,
): string | null {
  if (newShare === null) return null
  const total = shares.reduce((sum, share) => sum + share.sharePercent, 0) + newShare
  if (Math.abs(total - 100) < 0.001) return null
  const firstName = newOwnerName.trim().split(/\s+/)[0] || 'o novo proprietário'
  return `Com ${formatShare(newShare)} para ${firstName}, ajuste os demais para fechar 100%.`
}
