<script setup lang="ts">
import type { CalendarDay, DailyRainEntry, RainAccumulation } from '~/types/trafficability'
import { measuredRainCaption, RAIN_SOURCE_LABELS } from '~/utils/trafficabilityLabels'

/**
 * Daily rain of the field (Figma "Chuva no talhão", 84:1495): measured days as
 * solid bars, forecast days dashed and light, today highlighted. Bar heights are
 * only scaled to the tallest day of the series (drawing, not a rule); every
 * value comes from the API (ADR 0022).
 */
const props = withDefaults(defineProps<{
  entries: readonly DailyRainEntry[]
  /** Today's calendar day (first day of `days`); null leaves no day highlighted. */
  today?: CalendarDay | null
  /** Accumulation of the measured rain: source and time of the reading for the caption. */
  rain?: RainAccumulation | null
}>(), { today: null, rain: null })

const mmFormat = new Intl.NumberFormat('pt-BR', { maximumFractionDigits: 1 })

const titleId = useId()

const maxMm = computed(() => Math.max(0, ...props.entries.map(entry => entry.rainMm)))

const bars = computed(() => props.entries.map((entry) => {
  const isToday = entry.date === props.today
  const [, month, day] = entry.date.split('-')
  const kindLabel = entry.kind === 'measured' ? 'medida' : 'prevista'
  return {
    date: entry.date,
    kind: entry.kind,
    isToday,
    dayLabel: isToday ? 'hoje' : (day ?? entry.date),
    valueLabel: entry.rainMm > 0 ? mmFormat.format(entry.rainMm) : '·',
    ratio: maxMm.value > 0 ? entry.rainMm / maxMm.value : 0,
    accessibleLabel: `${isToday ? 'Hoje' : `${day}/${month}`}: ${mmFormat.format(entry.rainMm)} mm, ${kindLabel}`,
  }
}))

const measuredCaption = computed(() => (props.rain ? measuredRainCaption(props.rain) : null))

/** "Previsão: modelo (não é o talhão)", from the source of the forecast entries themselves. */
const forecastCaption = computed(() => {
  const sources = [...new Set(props.entries.filter(entry => entry.kind === 'forecast').map(entry => entry.source))]
  return sources.length ? `Previsão: ${sources.map(source => RAIN_SOURCE_LABELS[source]).join(', ')}` : null
})
</script>

<template>
  <section class="rain-chart" :aria-labelledby="titleId">
    <header class="rain-chart__header">
      <h3 :id="titleId" class="rain-chart__title text-label-meta-strong">Chuva no talhão (mm/dia)</h3>
      <ul class="rain-chart__legend text-label-tag" aria-hidden="true">
        <li class="rain-chart__legend-item">
          <span class="rain-chart__swatch rain-chart__swatch--measured" />Medida
        </li>
        <li class="rain-chart__legend-item">
          <span class="rain-chart__swatch rain-chart__swatch--forecast" />Previsão
        </li>
      </ul>
    </header>

    <p v-if="!bars.length" class="rain-chart__empty text-caption">Sem dados de chuva para este talhão.</p>

    <ol v-else class="rain-chart__bars" aria-label="Chuva por dia">
      <li
        v-for="bar in bars"
        :key="bar.date"
        class="rain-chart__day"
        :class="[`rain-chart__day--${bar.kind}`, { 'rain-chart__day--today': bar.isToday }]"
        :aria-current="bar.isToday ? 'date' : undefined"
      >
        <span class="visually-hidden">{{ bar.accessibleLabel }}</span>
        <span class="rain-chart__value" aria-hidden="true">{{ bar.valueLabel }}</span>
        <span class="rain-chart__bar" :style="{ '--rain-bar-ratio': bar.ratio }" aria-hidden="true" />
        <span class="rain-chart__label" aria-hidden="true">{{ bar.dayLabel }}</span>
      </li>
    </ol>

    <p v-if="measuredCaption || forecastCaption" class="rain-chart__caption text-caption-xs">
      <span v-if="measuredCaption">{{ measuredCaption }}</span>
      <span v-if="measuredCaption && forecastCaption" aria-hidden="true"> · </span>
      <span v-if="forecastCaption">{{ forecastCaption }}</span>
    </p>
  </section>
</template>

<style scoped>
.rain-chart {
  container-type: inline-size;
  display: flex;
  flex-direction: column;
  gap: var(--space-10);
  min-width: 0;
}

.rain-chart__header {
  display: flex;
  flex-direction: column;
  gap: var(--space-6);
}

.rain-chart__title {
  margin: 0;
  color: var(--color-text-primary);
}

.rain-chart__legend {
  display: flex;
  gap: var(--space-10);
  margin: 0;
  padding: 0;
  color: var(--color-text-secondary);
  list-style: none;
}

.rain-chart__legend-item {
  display: inline-flex;
  gap: var(--space-4);
  align-items: center;
}

.rain-chart__swatch {
  width: var(--size-status-dot);
  height: var(--size-status-dot);
  border-radius: var(--radius-legend-swatch);
}

.rain-chart__swatch--measured {
  background: var(--color-info-rain);
}

.rain-chart__swatch--forecast,
.rain-chart__day--forecast .rain-chart__bar {
  border: var(--border-width-hairline) dashed var(--color-info-rain);
  background: color-mix(in srgb, var(--color-info-rain) 30%, transparent);
}

.rain-chart__bars {
  display: flex;
  gap: var(--space-6);
  align-items: stretch;
  /* Plot + value and day labels (Figma 84:1506 is 150 px tall). */
  height: calc(var(--chart-plot-height) + 2 * var(--chart-label-height) + 2 * var(--space-2));
  margin: 0;
  padding: 0;
  list-style: none;
}

.rain-chart__day {
  display: flex;
  flex: 1 1 0;
  flex-direction: column;
  gap: var(--space-2);
  align-items: center;
  justify-content: flex-end;
  min-width: 0;
  border-radius: var(--radius-xs);
}

.rain-chart__day--today {
  background: var(--color-sand);
}

.rain-chart__value,
.rain-chart__label {
  font: var(--font-label-micro);
  font-weight: 600;
  white-space: nowrap;
}

.rain-chart__value {
  color: var(--color-text-secondary);
}

.rain-chart__day--forecast .rain-chart__value,
.rain-chart__label {
  color: var(--color-text-tertiary);
}

.rain-chart__day--today .rain-chart__label {
  color: var(--color-text-primary);
  font-weight: 700;
}

/* Scaled to the tallest day of the series; a dry day keeps a 2 px stub. */
.rain-chart__bar {
  flex-shrink: 0;
  width: min(var(--size-chart-bar), 100%);
  height: max(var(--space-2), calc(var(--chart-plot-height) * var(--rain-bar-ratio)));
  border-radius: var(--radius-chart-bar) var(--radius-chart-bar) 0 0;
  background: var(--color-info-rain);
}

/* Narrow cards (phones): tighter columns and no value labels, which would overlap; the value stays in the accessible text. */
@container (max-width: 360px) {
  .rain-chart__bars {
    gap: var(--space-2);
  }

  .rain-chart__value {
    visibility: hidden;
  }
}

.rain-chart__empty,
.rain-chart__caption {
  margin: 0;
  color: var(--color-text-tertiary);
}
</style>
