/**
 * Presentation rules of the login screen. With the SSO (ADR 0015) the form
 * exists only in the dev mock: the checks here just avoid a pointless request,
 * and the API (Keycloak) is the authority on credentials. The login by CPF left
 * the contract, so the identifier is the e-mail.
 */
import { CURATED_ERROR_MESSAGES, ERROR_MESSAGES, type ApiError, type ApiErrorCode } from '~/services/errors'

export type LoginField = 'identifier' | 'password'
export type LoginFieldErrors = Partial<Record<LoginField, string>>

export const LOGIN_MESSAGES = {
  identifierRequired: 'Informe seu e-mail.',
  identifierInvalid: 'Digite um e-mail válido.',
  passwordRequired: 'Informe sua senha.',
  invalidCredentials: 'E-mail ou senha inválidos.',
  network: 'Não foi possível conectar. Tente novamente.',
  generic: 'Não foi possível entrar. Tente novamente.',
} as const

const EMAIL_PATTERN = /^[^\s@]+@[^\s@]+\.[^\s@]+$/

/** True for something shaped like an e-mail. */
export function isLoginIdentifier(value: string): boolean {
  return EMAIL_PATTERN.test(value.trim())
}

export function validateLoginForm(values: { identifier: string, password: string }): LoginFieldErrors {
  const errors: LoginFieldErrors = {}
  const identifier = values.identifier.trim()
  if (!identifier) errors.identifier = LOGIN_MESSAGES.identifierRequired
  else if (!isLoginIdentifier(identifier)) errors.identifier = LOGIN_MESSAGES.identifierInvalid
  if (!values.password) errors.password = LOGIN_MESSAGES.passwordRequired
  return errors
}

// ---- why the session could not be used (SSO) ---------------------------------

/**
 * Reasons shown on /login as `?error=<code>`: the product-access 403 codes of
 * `GET /auth/me` and `GET /auth/callback`, the IdP outage (502 of the callback)
 * and the front-only `unavailable` (the API itself did not answer).
 */
export const LOGIN_ERROR_CODES = [
  'user_not_provisioned',
  'no_access_to_product',
  'ambiguous_role',
  'forbidden',
  'identity_provider_unavailable',
  'unavailable',
] as const

export type LoginErrorCode = (typeof LOGIN_ERROR_CODES)[number]

const LOGIN_ERROR_TEXT: Readonly<Record<LoginErrorCode, string>> = {
  user_not_provisioned: CURATED_ERROR_MESSAGES.user_not_provisioned ?? '',
  no_access_to_product: CURATED_ERROR_MESSAGES.no_access_to_product ?? '',
  ambiguous_role: CURATED_ERROR_MESSAGES.ambiguous_role ?? '',
  forbidden: 'Seu usuário ou a empresa dele está inativo. Fale com o administrador da sua empresa.',
  identity_provider_unavailable: CURATED_ERROR_MESSAGES.identity_provider_unavailable ?? '',
  unavailable: ERROR_MESSAGES.unavailable,
}

export const LOGIN_ERROR_TITLE = 'Não foi possível entrar'

export function isLoginErrorCode(value: unknown): value is LoginErrorCode {
  return typeof value === 'string' && (LOGIN_ERROR_CODES as readonly string[]).includes(value)
}

/** pt-BR text for a `?error=` value; null when absent or unknown (an unknown code is never echoed). */
export function loginErrorMessage(value: unknown): string | null {
  const candidate = Array.isArray(value) ? value[0] : value
  return isLoginErrorCode(candidate) ? LOGIN_ERROR_TEXT[candidate] : null
}

const PRODUCT_ACCESS_CODES: ReadonlySet<ApiErrorCode> = new Set<ApiErrorCode>([
  'user_not_provisioned',
  'no_access_to_product',
  'ambiguous_role',
  'forbidden',
])

/** True for a 403 that denies the product (not a plain "no permission for this action"). */
export function isProductAccessDenied(error: Pick<ApiError, 'code'> | null | undefined): boolean {
  return Boolean(error && PRODUCT_ACCESS_CODES.has(error.code))
}

/**
 * Reason to show on /login after `GET /auth/me` failed, or null when it is a
 * plain "not signed in" (401: just go to the login). Any other failure is
 * reported as `unavailable` instead of pretending the user is signed out.
 */
export function loginErrorFor(error: Pick<ApiError, 'code' | 'status'> | null | undefined): LoginErrorCode | null {
  if (!error || error.code === 'unauthorized' || error.status === 401) return null
  if (isProductAccessDenied(error)) return error.code as LoginErrorCode
  return 'unavailable'
}
