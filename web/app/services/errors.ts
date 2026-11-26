/**
 * Stable error codes of the contract (`Error.error.code` in api/openapi.yaml)
 * plus the front-only `network_error`/`unknown_error`. One list: the type and
 * the runtime check both derive from it.
 */
export const API_ERROR_CODES = [
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
  'agent_unavailable',
  'weather_unavailable',
  'conditions_changed',
  'user_not_provisioned',
  'no_access_to_product',
  'ambiguous_role',
  'identity_provider_unavailable',
  'document_storage_unavailable',
  'email_already_registered',
  'email_not_verified',
  'company_inactive',
  'company_has_active_users',
  'invitation_closed',
  'invitation_pending',
  'last_super_admin',
  'network_error',
  'unknown_error',
] as const

export type ApiErrorCode = (typeof API_ERROR_CODES)[number]

/**
 * Curated pt-BR text for codes whose meaning is fixed by the contract. It
 * replaces the server message (in the `ApiError` constructor) so every screen
 * shows the same, actionable sentence. Codes that carry context (`conflict`, `validation_failed`,
 * `not_found`, `forbidden`) keep the server text: they are mapped per screen.
 */
export const CURATED_ERROR_MESSAGES: Readonly<Partial<Record<ApiErrorCode, string>>> = {
  user_not_provisioned: 'Sua conta ainda não foi convidada para o Agentes Agro. Peça um convite ao administrador da sua empresa.',
  no_access_to_product: 'Sua conta não tem acesso ao Agentes Agro. Fale com o administrador da sua empresa.',
  ambiguous_role: 'Sua conta tem mais de um papel no Agentes Agro. Fale com o administrador para corrigir o acesso.',
  identity_provider_unavailable: 'O serviço de login (SSO Inceres) está indisponível. Nada foi alterado; tente novamente em instantes.',
  document_storage_unavailable: 'O armazenamento de documentos está indisponível. Nada foi alterado; tente novamente em instantes.',
  email_already_registered: 'Este e-mail já tem cadastro no Agentes Agro.',
  email_not_verified: 'Já existe uma conta com este e-mail, mas ele ainda não foi verificado. Enviamos um novo e-mail de verificação; convide de novo depois que a pessoa confirmar.',
  company_inactive: 'A empresa está inativa. Reative a empresa antes de continuar.',
  company_has_active_users: 'A empresa ainda tem usuários ativos ou convidados. Desative a empresa ou os usuários antes de excluir.',
  invitation_closed: 'O convite já foi aceito ou cancelado.',
  invitation_pending: 'Este usuário tem um convite pendente. Cancele o convite em vez de excluir.',
  last_super_admin: 'A plataforma precisa de ao menos um super admin ativo. Promova outro antes de desativar, excluir ou rebaixar este.',
}

/** True when the message of an error with this code is curated text (safe to show as is). */
export function hasCuratedMessage(code: ApiErrorCode): boolean {
  return code in CURATED_ERROR_MESSAGES
}

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
    // Contract codes with a fixed meaning always read the same, whoever raised them (HTTP body, mock, test).
    super(CURATED_ERROR_MESSAGES[params.code] ?? params.message, { cause: params.cause })
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
const KNOWN_CODES: ReadonlySet<string> = new Set<string>(API_ERROR_CODES)

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
  return { code: KNOWN_CODES.has(code) ? (code as ApiErrorCode) : 'unknown_error', message, details: fieldErrors }
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
