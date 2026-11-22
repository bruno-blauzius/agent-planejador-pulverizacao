import { toApiError } from '../errors'

export type HttpMethod = 'GET' | 'POST' | 'PUT' | 'PATCH' | 'DELETE'
export type QueryValue = string | number | undefined

export interface HttpRequestOptions {
  readonly method?: HttpMethod
  readonly query?: Readonly<Record<string, QueryValue>>
  /** JSON-serializable object or `FormData` for uploads. */
  readonly body?: object
}

/** Options handed to the underlying fetch (`$fetch` in the app, a fake in tests). */
export interface RawFetchOptions {
  readonly baseURL: string
  readonly method: HttpMethod
  readonly query?: Record<string, string | number>
  readonly body?: object
  readonly headers: Record<string, string>
}

export type RawFetch = (path: string, options: RawFetchOptions) => Promise<unknown>

/** Typed request against services/api. Rejects with `ApiError`. */
export type HttpClient = <T>(path: string, options?: HttpRequestOptions) => Promise<T>

export interface HttpClientConfig {
  readonly baseURL: string
  /** Read on every request so a login/logout takes effect immediately. */
  readonly getAccessToken: () => string | null | undefined
}

function compactQuery(query: Readonly<Record<string, QueryValue>> | undefined) {
  if (!query) return undefined
  const entries = Object.entries(query).filter(
    (entry): entry is [string, string | number] => entry[1] !== undefined && entry[1] !== '',
  )
  return entries.length ? Object.fromEntries(entries) : undefined
}

export function createHttpClient(config: HttpClientConfig, rawFetch: RawFetch): HttpClient {
  return async <T>(path: string, options: HttpRequestOptions = {}): Promise<T> => {
    const token = config.getAccessToken()
    const headers: Record<string, string> = { Accept: 'application/json' }
    if (token) headers.Authorization = `Bearer ${token}`

    try {
      const response = await rawFetch(path, {
        baseURL: config.baseURL,
        method: options.method ?? 'GET',
        query: compactQuery(options.query),
        body: options.body,
        headers,
      })
      // Single trust boundary: responses follow api/openapi.yaml. Runtime schema
      // validation (e.g. Zod) is deferred until the Go endpoints exist (ADR 0010).
      return response as T
    }
    catch (error) {
      throw toApiError(error)
    }
  }
}
