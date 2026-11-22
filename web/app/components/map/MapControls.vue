<script setup lang="ts">
/**
 * Map controls of the project (Figma 15:302: white box, 36 px buttons with
 * Figma icons). Replaces the MapLibre default controls (ADR 0012).
 * The pen button is optional (`drawLabel`): the parent decides what drawing
 * means (open the new-field screen, toggle the drawing tool).
 */
withDefaults(
  defineProps<{
    /** Accessible name of the pen button; omit to hide it. */
    drawLabel?: string
    /** Pressed state when the pen toggles a drawing tool; null when it is a plain action. */
    drawActive?: boolean | null
  }>(),
  { drawLabel: undefined, drawActive: null },
)

const emit = defineEmits<{
  'zoom-in': []
  'zoom-out': []
  fit: []
  draw: []
}>()
</script>

<template>
  <div class="map-controls" role="group" aria-label="Controles do mapa">
    <button type="button" class="map-controls__button" aria-label="Aproximar" title="Aproximar" @click="emit('zoom-in')">
      <AppIcon name="plus" :size="18" />
    </button>
    <button type="button" class="map-controls__button" aria-label="Afastar" title="Afastar" @click="emit('zoom-out')">
      <AppIcon name="minus" :size="18" />
    </button>
    <button
      type="button"
      class="map-controls__button"
      aria-label="Enquadrar perímetros"
      title="Enquadrar perímetros"
      @click="emit('fit')"
    >
      <AppIcon name="locate" :size="18" />
    </button>
    <button
      v-if="drawLabel"
      type="button"
      class="map-controls__button"
      :class="{ 'map-controls__button--active': drawActive }"
      :aria-label="drawLabel"
      :aria-pressed="drawActive ?? undefined"
      :title="drawLabel"
      @click="emit('draw')"
    >
      <AppIcon name="pen" :size="18" />
    </button>
  </div>
</template>

<style scoped>
.map-controls {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  padding: var(--space-4);
  border-radius: var(--radius-md);
  background: var(--color-white);
}

.map-controls__button {
  display: grid;
  place-items: center;
  width: var(--size-icon-tile-md);
  height: var(--size-icon-tile-md);
  border: 0;
  border-radius: var(--radius-sm);
  background: transparent;
  color: var(--color-text-secondary);
  cursor: pointer;
}

.map-controls__button:hover {
  background: var(--color-sand);
  color: var(--color-text-primary);
}

.map-controls__button--active,
.map-controls__button--active:hover {
  background: var(--color-green-800);
  color: var(--color-text-inverse);
}

.map-controls__button:focus-visible {
  outline: none;
  box-shadow: var(--focus-ring);
}
</style>
