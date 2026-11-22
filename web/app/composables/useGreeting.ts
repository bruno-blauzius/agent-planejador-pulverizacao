import { computed, shallowRef } from 'vue'
import { salutationFor } from '~/utils/format'
import { useSession } from './useSession'

/**
 * Header greeting ("Bom dia, Bruno"). The salutation depends on the viewer's
 * local hour, which the server (UTC) cannot know: SSR and hydration render the
 * neutral "Olá, Bruno", and the client switches to the salutation once Nuxt is
 * ready, avoiding a hydration mismatch.
 */
export function useGreeting(options: { now?: () => Date } = {}) {
  const now = options.now ?? (() => new Date())
  const { user } = useSession()
  const clientTime = shallowRef<Date | null>(null)

  onNuxtReady(() => {
    clientTime.value = now()
  })

  return {
    greeting: computed(() => {
      const salutation = clientTime.value ? salutationFor(clientTime.value) : 'Olá'
      const firstName = user.value?.name.trim().split(/\s+/)[0]
      return firstName ? `${salutation}, ${firstName}` : salutation
    }),
  }
}
