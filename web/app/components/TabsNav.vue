<script setup lang="ts" generic="T extends string">
/**
 * Tabs over a neutral divider (Figma "Aba" 27:162, instance "Abas" 15:165).
 * Switches subsections of the same entity. Emits the selected value via v-model;
 * the parent decides whether that maps to a route or to local state.
 */
export interface TabItem<V extends string = string> {
  value: V
  label: string
  count?: number
  /** Visible but not selectable (subsection without a screen yet). */
  disabled?: boolean
  /** Why a disabled tab is unavailable (accessible name and tooltip). */
  hint?: string
}

const props = defineProps<{
  items: readonly TabItem<T>[]
  /** Accessible name of the tab list. */
  label: string
  /** Prefix for `aria-controls`: the panel id is `${panelIdPrefix}-${value}`. */
  panelIdPrefix?: string
}>()

const model = defineModel<T>({ required: true })

function panelId(value: T): string | undefined {
  return props.panelIdPrefix ? `${props.panelIdPrefix}-${value}` : undefined
}

function tabId(value: T): string | undefined {
  return props.panelIdPrefix ? `${props.panelIdPrefix}-tab-${value}` : undefined
}

function onUpdate(value: unknown) {
  const item = props.items.find((candidate) => candidate.value === value)
  if (item) model.value = item.value
}
</script>

<template>
  <!--
    Facade over v-tabs (ADR 0011). Vuetify provides role="tablist"/"tab",
    aria-selected, roving focus (arrows, Home, End) and manual activation.
  -->
  <v-tabs
    :model-value="model"
    class="tabs"
    :aria-label="label"
    height="auto"
    :ripple="false"
    @update:model-value="onUpdate"
  >
    <v-tab
      v-for="item in items"
      :id="tabId(item.value)"
      :key="item.value"
      :value="item.value"
      class="tabs__tab"
      :class="{ 'tabs__tab--active': item.value === model }"
      :aria-controls="item.disabled ? undefined : panelId(item.value)"
      :disabled="item.disabled"
      :title="item.disabled ? item.hint : undefined"
      :ripple="false"
    >
      <span class="tabs__label">{{ item.label }}</span>
      <span v-if="item.count !== undefined" class="tabs__count text-label-counter">{{ item.count }}</span>
      <span v-if="item.disabled && item.hint" class="visually-hidden">({{ item.hint }})</span>
    </v-tab>
  </v-tabs>
</template>

<style scoped>
.tabs {
  height: auto;
  border-bottom: var(--border-width-hairline) solid var(--color-border);
}

.tabs :deep(.v-slide-group__content) {
  gap: var(--space-4);
}

/* The Figma indicator is the tab's own bottom border; Vuetify's slider and state layer are not used. */
.tabs :deep(.v-tab__slider),
.tabs__tab :deep(.v-btn__overlay),
.tabs__tab :deep(.v-btn__underlay) {
  display: none;
}

.tabs__tab :deep(.v-btn__content) {
  gap: var(--space-8);
}

.tabs__tab {
  display: inline-flex;
  min-width: 0;
  height: auto;
  border-radius: 0;
  letter-spacing: 0;
  gap: var(--space-8);
  align-items: center;
  /* Overlaps the container divider so the active indicator sits on it. */
  margin-bottom: calc(-1 * var(--border-width-hairline));
  padding: var(--space-12) var(--space-16);
  border: 0;
  border-bottom: var(--border-width-indicator) solid transparent;
  background: transparent;
  color: var(--color-text-secondary);
  font: var(--font-label-md);
  white-space: nowrap;
  cursor: pointer;
}

.tabs__tab:hover:not(.tabs__tab--active) {
  color: var(--color-text-primary);
}

.tabs__tab:focus-visible {
  outline: none;
  box-shadow: var(--focus-ring);
  border-radius: var(--radius-xs) var(--radius-xs) 0 0;
}

.tabs__tab.v-btn--disabled {
  opacity: 1;
  color: var(--color-text-tertiary);
  cursor: not-allowed;
}

.tabs__tab--active {
  border-bottom-color: var(--color-green-800);
  color: var(--color-green-800);
  font: var(--font-label-md-strong);
}

.tabs__count {
  padding: var(--padding-counter);
  border-radius: var(--radius-pill);
  background: var(--color-border);
  color: var(--color-text-secondary);
}

.tabs__tab--active .tabs__count {
  background: var(--color-green-800);
  color: var(--color-sprout-400);
}
</style>
