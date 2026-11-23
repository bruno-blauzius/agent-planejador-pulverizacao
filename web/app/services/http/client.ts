import { toApiError } from '../errors'

export type HttpMethod = 'GET' | 'POST' | 'PUT' | 'PATCH' | 'DELETE'
export type QueryValue = string | number | undefined

export interface HttpRequestOptions {
  readonly method?: HttpMethod
  readonly query?: Readonly<Record<string, QueryValue>>
  /** JSON-serializable object or `FormData` for uploads. */
  readonly body?: object
}

/** Response headers handed to `onHeaders` (the `Headers` of fetch, or a fake in tests). */
export interface ResponseHeaders {
  get: (name: string) => string | null
}

/** Options handed to the underlying fetch (`$fetch` in the app, a fake in tests). */
export interface RawFetchOptions {
  readonly baseURL: string
  readonly method: HttpMethod
  readonly query?: Record<string, string | number>
  readonly body?: object
  readonly headers: Record<string, string>
  /**
   * `include` sends the session cookie (`agro_session`, HttpOnly) cross-origin,
   * e.g. web on localhost:3000 and the API on localhost:8002. Absent when the
   * client was not configured with it.
   */
  readonly credentials?: 'include'
  /** Called with the headers of a successful response; transports that cannot read them may skip it. */
  readonly onHeaders?: (headers: ResponseHeaders) => void
}

/** Resolves with the response body (`undefined` for 204). Rejects like `$fetch` does. */
export type RawFetch = (path: string, options: RawFetchOptions) => Promise<unknown>

export interface HttpResult<T> {
  readonly data: T
  /** Empty (`get` always `null`) when the transport did not expose headers. */
  readonly headers: ResponseHeaders
}

/** Typed request against services/api. Rejects with `ApiError`. */
export interface HttpClient {
  <T>(path: string, options?: HttpRequestOptions): Promise<T>
  /** Same request, also returning the response headers (e.g. `X-Invitation-Email`). */
  withHeaders: <T>(path: string, options?: HttpRequestOptions) => Promise<HttpResult<T>>
}

export interface HttpClientConfig {
  readonly baseURL: string
  /** Send cookies on cross-origin requests (the browser; SSR forwards them with `getRequestHeaders`). */
  readonly credentials?: 'include'
  /**
   * Extra headers on every request, read at call time. SSR uses it to forward
   * the browser's `cookie` header to the API, which has no way to see it otherwise.
   */
  readonly getRequestHeaders?: () => Record<string, string>
  /** Called when the API answers 401, with the request path (the session expired or was revoked). */
  readonly onUnauthorized?: (path: string) => void
}

const NO_HEADERS: ResponseHeaders = { get: () => null }

function compactQuery(query: Readonly<Record<string, QueryValue>> | undefined) {
  if (!query) return undefined
  const entries = Object.entries(query).filter(
    (entry): entry is [string, string | number] => entry[1] !== undefined && entry[1] !== '',
  )
  return entries.length ? Object.fromEntries(entries) : undefined
}

export function createHttpClient(config: HttpClientConfig, rawFetch: RawFetch): HttpClient {
  async function send<T>(path: string, options: HttpRequestOptions = {}): Promise<HttpResult<T>> {
    let headers = NO_HEADERS
    try {
      const data = await rawFetch(path, {
        baseURL: config.baseURL,
        method: options.method ?? 'GET',
        query: compactQuery(options.query),
        body: options.body,
        headers: { Accept: 'application/json', ...config.getRequestHeaders?.() },
        ...(config.credentials ? { credentials: config.credentials } : {}),
        onHeaders: (received) => {
          headers = received
        },
      })
      // Single trust boundary: responses follow api/openapi.yaml. Runtime schema
      // validation (e.g. Zod) is deferred until the Go endpoints exist (ADR 0010).
      return { data: data as T, headers }
    }
    catch (error) {
      const apiError = toApiError(error)
      if (apiError.status === 401) config.onUnauthorized?.(path)
      throw apiError
    }
  }

  const call = async <T>(path: string, options?: HttpRequestOptions): Promise<T> => (await send<T>(path, options)).data
  return Object.assign(call, { withHeaders: send })
}
