/**
 * Presentation formatters (pt-BR). Pure functions, no business rules: values
 * such as area, stand and spray condition come computed from the API.
 */
import type { GeoPoint } from '~/types/geo'

const EMPTY = '—'

function onlyDigits(value: string): string {
  return value.replace(/\D/g, '')
}

/** 486.2 → "486,2 ha"; 1284 → "1.284 ha". */
export function formatHectares(value: number, maximumFractionDigits = 1): string {
  const number = new Intl.NumberFormat('pt-BR', { maximumFractionDigits }).format(value)
  return `${number} ha`
}

/** 60 → "60%"; null → "—". */
export function formatPercent(value: number | null): string {
  if (value === null) return EMPTY
  return `${new Intl.NumberFormat('pt-BR', { maximumFractionDigits: 1 }).format(value)}%`
}

/** "12327891605" → "123.278.916-05". Returns the input untouched if not 11 digits. */
export function formatCpf(value: string): string {
  const digits = onlyDigits(value)
  if (digits.length !== 11) return value
  return digits.replace(/^(\d{3})(\d{3})(\d{3})(\d{2})$/, '$1.$2.$3-$4')
}

/** "12345678000190" → "12.345.678/0001-90". Returns the input untouched if not 14 digits. */
export function formatCnpj(value: string): string {
  const digits = onlyDigits(value)
  if (digits.length !== 14) return value
  return digits.replace(/^(\d{2})(\d{3})(\d{3})(\d{4})(\d{2})$/, '$1.$2.$3/$4-$5')
}

/** CPF or CNPJ chosen by length. */
export function formatDocument(value: string): string {
  return onlyDigits(value).length === 14 ? formatCnpj(value) : formatCpf(value)
}

/**
 * Masked document for lists (LGPD): CPF shows only the middle digits
 * ("***.278.916-**"); a CNPJ is public company data and is shown in full.
 */
export function maskDocument(value: string): string {
  const digits = onlyDigits(value)
  if (digits.length === 14) return formatCnpj(digits)
  if (digits.length !== 11) return value
  return `***.${digits.slice(3, 6)}.${digits.slice(6, 9)}-**`
}

/** "35998124470" → "(35) 99812-4470"; landlines with 10 digits too. */
export function formatPhone(value: string | null): string {
  if (!value) return EMPTY
  const digits = onlyDigits(value)
  if (digits.length === 11) return digits.replace(/^(\d{2})(\d{5})(\d{4})$/, '($1) $2-$3')
  if (digits.length === 10) return digits.replace(/^(\d{2})(\d{4})(\d{4})$/, '($1) $2-$3')
  return value
}

/** "37800000" → "37800-000". */
export function formatPostalCode(value: string | null): string {
  if (!value) return EMPTY
  const digits = onlyDigits(value)
  return digits.length === 8 ? `${digits.slice(0, 5)}-${digits.slice(5)}` : value
}

/** { lat: -21.3, lng: -46.71 } → "-21,30 · -46,71". */
export function formatCoordinates(point: GeoPoint | null): string {
  if (!point) return EMPTY
  const format = (value: number) =>
    new Intl.NumberFormat('pt-BR', { minimumFractionDigits: 2, maximumFractionDigits: 2 }).format(value)
  return `${format(point.lat)} · ${format(point.lng)}`
}

/** "Bruno Blauzius" → "BB"; "Cooperativa Vale Verde" → "CV". */
export function initials(name: string): string {
  return name
    .trim()
    .split(/\s+/)
    .slice(0, 2)
    .map(word => word.charAt(0))
    .join('')
    .toUpperCase()
}

/** ISO date "2019-03-15" → "mar/2019". */
export function formatMonthYear(isoDate: string | null): string {
  if (!isoDate) return EMPTY
  const [year, month] = isoDate.split('-').map(Number)
  if (!year || !month) return isoDate
  const monthName = new Intl.DateTimeFormat('pt-BR', { month: 'short', timeZone: 'UTC' })
    .format(new Date(Date.UTC(year, month - 1, 1)))
    .replace('.', '')
  return `${monthName}/${year}`
}

/** ISO date "1979-03-12" → "12/03/1979" (no timezone shift). */
export function formatDate(isoDate: string | null): string {
  if (!isoDate) return EMPTY
  const [year, month, day] = isoDate.split('-')
  return year && month && day ? `${day}/${month}/${year}` : isoDate
}

const DAY_MS = 86_400_000

/** "hoje", "ontem", "há 12 dias"; null → "—". */
export function formatDaysAgo(date: Date | null, now: Date = new Date()): string {
  if (!date) return EMPTY
  const days = Math.floor((now.getTime() - date.getTime()) / DAY_MS)
  if (days <= 0) return 'hoje'
  if (days === 1) return 'ontem'
  return `há ${days} dias`
}

/** Salutation by local hour: "Bom dia" (< 12h), "Boa tarde" (< 18h), "Boa noite". */
export function salutationFor(date: Date): 'Bom dia' | 'Boa tarde' | 'Boa noite' {
  const hour = date.getHours()
  if (hour < 12) return 'Bom dia'
  if (hour < 18) return 'Boa tarde'
  return 'Boa noite'
}
