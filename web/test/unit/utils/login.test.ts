import { describe, expect, it } from 'vitest'
import { isLoginIdentifier, LOGIN_MESSAGES, validateLoginForm } from '~/utils/login'
import { resolveAuthRedirect, safeRedirect } from '~/utils/redirect'

describe('validateLoginForm', () => {
  it.each([
    ['bruno@fazendasantarita.com.br', true],
    ['123.278.916-05', true],
    ['12327891605', true],
    ['  bruno@x.test  ', true],
    ['bruno@', false],
    ['123.278.916', false],
    ['1232789160', false],
  ])('isLoginIdentifier(%j) = %s', (value, expected) => {
    expect(isLoginIdentifier(value)).toBe(expected)
  })

  it('requires both fields', () => {
    expect(validateLoginForm({ identifier: '  ', password: '' })).toEqual({
      identifier: LOGIN_MESSAGES.identifierRequired,
      password: LOGIN_MESSAGES.passwordRequired,
    })
  })

  it('flags a malformed identifier and accepts a valid form', () => {
    expect(validateLoginForm({ identifier: 'bruno', password: 'x' })).toEqual({ identifier: LOGIN_MESSAGES.identifierInvalid })
    expect(validateLoginForm({ identifier: 'bruno@x.test', password: 'x' })).toEqual({})
  })
})

describe('safeRedirect', () => {
  it.each([
    ['/fields?farm=1#map', '/fields?farm=1#map'],
    ['/farms/farm-santa-rita', '/farms/farm-santa-rita'],
    [['/owners', '/x'], '/owners'],
  ])('keeps internal path %j', (target, expected) => {
    expect(safeRedirect(target)).toBe(expected)
  })

  it.each([
    ['https://evil.test/phish'],
    ['//evil.test'],
    ['/\\evil.test'],
    ['\\\\evil.test'],
    ['javascript:alert(1)'],
    ['/\tevil'],
    [' /farms'],
    ['farms'],
    ['/login'],
    ['/login?redirect=/farms'],
    [''],
    [null],
    [undefined],
    [42],
    [[]],
  ])('falls back for %j', (target) => {
    expect(safeRedirect(target)).toBe('/farms')
  })

  it('uses the given fallback', () => {
    expect(safeRedirect('//evil.test', '/')).toBe('/')
  })
})

describe('resolveAuthRedirect', () => {
  const base = { path: '/farms', fullPath: '/farms?status=pending', isPublic: false, redirect: undefined, isAuthenticated: false }

  it('sends a guest on a private page to /login keeping the destination', () => {
    expect(resolveAuthRedirect(base)).toEqual({ path: '/login', query: { redirect: '/farms?status=pending' } })
  })

  it('lets an authenticated user through', () => {
    expect(resolveAuthRedirect({ ...base, isAuthenticated: true })).toBeNull()
  })

  it('leaves public pages open to guests', () => {
    expect(resolveAuthRedirect({ ...base, path: '/login', fullPath: '/login', isPublic: true })).toBeNull()
  })

  it('moves a signed-in user out of /login to a safe destination', () => {
    const login = { ...base, path: '/login', fullPath: '/login', isPublic: true, isAuthenticated: true }
    expect(resolveAuthRedirect({ ...login, redirect: '/owners' })).toEqual({ path: '/owners' })
    expect(resolveAuthRedirect({ ...login, redirect: 'https://evil.test' })).toEqual({ path: '/farms' })
  })
})
