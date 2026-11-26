<script setup lang="ts">
import type { MethodRelease, SoilStatus } from '~/types/trafficability'
import {
  SOIL_BASIS_LABELS,
  SOIL_STATE_LABELS,
  SOIL_TEXTURE_LABELS,
  sprayMethodLabel,
  THRESHOLDS_SOURCE_LABELS,
} from '~/utils/trafficabilityLabels'
import { DISPLAY_TIME_ZONE } from '~/utils/timeZone'

/**
 * Facts behind today's soil state (Figma "Umidade do solo e tráfego", 84:1556):
 * texture, accumulated rain, model moisture, where the limits come from, a
 * still-valid inspection and the estimated release of each ground method.
 * Values only formatted here; states and release days come from the API.
 */
const props = withDefaults(defineProps<{
  soil: SoilStatus
  releases: readonly MethodRelease[]
  /** Projection days asked for, for "sem liberação prevista nos próximos N dias". */
  horizonDays: number
  /** `weather_unavailable`: without a forecast there is no horizon to estimate a release. */
  forecastUnavailable?: boolean
}>(), { forecastUnavailable: false })

interface Fact {
  readonly label: string
  readonly value: string
}

const mmFormat = new Intl.NumberFormat('pt-BR', { maximumFractionDigits: 1 })
const moistureFormat = new Intl.NumberFormat('pt-BR', { minimumFractionDigits: 2, maximumFractionDigits: 2 })
const percentFormat = new Intl.NumberFormat('pt-BR', { maximumFractionDigits: 0 })
const weekdayFormat = new Intl.DateTimeFormat('pt-BR', { weekday: 'short', timeZone: 'UTC' })
const instantFormat = new Intl.DateTimeFormat('pt-BR', {
  day: '2-digit',
  month: '2-digit',
  hour: '2-digit',
  minute: '2-digit',
  timeZone: DISPLAY_TIME_ZONE,
})

const titleId = useId()

function mm(value: number): string {
  return `${mmFormat.format(value)} mm`
}

/** "2026-11-29" → "sáb 29/11", without shifting the calendar day to the browser zone. */
function calendarDayLabel(day: string): string {
  const [year, month, date] = day.split('-').map(Number)
  if (!year || !month || !date) return day
  const weekday = weekdayFormat.format(new Date(Date.UTC(year, month - 1, date))).replace('.', '')
  return `${weekday} ${String(date).padStart(2, '0')}/${String(month).padStart(2, '0')}`
}

const facts = computed<readonly Fact[]>(() => {
  const { soil } = props
  const rows: Fact[] = [
    { label: 'Textura', value: soil.texture ? SOIL_TEXTURE_LABELS[soil.texture] : 'Não cadastrada' },
  ]
  if (soil.rain) {
    rows.push(
      { label: 'Chuva 24 h', value: mm(soil.rain.last24hMm) },
      { label: 'Chuva 48 h', value: mm(soil.rain.last48hMm) },
      { label: 'Chuva 72 h', value: mm(soil.rain.last72hMm) },
      { label: 'Chuva 7 dias', value: mm(soil.rain.last7dMm) },
    )
  }
  if (soil.modelMoisture) {
    rows.push(
      { label: 'Umidade 0–27 cm', value: `${moistureFormat.format(soil.modelMoisture.volumetricM3m3)} m³/m³ (modelo)` },
      {
        label: 'Capacidade de campo',
        value: `${moistureFormat.format(soil.modelMoisture.fieldCapacityM3m3)} m³/m³ · ${percentFormat.format(soil.modelMoisture.fieldCapacityPct)} %`,
      },
    )
  }
  rows.push({ label: 'Estado definido por', value: SOIL_BASIS_LABELS[soil.basis] })
  // Heuristic limits get a discreet note below the list instead of a row.
  if (soil.thresholdsSource !== 'heuristic') {
    rows.push({ label: 'Limites', value: THRESHOLDS_SOURCE_LABELS[soil.thresholdsSource] })
  }
  if (soil.inspection) {
    const { inspection } = soil
    rows.push({
      label: 'Vistoria válida',
      value: `${SOIL_STATE_LABELS[inspection.state]} · ${inspection.inspectedBy.name}, até ${instantFormat.format(inspection.validUntil)}`,
    })
  }
  return rows
})

const isHeuristic = computed(() => props.soil.thresholdsSource === 'heuristic')

const releaseRows = computed<readonly Fact[]>(() =>
  props.releases.map(release => ({
    label: sprayMethodLabel(release.method),
    value: release.date
      ? `${calendarDayLabel(release.date)} (solo ${SOIL_STATE_LABELS[release.requiredState].toLowerCase()})`
      : props.forecastUnavailable
        ? 'sem previsão para estimar a liberação'
        : `sem liberação prevista nos próximos ${props.horizonDays} dias`,
  })),
)
</script>

<template>
  <div class="soil-facts">
    <dl class="soil-facts__list text-caption">
      <div v-for="fact in facts" :key="fact.label" class="soil-facts__row">
        <dt>{{ fact.label }}</dt>
        <dd>{{ fact.value }}</dd>
      </div>
    </dl>
    <p v-if="isHeuristic" class="soil-facts__note text-caption-xs">Limites iniciais, ainda não calibrados.</p>

    <section v-if="releaseRows.length" class="soil-facts__releases" :aria-labelledby="titleId">
      <h4 :id="titleId" class="soil-facts__subtitle text-label-tag">Libera, se não chover</h4>
      <dl class="soil-facts__list text-caption">
        <div v-for="row in releaseRows" :key="row.label" class="soil-facts__row">
          <dt>{{ row.label }}</dt>
          <dd>{{ row.value }}</dd>
        </div>
      </dl>
    </section>
  </div>
</template>

<style scoped>
.soil-facts {
  display: flex;
  flex-direction: column;
  gap: var(--space-12);
}

.soil-facts__list {
  display: flex;
  flex-direction: column;
  gap: var(--space-10);
  margin: 0;
}

.soil-facts__row {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-2) var(--space-8);
}

.soil-facts__row dt {
  flex: 1 1 auto;
  color: var(--color-text-secondary);
  font-weight: 500;
}

.soil-facts__row dd {
  margin: 0;
  color: var(--color-text-primary);
  font-weight: 700;
  text-align: right;
}

.soil-facts__note {
  margin: calc(-1 * var(--space-4)) 0 0;
  color: var(--color-text-tertiary);
}

.soil-facts__releases {
  display: flex;
  flex-direction: column;
  gap: var(--space-8);
  padding-top: var(--space-10);
  border-top: var(--border-width-hairline) solid var(--color-coffee-100);
}

.soil-facts__subtitle {
  margin: 0;
  color: var(--color-text-tertiary);
}
</style>
