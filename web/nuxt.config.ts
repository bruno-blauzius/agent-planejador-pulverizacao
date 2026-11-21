// https://nuxt.com/docs/api/configuration/nuxt-config
export default defineNuxtConfig({
  compatibilityDate: '2026-11-01',

  devtools: { enabled: true },

  $production: {
    devtools: { enabled: false },
  },

  modules: ['@nuxt/eslint'],

  typescript: {
    strict: true,
    // Type checking runs in `pnpm typecheck`, keeping the build fast.
    typeCheck: false,
  },

  runtimeConfig: {
    // Server-side only. The front talks exclusively to services/api (Go BFF),
    // never to the Python agent services. Overridden by NUXT_API_BASE_URL.
    apiBaseUrl: 'http://localhost:8080',
  },
})
