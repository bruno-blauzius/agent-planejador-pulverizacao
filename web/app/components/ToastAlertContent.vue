<script setup lang="ts">
import type { IconName } from './app/AppIcon.vue'
import type { AlertTone } from './ToastAlert.vue'

/**
 * Inner layout of ToastAlert (icon tile, title, message, actions, dismiss).
 * Shared by the inline (v-alert) and floating (v-snackbar) renderings.
 */
defineProps<{
  tone: AlertTone
  icon: IconName
  title: string
  message?: string
  dismissible: boolean
  dismissLabel: string
}>()

const emit = defineEmits<{
  dismiss: []
}>()

defineSlots<{
  actions?: () => unknown
}>()
</script>

<template>
  <div class="alert-content" :class="`alert-content--${tone}`">
    <span class="alert-content__icon" aria-hidden="true">
      <AppIcon :name="icon" :size="16" />
    </span>
    <div class="alert-content__text">
      <p class="alert-content__title text-label-strong">{{ title }}</p>
      <p v-if="message" class="alert-content__message text-body-sm">{{ message }}</p>
      <div v-if="$slots.actions" class="alert-content__actions">
        <slot name="actions" />
      </div>
    </div>
    <button
      v-if="dismissible"
      type="button"
      class="alert-content__dismiss"
      :aria-label="dismissLabel"
      @click="emit('dismiss')"
    >
      <AppIcon name="x" :size="16" />
    </button>
  </div>
</template>

<style scoped>
.alert-content {
  display: flex;
  gap: var(--space-12);
  align-items: flex-start;
  width: 100%;
}

.alert-content__icon {
  display: grid;
  place-items: center;
  width: var(--size-icon-tile-sm);
  height: var(--size-icon-tile-sm);
  flex-shrink: 0;
  border-radius: var(--radius-tile);
  background: var(--color-white);
  color: var(--alert-accent);
}

.alert-content--agent .alert-content__icon {
  background: var(--color-sprout-soft);
}

.alert-content__text {
  display: flex;
  flex: 1;
  flex-direction: column;
  gap: var(--space-2);
  min-width: 0;
}

.alert-content__title {
  color: var(--color-text-primary);
}

.alert-content__message {
  color: var(--color-text-secondary);
}

.alert-content--agent .alert-content__title {
  color: var(--color-text-inverse);
}

.alert-content--agent .alert-content__message {
  color: var(--color-text-inverse-body);
}

.alert-content__actions {
  display: flex;
  gap: var(--space-8);
  margin-top: var(--space-8);
}

.alert-content__dismiss {
  display: grid;
  place-items: center;
  padding: 0;
  border: 0;
  border-radius: var(--radius-xs);
  background: transparent;
  color: var(--color-text-tertiary);
  cursor: pointer;
}

.alert-content--agent .alert-content__dismiss {
  color: var(--color-text-inverse-strong);
}

.alert-content__dismiss:hover {
  color: var(--color-text-primary);
}

.alert-content--agent .alert-content__dismiss:hover {
  color: var(--color-text-inverse);
}

.alert-content__dismiss:focus-visible {
  outline: none;
  box-shadow: var(--focus-ring);
}
</style>
