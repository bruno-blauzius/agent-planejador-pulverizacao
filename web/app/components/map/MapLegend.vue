<script setup lang="ts">
/**
 * Map legend (Figma 15:321): white box with a color swatch per category.
 * Colors come from theme/map-style.ts (fieldColor, STATUS_COLORS), so the
 * legend always matches the fills drawn on the map.
 */
export interface MapLegendItem {
  readonly key: string
  readonly label: string
  readonly color: string
}

defineProps<{
  items: readonly MapLegendItem[]
  /** Accessible name of the legend. */
  label?: string
}>()
</script>

<template>
  <ul v-if="items.length" class="map-legend" :aria-label="label ?? 'Legenda do mapa'">
    <li v-for="item in items" :key="item.key" class="map-legend__item text-label-chip">
      <span class="map-legend__swatch" :style="{ backgroundColor: item.color }" aria-hidden="true" />
      {{ item.label }}
    </li>
  </ul>
</template>

<style scoped>
.map-legend {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-6) var(--space-14);
  margin: 0;
  padding: var(--space-10) var(--space-14);
  border-radius: var(--radius-md);
  background: var(--color-white);
  list-style: none;
}

.map-legend__item {
  display: inline-flex;
  gap: var(--space-6);
  align-items: center;
  color: var(--color-text-secondary);
  white-space: nowrap;
}

.map-legend__swatch {
  flex-shrink: 0;
  width: var(--size-legend-swatch);
  height: var(--size-legend-swatch);
  border-radius: var(--radius-legend-swatch);
}
</style>
