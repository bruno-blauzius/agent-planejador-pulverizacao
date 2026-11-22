<script setup lang="ts">
/**
 * Floating card over the map (Figma "Popover · Talhão 3", 15:334): title,
 * subtitle, close button, free body and an actions row. Non-modal dialog:
 * Escape or the close button emits `close`. MapView positions it next to the
 * selected feature (`popover` slot); the content comes from the parent.
 */
defineProps<{
  title: string
  subtitle?: string
}>()

const emit = defineEmits<{
  close: []
}>()

defineSlots<{
  default?: () => unknown
  actions?: () => unknown
}>()

const titleId = useId()
</script>

<template>
  <section
    class="map-popover"
    role="dialog"
    :aria-labelledby="titleId"
    @keydown.esc.prevent="emit('close')"
  >
    <header class="map-popover__header">
      <div class="map-popover__heading">
        <h2 :id="titleId" class="map-popover__title text-metric">{{ title }}</h2>
        <p v-if="subtitle" class="map-popover__subtitle text-caption">{{ subtitle }}</p>
      </div>
      <button type="button" class="map-popover__close" aria-label="Fechar" title="Fechar" @click="emit('close')">
        <AppIcon name="x" :size="18" />
      </button>
    </header>
    <slot />
    <div v-if="$slots.actions" class="map-popover__actions">
      <slot name="actions" />
    </div>
  </section>
</template>

<style scoped>
.map-popover {
  display: flex;
  flex-direction: column;
  gap: var(--space-12);
  width: var(--map-popover-width);
  max-width: 100%;
  padding: var(--space-18);
  border-radius: var(--radius-popover);
  background: var(--color-white);
  box-shadow: var(--shadow-popover);
}

.map-popover__header {
  display: flex;
  gap: var(--space-8);
  align-items: center;
}

.map-popover__heading {
  display: flex;
  flex: 1;
  flex-direction: column;
  gap: var(--space-2);
  min-width: 0;
}

.map-popover__title {
  margin: 0;
  overflow: hidden;
  color: var(--color-text-primary);
  text-overflow: ellipsis;
  white-space: nowrap;
}

.map-popover__subtitle {
  overflow: hidden;
  color: var(--color-text-secondary);
  text-overflow: ellipsis;
  white-space: nowrap;
}

.map-popover__close {
  display: grid;
  flex-shrink: 0;
  place-items: center;
  /* Larger hit area without growing the 18 px icon slot of the Figma. */
  margin: calc(-1 * var(--space-4));
  padding: var(--space-4);
  border: 0;
  border-radius: var(--radius-xs);
  background: transparent;
  color: var(--color-text-tertiary);
  cursor: pointer;
}

.map-popover__close:hover {
  background: var(--color-sand);
  color: var(--color-text-primary);
}

.map-popover__close:focus-visible {
  outline: none;
  box-shadow: var(--focus-ring);
}

.map-popover__actions {
  display: flex;
  gap: var(--space-8);
}
</style>
