<script setup lang="ts">
import type { TerrainPopoverView } from '~/utils/terrain'

/**
 * Popover of the selected field on the relief map (Figma "Popover · Talhão 4",
 * 68:1083): class chip, mean / max / relief tiles, the equipment warning and
 * "Ver relevo do talhão". MapView positions it next to the polygon.
 */
defineProps<{
  popover: TerrainPopoverView
}>()

const emit = defineEmits<{
  close: []
  'open-detail': []
}>()
</script>

<template>
  <MapPopover :title="popover.name" :subtitle="popover.subtitle" @close="emit('close')">
    <div class="terrain-popover__badge">
      <TerrainClassBadge :badge="popover.badge" />
    </div>

    <dl class="terrain-popover__metrics">
      <TerrainIndicatorTile :indicator="popover.slopeMean" size="sm" />
      <TerrainIndicatorTile :indicator="popover.slopeMax" size="sm" />
      <TerrainIndicatorTile :indicator="popover.relief" size="sm" />
    </dl>

    <p v-if="popover.methodWarning" class="terrain-popover__warning text-label-chip" role="status">
      {{ popover.methodWarning }}
    </p>

    <template v-if="popover.canOpenDetail" #actions>
      <BaseButton block @click="emit('open-detail')">Ver relevo do talhão</BaseButton>
    </template>
  </MapPopover>
</template>

<style scoped>
.terrain-popover__badge {
  display: flex;
}

.terrain-popover__metrics {
  display: flex;
  gap: var(--space-8);
  margin: 0;
}

.terrain-popover__warning {
  margin: 0;
  padding: var(--space-10) var(--space-12);
  border-radius: var(--radius-tile);
  background: var(--color-status-attention-bg);
  color: var(--color-coffee-700);
}
</style>
