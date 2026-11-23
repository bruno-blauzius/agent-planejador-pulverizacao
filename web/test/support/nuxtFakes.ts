/**
 * Minimal in-memory stand-ins for the Nuxt auto-imports used by composables
 * (`useNuxtApp`, `useState`, `useCookie`, `useAsyncData`), so composables can be
 * tested with plain Vitest and the mock services, without `@nuxt/test-utils`.
 */
import { reactive, ref, shallowRef, toValue, watch, type MaybeRefOrGetter, type Ref } from 'vue'
import { vi } from 'vitest'
import type { ApiServices } from '~/services/ports'

interface AsyncDataOptions<T> {
  default?: () => T
}

export interface NuxtFakes {
  /** Value of a cookie created through `useCookie`. */
  cookie: (name: string) => Ref<string | null>
  /** Expiry the cookie would be written with (Nuxt resolves `expires` on each write). */
  cookieExpires: (name: string) => Date | undefined
  /** `useRuntimeConfig().public`; tests set what the code under test reads. */
  publicConfig: Record<string, unknown>
  /** Route returned by `useRoute()`; tests mutate its query. */
  route: { path: string, fullPath: string, query: Record<string, unknown>, meta: { public?: boolean } }
  /** Targets passed to `navigateTo`, in order. */
  navigations: unknown[]
  /** Times `clearNuxtData()` was called (session changes drop cached page data). */
  clearedNuxtData: number
  /** Simulates a new server request: drops `useState` values, keeps cookies. */
  newRequest: () => void
  /** Last function given to `defineNuxtRouteMiddleware`. */
  routeMiddleware: FakeRouteMiddleware | null
  /** When false, `onMounted` callbacks wait for `mount()` (default: run right away). */
  autoMount: boolean
  /** Runs the `onMounted` callbacks held while `autoMount` was false. */
  mount: () => void
}

export interface FakeRouteTarget {
  path: string
  fullPath: string
  meta: { public?: boolean, permission?: string }
  query: Record<string, unknown>
}

export type FakeRouteMiddleware = (to: FakeRouteTarget) => Promise<unknown>

function isQueryOnlyTarget(target: unknown): target is { query: Record<string, unknown> } {
  return typeof target === 'object' && target !== null && 'query' in target && !('path' in target)
    && typeof target.query === 'object' && target.query !== null
}

/** `api` may be a factory, to wire services that read the fake cookies. */
export function installNuxtFakes(api: ApiServices | ((fakes: NuxtFakes) => ApiServices)): NuxtFakes {
  const states = new Map<string, Ref<unknown>>()
  const mountedCallbacks: (() => void)[] = []
  const cookieOptions = new Map<string, { expires?: Date | (() => Date | undefined) }>()
  const fakes: NuxtFakes = {
    cookie: name => shared<string | null>(`cookie:${name}`, () => null),
    cookieExpires: (name) => {
      const expires = cookieOptions.get(name)?.expires
      return typeof expires === 'function' ? expires() : expires
    },
    publicConfig: {},
    route: reactive({ path: '/login', fullPath: '/login', query: {}, meta: {} }),
    navigations: [],
    routeMiddleware: null,
    autoMount: true,
    mount: () => mountedCallbacks.splice(0).forEach(callback => callback()),
    clearedNuxtData: 0,
    newRequest: () => {
      for (const key of [...states.keys()]) {
        if (key.startsWith('state:')) states.delete(key)
      }
    },
  }
  const services = typeof api === 'function' ? api(fakes) : api

  function shared<T>(key: string, init: () => T): Ref<T> {
    if (!states.has(key)) states.set(key, ref(init()))
    return states.get(key) as Ref<T>
  }

  // Same instance on every call, like the real app (registries key on it).
  const nuxtApp = { $api: services }
  vi.stubGlobal('useNuxtApp', () => nuxtApp)
  vi.stubGlobal('onMounted', (callback: () => void) => {
    if (fakes.autoMount) callback()
    else mountedCallbacks.push(callback)
  })
  vi.stubGlobal('useRoute', () => fakes.route)
  vi.stubGlobal('useRuntimeConfig', () => ({ public: fakes.publicConfig }))
  // The fake useAsyncData keeps no shared cache, so there is nothing to clear; tests can count calls.
  vi.stubGlobal('clearNuxtData', () => {
    fakes.clearedNuxtData += 1
  })
  // Client already hydrated: run right away (SSR would never call it).
  vi.stubGlobal('onNuxtReady', (callback: () => void) => callback())
  vi.stubGlobal('navigateTo', async (target: unknown, options?: { replace?: boolean }) => {
    fakes.navigations.push(options ? { target, options } : target)
    // Query-only navigations update the fake route, like the router would.
    if (isQueryOnlyTarget(target)) {
      fakes.route.query = Object.fromEntries(Object.entries(target.query).filter(([, value]) => value !== undefined))
    }
    return target
  })
  vi.stubGlobal('defineNuxtRouteMiddleware', (middleware: FakeRouteMiddleware) => {
    fakes.routeMiddleware = middleware
    return middleware
  })
  vi.stubGlobal('useState', <T>(key: string, init: () => T) => shared(`state:${key}`, init))
  vi.stubGlobal('useCookie', <T>(name: string, options: { default?: () => T, expires?: () => Date | undefined } = {}) => {
    cookieOptions.set(name, options)
    return shared(`cookie:${name}`, () => options.default?.() ?? null)
  })
  vi.stubGlobal('useAsyncData', <T>(
    key: MaybeRefOrGetter<string>,
    handler: () => Promise<T>,
    options: AsyncDataOptions<T> = {},
  ) => {
    const data = ref(options.default?.()) as Ref<T | undefined>
    const status = ref<'idle' | 'pending' | 'success' | 'error'>('idle')
    const error = shallowRef<unknown>(undefined)

    async function execute() {
      status.value = 'pending'
      error.value = undefined
      try {
        data.value = await handler()
        status.value = 'success'
      }
      catch (caught) {
        // Nuxt wraps the handler error in a NuxtError, keeping it as `cause`.
        error.value = Object.assign(new Error('async data failed'), { cause: caught })
        status.value = 'error'
      }
    }

    watch(() => toValue(key), () => void execute())
    void execute()
    return { data, status, error, refresh: execute }
  })

  return fakes
}

/** Lets pending promises (mock services, watchers) settle. */
export function flushPromises(): Promise<void> {
  return new Promise(resolve => setTimeout(resolve, 0))
}
