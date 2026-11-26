<script setup lang="ts">
import type { TerrainIndicatorView } from '~/utils/terrain'

/**
 * Sand tile with label, value and an optional hint (Figma "Indicador", 68:991 and
 * 70:1084). `md` = value in the display font (panel, detail); `sm` = compact
 * value of the map popover (68:1092).
 */
withDefaults(
  defineProps<{
    indicator: TerrainIndicatorView
    size?: 'sm' | 'md' | 'lg'
  }>(),
  { size: 'md' },
)
</script>

<template>
  <div class="terrain-tile" :class="`terrain-tile--${size}`">
    <dt class="terrain-tile__label text-label-tag">{{ indicator.label }}</dt>
    <dd class="terrain-tile__value" :class="size === 'sm' ? 'text-label-meta-strong' : size === 'md' ? 'text-h4' : 'text-metric'">
      {{ indicator.value }}
    </dd>
    <dd v-if="indicator.hint" class="terrain-tile__hint text-caption-xs">{{ indicator.hint }}</dd>
  </div>
</template>

<style scoped>
.terrain-tile {
  display: flex;
  flex: 1 1 0;
  flex-direction: column;
  gap: var(--space-2);
  min-width: 0;
  padding: var(--space-12);
  border-radius: var(--radius-md);
  background: var(--color-sand);
}

.terrain-tile--md {
  /* Grows with long values ("702–1.253 m") instead of wrapping them. */
  flex-basis: auto;
}

.terrain-tile--md .terrain-tile__label,
.terrain-tile--md .terrain-tile__value {
  white-space: nowrap;
}

.terrain-tile--sm {
  padding: var(--space-10);
  border-radius: var(--radius-tile);
}

.terrain-tile__label,
.terrain-tile__hint {
  color: var(--color-text-tertiary);
}

.terrain-tile__value {
  margin: 0;
  color: var(--color-text-primary);
  overflow-wrap: anywhere;
}

.terrain-tile__hint {
  margin: 0;
}
</style>
