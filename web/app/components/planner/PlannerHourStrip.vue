<script setup lang="ts">
import type { SprayCondition } from '~/types/spray'
import { SPRAY_CONDITION_LABELS } from '~/utils/labels'

/**
 * "Hoje, hora a hora" strip of the mobile operator view (Figma 20:81): one
 * colored cell per hour, the current hour outlined. Conditions come ready.
 */
export interface HourStripItem {
  /** Hour label, e.g. "06". */
  label: string
  condition: SprayCondition
  current?: boolean
}

defineProps<{
  title: string
  hours: readonly HourStripItem[]
}>()

const titleId = useId()
</script>

<template>
  <section class="hour-strip" :aria-labelledby="titleId">
    <h2 :id="titleId" class="hour-strip__title text-label-strong">{{ title }}</h2>
    <ol class="hour-strip__list">
      <li v-for="hour in hours" :key="hour.label" class="hour-strip__item">
        <span
          class="hour-strip__cell"
          :class="[`hour-strip__cell--${hour.condition}`, { 'hour-strip__cell--current': hour.current }]"
        />
        <span class="hour-strip__label" :class="{ 'hour-strip__label--current': hour.current }">
          {{ hour.label }}
          <span class="visually-hidden">h: {{ SPRAY_CONDITION_LABELS[hour.condition].toLowerCase() }}{{ hour.current ? ', agora' : '' }}</span>
        </span>
      </li>
    </ol>
  </section>
</template>

<style scoped>
.hour-strip {
  display: flex;
  flex-direction: column;
  gap: var(--space-10);
  padding: var(--space-16);
  border: var(--border-width-hairline) solid var(--color-border);
  border-radius: var(--radius-card-lg);
  background: var(--color-white);
}

.hour-strip__title {
  margin: 0;
  color: var(--color-text-primary);
}

.hour-strip__list {
  display: flex;
  gap: var(--space-4);
  margin: 0;
  padding: 0;
  list-style: none;
}

.hour-strip__item {
  display: flex;
  flex: 1 1 0;
  flex-direction: column;
  gap: var(--space-4);
  align-items: center;
  min-width: 0;
}

.hour-strip__cell {
  box-sizing: border-box;
  width: 100%;
  height: var(--size-planner-hour-cell);
  border-radius: var(--radius-planner-hour);
}

.hour-strip__cell--suitable {
  background: var(--color-status-suitable);
}

.hour-strip__cell--attention {
  background: var(--color-status-attention);
}

.hour-strip__cell--unsuitable {
  background: var(--color-status-unsuitable);
}

.hour-strip__cell--rain {
  background: var(--color-info-rain);
}

.hour-strip__cell--current {
  border: var(--planner-hour-selected-border) solid var(--color-green-950);
}

.hour-strip__label {
  font: var(--font-planner-hour);
  color: var(--color-text-tertiary);
  white-space: nowrap;
}

.hour-strip__label--current {
  font: var(--font-planner-hour-strong);
  color: var(--color-text-primary);
}
</style>
