<script setup lang="ts">
import type { DayConditionsView } from '~/utils/planner'

/**
 * Hourly conditions chart (Figma 18:335): Delta T (line), wind (bars), ideal
 * Delta T band and the suggested window. Drawn with CSS plus one SVG polyline
 * (no chart library). It only draws the 0–1 ratios, ticks and window of the
 * view model; Delta T (left axis) and wind (bars) each have their own scale.
 */
const props = defineProps<{
  view: DayConditionsView
  /** Text alternative of the chart (the numbers also go in a hidden table). */
  summary: string
}>()

const titleId = useId()

/** Ratios may drift slightly outside 0–1 (e.g. a value above the axis): keep them drawable. */
function percent(ratio: number): number {
  return Math.min(100, Math.max(0, ratio * 100))
}

function centerX(index: number): number {
  return ((index + 0.5) / props.view.points.length) * 100
}

const linePoints = computed(() =>
  props.view.points.map((point, index) => `${centerX(index)},${100 - percent(point.deltaTRatio)}`).join(' '),
)

const windowRange = computed(() => {
  const { window, points } = props.view
  if (!window || points.length === 0) return undefined
  const indexes = points.flatMap((point, index) =>
    point.hour >= window.startHour && point.hour < window.endHour ? [index] : [],
  )
  const first = indexes[0]
  const last = indexes[indexes.length - 1]
  if (first === undefined || last === undefined) return undefined
  return { first, last, left: (first / points.length) * 100, width: ((last - first + 1) / points.length) * 100 }
})

const dots = computed(() => {
  const range = windowRange.value
  if (!range) return []
  return props.view.points
    .map((point, index) => ({ index, x: centerX(index), y: percent(point.deltaTRatio) }))
    .filter(({ index }) => index >= range.first && index <= range.last)
})

const idealBand = computed(() => {
  const { minRatio, maxRatio } = props.view.idealRange.deltaT
  return { bottom: `${percent(minRatio)}%`, height: `${percent(maxRatio) - percent(minRatio)}%` }
})

const windTicks = computed(() =>
  props.view.windAxis.ticks.map(value => ({ value, bottom: `${percent(value / props.view.windAxis.max)}%` })),
)

const ticks = computed(() =>
  props.view.deltaTAxis.ticks.map(value => ({ value, bottom: `${percent(value / props.view.deltaTAxis.max)}%` })),
)
</script>

<template>
  <section v-if="!view.isEmpty" class="conditions-chart" :aria-labelledby="titleId">
    <header class="conditions-chart__header">
      <h2 :id="titleId" class="conditions-chart__title text-h4">{{ view.title }}</h2>
      <ul class="conditions-chart__legend" aria-label="Legenda">
        <li class="conditions-chart__legend-item text-label-tag">
          <span class="conditions-chart__key conditions-chart__key--delta-t" aria-hidden="true" />
          Delta T (°C)
        </li>
        <li class="conditions-chart__legend-item text-label-tag">
          <span class="conditions-chart__key conditions-chart__key--wind" aria-hidden="true" />
          Vento (km/h)
        </li>
        <li class="conditions-chart__legend-item text-label-tag">
          <span class="conditions-chart__key conditions-chart__key--ideal" aria-hidden="true" />
          Faixa ideal
        </li>
      </ul>
    </header>

    <div class="conditions-chart__figure" role="img" :aria-label="summary">
      <div class="conditions-chart__y-axis" aria-hidden="true">
        <span
          v-for="tick in ticks"
          :key="tick.value"
          class="conditions-chart__y-label"
          :style="{ bottom: tick.bottom }"
        >{{ tick.value }}</span>
      </div>

      <div class="conditions-chart__plot" aria-hidden="true">
        <span
          v-for="tick in ticks"
          :key="tick.value"
          class="conditions-chart__grid-line"
          :style="{ bottom: tick.bottom }"
        />
        <span class="conditions-chart__ideal" :style="idealBand" />
        <span
          v-if="windowRange && view.window"
          class="conditions-chart__window"
          :style="{ left: `${windowRange.left}%`, width: `${windowRange.width}%` }"
        >
          <span class="conditions-chart__window-label text-label-badge">{{ view.window?.label }}</span>
        </span>

        <span class="conditions-chart__bars">
          <span v-for="point in view.points" :key="point.hour" class="conditions-chart__slot">
            <span class="conditions-chart__bar" :style="{ height: `${percent(point.windRatio)}%` }" />
          </span>
        </span>

        <svg class="conditions-chart__line" viewBox="0 0 100 100" preserveAspectRatio="none" focusable="false">
          <polyline :points="linePoints" />
        </svg>

        <span
          v-for="dot in dots"
          :key="dot.index"
          class="conditions-chart__dot"
          :style="{ left: `${dot.x}%`, bottom: `${dot.y}%` }"
        />
      </div>

      <div class="conditions-chart__y-axis conditions-chart__y-axis--wind" aria-hidden="true">
        <span
          v-for="tick in windTicks"
          :key="tick.value"
          class="conditions-chart__y-label"
          :style="{ bottom: tick.bottom }"
        >{{ tick.value }}</span>
      </div>

      <div class="conditions-chart__x-axis" aria-hidden="true">
        <span
          v-for="point in view.points"
          :key="point.hour"
          class="conditions-chart__x-label"
        >{{ point.tickLabel }}</span>
      </div>
    </div>

    <table class="visually-hidden">
      <caption>{{ view.title }}</caption>
      <thead>
        <tr>
          <th scope="col">Hora</th>
          <th scope="col">Delta T (°C)</th>
          <th scope="col">Vento (km/h)</th>
        </tr>
      </thead>
      <tbody>
        <tr v-for="point in view.points" :key="point.hour">
          <th scope="row">{{ point.hour }}h</th>
          <td>{{ point.deltaT }}</td>
          <td>{{ point.windKmh }}</td>
        </tr>
      </tbody>
    </table>
  </section>
</template>

<style scoped>
.conditions-chart {
  display: flex;
  flex-direction: column;
  gap: var(--space-12);
  min-width: 0;
}

.conditions-chart__header {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-12);
  align-items: center;
}

.conditions-chart__title {
  flex: 1 1 auto;
  margin: 0;
  color: var(--color-text-primary);
}

.conditions-chart__legend {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-12);
  margin: 0;
  padding: 0;
  list-style: none;
}

.conditions-chart__legend-item {
  display: inline-flex;
  gap: var(--space-6);
  align-items: center;
  color: var(--color-text-secondary);
}

.conditions-chart__key {
  width: var(--size-planner-chart-legend-line);
  height: var(--size-planner-chart-legend-line-height);
  border-radius: var(--radius-chart-bar);
}

.conditions-chart__key--delta-t {
  background: var(--color-green-700);
}

.conditions-chart__key--wind {
  background: var(--color-coffee-600);
}

.conditions-chart__key--ideal {
  background: var(--color-sprout-400);
}

/* Axis column | plot, then the hour labels under the plot. */
.conditions-chart__figure {
  display: grid;
  grid-template-columns: var(--planner-chart-axis-width) minmax(0, 1fr) var(--planner-chart-axis-width);
  grid-template-rows: calc(var(--planner-chart-plot-top) + var(--planner-chart-plot-height)) var(--planner-chart-x-axis-height);
  font: var(--font-planner-axis);
  color: var(--color-text-tertiary);
}

.conditions-chart__y-axis {
  position: relative;
  margin-top: var(--planner-chart-plot-top);
}

.conditions-chart__y-axis--wind .conditions-chart__y-label {
  right: var(--space-6);
  left: auto;
  color: var(--color-coffee-600);
}

.conditions-chart__y-label {
  position: absolute;
  left: var(--space-6);
  transform: translateY(50%);
}

.conditions-chart__plot {
  position: relative;
  margin-top: var(--planner-chart-plot-top);
}

.conditions-chart__grid-line {
  position: absolute;
  right: 0;
  left: 0;
  height: var(--border-width-hairline);
  background: var(--color-border);
}

.conditions-chart__ideal {
  position: absolute;
  right: 0;
  left: 0;
  background: var(--color-sprout-soft);
}

.conditions-chart__window {
  position: absolute;
  top: 0;
  bottom: 0;
  background: var(--color-sprout-tint);
}

.conditions-chart__window-label {
  position: absolute;
  top: var(--space-6);
  left: var(--space-6);
  padding: var(--space-4) var(--space-8);
  border-radius: var(--radius-sm);
  background: var(--color-green-900);
  color: var(--color-sprout-400);
  white-space: nowrap;
}

.conditions-chart__bars {
  position: absolute;
  inset: 0;
  display: flex;
}

.conditions-chart__slot {
  display: flex;
  flex: 1 1 0;
  align-items: flex-end;
  justify-content: center;
  min-width: 0;
}

.conditions-chart__bar {
  width: min(var(--planner-chart-bar-width), 60%);
  border-radius: var(--radius-chart-bar) var(--radius-chart-bar) 0 0;
  background: var(--color-planner-wind-bar);
}

.conditions-chart__line {
  position: absolute;
  inset: 0;
  width: 100%;
  height: 100%;
  overflow: visible;
  fill: none;
}

.conditions-chart__line polyline {
  stroke: var(--color-green-700);
  stroke-width: var(--planner-chart-line-width);
  stroke-linecap: round;
  stroke-linejoin: round;
  vector-effect: non-scaling-stroke;
}

.conditions-chart__dot {
  position: absolute;
  width: var(--size-planner-chart-dot);
  height: var(--size-planner-chart-dot);
  border: var(--border-width-strong) solid var(--color-green-700);
  border-radius: var(--radius-pill);
  background: var(--color-white);
  box-sizing: content-box;
  transform: translate(-50%, 50%);
}

.conditions-chart__x-axis {
  grid-column: 2;
  display: flex;
  align-items: flex-end;
}

.conditions-chart__x-label {
  flex: 1 1 0;
  min-width: 0;
  text-align: center;
  white-space: nowrap;
}
</style>
