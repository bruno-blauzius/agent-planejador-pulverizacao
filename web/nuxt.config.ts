// https://nuxt.com/docs/api/configuration/nuxt-config
import vuetify, { transformAssetUrls } from 'vite-plugin-vuetify'

export default defineNuxtConfig({
  compatibilityDate: '2026-11-01',

  devtools: { enabled: true },

  $production: {
    devtools: { enabled: false },
  },

  modules: ['@nuxt/eslint'],

  build: {
    transpile: ['vuetify'],
  },

  vite: {
    // Vuetify via its official Vite plugin (ADR 0011): auto-import for tree-shaking
    // and the SASS settings file that removes the Material accent. Paths are
    // relative to the Vite root, which Nuxt sets to the app/ directory.
    plugins: [
      vuetify({
        autoImport: true,
        styles: { configFile: 'assets/styles/vuetify-settings.scss' },
      }),
    ],
    vue: {
      template: { transformAssetUrls },
    },
  },

  css: ['~/assets/css/main.css'],

  app: {
    head: {
      htmlAttrs: { lang: 'pt-BR' },
      // Fonts from the Figma foundations. Loaded from Google Fonts to avoid a new
      // dependency; @nuxt/fonts (self-hosting) can replace this if approved.
      link: [
        { rel: 'icon', type: 'image/svg+xml', href: '/brand/icon.svg' },
        { rel: 'preconnect', href: 'https://fonts.googleapis.com' },
        { rel: 'preconnect', href: 'https://fonts.gstatic.com', crossorigin: '' },
        {
          rel: 'stylesheet',
          href: 'https://fonts.googleapis.com/css2?family=Bricolage+Grotesque:opsz,wght@12..96,600;12..96,700;12..96,800&family=Plus+Jakarta+Sans:wght@400;500;600;700&display=swap',
        },
      ],
    },
  },

  typescript: {
    strict: true,
    // Type checking runs in `pnpm typecheck`, keeping the build fast.
    typeCheck: false,
  },

  // The front talks exclusively to services/api (Go BFF), never to the Python
  // agent services. Services are wired in app/plugins/api.ts (ADR 0010).
  runtimeConfig: {
    // Server-side (SSR) base URL, e.g. the internal Docker network address.
    // Overridden by NUXT_API_BASE_URL; empty falls back to public.apiBase.
    apiBaseUrl: '',
    public: {
      // Browser base URL. Overridden by NUXT_PUBLIC_API_BASE. The session is an
      // HttpOnly cookie set by this host (ADR 0015): open the web app on the same
      // host name (localhost, not 127.0.0.1) so the cookie is same-site.
      apiBase: 'http://localhost:8080',
      // true = in-memory services instead of HTTP, login form included (dev builds
      // only). NUXT_PUBLIC_API_MOCK=true.
      apiMock: false,
      // With the real API: endpoints still `planned` in the contract (farms, owners,
      // dashboard, planner...) use the in-memory mock, acting as the seed user of the
      // signed-in user's role. Dev builds only. NUXT_PUBLIC_API_MOCK_PENDING=true.
      apiMockPending: false,
      // Keycloak account page of the Inceres SSO, where the password is changed
      // (ADR 0015). NUXT_PUBLIC_ACCOUNT_URL.
      accountUrl: 'http://localhost:8180/realms/inceres/account',
      // Simulated latency of the mock. NUXT_PUBLIC_API_MOCK_LATENCY_MS.
      apiMockLatencyMs: 300,
      // Vector tiles (TileJSON) for the map basemap (ADR 0012). NUXT_PUBLIC_MAP_TILES_URL.
      mapTilesUrl: 'https://tiles.openfreemap.org/planet',
      // Satellite raster tiles ({z}/{x}/{y}); empty disables "Satélite" (ADR 0012).
      // NUXT_PUBLIC_MAP_SATELLITE_URL.
      mapSatelliteUrl: '',
    },
  },
})
