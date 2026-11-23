<script setup lang="ts">
import type { IconName } from '../app/AppIcon.vue'

/**
 * Summary of one condition for the selected window (Figma 18:390): label,
 * value and a hint with the reference range. All texts come ready.
 */
defineProps<{
  icon: IconName
  label: string
  value: string
  hint: string
}>()
</script>

<template>
  <div class="condition-stat">
    <span class="condition-stat__icon" aria-hidden="true">
      <AppIcon :name="icon" :size="16" />
    </span>
    <p class="condition-stat__text">
      <span class="condition-stat__label text-caption-xs">{{ label }}</span>
      <span class="condition-stat__value text-label-md-strong">{{ value }}</span>
      <span class="condition-stat__hint">{{ hint }}</span>
    </p>
  </div>
</template>

<style scoped>
.condition-stat {
  display: flex;
  flex: 1 1 0;
  gap: var(--space-10);
  align-items: center;
  min-width: 0;
  padding: var(--space-12);
  border-radius: var(--radius-tile-lg);
  background: var(--color-sand);
}

.condition-stat__icon {
  display: inline-flex;
  flex-shrink: 0;
  align-items: center;
  justify-content: center;
  width: var(--size-planner-tile-icon);
  height: var(--size-planner-tile-icon);
  border-radius: var(--radius-tile);
  background: var(--color-white);
  color: var(--color-green-700);
}

.condition-stat__text {
  display: flex;
  flex-direction: column;
  gap: 1px;
  min-width: 0;
  margin: 0;
}

.condition-stat__label {
  color: var(--color-text-tertiary);
}

.condition-stat__value {
  color: var(--color-text-primary);
}

.condition-stat__hint {
  font: var(--font-planner-axis);
  color: var(--color-text-tertiary);
}
</style>
