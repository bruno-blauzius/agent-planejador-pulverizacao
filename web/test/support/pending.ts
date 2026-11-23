import { createHttpClient } from '~/services/http/client'
import { createHttpServices } from '~/services/http/services'
import type { ApiServices } from '~/services/ports'

/** The real HTTP services over a transport that must never be reached (identity checks only). */
export function createSsoRealServices(): ApiServices {
  const http = createHttpClient({ baseURL: 'http://api.test' }, async () => {
    throw new Error('the network must not be used in this test')
  })
  return createHttpServices(http, 'http://localhost:8002')
}
