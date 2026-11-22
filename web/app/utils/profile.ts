/**
 * Presentation rules of the profile screen (11). The password rule shown here
 * mirrors the contract text ("mínimo de 8 caracteres, com letras e números");
 * the API is the one that accepts or rejects the new password.
 */
import type { FieldError } from '~/services/errors'
import type { Profile } from '~/types/profile'
import { initials } from './format'
import { USER_ROLE_LABELS } from './labels'

export type ProfileField = 'name' | 'currentPassword' | 'newPassword' | 'confirmPassword'
export type ProfileFieldErrors = Partial<Record<ProfileField, string>>

export const PASSWORD_MIN_LENGTH = 8

export const PROFILE_MESSAGES = {
  nameRequired: 'Informe seu nome completo.',
  currentPasswordRequired: 'Informe a senha atual para trocar a senha.',
  newPasswordRequired: 'Informe a nova senha.',
  newPasswordWeak: 'Use ao menos 8 caracteres, com letras e números.',
  confirmMismatch: 'A confirmação não confere com a nova senha.',
  passwordHint: 'Mínimo de 8 caracteres, com letras e números.',
  managedTitle: 'Gerenciado pelo administrador',
  managedText: 'E-mail, papel e empresa só podem ser alterados por um administrador.',
  profileSaved: 'Perfil atualizado',
  passwordSaved: 'Senha alterada',
  allSaved: 'Perfil e senha atualizados',
  nameSavedPasswordFailed: 'Nome salvo, mas a senha não foi alterada.',
  saveFailed: 'Não foi possível salvar. Tente novamente.',
  photoUnavailable: 'Em breve',
} as const

export interface ProfileIdentityView {
  readonly name: string
  readonly initials: string
  readonly email: string
  readonly roleLabel: string
  readonly companyName: string
  readonly managedTitle: string
  readonly managedText: string
}

export interface PasswordRuleView {
  readonly key: 'length' | 'letter' | 'number'
  readonly label: string
  readonly met: boolean
}

export interface PasswordValues {
  readonly currentPassword: string
  readonly newPassword: string
  readonly confirmPassword: string
}

/** Role label is neutral: the front does not know the user's gender. */
export function toProfileIdentityView(profile: Profile): ProfileIdentityView {
  return {
    name: profile.name,
    initials: initials(profile.name),
    email: profile.email,
    roleLabel: USER_ROLE_LABELS[profile.role],
    companyName: profile.company.name,
    managedTitle: PROFILE_MESSAGES.managedTitle,
    managedText: PROFILE_MESSAGES.managedText,
  }
}

export function passwordRules(password: string): PasswordRuleView[] {
  return [
    { key: 'length', label: `Ao menos ${PASSWORD_MIN_LENGTH} caracteres`, met: password.length >= PASSWORD_MIN_LENGTH },
    { key: 'letter', label: 'Letras', met: /\p{L}/u.test(password) },
    { key: 'number', label: 'Números', met: /\d/.test(password) },
  ]
}

/** The password section is optional: nothing typed means "do not change". */
export function wantsPasswordChange(values: PasswordValues): boolean {
  return Boolean(values.currentPassword || values.newPassword || values.confirmPassword)
}

export function validateProfileForm(name: string, values: PasswordValues): ProfileFieldErrors {
  const errors: ProfileFieldErrors = {}
  if (!name.trim()) errors.name = PROFILE_MESSAGES.nameRequired
  if (!wantsPasswordChange(values)) return errors
  if (!values.currentPassword) errors.currentPassword = PROFILE_MESSAGES.currentPasswordRequired
  if (!values.newPassword) errors.newPassword = PROFILE_MESSAGES.newPasswordRequired
  else if (!passwordRules(values.newPassword).every(rule => rule.met)) errors.newPassword = PROFILE_MESSAGES.newPasswordWeak
  if (values.newPassword !== values.confirmPassword) errors.confirmPassword = PROFILE_MESSAGES.confirmMismatch
  return errors
}

const API_FIELDS: Readonly<Record<string, ProfileField>> = {
  name: 'name',
  current_password: 'currentPassword',
  new_password: 'newPassword',
}

/** 422 `details` → form fields; unknown fields are ignored (the form error covers them). */
export function profileErrorsFromApi(details: readonly FieldError[]): ProfileFieldErrors {
  const errors: ProfileFieldErrors = {}
  for (const detail of details) {
    const field = API_FIELDS[detail.field]
    if (field) errors[field] ??= detail.message
  }
  return errors
}

const MONTH_DAYS = 30
const YEAR_DAYS = 365

/** "Última alteração: há 3 meses"; null when the password was never changed. */
export function passwordChangedLabel(changedAt: Date | null, now: Date): string | null {
  if (!changedAt) return null
  const days = Math.max(0, Math.floor((now.getTime() - changedAt.getTime()) / 86_400_000))
  let ago: string
  if (days === 0) ago = 'hoje'
  else if (days === 1) ago = 'ontem'
  else if (days < MONTH_DAYS) ago = `há ${days} dias`
  else if (days < YEAR_DAYS) {
    const months = Math.floor(days / MONTH_DAYS)
    ago = months === 1 ? 'há 1 mês' : `há ${months} meses`
  }
  else {
    const years = Math.floor(days / YEAR_DAYS)
    ago = years === 1 ? 'há 1 ano' : `há ${years} anos`
  }
  return `Última alteração: ${ago}`
}
