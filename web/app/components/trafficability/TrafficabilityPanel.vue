<script setup lang="ts">
import type { FieldTrafficability, SoilProfile, TrafficabilityDataIssue } from '~/types/trafficability'
import { TRAFFICABILITY_DATA_ISSUE_LABELS, TRAFFICABILITY_DATA_ISSUE_TITLES } from '~/utils/trafficabilityLabels'
// Explicit imports: auto-import would prefix these with the folder name (TrafficabilityRainDailyChart…).
import AgentPlannerNote from './AgentPlannerNote.vue'
import MethodEligibilityTable from './MethodEligibilityTable.vue'
import RainDailyChart from './RainDailyChart.vue'
import SoilFactsList from './SoilFactsList.vue'
import SoilProfileSummary from './SoilProfileSummary.vue'
import SoilStateScale from './SoilStateScale.vue'

/**
 * View "Chuva e solo" of a field in the Terreno tab (Figma 04f, 84:1296; ADR
 * 0022, decision 10). Presentation only: the data and its loading state come
 * from the tab (`useFarmTerrainTab`); state, verdicts and release days are the
 * API's. A failure shows an honest message and no numbers.
 */
const props = withDefaults(defineProps<{
  /** Null before the first load or after a failure. */
  trafficability: FieldTrafficability | null
  pending?: boolean
  /** Reloading with the previous answer on screen. */
  refreshing?: boolean
  /** Message of a failed load (shown with "Tentar novamente"). */
  error?: string | null
  /** Projection days asked for ("próximos N dias"). */
  horizonDays: number
  /** Texture and linked gauge of the field (phase 2). */
  soilProfile?: SoilProfile | null
  soilProfilePending?: boolean
  /** `registry.write`: edit the soil profile (also from the data gap alerts). */
  canEditProfile?: boolean
}>(), { pending: false, refreshing: false, error: null, soilProfile: null, soilProfilePending: false, canEditProfile: false })

const emit = defineEmits<{
  retry: []
  editProfile: []
}>()

/** Data gaps the soil profile fixes: their alert offers to edit it. */
const PROFILE_ISSUES: ReadonlySet<TrafficabilityDataIssue> = new Set(['no_soil_texture', 'no_rain_source'])

function issueOffersProfile(issue: TrafficabilityDataIssue): boolean {
  return props.canEditProfile && PROFILE_ISSUES.has(issue)
}

const soilTitleId = useId()

const today = computed(() => props.trafficability?.days[0]?.date ?? null)
/**
 * Skeleton also while nothing arrived yet and nothing failed: on a direct link the
 * tab may not have started the request (fields still loading), and an empty card
 * would read as "no data".
 */
const isLoading = computed(() => props.pending || (props.trafficability === null && props.error === null))
const forecastUnavailable = computed(() => props.trafficability?.dataIssues.includes('weather_unavailable') ?? false)
</script>

<template>
  <div class="traffic-panel" :aria-busy="isLoading || refreshing || undefined">
    <ToastAlert
      v-if="error"
      tone="error"
      title="Não foi possível carregar a chuva e o solo do talhão"
      :message="error"
      :dismissible="false"
    >
      <template #actions>
        <BaseButton variant="secondary" size="sm" icon="undo" @click="emit('retry')">Tentar novamente</BaseButton>
      </template>
    </ToastAlert>

    <div v-else-if="isLoading" class="traffic-panel__skeleton" role="status" aria-label="Carregando chuva e solo do talhão">
      <div class="traffic-panel__skeleton-row" />
      <div class="traffic-panel__skeleton-row traffic-panel__skeleton-row--tall" />
      <div class="traffic-panel__skeleton-row traffic-panel__skeleton-row--tall" />
    </div>

    <template v-else-if="trafficability">
      <ToastAlert
        v-for="issue in trafficability.dataIssues"
        :key="issue"
        tone="attention"
        :title="TRAFFICABILITY_DATA_ISSUE_TITLES[issue]"
        :message="TRAFFICABILITY_DATA_ISSUE_LABELS[issue]"
        :dismissible="false"
      >
        <template v-if="issueOffersProfile(issue)" #actions>
          <BaseButton variant="secondary" size="sm" icon="pen" @click="emit('editProfile')">Editar perfil de solo</BaseButton>
        </template>
      </ToastAlert>

      <TrafficabilitySummaryTiles :soil="trafficability.soil" />

      <div class="traffic-panel__split">
        <RainDailyChart
          class="traffic-panel__card"
          :entries="trafficability.rainDaily"
          :today="today"
          :rain="trafficability.soil.rain"
        />
        <section class="traffic-panel__card" :aria-labelledby="soilTitleId">
          <h3 :id="soilTitleId" class="traffic-panel__card-title text-label-meta-strong">Umidade do solo e tráfego</h3>
          <SoilStateScale :state="trafficability.soil.state" />
          <SoilFactsList
            :soil="trafficability.soil"
            :releases="trafficability.releases"
            :horizon-days="horizonDays"
            :forecast-unavailable="forecastUnavailable"
          />
          <SoilProfileSummary
            :profile="soilProfile"
            :pending="soilProfilePending"
            :can-edit="canEditProfile"
            @edit="emit('editProfile')"
          />
        </section>
      </div>

      <MethodEligibilityTable :days="trafficability.days" />

      <AgentPlannerNote />
    </template>
  </div>
</template>

<style scoped>
.traffic-panel {
  display: flex;
  flex-direction: column;
  gap: var(--space-18);
  min-width: 0;
}

.traffic-panel__split {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(260px, 1fr));
  gap: var(--space-16);
}

.traffic-panel__card {
  display: flex;
  flex-direction: column;
  gap: var(--space-10);
  min-width: 0;
  padding: var(--space-14);
  border: var(--border-width-hairline) solid var(--color-border);
  border-radius: var(--radius-lg);
}

.traffic-panel__card-title {
  margin: 0;
  color: var(--color-text-primary);
}

.traffic-panel__skeleton {
  display: flex;
  flex-direction: column;
  gap: var(--space-12);
}

.traffic-panel__skeleton-row {
  height: var(--size-control-lg);
  border-radius: var(--radius-md);
  background: var(--color-sand);
  animation: traffic-panel-pulse 1.4s var(--easing-standard) infinite;
}

.traffic-panel__skeleton-row--tall {
  height: 200px;
}

@keyframes traffic-panel-pulse {
  50% {
    opacity: 0.6;
  }
}

@media (prefers-reduced-motion: reduce) {
  .traffic-panel__skeleton-row {
    animation: none;
  }
}
</style>
