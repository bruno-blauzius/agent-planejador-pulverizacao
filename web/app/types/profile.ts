/** Profile of the signed-in user (screen 11, ADR 0013). */
import type { CompanyRef, FarmScope, UserRole } from './auth'

export interface Profile {
  readonly id: string
  readonly name: string
  /** Managed by an administrator: read-only here. */
  readonly email: string
  readonly role: UserRole
  readonly company: CompanyRef
  readonly farmScope: FarmScope
  readonly passwordChangedAt: Date | null
}

export interface ProfileInput {
  readonly name: string
}

export interface PasswordChangeInput {
  readonly currentPassword: string
  readonly newPassword: string
}
