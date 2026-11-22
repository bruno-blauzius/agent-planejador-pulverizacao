<script setup lang="ts">
/**
 * Area of the drawn field (Figma 16:438): ruler tile, value and caption
 * ("aproximados, calculados a partir do desenho" — ADR 0012), plus the action
 * of the current step ("Ajustar" back to the drawing, or "Continuar").
 */
defineProps<{
  value: string
  caption: string
  actionLabel: string
  /** Validation message (no drawing yet). */
  error?: string
}>()

const emit = defineEmits<{
  action: []
}>()

</script>

<template>
  <div class="field-area">
    <div class="field-area__card">
      <span class="field-area__icon" aria-hidden="true"><AppIcon name="ruler" :size="20" /></span>
      <p class="field-area__text">
        <span class="field-area__value text-title-md">{{ value }}</span>
        <span class="field-area__caption text-caption">{{ caption }}</span>
      </p>
      <button type="button" class="field-area__action text-label-meta-strong" data-area-action @click="emit('action')">
        {{ actionLabel }}
      </button>
    </div>
    <p v-if="error" class="field-area__error text-body-sm" role="alert">{{ error }}</p>
  </div>
</template>

<style scoped>
.field-area {
  display: flex;
  flex-direction: column;
  gap: var(--space-6);
}

.field-area__card {
  display: flex;
  gap: var(--space-12);
  align-items: center;
  padding: var(--space-14);
  border-radius: var(--radius-tile-lg);
  background: var(--color-green-900);
}

.field-area__icon {
  display: grid;
  flex-shrink: 0;
  place-items: center;
  width: var(--size-icon-tile-area);
  height: var(--size-icon-tile-area);
  border-radius: var(--radius-tile);
  background: var(--color-sprout-400);
  color: var(--color-green-950);
}

.field-area__text {
  display: flex;
  flex: 1;
  flex-direction: column;
  gap: var(--space-2);
  min-width: 0;
  margin: 0;
}

.field-area__value {
  color: var(--color-text-inverse);
}

.field-area__caption {
  color: var(--color-text-inverse-muted);
}

.field-area__action {
  flex-shrink: 0;
  padding: var(--space-4) var(--space-6);
  border: 0;
  border-radius: var(--radius-xs);
  background: transparent;
  color: var(--color-sprout-400);
  cursor: pointer;
}

.field-area__action:hover {
  background: var(--color-surface-inverse-glass);
}

.field-area__action:focus-visible {
  outline: none;
  box-shadow: var(--focus-ring);
}

.field-area__error {
  color: var(--color-status-unsuitable);
}
</style>
