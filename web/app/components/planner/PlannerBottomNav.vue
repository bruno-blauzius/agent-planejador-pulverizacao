<script setup lang="ts">
import type { IconName } from '../app/AppIcon.vue'

/**
 * Bottom navigation of the mobile operator view (Figma 20:135): four tabs with
 * the agent button highlighted in the middle. Items with `to` render links;
 * without it they render buttons that emit `select` (the page decides what to
 * do). Positioning (fixed or in the flow) is up to the page.
 */
export interface BottomNavItem {
  key: string
  label: string
  icon: IconName
  to?: string
  /** The destination does not exist yet: the tab stays focusable but inert, and the reason is announced. */
  unavailableHint?: string
}

const props = withDefaults(
  defineProps<{
    /** Tabs in display order; the agent button is inserted after the second. */
    items: readonly BottomNavItem[]
    activeKey: string
    agentLabel?: string
    agentTo?: string
  }>(),
  { agentLabel: 'Perguntar ao agente', agentTo: undefined },
)

const emit = defineEmits<{
  select: [key: string]
  agent: []
}>()

const AGENT_POSITION = 2

type NavSlot = { kind: 'tab'; item: BottomNavItem } | { kind: 'agent' }

const entries = computed<NavSlot[]>(() => [
  ...props.items.slice(0, AGENT_POSITION).map((item): NavSlot => ({ kind: 'tab', item })),
  { kind: 'agent' },
  ...props.items.slice(AGENT_POSITION).map((item): NavSlot => ({ kind: 'tab', item })),
])

const NuxtLink = resolveComponent('NuxtLink')

function isLink(item: BottomNavItem): boolean {
  return Boolean(item.to) && !item.unavailableHint
}

function onSelect(item: BottomNavItem) {
  if (item.unavailableHint) return
  emit('select', item.key)
}

function tabAttrs(item: BottomNavItem) {
  return {
    ...(isLink(item) ? { to: item.to } : { type: 'button' }),
    ...(item.unavailableHint ? { 'aria-disabled': 'true', title: item.unavailableHint } : {}),
    'aria-current': item.key === props.activeKey ? ('page' as const) : undefined,
  }
}
</script>

<template>
  <nav class="bottom-nav" aria-label="Navegação principal">
    <div v-for="entry in entries" :key="entry.kind === 'tab' ? entry.item.key : 'agent'" class="bottom-nav__slot">
      <component
        :is="agentTo ? NuxtLink : 'button'"
        v-if="entry.kind === 'agent'"
        v-bind="agentTo ? { to: agentTo } : { type: 'button' }"
        class="bottom-nav__agent"
        :aria-label="agentLabel"
        @click="emit('agent')"
      >
        <AppIcon name="sparkles" :size="24" />
      </component>
      <component
        :is="isLink(entry.item) ? NuxtLink : 'button'"
        v-else
        v-bind="tabAttrs(entry.item)"
        class="bottom-nav__tab"
        :class="{
          'bottom-nav__tab--active': entry.item.key === activeKey,
          'bottom-nav__tab--unavailable': entry.item.unavailableHint,
        }"
        @click="onSelect(entry.item)"
      >
        <AppIcon :name="entry.item.icon" :size="22" />
        <span class="bottom-nav__label text-caption-xs">{{ entry.item.label }}</span>
        <span v-if="entry.item.unavailableHint" class="visually-hidden">({{ entry.item.unavailableHint }})</span>
      </component>
    </div>
  </nav>
</template>

<style scoped>
.bottom-nav {
  display: flex;
  align-items: center;
  padding: var(--space-10) var(--space-14) var(--planner-nav-padding-bottom);
  border-top: var(--border-width-hairline) solid var(--color-border);
  background: var(--color-white);
}

.bottom-nav__slot {
  display: flex;
  flex: 1 1 0;
  justify-content: center;
  min-width: 0;
}

.bottom-nav__tab {
  display: flex;
  flex-direction: column;
  gap: var(--space-4);
  align-items: center;
  padding: 0;
  border: 0;
  border-radius: var(--radius-sm);
  background: transparent;
  color: var(--color-text-tertiary);
  text-decoration: none;
  cursor: pointer;
}

.bottom-nav__tab:focus-visible,
.bottom-nav__agent:focus-visible {
  outline: none;
  box-shadow: var(--focus-ring);
}

.bottom-nav__tab--unavailable {
  cursor: not-allowed;
  opacity: var(--opacity-skeleton-dim);
}

.bottom-nav__tab--active {
  color: var(--color-green-800);
}

.bottom-nav__label {
  font-weight: 500;
}

.bottom-nav__tab--active .bottom-nav__label {
  font-weight: 700;
}

.bottom-nav__agent {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: var(--size-planner-fab);
  height: var(--size-planner-fab);
  border: 0;
  border-radius: var(--radius-planner-fab);
  background: var(--color-sprout-400);
  color: var(--color-green-950);
  cursor: pointer;
  transition: background-color var(--duration-fast) var(--easing-standard);
}

.bottom-nav__agent:hover {
  background: var(--color-sprout-500);
}
</style>
