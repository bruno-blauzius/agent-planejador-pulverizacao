import { describe, expect, it } from 'vitest'
import {
  passwordChangedLabel,
  passwordRules,
  profileErrorsFromApi,
  PROFILE_MESSAGES,
  validateProfileForm,
} from '~/utils/profile'

const empty = { currentPassword: '', newPassword: '', confirmPassword: '' }

describe('profile form rules', () => {
  it('ignores an empty password section and requires the name', () => {
    expect(validateProfileForm('Marina', empty)).toEqual({})
    expect(validateProfileForm('  ', empty)).toEqual({ name: PROFILE_MESSAGES.nameRequired })
  })

  it('accepts a strong password confirmed', () => {
    expect(validateProfileForm('Marina', { currentPassword: 'x', newPassword: 'abcdef12', confirmPassword: 'abcdef12' })).toEqual({})
    expect(passwordRules('ação2026').every(rule => rule.met)).toBe(true)
  })

  it('maps API fields', () => {
    expect(profileErrorsFromApi([
      { field: 'current_password', message: 'incorreta' },
      { field: 'new_password', message: 'fraca' },
      { field: 'other', message: '?' },
    ])).toEqual({ currentPassword: 'incorreta', newPassword: 'fraca' })
  })
})

describe('passwordChangedLabel', () => {
  const now = new Date('2026-11-21T12:00:00Z')
  it('describes days, months and years', () => {
    expect(passwordChangedLabel(null, now)).toBeNull()
    expect(passwordChangedLabel(new Date('2026-11-21T08:00:00Z'), now)).toBe('Última alteração: hoje')
    expect(passwordChangedLabel(new Date('2026-11-11T12:00:00Z'), now)).toBe('Última alteração: há 10 dias')
    expect(passwordChangedLabel(new Date('2026-10-15T12:00:00Z'), now)).toBe('Última alteração: há 1 mês')
    expect(passwordChangedLabel(new Date('2024-10-01T12:00:00Z'), now)).toBe('Última alteração: há 2 anos')
  })
})
