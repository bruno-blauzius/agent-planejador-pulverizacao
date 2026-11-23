import { describe, expect, it } from 'vitest'
import { ApiError, CURATED_ERROR_MESSAGES, hasCuratedMessage, toApiError } from '~/services/errors'
import { invitationCreatedNotice, invitationResentNotice } from '~/utils/invitation'
import { isProductAccessDenied, loginErrorFor, loginErrorMessage } from '~/utils/login'
import { resolveAuthRedirect } from '~/utils/redirect'

const person = { name: 'Ana Lúcia', email: 'ana@email.test' }

describe('invitation notices (X-Invitation-Email)', () => {
  it('celebrates only what was confirmed', () => {
    expect(invitationCreatedNotice(person, 'sent')).toMatchObject({ kind: 'success', title: 'Convite enviado' })
    expect(invitationCreatedNotice(person, 'not_needed')).toMatchObject({ kind: 'success', title: 'Convite criado' })
  })

  it('warns when the e-mail did not leave: the invitation exists and can be resent', () => {
    const notice = invitationCreatedNotice(person, 'not_sent')
    expect(notice.kind).toBe('attention')
    expect(notice.title).toBe('Convite criado, mas o e-mail não foi enviado')
    expect(notice.description).toContain('continua válido')
    expect(notice.description).toContain('Reenviar convite')
  })

  it('does not promise a delivery when the header was unreadable', () => {
    const notice = invitationCreatedNotice(person, 'unknown')
    expect(notice.kind).toBe('attention')
    expect(notice.description).toContain('Não foi possível confirmar o envio')
  })

  it('has the same honesty on resend', () => {
    expect(invitationResentNotice(person.email, 'sent')).toMatchObject({ kind: 'success', title: 'Convite reenviado' })
    expect(invitationResentNotice(person.email, 'not_sent')).toMatchObject({ kind: 'attention' })
    expect(invitationResentNotice(person.email, 'unknown')).toMatchObject({ kind: 'attention' })
  })
})

describe('error mapping by stable code', () => {
  it('replaces the server text by the curated pt-BR one, keeping code and status', () => {
    const error = toApiError({
      name: 'FetchError',
      statusCode: 409,
      data: { error: { code: 'last_super_admin', message: 'a plataforma precisa de ao menos um super admin ativo' } },
    })
    expect(error).toMatchObject({ code: 'last_super_admin', status: 409 })
    expect(error.message).toBe(CURATED_ERROR_MESSAGES.last_super_admin)
  })

  it('keeps contextual server messages (conflict, validation, not found)', () => {
    for (const code of ['conflict', 'validation_failed', 'not_found', 'forbidden'] as const) {
      expect(hasCuratedMessage(code)).toBe(false)
    }
    const error = toApiError({ name: 'FetchError', statusCode: 409, data: { error: { code: 'conflict', message: 'CPF já cadastrado' } } })
    expect(error.message).toBe('CPF já cadastrado')
  })

  it('survives the SSR payload round trip with the same code and curated message', () => {
    const original = toApiError({ name: 'FetchError', statusCode: 403, data: { error: { code: 'ambiguous_role', message: 'x' } } })
    const rebuilt = toApiError({ statusCode: 403, data: original.data })
    expect(rebuilt).toBeInstanceOf(ApiError)
    expect(rebuilt).toMatchObject({ code: 'ambiguous_role', message: original.message })
  })

  it('turns a code the front does not know into unknown_error without echoing the server text as a code', () => {
    const error = toApiError({ name: 'FetchError', statusCode: 400, data: { error: { code: 'brand_new_code', message: 'novo' } } })
    expect(error.code).toBe('unknown_error')
  })
})

describe('login error reasons', () => {
  const denied = (code: ApiError['code'], status = 403) => new ApiError({ code, message: code, status })

  it('treats the product-access 403 codes as a denied product', () => {
    expect(isProductAccessDenied(denied('user_not_provisioned'))).toBe(true)
    expect(isProductAccessDenied(denied('no_access_to_product'))).toBe(true)
    expect(isProductAccessDenied(denied('ambiguous_role'))).toBe(true)
    expect(isProductAccessDenied(denied('forbidden'))).toBe(true)
    expect(isProductAccessDenied(denied('not_found', 404))).toBe(false)
    expect(isProductAccessDenied(null)).toBe(false)
  })

  it('maps a failed session check to the reason shown on /login', () => {
    expect(loginErrorFor(null)).toBeNull()
    expect(loginErrorFor(denied('unauthorized', 401))).toBeNull()
    expect(loginErrorFor(denied('user_not_provisioned'))).toBe('user_not_provisioned')
    expect(loginErrorFor(denied('network_error', 0))).toBe('unavailable')
    expect(loginErrorFor(denied('internal_error', 500))).toBe('unavailable')
  })

  it('writes the reason in pt-BR for known codes only, reading the first of an array', () => {
    expect(loginErrorMessage('ambiguous_role')).toContain('mais de um papel')
    expect(loginErrorMessage(['no_access_to_product', 'x'])).toContain('não tem acesso')
    expect(loginErrorMessage('nope')).toBeNull()
    expect(loginErrorMessage(undefined)).toBeNull()
  })

  it('puts the reason in the /login query for a guest on a private page', () => {
    const base = { path: '/farms', fullPath: '/farms', isPublic: false, redirect: undefined, isAuthenticated: false }
    expect(resolveAuthRedirect({ ...base, loginError: 'user_not_provisioned' }))
      .toEqual({ path: '/login', query: { error: 'user_not_provisioned', redirect: '/farms' } })
    expect(resolveAuthRedirect({ ...base, loginError: null })).toEqual({ path: '/login', query: { redirect: '/farms' } })
  })
})
