/**
 * Color palette of the Figma "Agentes Agro" foundations (page 0:1).
 *
 * Single source of truth for colors (ADR 0011): it feeds the Vuetify theme,
 * which publishes each entry as `--v-theme-<name>` (RGB triplet). `tokens.css`
 * keeps the project names (`--color-*`) as aliases of those variables.
 *
 * Figma name -> key: cor/verde-* -> green-*, cor/broto-* -> sprout-*,
 * cor/cafe-* -> coffee-*, cor/neutro-* -> white/sand/border, cor/texto-* -> text-*,
 * cor/status-apta|atencao|inapta -> status-suitable|attention|unsuitable,
 * cor/info-chuva -> info-rain.
 */
export const palette = {
  'green-950': '#141f0c',
  'green-900': '#1d2c11',
  'green-800': '#273a16',
  'green-700': '#335020',
  'green-600': '#436b29',
  'green-100': '#e6eedb',

  'sprout-400': '#b5e36a',
  'sprout-500': '#8fcb3e',

  'coffee-700': '#6e5c43',
  'coffee-600': '#927d5e',
  'coffee-100': '#f1eadf',

  'white': '#ffffff',
  'sand': '#f7f4ee',
  'border': '#e5dfd3',

  'text-primary': '#18200f',
  'text-secondary': '#5d6553',
  'text-tertiary': '#8d927f',
  'text-inverse': '#ffffff',

  'status-suitable': '#2e9e4f',
  'status-suitable-bg': '#e3f4e8',
  'status-attention': '#c98316',
  'status-attention-bg': '#fbf0dc',
  'status-unsuitable': '#c2413b',
  'status-unsuitable-bg': '#f9e3e1',
  'info-rain': '#2f6fd1',
  'info-rain-bg': '#e2ecfb',

  // Map illustration (Figma 7:174, 15:265): dark land, roads and field fills.
  'map-land': '#24331a',
  'map-land-shade': '#1a2712',
  'map-road': '#b9a57e',
  'map-field-1': '#4e7a2b',
  'map-field-2': '#7fa342',
  'map-field-3': '#2f5a1c',
  'map-field-4': '#c9a54a',
  'map-field-5': '#6b8f35',
  'map-field-6': '#8a7350',
} as const

export type PaletteColor = keyof typeof palette

/** Vuetify semantic colors mapped to the design (ADR 0011, section 2). */
export const semanticColors = {
  'primary': palette['green-900'],
  'secondary': palette['coffee-700'],
  'accent': palette['sprout-400'],
  'success': palette['status-suitable'],
  'warning': palette['status-attention'],
  'error': palette['status-unsuitable'],
  'info': palette['info-rain'],
  'background': palette.sand,
  'surface': palette.white,
  'on-background': palette['text-primary'],
  'on-surface': palette['text-primary'],
} as const
