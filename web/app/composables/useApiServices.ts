import type { ApiServices } from '~/services/ports'

/** Ports wired by `plugins/api.ts` (HTTP or mock, per `runtimeConfig.public.apiMock`). */
export function useApiServices(): ApiServices {
  return useNuxtApp().$api
}
