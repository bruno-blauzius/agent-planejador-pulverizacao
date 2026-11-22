import { afterEach, describe, expect, it, vi } from 'vitest'
import { FLASH_COOKIE, useFlash } from '~/composables/useFlash'
import { PERMISSION_DENIED_FLASH } from '~/utils/permissions'
import { createTestApi } from '../../support/mockApi'
import { installNuxtFakes } from '../../support/nuxtFakes'

afterEach(() => vi.unstubAllGlobals())

describe('useFlash', () => {
  it('delivers a message across client-side navigation once', () => {
    installNuxtFakes(createTestApi().api)
    useFlash().show(PERMISSION_DENIED_FLASH)

    const flash = useFlash()
    expect(flash.message.value).toEqual(PERMISSION_DENIED_FLASH)
    expect(flash.consume()).toEqual(PERMISSION_DENIED_FLASH)
    expect(flash.consume()).toBeNull()
  })

  it('survives a server redirect through the short-lived cookie and clears it on consume', () => {
    const fakes = installNuxtFakes(createTestApi().api)
    // Request 1 (SSR): the middleware denies the page and redirects.
    useFlash().show(PERMISSION_DENIED_FLASH)

    // Request 2: the state of request 1 is gone, the cookie came back with the browser.
    fakes.newRequest()
    const flash = useFlash()
    expect(flash.consume()).toEqual(PERMISSION_DENIED_FLASH)

    expect(fakes.cookie(FLASH_COOKIE).value).toBeNull()
    fakes.newRequest()
    expect(useFlash().consume()).toBeNull()
  })
})
