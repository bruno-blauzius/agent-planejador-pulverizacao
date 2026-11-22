<script setup lang="ts">
import type { IconName } from './app/AppIcon.vue'

/**
 * Empty state of a list (library "Estado vazio", 31:306): icon tile, title,
 * short text and a call to action in the `actions` slot.
 * `variant="forbidden"`: honest "no permission" state (attention tone, role status).
 */
withDefaults(
  defineProps<{
    icon: IconName
    title: string
    description: string
    variant?: 'default' | 'forbidden'
  }>(),
  { variant: 'default' },
)

defineSlots<{
  actions?: () => unknown
}>()
</script>

<template>
  <section
    class="empty-state"
    :class="`empty-state--${variant}`"
    :role="variant === 'forbidden' ? 'status' : undefined"
  >
    <span class="empty-state__icon" aria-hidden="true"><AppIcon :name="icon" :size="24" /></span>
    <h2 class="empty-state__title text-h4">{{ title }}</h2>
    <p class="empty-state__text text-body-md">{{ description }}</p>
    <slot name="actions" />
  </section>
</template>

<style scoped>
.empty-state {
  display: flex;
  flex-direction: column;
  gap: var(--space-12);
  align-items: center;
  padding: var(--space-48) var(--space-24);
  border: var(--border-width-control) dashed var(--color-border);
  border-radius: var(--radius-xl);
  background: var(--color-white);
  text-align: center;
}

.empty-state__icon {
  display: grid;
  place-items: center;
  width: var(--size-icon-tile-lg);
  height: var(--size-icon-tile-lg);
  border-radius: var(--radius-tile-lg);
  background: var(--color-green-100);
  color: var(--color-green-800);
}

.empty-state--forbidden .empty-state__icon {
  background: var(--color-status-attention-bg);
  color: var(--color-status-attention);
}

.empty-state__title {
  color: var(--color-text-primary);
}

.empty-state__text {
  max-width: var(--empty-state-text-width);
  color: var(--color-text-secondary);
}
</style>
