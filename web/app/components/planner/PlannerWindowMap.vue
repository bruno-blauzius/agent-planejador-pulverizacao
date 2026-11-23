<script setup lang="ts">
import type { SprayCondition } from '~/types/spray'
import { SPRAY_CONDITION_LABELS } from '~/utils/labels'

/**
 * Window map (Figma 18:204): days × hours grid per field, colored by the
 * condition that the API computed for each slot. The component never derives a
 * condition; `selected` marks the slots of the plan the agent suggested.
 * Below `--planner-map-min-width` the grid scrolls horizontally instead of
 * squeezing the cells.
 */
export interface WindowMapCell {
  condition: SprayCondition
  selected?: boolean
}

export interface WindowMapDay {
  key: string
  /** e.g. "Qua 26". */
  label: string
  /** One label per slot, e.g. ["06", "09", "12", "15", "18"]. */
  hourLabels: readonly string[]
}

export interface WindowMapRow {
  key: string
  /** Field code, e.g. "T3". */
  label: string
  /** `cells[dayIndex][slotIndex]`, same order as `days` and their `hourLabels`. */
  cells: readonly (readonly WindowMapCell[])[]
}

defineProps<{
  title: string
  days: readonly WindowMapDay[]
  rows: readonly WindowMapRow[]
  /** Day whose hourly conditions are shown; makes the day headings pressable. */
  activeDayKey?: string | null
}>()

const emit = defineEmits<{
  'select-day': [dayKey: string]
}>()

const LEGEND: readonly SprayCondition[] = ['suitable', 'attention', 'unsuitable', 'rain']

const titleId = useId()

function cellLabel(row: WindowMapRow, day: WindowMapDay, slot: number, cell: WindowMapCell | undefined): string {
  const condition = cell ? SPRAY_CONDITION_LABELS[cell.condition] : 'sem dado'
  const suffix = cell?.selected ? ', no plano sugerido' : ''
  return `${row.label}, ${day.label}, ${day.hourLabels[slot]}h: ${condition}${suffix}`
}
</script>

<template>
  <section class="window-map" :aria-labelledby="titleId">
    <header class="window-map__header">
      <h2 :id="titleId" class="window-map__title text-h4">{{ title }}</h2>
      <ul class="window-map__legend" aria-label="Legenda">
        <li v-for="condition in LEGEND" :key="condition" class="window-map__legend-item text-label-tag">
          <span class="window-map__swatch" :class="`window-map__swatch--${condition}`" aria-hidden="true" />
          {{ SPRAY_CONDITION_LABELS[condition] }}
        </li>
      </ul>
    </header>

    <div class="window-map__scroller" tabindex="0" role="group" :aria-labelledby="titleId">
      <div class="window-map__grid" :style="{ '--day-count': days.length }">
        <div class="window-map__line window-map__line--head">
          <span class="window-map__label-spacer" aria-hidden="true" />
          <div v-for="day in days" :key="day.key" class="window-map__day">
            <button
              class="window-map__day-name window-map__day-button text-label-badge"
              type="button"
              :aria-pressed="activeDayKey === day.key"
              :aria-label="`Ver condições de ${day.label}`"
              @click="emit('select-day', day.key)"
            >
              {{ day.label }}
            </button>
            <span class="window-map__hours text-label-micro" aria-hidden="true">
              <span v-for="hour in day.hourLabels" :key="hour" class="window-map__hour">{{ hour }}</span>
            </span>
          </div>
        </div>

        <div v-for="row in rows" :key="row.key" class="window-map__line" role="group" :aria-label="`Talhão ${row.label}`">
          <span class="window-map__row-label text-label-field" aria-hidden="true">{{ row.label }}</span>
          <div v-for="(day, dayIndex) in days" :key="day.key" class="window-map__cells">
            <span
              v-for="(hour, slot) in day.hourLabels"
              :key="hour"
              class="window-map__cell"
              :class="[
                `window-map__cell--${row.cells[dayIndex]?.[slot]?.condition ?? 'none'}`,
                { 'window-map__cell--selected': row.cells[dayIndex]?.[slot]?.selected },
              ]"
              role="img"
              :aria-label="cellLabel(row, day, slot, row.cells[dayIndex]?.[slot])"
            >
              <AppIcon v-if="row.cells[dayIndex]?.[slot]?.selected" name="check" :size="14" />
            </span>
          </div>
        </div>
      </div>
    </div>
  </section>
</template>

<style scoped>
.window-map {
  display: flex;
  flex-direction: column;
  gap: var(--space-14);
  min-width: 0;
  padding: var(--space-22);
  border: var(--border-width-hairline) solid var(--color-border);
  border-radius: var(--radius-card-xl);
  background: var(--color-white);
}

.window-map__header {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-12);
  align-items: center;
}

.window-map__title {
  flex: 1 1 auto;
  margin: 0;
  color: var(--color-text-primary);
}

.window-map__legend {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-12);
  margin: 0;
  padding: 0;
  list-style: none;
}

.window-map__legend-item {
  display: inline-flex;
  gap: var(--space-6);
  align-items: center;
  color: var(--color-text-secondary);
}

.window-map__swatch {
  width: var(--size-legend-swatch);
  height: var(--size-legend-swatch);
  border-radius: var(--radius-legend-swatch);
}

.window-map__scroller {
  overflow-x: auto;
}

.window-map__scroller:focus-visible {
  outline: none;
  box-shadow: var(--focus-ring);
}

.window-map__grid {
  display: flex;
  flex-direction: column;
  gap: var(--space-14);
  min-width: var(--planner-map-min-width);
}

.window-map__line {
  display: grid;
  grid-template-columns: var(--size-planner-row-label) repeat(var(--day-count), minmax(0, 1fr));
  column-gap: var(--space-10);
  align-items: center;
}

.window-map__line--head {
  align-items: start;
}

.window-map__day-button {
  align-self: flex-start;
  padding: 0;
  border: 0;
  border-radius: var(--radius-xs);
  background: transparent;
  color: inherit;
  cursor: pointer;
}

.window-map__day-button[aria-pressed='true'] {
  text-decoration: underline;
  text-underline-offset: var(--space-4);
}

.window-map__day-button:focus-visible {
  outline: none;
  box-shadow: var(--focus-ring);
}

.window-map__day {
  display: flex;
  flex-direction: column;
  gap: var(--space-4);
  min-width: 0;
}

.window-map__day-name {
  color: var(--color-text-primary);
  white-space: nowrap;
}

.window-map__hours {
  display: flex;
  gap: var(--planner-map-cell-gap);
  font: var(--font-planner-axis);
  color: var(--color-text-tertiary);
  text-align: center;
}

.window-map__hour {
  flex: 1 1 0;
  min-width: 0;
}

.window-map__row-label {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  height: var(--size-planner-cell);
  border-radius: var(--radius-tile);
  background: var(--color-green-100);
  color: var(--color-green-800);
  font-weight: 700;
}

.window-map__cells {
  display: flex;
  gap: var(--planner-map-cell-gap);
  min-width: 0;
}

.window-map__cell {
  display: inline-flex;
  flex: 1 1 0;
  align-items: center;
  justify-content: center;
  min-width: 0;
  height: var(--size-planner-cell);
  border: var(--border-width-indicator) solid transparent;
  border-radius: var(--radius-sm);
  color: var(--color-text-inverse);
}

.window-map__cell--selected {
  border-color: var(--color-green-950);
}

.window-map__cell--none {
  background: var(--color-sand);
}

.window-map__cell--suitable,
.window-map__swatch--suitable {
  background: var(--color-status-suitable);
}

.window-map__cell--attention,
.window-map__swatch--attention {
  background: var(--color-status-attention);
}

.window-map__cell--unsuitable,
.window-map__swatch--unsuitable {
  background: var(--color-status-unsuitable);
}

.window-map__cell--rain,
.window-map__swatch--rain {
  background: var(--color-info-rain);
}
</style>
