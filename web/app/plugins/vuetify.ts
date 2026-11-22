import 'vuetify/styles'
import { createAppVuetify } from '~/theme/vuetify'

/**
 * Vuetify integration (ADR 0011): official Vite integration, no third-party
 * Nuxt module. Vuetify 4 ships its CSS in cascade layers (`vuetify-*`), so the
 * project's unlayered styles (main.css and scoped styles) win by layer order,
 * without `!important`.
 */
// The server has no viewport: render as the 1440 px desktop of the Figma frames
// so the drawer is not first rendered as mobile and then flipped on hydration.
const SSR_VIEWPORT = { clientWidth: 1440, clientHeight: 900 }

export default defineNuxtPlugin((nuxtApp) => {
  nuxtApp.vueApp.use(createAppVuetify({ ssr: SSR_VIEWPORT }))
})
