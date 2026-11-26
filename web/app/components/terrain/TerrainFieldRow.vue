<script setup lang="ts">
import { TERRAIN_CLASS_TONES } from '~/theme/map-style'
import type { TerrainFieldRowView } from '~/utils/terrain'

/**
 * Field row of the relief list (Figma "Relevo · Linha do talhão", 67:2240; selected
 * 68:1036): class color bar, name, detail and the class chip. Selecting it
 * highlights the polygon on the map.
 */
const props = defineProps<{
  field: TerrainFieldRowView
}>()

const emit = defineEmits<{
  select: [id: string]
}>()

// Data-driven color: the class is only known at runtime.
const barStyle = computed(() => ({ backgroundColor: TERRAIN_CLASS_TONES[props.field.colorKey].bar }))
</script>

<template>
  <button
    type="button"
    class="terrain-row"
    :class="{ 'terrain-row--selected': field.selected }"
    :aria-pressed="field.selected"
    :data-field-id="field.id"
    @click="emit('select', field.id)"
  >
    <span class="terrain-row__bar" :style="barStyle" aria-hidden="true" />
    <span class="terrain-row__text">
      <span class="terrain-row__name text-label-meta-strong">
        {{ field.name }}
        <span v-if="field.isAdjusted" class="terrain-row__adjusted text-label-micro">ajustado</span>
      </span>
      <span class="terrain-row__detail text-caption" :class="{ 'terrain-row__detail--failed': field.isFailed }">
        {{ field.detail }}
      </span>
    </span>
    <TerrainClassBadge :badge="field.badge" />
  </button>
</template>

<style scoped>
.terrain-row {
  display: flex;
  gap: var(--space-12);
  align-items: center;
  width: 100%;
  padding: var(--space-10);
  border: var(--border-width-control) solid transparent;
  border-radius: var(--radius-md);
  background: transparent;
  color: inherit;
  font: inherit;
  text-align: left;
  cursor: pointer;
}

.terrain-row:hover:not(.terrain-row--selected) {
  background: var(--color-sand);
}

.terrain-row:focus-visible {
  outline: none;
  box-shadow: var(--focus-ring);
}

.terrain-row--selected {
  border-color: var(--color-green-700);
  background: var(--color-green-100);
}

.terrain-row__bar {
  flex-shrink: 0;
  width: var(--size-field-swatch-width);
  height: var(--size-field-swatch-height);
  border-radius: var(--radius-field-swatch);
}

.terrain-row__text {
  display: flex;
  flex: 1;
  flex-direction: column;
  gap: var(--space-2);
  min-width: 0;
}

.terrain-row__name {
  display: flex;
  gap: var(--space-6);
  align-items: center;
  color: var(--color-text-primary);
}

.terrain-row__adjusted {
  padding: 1px var(--space-6);
  border-radius: var(--radius-pill);
  background: var(--color-coffee-100);
  color: var(--color-coffee-700);
  letter-spacing: 0.02em;
}

.terrain-row__detail {
  overflow: hidden;
  color: var(--color-text-secondary);
  text-overflow: ellipsis;
  white-space: nowrap;
}

.terrain-row__detail--failed {
  color: var(--color-status-unsuitable);
}
</style>
