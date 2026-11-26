<script setup lang="ts">
import type { TerrainProgressView } from '~/utils/terrain'

/**
 * Calculation progress (Figma 72:1172: "Estado durante o cálculo", label on the
 * right and an 8 px green bar). `role="progressbar"` with the same text as value.
 */
defineProps<{
  progress: TerrainProgressView
  /** Left caption ("Estado durante o cálculo"); also the accessible name. */
  label: string
}>()

const labelId = useId()
</script>

<template>
  <div class="terrain-progress">
    <div class="terrain-progress__head text-caption">
      <span :id="labelId" class="terrain-progress__title">{{ label }}</span>
      <span class="terrain-progress__count">{{ progress.label }}</span>
    </div>
    <div
      class="terrain-progress__track"
      role="progressbar"
      :aria-labelledby="labelId"
      aria-valuemin="0"
      aria-valuemax="100"
      :aria-valuenow="Math.round(progress.percent)"
      :aria-valuetext="progress.label"
    >
      <span class="terrain-progress__fill" :style="{ width: `${progress.percent}%` }" />
    </div>
  </div>
</template>

<style scoped>
.terrain-progress {
  display: flex;
  flex-direction: column;
  gap: var(--space-8);
}

.terrain-progress__head {
  display: flex;
  gap: var(--space-8);
}

.terrain-progress__title {
  flex: 1;
  min-width: 0;
  color: var(--color-text-primary);
  font-weight: 700;
}

.terrain-progress__count {
  color: var(--color-text-secondary);
  font-weight: 600;
  white-space: nowrap;
}

.terrain-progress__track {
  height: var(--space-8);
  overflow: hidden;
  border-radius: var(--radius-pill);
  background: var(--color-coffee-100);
}

.terrain-progress__fill {
  display: block;
  height: 100%;
  border-radius: var(--radius-pill);
  background: var(--color-green-600);
  transition: width var(--duration-fast) var(--easing-standard);
}

@media (prefers-reduced-motion: reduce) {
  .terrain-progress__fill {
    transition: none;
  }
}
</style>
