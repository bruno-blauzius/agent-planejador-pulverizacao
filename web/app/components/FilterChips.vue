<script setup lang="ts" generic="T extends string">
/**
 * Filter chips with counter (Figma 7:134; library "Chip de filtro" 26:246).
 * One active filter at a time. Facade over v-chip-group (ADR 0011); each chip
 * is a toggle button (aria-pressed) and the group has an accessible name.
 */
export interface FilterChipItem<V extends string = string> {
  value: V
  label: string
  count?: number
}

const props = withDefaults(
  defineProps<{
    items: readonly FilterChipItem<T>[]
    /** Accessible name of the group. */
    label: string
    /** `sm`: compact chips inside a table toolbar (Figma 39:640, "Todos · PF · PJ"). */
    size?: 'md' | 'sm'
  }>(),
  { size: 'md' },
)

const model = defineModel<T>({ required: true })

function onUpdate(value: unknown) {
  const item = props.items.find((candidate) => candidate.value === value)
  if (item) model.value = item.value
}
</script>

<template>
  <v-chip-group
    :model-value="model"
    class="filter-chips"
    :class="`filter-chips--${size}`"
    role="group"
    :aria-label="label"
    mandatory
    column
    @update:model-value="onUpdate"
  >
    <v-chip
      v-for="item in items"
      :key="item.value"
      :value="item.value"
      tag="button"
      type="button"
      class="filter-chips__chip"
      :class="[size === 'sm' ? 'text-label-chip' : 'text-label-meta', { 'filter-chips__chip--active': item.value === model }]"
      :aria-pressed="item.value === model"
      :ripple="false"
      variant="flat"
    >
      {{ item.label }}
      <span v-if="item.count !== undefined" class="filter-chips__count text-label-counter">{{ item.count }}</span>
    </v-chip>
  </v-chip-group>
</template>

<style scoped>
.filter-chips :deep(.v-slide-group__content) {
  gap: var(--space-10);
  padding: 0;
}

.filter-chips__chip {
  height: auto;
  margin: 0;
  padding: var(--padding-segment-y) var(--space-14);
  border: var(--border-width-hairline) solid var(--color-border);
  border-radius: var(--radius-pill);
  background: var(--color-white);
  color: var(--color-text-secondary);
  letter-spacing: 0;
  cursor: pointer;
}

.filter-chips__chip :deep(.v-chip__overlay),
.filter-chips__chip :deep(.v-chip__underlay) {
  display: none;
}

.filter-chips__chip :deep(.v-chip__content) {
  gap: var(--space-8);
}

.filter-chips__chip:hover:not(.filter-chips__chip--active) {
  color: var(--color-text-primary);
}

.filter-chips__chip:focus-visible {
  outline: none;
  box-shadow: var(--focus-ring);
}

.filter-chips__chip--active {
  border-color: var(--color-green-800);
  background: var(--color-green-800);
  color: var(--color-text-inverse);
}

.filter-chips__count {
  padding: var(--padding-counter);
  border-radius: var(--radius-pill);
  background: var(--color-sand);
  color: var(--color-text-secondary);
}

.filter-chips__chip--active .filter-chips__count {
  background: var(--color-sprout-400);
  color: var(--color-green-950);
}
.filter-chips--sm .filter-chips__chip {
  padding: var(--space-8) var(--space-12);
  background: transparent;
}

.filter-chips--sm .filter-chips__chip--active {
  background: var(--color-green-800);
}
</style>
