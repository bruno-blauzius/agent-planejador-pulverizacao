/**
 * Presentation-only checks of the login form. The API is the authority on
 * credentials and CPF check digits; these rules only avoid a pointless request.
 */
export type LoginField = 'identifier' | 'password'
export type LoginFieldErrors = Partial<Record<LoginField, string>>

export const LOGIN_MESSAGES = {
  identifierRequired: 'Informe seu e-mail ou CPF.',
  identifierInvalid: 'Digite um e-mail ou CPF válido.',
  passwordRequired: 'Informe sua senha.',
  invalidCredentials: 'E-mail/CPF ou senha inválidos.',
  network: 'Não foi possível conectar. Tente novamente.',
  generic: 'Não foi possível entrar. Tente novamente.',
} as const

const EMAIL_PATTERN = /^[^\s@]+@[^\s@]+\.[^\s@]+$/
const CPF_PATTERN = /^(\d{11}|\d{3}\.\d{3}\.\d{3}-\d{2})$/

/** True for something shaped like an e-mail or a CPF (11 digits, masked or not). */
export function isLoginIdentifier(value: string): boolean {
  const trimmed = value.trim()
  return EMAIL_PATTERN.test(trimmed) || CPF_PATTERN.test(trimmed)
}

export function validateLoginForm(values: { identifier: string, password: string }): LoginFieldErrors {
  const errors: LoginFieldErrors = {}
  const identifier = values.identifier.trim()
  if (!identifier) errors.identifier = LOGIN_MESSAGES.identifierRequired
  else if (!isLoginIdentifier(identifier)) errors.identifier = LOGIN_MESSAGES.identifierInvalid
  if (!values.password) errors.password = LOGIN_MESSAGES.passwordRequired
  return errors
}
