/** Stable error codes of the contract plus front-only `network_error`/`unknown_error`. */
export type ApiErrorCode =
  | 'not_found'
  | 'method_not_allowed'
  | 'internal_error'
  | 'unauthorized'
  | 'validation_failed'
  | 'conflict'
  | 'payload_too_large'
  | 'unsupported_media_type'
  | 'forbidden'
  | 'out_of_scope'
  | 'network_error'
  | 'unknown_error'

export interface FieldError {
  readonly field: string
  readonly message: string
}

/** The only error type services reject with. */
export class ApiError extends Error {
  readonly code: ApiErrorCode
  /** HTTP status; 0 when the request never got a response. */
  readonly status: number
  readonly details: readonly FieldError[]

  constructor(params: {
    code: ApiErrorCode
    message: string
    status: number
    details?: readonly FieldError[]
    cause?: unknown
  }) {
    super(params.message, { cause: params.cause })
    this.name = 'ApiError'
    this.code = params.code
    this.status = params.status
    this.details = params.details ?? []
  }

  /**
   * Contract error body. With SSR, `useAsyncData` turns the rejection into a
   * NuxtError (h3 `createError` copies `data` and `status`) and only `toJSON()`
   * reaches the client payload: `toApiError` rebuilds the ApiError from it, so
   * `code` and `details` survive hydration.
   */
  get data(): { error: { code: ApiErrorCode, message: string, details: readonly FieldError[] } } {
    return { error: { code: this.code, message: this.message, details: this.details } }
  }
}

/**
 * User-facing pt-BR messages for failures without an API error body. The raw
 * exception text (e.g. "fetch failed", a TypeError) stays only in `cause`.
 */
export const ERROR_MESSAGES = {
  network: 'Não foi possível conectar ao servidor. Verifique sua conexão e tente novamente.',
  unavailable: 'O serviço está indisponível no momento. Tente novamente em instantes.',
  unexpected: 'Ocorreu um erro inesperado. Tente novamente.',
} as const

// Front-only codes are accepted too: they come back in the SSR payload (see `ApiError.data`).
const KNOWN_CODES: ReadonlySet<string> = new Set<ApiErrorCode>([
  'not_found',
  'method_not_allowed',
  'internal_error',
  'unauthorized',
  'validation_failed',
  'conflict',
  'payload_too_large',
  'unsupported_media_type',
  'forbidden',
  'out_of_scope',
  'network_error',
  'unknown_error',
])

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null
}

function readErrorBody(data: unknown): { code: ApiErrorCode, message: string, details: FieldError[] } | null {
  if (!isRecord(data) || !isRecord(data.error)) return null
  const { code, message, details } = data.error
  if (typeof code !== 'string' || typeof message !== 'string') return null
  const fieldErrors = Array.isArray(details)
    ? details.filter(
        (item): item is FieldError =>
          isRecord(item) && typeof item.field === 'string' && typeof item.message === 'string',
      )
    : []
  return {
    code: KNOWN_CODES.has(code) ? (code as ApiErrorCode) : 'unknown_error',
    message,
    details: fieldErrors,
  }
}

function readStatus(error: Record<string, unknown>): number {
  const status = error.statusCode ?? error.status
  return typeof status === 'number' ? status : 0
}

/**
 * Normalizes anything thrown by `$fetch`, by `useAsyncData` (which wraps the
 * original error in `cause`) or by our own services into an `ApiError`.
 */
export function toApiError(error: unknown): ApiError {
  if (error instanceof ApiError) return error
  if (!isRecord(error)) {
    return new ApiError({ code: 'unknown_error', message: ERROR_MESSAGES.unexpected, status: 0, cause: error })
  }
  if (error.cause instanceof ApiError) return error.cause

  const status = readStatus(error)
  const body = readErrorBody(error.data)
  if (body) return new ApiError({ ...body, status, cause: error })

  // ofetch raises FetchError without a status when no response arrived.
  if (status === 0 && error.name === 'FetchError') {
    return new ApiError({
      code: 'network_error',
      message: ERROR_MESSAGES.network,
      status,
      cause: error,
    })
  }
  const message = status >= 500 ? ERROR_MESSAGES.unavailable : ERROR_MESSAGES.unexpected
  return new ApiError({ code: 'unknown_error', message, status, cause: error })
}
