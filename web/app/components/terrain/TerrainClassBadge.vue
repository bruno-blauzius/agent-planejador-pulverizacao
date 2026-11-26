<script setup lang="ts">
import { TERRAIN_CLASS_TONES } from '~/theme/map-style'
import type { SlopeClassBadgeView } from '~/utils/terrain'

/**
 * Relief class chip (Figma "Relevo · Selo de classe", 67:2239): dot and label in the
 * tone of the class. The class comes ready from the API; this only presents it.
 */
const props = defineProps<{
  badge: SlopeClassBadgeView
}>()

// Data-driven colors: the class is only known at runtime.
const toneStyle = computed(() => {
  const tone = TERRAIN_CLASS_TONES[props.badge.key]
  return { backgroundColor: tone.badgeBackground, color: tone.badgeText, '--terrain-badge-dot': tone.bar }
})
</script>

<template>
  <span class="terrain-badge text-label-badge" :style="toneStyle">
    <span class="terrain-badge__dot" aria-hidden="true" />
    <span class="visually-hidden">Classe de relevo: </span>{{ badge.label }}
  </span>
</template>

<style scoped>
.terrain-badge {
  display: inline-flex;
  flex-shrink: 0;
  gap: var(--space-6);
  align-items: center;
  padding: var(--space-4) var(--space-10);
  border-radius: var(--radius-pill);
  white-space: nowrap;
}

.terrain-badge__dot {
  flex-shrink: 0;
  width: 7px;
  height: 7px;
  border-radius: var(--radius-pill);
  background: var(--terrain-badge-dot);
}
</style>
