import type { Permission } from './auth'

declare module '#app' {
  interface PageMeta {
    /** Page reachable without a session (skips `middleware/auth.global.ts`). */
    public?: boolean
    /** Capability required to open the page (ADR 0013); without it the user goes to their home page. */
    permission?: Permission
  }
}

export {}
