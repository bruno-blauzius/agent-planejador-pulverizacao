<script setup lang="ts">
import type { DashboardChartView } from '~/utils/dashboard'

/**
 * "Janelas aptas × aplicações" (Figma 54:652): paired bars per day drawn with
 * CSS (no chart library). Each series has its own scale (ratios come ready
 * from the logic); every day has an accessible label with the real values.
 */
defineProps<{
  chart: DashboardChartView
}>()

const titleId = useId()
</script>

<template>
  <section class="windows-chart" :aria-labelledby="titleId">
    <header class="windows-chart__header">
      <h2 :id="titleId" class="windows-chart__title text-h4">{{ chart.title }}</h2>
      <ul class="windows-chart__legend" aria-label="Legenda">
        <li v-for="item in chart.legend" :key="item.key" class="windows-chart__legend-item text-label-badge">
          <span class="windows-chart__swatch" :class="`windows-chart__swatch--${item.key}`" aria-hidden="true" />
          {{ item.label }}
        </li>
      </ul>
    </header>

    <p v-if="chart.isEmpty" class="windows-chart__empty text-body-md">
      Sem janelas aptas nem aplicações no período.
    </p>
    <ol v-else class="windows-chart__plot">
      <li v-for="day in chart.days" :key="day.date" class="windows-chart__day">
        <span class="visually-hidden">{{ day.ariaLabel }}</span>
        <span class="windows-chart__bars" aria-hidden="true">
          <span class="windows-chart__bar windows-chart__bar--hours" :style="{ '--bar-ratio': day.hoursRatio }" />
          <span class="windows-chart__bar windows-chart__bar--applications" :style="{ '--bar-ratio': day.applicationsRatio }" />
        </span>
        <span class="windows-chart__label text-body-sm" aria-hidden="true">{{ day.showLabel ? day.label : '' }}</span>
      </li>
    </ol>

    <p v-if="chart.summary" class="windows-chart__summary text-body-sm">
      <AppIcon name="sparkles" :size="16" class="windows-chart__summary-icon" />
      {{ chart.summary }}
    </p>
  </section>
</template>

<style scoped>
.windows-chart {
  display: flex;
  flex-direction: column;
  gap: var(--space-16);
  min-width: 0;
  padding: var(--space-22);
  border: var(--border-width-hairline) solid var(--color-border);
  border-radius: var(--radius-xl);
  background: var(--color-white);
}

.windows-chart__header {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-12);
  align-items: center;
}

.windows-chart__title {
  flex: 1;
  min-width: 0;
  margin: 0;
  color: var(--color-text-primary);
}

.windows-chart__legend {
  display: flex;
  gap: var(--space-12);
  margin: 0;
  padding: 0;
  list-style: none;
}

.windows-chart__legend-item {
  display: inline-flex;
  gap: var(--space-6);
  align-items: center;
  color: var(--color-text-secondary);
}

.windows-chart__swatch {
  width: var(--size-chart-swatch);
  height: var(--size-chart-swatch);
  border-radius: var(--radius-chart-bar);
}

.windows-chart__swatch--hours,
.windows-chart__bar--hours {
  background: var(--color-sprout-500);
}

.windows-chart__swatch--applications,
.windows-chart__bar--applications {
  background: var(--color-green-800);
}

/* Three guide lines (top, middle, bottom) behind the bars, as in the Figma. */
.windows-chart__plot {
  display: flex;
  gap: var(--space-4);
  margin: 0;
  padding: 0 var(--space-24);
  list-style: none;
  background:
    linear-gradient(var(--color-border), var(--color-border)) 0 0 / 100% var(--border-width-hairline) no-repeat,
    linear-gradient(var(--color-border), var(--color-border)) 0 calc(var(--chart-plot-height) / 2) / 100% var(--border-width-hairline) no-repeat,
    linear-gradient(var(--color-border), var(--color-border)) 0 var(--chart-plot-height) / 100% var(--border-width-hairline) no-repeat;
}

.windows-chart__day {
  display: flex;
  flex: 1;
  flex-direction: column;
  gap: var(--space-8);
  align-items: center;
  min-width: 0;
}

.windows-chart__bars {
  display: flex;
  gap: var(--space-4);
  align-items: flex-end;
  height: var(--chart-plot-height);
}

.windows-chart__bar {
  width: var(--size-chart-bar);
  height: calc(var(--chart-plot-height) * var(--bar-ratio));
  border-radius: var(--radius-chart-bar) var(--radius-chart-bar) 0 0;
}

.windows-chart__label {
  min-height: var(--chart-label-height);
  color: var(--color-text-tertiary);
  white-space: nowrap;
}

.windows-chart__empty {
  margin: 0;
  padding: var(--space-24);
  color: var(--color-text-secondary);
  text-align: center;
}

.windows-chart__summary {
  display: flex;
  gap: var(--space-10);
  align-items: center;
  margin: 0;
  padding: var(--space-12);
  border-radius: var(--radius-md);
  background: var(--color-green-100);
  color: var(--color-green-800);
}

.windows-chart__summary-icon {
  flex-shrink: 0;
}

@media (max-width: 599px) {
  .windows-chart__plot {
    padding: 0;
  }

  .windows-chart__bars {
    gap: var(--space-2);
  }

  .windows-chart__bar {
    width: var(--size-chart-bar-sm);
  }
}
</style>
