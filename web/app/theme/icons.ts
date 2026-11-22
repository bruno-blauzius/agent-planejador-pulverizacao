import { h } from 'vue'
import type { IconAliases, IconProps, IconSet } from 'vuetify'
import { aliases as svgFallbackAliases } from 'vuetify/iconsets/mdi-svg'
import AppIcon from '~/components/app/AppIcon.vue'
import type { IconName } from '~/components/app/AppIcon.vue'

/**
 * Vuetify icon set that renders the Figma icons through AppIcon (ADR 0011, §4),
 * so `v-icon` and Vuetify internals (close, arrows, expand) use the Figma drawing.
 *
 * The Figma library only has right/down chevrons; left/up are rotations.
 */
const ROTATED: Record<string, { name: IconName; rotate: number }> = {
  'chevron-left': { name: 'chevron-right', rotate: 180 },
  'chevron-up': { name: 'chevron-down', rotate: 180 },
}

function resolve(icon: unknown): { name: IconName; rotate: number } | null {
  if (typeof icon !== 'string') return null
  return ROTATED[icon] ?? { name: icon as IconName, rotate: 0 }
}

const AppIconAdapter = (props: IconProps) => {
  const resolved = resolve(props.icon)
  const style = resolved?.rotate ? { transform: `rotate(${resolved.rotate}deg)` } : undefined
  return h(
    props.tag,
    {},
    resolved ? h(AppIcon, { name: resolved.name, size: '100%', style }) : undefined,
  )
}

export const appIconSet: IconSet = { component: AppIconAdapter }

/**
 * Internal Vuetify aliases mapped to Figma icons. Aliases without a Figma
 * equivalent (checkbox, radio, rating, sort…) fall back to the SVG paths that
 * ship inside Vuetify (no icon font is loaded).
 */
export const appIconAliases: Partial<IconAliases> = {
  ...svgFallbackAliases,
  collapse: 'chevron-up',
  complete: 'check',
  cancel: 'x',
  close: 'x',
  delete: 'x',
  clear: 'x',
  success: 'check',
  info: 'info',
  warning: 'alert',
  error: 'alert',
  prev: 'chevron-left',
  next: 'chevron-right',
  expand: 'chevron-down',
  menu: 'list',
  subgroup: 'chevron-down',
  dropdown: 'chevron-down',
  edit: 'pen',
  plus: 'plus',
  minus: 'minus',
  calendar: 'calendar',
  clock: 'clock',
  upload: 'upload',
  file: 'document',
  treeviewCollapse: 'chevron-down',
  treeviewExpand: 'chevron-right',
}
