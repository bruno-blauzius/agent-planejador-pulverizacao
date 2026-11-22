/**
 * App shell measures (Figma frame 7:16). Single source: Vuetify's layout
 * components need them as numbers, and the components publish them as CSS
 * variables (`--sidebar-width`, `--topbar-height`) for their own styles.
 */

/** Sidebar width (Figma 7:17). */
export const SIDEBAR_WIDTH = 256

/** Top bar height (Figma 7:87). */
export const TOPBAR_HEIGHT = 100

/** Distance from the top of the page to the top bar (Figma 7:87 at y = 28). */
export const TOPBAR_OFFSET_TOP = 28

/** Total height of the v-app-bar, which offsets v-main. */
export const APP_BAR_HEIGHT = TOPBAR_HEIGHT + TOPBAR_OFFSET_TOP

/** Auto-dismiss of transient shell notices (e.g. "Acesso não permitido"), in ms. */
export const NOTICE_TIMEOUT_MS = 6000

export function px(value: number): string {
  return `${value}px`
}
