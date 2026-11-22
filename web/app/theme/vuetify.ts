import { createVuetify } from 'vuetify'
import type { VuetifyOptions } from 'vuetify'
import { appIconAliases, appIconSet } from './icons'
import { palette, semanticColors } from './palette'

export const APP_THEME = 'agentesAgro'

/**
 * Vuetify configuration shared by the Nuxt plugin and the test support
 * (ADR 0011). Fidelity to Figma comes in layers: theme (palette), SASS
 * variables (assets/styles/vuetify-settings.scss), these defaults and, last,
 * scoped styles inside the Base and App facades.
 */
export function createAppVuetify(options: Pick<VuetifyOptions, 'ssr'> = {}) {
  return createVuetify({
    ...options,
    theme: {
      defaultTheme: APP_THEME,
      themes: {
        [APP_THEME]: {
          dark: false,
          colors: { ...palette, ...semanticColors },
        },
      },
    },
    icons: {
      defaultSet: 'app',
      sets: { app: appIconSet },
      aliases: appIconAliases,
    },
    display: {
      // Below the 1024 px desktop layout the navigation drawer becomes temporary.
      mobileBreakpoint: 1024,
    },
    defaults: {
      global: { ripple: false },
      VBtn: { variant: 'flat', rounded: 'md', elevation: 0 },
      VTextField: { variant: 'outlined', hideDetails: 'auto', persistentPlaceholder: true },
      VSelect: { variant: 'outlined', hideDetails: 'auto', persistentPlaceholder: true },
      VAutocomplete: { variant: 'outlined', hideDetails: 'auto', persistentPlaceholder: true },
      VCard: { elevation: 0, border: true },
      VDialog: { maxWidth: 'var(--modal-width-md)', scrim: 'green-950' },
      VSnackbar: { location: 'top right', timeout: -1 },
    },
  })
}
