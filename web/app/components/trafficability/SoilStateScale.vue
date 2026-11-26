<script setup lang="ts">
import type { SoilState } from '~/types/trafficability'
import { SOIL_STATE_LABELS } from '~/utils/trafficabilityLabels'

/**
 * Soil moisture scale (Figma "Escala", 84:1558): the four bands from dry to
 * waterlogged with the current one highlighted. `unknown` highlights none and
 * says so. The state comes from the API; this never derives it (ADR 0022).
 */
const props = defineProps<{
  state: SoilState
}>()

type KnownSoilState = Exclude<SoilState, 'unknown'>

const BANDS: readonly KnownSoilState[] = ['dry', 'ideal', 'moist', 'waterlogged']

const isUnknown = computed(() => props.state === 'unknown')
</script>

<template>
  <div class="soil-scale">
    <ol class="soil-scale__bands" aria-label="Escala de umidade do solo">
      <li
        v-for="band in BANDS"
        :key="band"
        class="soil-scale__band text-label-micro"
        :class="[`soil-scale__band--${band}`, { 'soil-scale__band--current': band === state }]"
        :aria-current="band === state ? 'true' : undefined"
      >
        {{ SOIL_STATE_LABELS[band] }}
      </li>
    </ol>
    <p v-if="isUnknown" class="soil-scale__unknown text-caption">
      Estado atual: <strong>{{ SOIL_STATE_LABELS.unknown }}</strong>
    </p>
  </div>
</template>

<style scoped>
.soil-scale {
  display: flex;
  flex-direction: column;
  gap: var(--space-6);
}

.soil-scale__bands {
  display: flex;
  gap: var(--space-2);
  margin: 0;
  padding: 0;
  list-style: none;
}

.soil-scale__band {
  --soil-band-bg: var(--color-sand);
  --soil-band-accent: var(--color-text-secondary);

  flex: 1 1 0;
  min-width: 0;
  padding: var(--space-6) 0;
  border: var(--border-width-strong) solid transparent;
  border-radius: var(--radius-xs);
  background: var(--soil-band-bg);
  color: var(--color-text-secondary);
  font-weight: 500;
  text-align: center;
  overflow-wrap: anywhere;
}

.soil-scale__band--dry {
  --soil-band-bg: var(--color-coffee-100);
  --soil-band-accent: var(--color-coffee-700);
}

.soil-scale__band--ideal {
  --soil-band-bg: var(--color-status-suitable-bg);
  --soil-band-accent: var(--color-status-suitable);
}

.soil-scale__band--moist {
  --soil-band-bg: var(--color-status-attention-bg);
  --soil-band-accent: var(--color-status-attention);
}

.soil-scale__band--waterlogged {
  --soil-band-bg: var(--color-info-rain-bg);
  --soil-band-accent: var(--color-info-rain);
}

.soil-scale__band--current {
  border-color: var(--soil-band-accent);
  color: var(--soil-band-accent);
  font-weight: 700;
}

.soil-scale__unknown {
  margin: 0;
  color: var(--color-text-secondary);
}
</style>
