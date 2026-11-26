<script setup lang="ts">
import { TERRAIN_CLASS_COLORS, TERRAIN_CLASS_TONES } from '~/theme/map-style'
import type { MethodFitStatus } from '~/types/terrain'
import type { FieldTerrainDetailView, TerrainDetailView } from '~/utils/terrain'
import type { SegmentOption } from '../SegmentedControl.vue'
import type { StatusTone } from '../StatusBadge.vue'

/**
 * Relief detail of one field (Figma "Relevo · detalhe do Talhão 4", 70:1069):
 * title and class, four indicators, area per class (stacked bar), position and
 * altitude profile, calculated × adjusted comparison and the equipment table.
 * Presentation only: every value and label comes ready in `detail` (ADR 0021).
 * With the `rain` slot, a "Relevo | Chuva e solo" switch (v-model:view) shows
 * that slot in place of the relief content (Figma 04f, ADR 0022). Header and
 * switch do not wait for the relief: the rain view works even when the relief
 * is still loading or failed (the title falls back to `fieldName`).
 */
const props = withDefaults(defineProps<{
  /** Null while loading or when the request failed. */
  detail: FieldTerrainDetailView | null
  pending?: boolean
  /** Message of a failed load (shown with "Tentar novamente"). */
  error?: string | null
  /** `registry.write`: shows "Ajustar manualmente" and "Remover ajuste". */
  canWrite?: boolean
  removing?: boolean
  removeError?: string | null
  /** Title while the relief has not loaded (or failed). */
  fieldName?: string | null
}>(), { pending: false, error: null, canWrite: false, removing: false, removeError: null, fieldName: null })

const emit = defineEmits<{
  back: []
  adjust: []
  remove: []
  retry: []
}>()

const view = defineModel<TerrainDetailView>('view', { default: 'relief' })

const slots = defineSlots<{
  /** Content of the "Chuva e solo" view; without it there is no view switch. */
  rain?: () => unknown
  /** Header actions of the "Chuva e solo" view (e.g. "Lançar pluviômetro"). */
  rainActions?: () => unknown
}>()

const VIEW_OPTIONS: readonly SegmentOption<TerrainDetailView>[] = [
  { value: 'relief', label: 'Relevo' },
  { value: 'rain', label: 'Chuva e solo' },
]

/** Viewbox of the profile drawing (Figma 70:1135 is 268 × 72). */
const PROFILE_WIDTH = 268
const PROFILE_HEIGHT = 72
const PROFILE_TOP = 8
const PROFILE_BOTTOM = 66

const STATUS_TONE: Readonly<Record<MethodFitStatus, StatusTone>> = {
  compatible: 'suitable',
  restricted: 'attention',
  unsuitable: 'unsuitable',
}

const titleId = useId()
const isRainView = computed(() => view.value === 'rain' && Boolean(slots.rain))
const title = computed(() => props.detail?.title ?? props.fieldName ?? null)
const rootRef = ref<HTMLElement | null>(null)

/** Moves focus to the card (e.g. right after "Ver relevo do talhão"), so keyboard users land on it. */
function focus() {
  rootRef.value?.focus()
}

defineExpose({ focus })

const profilePaths = computed(() => {
  const profile = props.detail?.profile
  if (!profile || profile.points.length < 2) return null
  const coordinates = profile.points.map((point) => {
    const x = point.x * PROFILE_WIDTH
    const y = PROFILE_TOP + (1 - point.y) * (PROFILE_BOTTOM - PROFILE_TOP)
    return `${x.toFixed(1)} ${y.toFixed(1)}`
  })
  const line = `M${coordinates.join(' L')}`
  return { line, area: `${line} L${PROFILE_WIDTH} ${PROFILE_HEIGHT} L0 ${PROFILE_HEIGHT} Z` }
})

// Data-driven colors: the class of the field is only known at runtime.
const profileStyle = computed(() => {
  const tone = props.detail ? TERRAIN_CLASS_TONES[props.detail.badge.key] : null
  return { '--terrain-profile-line': tone?.bar, '--terrain-profile-fill': tone?.badgeBackground }
})

const classBarLabel = computed(() =>
  (props.detail?.classAreas ?? []).map((row) => `${row.label} ${row.shareLabel}`).join(', '),
)

const barSegments = computed(() => (props.detail?.classAreas ?? []).filter((row) => row.sharePct > 0))

const profileLabel = computed(() => {
  const profile = props.detail?.profile
  return profile ? `Perfil de altitude da linha de maior declive: ${profile.topLabel}; ${profile.baseLabel}` : undefined
})

function segmentStyle(key: keyof typeof TERRAIN_CLASS_COLORS, sharePct: number) {
  return { width: `${sharePct}%`, backgroundColor: TERRAIN_CLASS_COLORS[key] }
}
</script>

<template>
  <section
    ref="rootRef"
    class="terrain-detail"
    tabindex="-1"
    :aria-label="isRainView ? 'Chuva e solo do talhão' : 'Relevo do talhão'"
    :aria-busy="(!isRainView && pending) || undefined"
  >
    <div class="terrain-detail__back">
      <BaseButton variant="ghost" size="sm" @click="emit('back')">← Voltar ao mapa</BaseButton>
    </div>

    <header v-if="title" class="terrain-detail__header">
      <div class="terrain-detail__heading">
        <div class="terrain-detail__title-row">
          <h2 :id="titleId" class="terrain-detail__title text-h3">{{ title }}</h2>
          <TerrainClassBadge v-if="detail" :badge="detail.badge" />
        </div>
        <p v-if="detail" class="terrain-detail__subtitle text-caption">{{ detail.subtitle }}</p>
      </div>
      <div v-if="isRainView && slots.rainActions" class="terrain-detail__actions">
        <slot name="rainActions" />
      </div>
      <div v-else-if="canWrite && detail && !isRainView" class="terrain-detail__actions">
        <BaseButton variant="secondary" size="sm" @click="emit('adjust')">Ajustar manualmente</BaseButton>
        <BaseButton v-if="detail.hasAdjustment" variant="ghost" size="sm" :loading="removing" @click="emit('remove')">
          Remover ajuste
        </BaseButton>
      </div>
    </header>

    <div v-if="slots.rain" class="terrain-detail__views">
      <SegmentedControl v-model="view" :options="VIEW_OPTIONS" label="Visão do talhão" />
      <p class="terrain-detail__views-hint text-label-tag">Relevo é fixo do talhão · chuva e solo mudam a cada leitura</p>
    </div>

    <slot v-if="isRainView" name="rain" />

    <ToastAlert
      v-else-if="error"
      tone="error"
      title="Não foi possível carregar o relevo do talhão"
      :message="error"
      :dismissible="false"
    >
      <template #actions>
        <BaseButton variant="secondary" size="sm" icon="undo" @click="emit('retry')">Tentar novamente</BaseButton>
      </template>
    </ToastAlert>

    <div v-else-if="pending && !detail" class="terrain-detail__skeleton" role="status" aria-label="Carregando relevo do talhão">
      <div v-if="!title" class="terrain-detail__skeleton-row terrain-detail__skeleton-row--title" />
      <div class="terrain-detail__skeleton-row" />
      <div class="terrain-detail__skeleton-row terrain-detail__skeleton-row--tall" />
    </div>

    <template v-else-if="detail">
      <ToastAlert
        v-if="detail.failureMessage"
        tone="error"
        title="Falha no cálculo do relevo"
        :message="detail.failureMessage"
        :dismissible="false"
      />
      <ToastAlert
        v-if="detail.outdatedWarning"
        tone="attention"
        title="Ajuste desatualizado"
        :message="detail.outdatedWarning"
        :dismissible="false"
      />
      <ToastAlert
        v-if="removeError"
        tone="error"
        title="Não foi possível remover o ajuste"
        :message="removeError"
        :dismissible="false"
      />

      <dl class="terrain-detail__indicators">
        <TerrainIndicatorTile :indicator="detail.indicators.slopeMean" size="lg" />
        <TerrainIndicatorTile :indicator="detail.indicators.slopeP90" size="lg" />
        <TerrainIndicatorTile :indicator="detail.indicators.slopeMax" size="lg" />
        <TerrainIndicatorTile :indicator="detail.indicators.elevation" size="lg" />
      </dl>

      <section v-if="detail.hasAdjustment" class="terrain-card" aria-label="Valor calculado e ajuste manual">
        <h3 class="terrain-card__title text-label-meta-strong">Calculado × ajustado</h3>
        <table class="terrain-detail__comparison text-caption">
          <thead>
            <tr>
              <th scope="col" class="terrain-detail__col-label"><span class="visually-hidden">Indicador</span></th>
              <th scope="col">Calculado</th>
              <th scope="col">Ajustado</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="row in detail.comparison" :key="row.label">
              <th scope="row" class="terrain-detail__col-label">{{ row.label }}</th>
              <td>{{ row.calculated }}</td>
              <td class="terrain-detail__adjusted">{{ row.adjusted }}</td>
            </tr>
          </tbody>
        </table>
        <p v-if="detail.adjustmentNote" class="terrain-detail__note text-caption">{{ detail.adjustmentNote }}</p>
      </section>

      <div class="terrain-detail__split">
        <section v-if="detail.classAreas.length" class="terrain-card" aria-labelledby="terrain-class-areas">
          <h3 id="terrain-class-areas" class="terrain-card__title text-label-meta-strong">Área por classe de declividade</h3>
          <div class="terrain-detail__bar" role="img" :aria-label="`Área por classe: ${classBarLabel}`">
            <span
              v-for="row in barSegments"
              :key="row.key"
              class="terrain-detail__segment"
              :style="segmentStyle(row.key, row.sharePct)"
            />
          </div>
          <ul class="terrain-detail__class-list">
            <li
              v-for="row in detail.classAreas"
              :key="row.key"
              class="terrain-detail__class-row text-caption"
              :class="{ 'terrain-detail__class-row--empty': row.sharePct === 0 }"
            >
              <span class="terrain-detail__swatch" :style="{ backgroundColor: TERRAIN_CLASS_COLORS[row.key] }" aria-hidden="true" />
              <span class="terrain-detail__class-name">{{ row.label }}</span>
              <span class="terrain-detail__class-area">{{ row.areaLabel }}</span>
              <span class="terrain-detail__class-share">{{ row.shareLabel }}</span>
            </li>
          </ul>
        </section>

        <section class="terrain-card" aria-labelledby="terrain-position">
          <h3 id="terrain-position" class="terrain-card__title text-label-meta-strong">Posição e perfil</h3>
          <template v-if="detail.profile && profilePaths">
            <svg
              class="terrain-detail__profile"
              :style="profileStyle"
              :viewBox="`0 0 ${PROFILE_WIDTH} ${PROFILE_HEIGHT}`"
              preserveAspectRatio="none"
              role="img"
              :aria-label="profileLabel"
            >
              <path :d="profilePaths.area" class="terrain-detail__profile-area" />
              <path :d="profilePaths.line" class="terrain-detail__profile-line" vector-effect="non-scaling-stroke" />
            </svg>
            <p class="terrain-detail__profile-labels text-label-tag">
              <span>{{ detail.profile.topLabel }}</span>
              <span>{{ detail.profile.baseLabel }}</span>
            </p>
          </template>
          <dl class="terrain-detail__facts text-caption">
            <div v-for="fact in detail.facts" :key="fact.label" class="terrain-detail__fact">
              <dt>{{ fact.label }}</dt>
              <dd>{{ fact.value }}</dd>
            </div>
          </dl>
        </section>
      </div>

      <section v-if="detail.methods.length" class="terrain-card terrain-card--flush" aria-labelledby="terrain-methods">
        <h3 id="terrain-methods" class="terrain-detail__methods-title text-label-meta-strong">Equipamentos para este relevo</h3>
        <table class="terrain-detail__methods text-caption">
          <thead>
            <tr>
              <th scope="col">Método</th>
              <th scope="col">Limite de declividade</th>
              <th scope="col">Situação no talhão</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="row in detail.methods" :key="row.method">
              <th scope="row">{{ row.label }}</th>
              <td>{{ row.limitLabel }}</td>
              <td><StatusBadge :tone="STATUS_TONE[row.status]" :label="row.statusLabel" /></td>
            </tr>
          </tbody>
        </table>
        <p class="terrain-detail__methods-note text-caption-xs">{{ detail.methodsNote }}</p>
      </section>
    </template>
  </section>
</template>

<style scoped>
.terrain-detail {
  display: flex;
  flex-direction: column;
  gap: var(--space-18);
  min-width: 0;
  padding: var(--space-24);
  border: var(--border-width-hairline) solid var(--color-border);
  border-radius: var(--radius-xl);
  background: var(--color-white);
}

.terrain-detail__back {
  display: flex;
  margin: calc(-1 * var(--space-8)) 0 calc(-1 * var(--space-8)) calc(-1 * var(--space-14));
}

.terrain-detail__header {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-12);
  align-items: center;
}

.terrain-detail__heading {
  display: flex;
  flex: 1 1 320px;
  flex-direction: column;
  gap: var(--space-4);
  min-width: 0;
}

.terrain-detail__title-row {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-10);
  align-items: center;
}

.terrain-detail__title {
  margin: 0;
  color: var(--color-text-primary);
}

.terrain-detail__subtitle {
  margin: 0;
  color: var(--color-text-secondary);
}

.terrain-detail__actions {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-8);
}

.terrain-detail__views {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-8) var(--space-12);
  align-items: center;
}

.terrain-detail__views-hint {
  margin: 0;
  color: var(--color-text-tertiary);
}

.terrain-detail__indicators {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(150px, 1fr));
  gap: var(--space-10);
  margin: 0;
}

.terrain-detail__split {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(280px, 1fr));
  gap: var(--space-16);
}

.terrain-card {
  display: flex;
  flex-direction: column;
  gap: var(--space-10);
  min-width: 0;
  padding: var(--space-14);
  border: var(--border-width-hairline) solid var(--color-border);
  border-radius: var(--radius-lg);
}

.terrain-card--flush {
  gap: 0;
  padding: 0;
  overflow: hidden;
}

.terrain-card__title {
  margin: 0;
  color: var(--color-text-primary);
}

.terrain-detail__bar {
  display: flex;
  height: var(--space-12);
  overflow: hidden;
  border-radius: var(--radius-xs);
  background: var(--color-sand);
}

.terrain-detail__segment {
  display: block;
  height: 100%;
}

.terrain-detail__class-list {
  display: flex;
  flex-direction: column;
  gap: var(--space-10);
  margin: 0;
  padding: 0;
  list-style: none;
}

.terrain-detail__class-row {
  display: flex;
  gap: var(--space-8);
  align-items: center;
  color: var(--color-text-primary);
}

.terrain-detail__class-row--empty .terrain-detail__class-name,
.terrain-detail__class-row--empty .terrain-detail__class-share {
  color: var(--color-text-tertiary);
}

.terrain-detail__swatch {
  flex-shrink: 0;
  width: var(--size-legend-swatch);
  height: var(--size-legend-swatch);
  border-radius: var(--radius-legend-swatch);
}

.terrain-detail__class-name {
  flex: 1;
  min-width: 0;
  font-weight: 600;
}

.terrain-detail__class-area {
  color: var(--color-text-secondary);
  white-space: nowrap;
}

.terrain-detail__class-share {
  min-width: 2.5em;
  font-weight: 700;
  text-align: right;
  white-space: nowrap;
}

.terrain-detail__profile {
  display: block;
  width: 100%;
  height: 72px;
}

.terrain-detail__profile-area {
  fill: var(--terrain-profile-fill);
}

.terrain-detail__profile-line {
  fill: none;
  stroke: var(--terrain-profile-line);
  stroke-width: 2;
}

.terrain-detail__profile-labels {
  display: flex;
  gap: var(--space-8);
  justify-content: space-between;
  margin: 0;
  color: var(--color-text-tertiary);
}

.terrain-detail__facts {
  display: flex;
  flex-direction: column;
  gap: var(--space-10);
  margin: 0;
}

.terrain-detail__fact {
  display: flex;
  gap: var(--space-8);
}

.terrain-detail__fact dt {
  flex: 1;
  color: var(--color-text-secondary);
}

.terrain-detail__fact dd {
  margin: 0;
  color: var(--color-text-primary);
  font-weight: 700;
  text-align: right;
}

.terrain-detail__comparison,
.terrain-detail__methods {
  width: 100%;
  border-collapse: collapse;
  text-align: left;
}

.terrain-detail__comparison th,
.terrain-detail__comparison td {
  padding: var(--space-6) var(--space-8);
  font-weight: 600;
  color: var(--color-text-secondary);
}

.terrain-detail__comparison tbody th {
  color: var(--color-text-primary);
}

.terrain-detail__comparison .terrain-detail__adjusted {
  color: var(--color-text-primary);
  font-weight: 700;
}

.terrain-detail__note {
  margin: 0;
  color: var(--color-text-secondary);
}

.terrain-detail__methods-title {
  margin: 0;
  padding: var(--space-12) var(--space-14) var(--space-8);
  color: var(--color-text-primary);
}

.terrain-detail__methods th,
.terrain-detail__methods td {
  padding: var(--space-8) var(--space-14);
  text-align: left;
  vertical-align: middle;
}

.terrain-detail__methods thead th {
  background: var(--color-sand);
  color: var(--color-text-tertiary);
  font-weight: 600;
}

.terrain-detail__methods tbody tr {
  border-top: var(--border-width-hairline) solid var(--color-coffee-100);
}

.terrain-detail__methods tbody th {
  color: var(--color-text-primary);
  font-weight: 700;
}

.terrain-detail__methods td {
  color: var(--color-text-secondary);
}

.terrain-detail__methods td:last-child {
  padding-block: var(--space-4);
}

.terrain-detail__methods-note {
  margin: 0;
  padding: var(--space-8) var(--space-14) var(--space-12);
  color: var(--color-text-tertiary);
}

.terrain-detail__skeleton {
  display: flex;
  flex-direction: column;
  gap: var(--space-12);
}

.terrain-detail__skeleton-row {
  height: var(--size-control-lg);
  border-radius: var(--radius-md);
  background: var(--color-sand);
  animation: terrain-detail-pulse 1.4s var(--easing-standard) infinite;
}

.terrain-detail__skeleton-row--title {
  width: 40%;
}

.terrain-detail__skeleton-row--tall {
  height: 200px;
}

@keyframes terrain-detail-pulse {
  50% {
    opacity: 0.6;
  }
}

@media (prefers-reduced-motion: reduce) {
  .terrain-detail__skeleton-row {
    animation: none;
  }
}

@media (max-width: 1099px) {
  .terrain-detail {
    padding: var(--space-16);
  }
}
</style>
