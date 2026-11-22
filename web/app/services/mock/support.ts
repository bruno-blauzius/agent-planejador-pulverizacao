/** Helpers shared by the in-memory adapters (contract errors, search, paging). */
import { ApiError, type ApiErrorCode, type FieldError } from '../errors'
import type { Page, PageRequest } from '~/types/pagination'

export const DEFAULT_PAGE_SIZE = 20

export const STATUS_BY_CODE: Record<Exclude<ApiErrorCode, 'network_error' | 'unknown_error'>, number> = {
  not_found: 404,
  method_not_allowed: 405,
  internal_error: 500,
  unauthorized: 401,
  validation_failed: 422,
  conflict: 409,
  payload_too_large: 413,
  unsupported_media_type: 415,
  forbidden: 403,
  out_of_scope: 403,
}

export type MockErrorCode = keyof typeof STATUS_BY_CODE

export function fail(code: MockErrorCode, message: string, details?: FieldError[]): never {
  throw new ApiError({ code, message, status: STATUS_BY_CODE[code], details })
}

export function failIfInvalid(details: FieldError[]): void {
  if (details.length) fail('validation_failed', 'dados inválidos', details)
}

export function normalize(text: string): string {
  return text.normalize('NFD').replace(/[̀-ͯ]/g, '').toLowerCase().trim()
}

export function digitsOf(text: string): string {
  return text.replace(/\D/g, '')
}

export function paginate<T>(items: readonly T[], request: PageRequest): Page<T> {
  const page = Math.max(1, request.page ?? 1)
  const pageSize = Math.max(1, request.pageSize ?? DEFAULT_PAGE_SIZE)
  const start = (page - 1) * pageSize
  return { items: items.slice(start, start + pageSize), total: items.length, page, pageSize }
}

export function roundOneDecimal(value: number): number {
  return Math.round(value * 10) / 10
}
