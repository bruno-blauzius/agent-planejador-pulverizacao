import { createAppVuetify } from '~/theme/vuetify'

/**
 * Vuetify instance for component tests that mount Base and App facades
 * (ADR 0011, §6). Same theme, icons and defaults as the app, without SSR.
 *
 * Usage (Vue Test Utils): `mount(Component, { global: { plugins: [createTestVuetify()] } })`.
 * Mounting .vue files also needs the Vue plugin in vitest.config.ts and a DOM
 * environment (happy-dom/jsdom); neither is configured yet.
 */
export function createTestVuetify() {
  return createAppVuetify({ ssr: false })
}
