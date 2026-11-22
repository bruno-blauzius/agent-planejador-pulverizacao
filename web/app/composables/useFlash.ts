import { computed } from 'vue'
import type { FlashMessage } from '~/types/flash'

export const FLASH_COOKIE = 'agro_flash'
/** Long enough to survive one redirect, short enough to never resurface later. */
export const FLASH_COOKIE_MAX_AGE_SECONDS = 60

/**
 * One-shot message carried from one screen to the next (e.g. "Fazenda X
 * criada" after the wizard redirects to the list). Kept out of the URL, so it
 * is not repeated on reload nor shared by copying the link.
 *
 * Two carriers: shared state (`useState`) for client-side navigation, and a
 * short-lived cookie for server redirects (a middleware denying a page opened
 * by URL answers 302; the state of that request dies, the Set-Cookie travels
 * with the redirect). `consume()` reads either and clears both.
 */
export function useFlash() {
  const message = useState<FlashMessage | null>('flash:message', () => null)
  const carried = useCookie<FlashMessage | null>(FLASH_COOKIE, {
    default: () => null,
    maxAge: FLASH_COOKIE_MAX_AGE_SECONDS,
    path: '/',
    sameSite: 'lax',
  })

  return {
    message: computed(() => message.value ?? carried.value),
    show(next: FlashMessage) {
      message.value = next
      carried.value = next
    },
    /** Returns the pending message once and clears it. */
    consume(): FlashMessage | null {
      const current = message.value ?? carried.value
      message.value = null
      carried.value = null
      return current
    },
  }
}
