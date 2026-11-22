<script setup lang="ts">
/**
 * Hint over a drawing map (Figma 16:410): pen icon, what to do next and the
 * undo button. Live region, so the vertex count and the next step are read
 * out while drawing. Reused by every screen that draws (field, farm perimeter).
 * "Desfazer" only appears when there is something to undo, so the hint stays
 * short on phones (e.g. the single-point mode of the wizard).
 */
withDefaults(
  defineProps<{
    text: string
    /** How to edit (delete/insert vertices); read with the text and shown as tooltip. */
    help?: string
    canUndo?: boolean
    /** Id of the description (MapView `hint` slot prop `descriptionId`). */
    descriptionId?: string
  }>(),
  { help: undefined, canUndo: false, descriptionId: undefined },
)

const emit = defineEmits<{
  undo: []
}>()
</script>

<template>
  <div class="map-draw-hint" :title="help">
    <AppIcon name="pen" :size="16" class="map-draw-hint__icon" />
    <p :id="descriptionId" class="map-draw-hint__text text-label-chip" aria-live="polite">
      {{ text }}<span v-if="help" class="visually-hidden">. {{ help }}</span>
    </p>
    <button
      v-if="canUndo"
      type="button"
      class="map-draw-hint__undo text-label-chip"
      @click="emit('undo')"
    >
      <AppIcon name="undo" :size="14" />
      Desfazer
    </button>
  </div>
</template>

<style scoped>
.map-draw-hint {
  display: flex;
  gap: var(--space-10);
  align-items: center;
  padding: var(--space-10) var(--space-14);
  border-radius: var(--radius-md);
  background: var(--color-white);
  box-shadow: var(--shadow-card);
}

.map-draw-hint__icon {
  flex-shrink: 0;
  color: var(--color-text-secondary);
}

.map-draw-hint__text {
  min-width: 0;
  margin: 0;
  color: var(--color-text-secondary);
}

.map-draw-hint__undo {
  display: inline-flex;
  flex-shrink: 0;
  gap: var(--space-6);
  align-items: center;
  padding: var(--padding-badge-y) var(--space-10);
  border: 0;
  border-radius: var(--radius-sm);
  background: var(--color-sand);
  color: var(--color-text-secondary);
  cursor: pointer;
}

.map-draw-hint__undo:hover {
  color: var(--color-text-primary);
}

.map-draw-hint__undo:focus-visible {
  outline: none;
  box-shadow: var(--focus-ring);
}

</style>
