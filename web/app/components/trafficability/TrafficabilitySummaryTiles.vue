<script setup lang="ts">
import type { TerrainIndicatorView } from '~/utils/terrain'
import type { SoilStatus } from '~/types/trafficability'
import { SOIL_BASIS_LABELS, SOIL_STATE_LABELS } from '~/utils/trafficabilityLabels'

/**
 * Indicators of the "Chuva e solo" view (Figma "Indicadores", 84:1304): rain of
 * the last 72 h and 7 days and today's soil state, on the relief indicator tile.
 * Accumulations and state come ready from the API.
 */
const props = defineProps<{
  soil: SoilStatus
}>()

const mmFormat = new Intl.NumberFormat('pt-BR', { maximumFractionDigits: 1 })
const percentFormat = new Intl.NumberFormat('pt-BR', { maximumFractionDigits: 0 })

function mm(value: number): string {
  return `${mmFormat.format(value)} mm`
}

const rainTiles = computed<readonly TerrainIndicatorView[]>(() => {
  const rain = props.soil.rain
  if (!rain) return []
  return [
    { label: 'Chuva nas últimas 72 h', value: mm(rain.last72hMm), hint: `24 h: ${mm(rain.last24hMm)} · 48 h: ${mm(rain.last48hMm)}` },
    { label: 'Últimos 7 dias', value: mm(rain.last7dMm), hint: null },
  ]
})

const stateHint = computed(() => {
  const moisture = props.soil.modelMoisture
  if (moisture) return `${percentFormat.format(moisture.fieldCapacityPct)} % da cap. de campo`
  return SOIL_BASIS_LABELS[props.soil.basis]
})
</script>

<template>
  <dl class="traffic-tiles">
    <TerrainIndicatorTile v-for="tile in rainTiles" :key="tile.label" :indicator="tile" size="lg" />
    <div class="traffic-tiles__state" :class="`traffic-tiles__state--${soil.state}`">
      <dt class="text-label-tag">Estado do solo</dt>
      <dd class="traffic-tiles__state-value text-metric">{{ SOIL_STATE_LABELS[soil.state] }}</dd>
      <dd class="traffic-tiles__state-hint text-caption-xs">{{ stateHint }}</dd>
    </div>
  </dl>
</template>

<style scoped>
.traffic-tiles {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(150px, 1fr));
  gap: var(--space-10);
  margin: 0;
}

.traffic-tiles__state {
  --soil-tile-bg: var(--color-sand);
  --soil-tile-accent: var(--color-text-primary);

  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  min-width: 0;
  padding: var(--space-12);
  border-radius: var(--radius-md);
  background: var(--soil-tile-bg);
  color: var(--color-text-tertiary);
}

.traffic-tiles__state--dry {
  --soil-tile-bg: var(--color-coffee-100);
  --soil-tile-accent: var(--color-coffee-700);
}

.traffic-tiles__state--ideal {
  --soil-tile-bg: var(--color-status-suitable-bg);
  --soil-tile-accent: var(--color-status-suitable);
}

.traffic-tiles__state--moist {
  --soil-tile-bg: var(--color-status-attention-bg);
  --soil-tile-accent: var(--color-status-attention);
}

.traffic-tiles__state--waterlogged {
  --soil-tile-bg: var(--color-info-rain-bg);
  --soil-tile-accent: var(--color-info-rain);
}

.traffic-tiles__state dd {
  margin: 0;
}

.traffic-tiles__state-value {
  color: var(--soil-tile-accent);
}
</style>
