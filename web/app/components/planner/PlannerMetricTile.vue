<script setup lang="ts">
import type { IconName } from '../app/AppIcon.vue'
import type { SprayCondition } from '~/types/spray'
import { SPRAY_CONDITION_LABELS } from '~/utils/labels'

/**
 * Condition tile of the mobile operator view (Figma 20:49): icon, status mark,
 * value and label. The status is computed by the API; the tile only shows it.
 */
const props = withDefaults(
  defineProps<{
    icon: IconName
    /** e.g. "4,2 °C". */
    value: string
    /** e.g. "Delta T". */
    label: string
    condition?: SprayCondition
  }>(),
  { condition: 'suitable' },
)

const STATUS_ICON: Record<SprayCondition, IconName> = {
  suitable: 'check',
  attention: 'alert',
  unsuitable: 'x',
  rain: 'rain',
}

const statusIcon = computed(() => STATUS_ICON[props.condition])
</script>

<template>
  <div class="metric-tile">
    <div class="metric-tile__top">
      <span class="metric-tile__icon" aria-hidden="true">
        <AppIcon :name="icon" :size="16" />
      </span>
      <span class="metric-tile__status" :class="`metric-tile__status--${condition}`">
        <span class="visually-hidden">{{ SPRAY_CONDITION_LABELS[condition].toLowerCase() }}</span>
        <AppIcon :name="statusIcon" :size="12" />
      </span>
    </div>
    <p class="metric-tile__value text-metric">{{ value }}</p>
    <p class="metric-tile__label text-caption-xs">{{ label }}</p>
  </div>
</template>

<style scoped>
.metric-tile {
  display: flex;
  flex: 1 1 0;
  flex-direction: column;
  gap: var(--space-6);
  min-width: 0;
  padding: var(--space-14);
  border: var(--border-width-hairline) solid var(--color-border);
  border-radius: var(--radius-card-lg);
  background: var(--color-white);
}

.metric-tile__top {
  display: flex;
  align-items: center;
  justify-content: space-between;
}

.metric-tile__icon {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: var(--size-planner-metric-icon);
  height: var(--size-planner-metric-icon);
  border-radius: var(--radius-tile);
  background: var(--color-green-100);
  color: var(--color-green-700);
}

.metric-tile__status {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: var(--size-planner-status-badge);
  height: var(--size-planner-status-badge);
  border-radius: var(--radius-pill);
  color: var(--color-text-inverse);
}

.metric-tile__status--suitable {
  background: var(--color-status-suitable);
}

.metric-tile__status--attention {
  background: var(--color-status-attention);
}

.metric-tile__status--unsuitable {
  background: var(--color-status-unsuitable);
}

.metric-tile__status--rain {
  background: var(--color-info-rain);
}

.metric-tile__value {
  margin: 0;
  color: var(--color-text-primary);
  white-space: nowrap;
}

.metric-tile__label {
  margin: 0;
  color: var(--color-text-tertiary);
  white-space: nowrap;
}
</style>
