<script setup lang="ts">
import type {
  FarmTerrainIndicatorsView,
  TerrainBlockView,
  TerrainFailureView,
  TerrainMissingBannerView,
  TerrainPanelHeaderView,
  TerrainProgressView,
} from '~/utils/terrain'

/**
 * "Relevo por talhão" card (Figma 68:986): title and model, three indicators,
 * collapsible blocks with the field rows. Also hosts the calculation progress,
 * the per-field failures and the "N talhões sem relevo" banner. Presentation
 * only: selection and expansion come from the parent.
 */
withDefaults(defineProps<{
  header: TerrainPanelHeaderView
  indicators: FarmTerrainIndicatorsView
  blocks: readonly TerrainBlockView[]
  /** First load: skeleton rows instead of the blocks. */
  loading?: boolean
  /** Message when the relief request failed (the list may still show fields). */
  error?: string | null
  progress?: TerrainProgressView | null
  failures?: readonly TerrainFailureView[]
  missingBanner?: TerrainMissingBannerView | null
  /** A calculation request is in flight (the banner button shows loading). */
  calculating?: boolean
}>(), { loading: false, error: null, progress: null, failures: () => [], missingBanner: null, calculating: false })

const emit = defineEmits<{
  select: [id: string]
  toggle: [key: string]
  calculate: []
  retry: []
}>()

const rootRef = ref<HTMLElement | null>(null)
const titleId = useId()
const blockIdPrefix = useId()

/** Moves focus to a field row (e.g. back from the map popover or the detail). */
function focusField(id: string) {
  const rows = rootRef.value?.querySelectorAll<HTMLElement>('[data-field-id]') ?? []
  for (const row of rows) {
    if (row.dataset.fieldId === id) {
      row.focus()
      return
    }
  }
}

defineExpose({ focusField })
</script>

<template>
  <section ref="rootRef" class="terrain-panel" :aria-labelledby="titleId" :aria-busy="loading || undefined">
    <header class="terrain-panel__header">
      <h2 :id="titleId" class="terrain-panel__title text-label-lg">{{ header.title }}</h2>
      <p class="terrain-panel__subtitle text-caption">{{ header.subtitle }}</p>
    </header>

    <dl class="terrain-panel__indicators">
      <TerrainIndicatorTile :indicator="indicators.slopeMean" />
      <TerrainIndicatorTile :indicator="indicators.restricted" />
      <TerrainIndicatorTile :indicator="indicators.elevation" />
    </dl>

    <ToastAlert
      v-if="error"
      tone="error"
      title="Não foi possível carregar o relevo"
      :message="error"
      :dismissible="false"
    >
      <template #actions>
        <BaseButton variant="secondary" size="sm" icon="undo" @click="emit('retry')">Tentar novamente</BaseButton>
      </template>
    </ToastAlert>

    <TerrainProgressBar v-if="progress" :progress="progress" label="Calculando o relevo" />

    <ul v-if="failures.length" class="terrain-panel__failures" aria-label="Talhões com falha no cálculo">
      <li v-for="failure in failures" :key="failure.fieldId" class="terrain-panel__failure text-caption-xs">
        <strong>{{ failure.name }}:</strong> {{ failure.reason }}
      </li>
    </ul>

    <div v-if="missingBanner" class="terrain-panel__missing">
      <p class="terrain-panel__missing-text text-label-chip">{{ missingBanner.label }}</p>
      <BaseButton
        v-if="missingBanner.calculateLabel"
        variant="secondary"
        size="sm"
        :loading="calculating"
        @click="emit('calculate')"
      >
        {{ missingBanner.calculateLabel }}
      </BaseButton>
    </div>

    <div v-if="loading" class="terrain-panel__skeleton" aria-label="Carregando relevo dos talhões">
      <div v-for="index in 5" :key="index" class="terrain-panel__skeleton-row" />
    </div>

    <div v-else class="terrain-panel__blocks">
      <section v-for="block in blocks" :key="block.key" class="terrain-block">
        <h3 class="terrain-block__heading">
          <button
            type="button"
            class="terrain-block__toggle"
            :aria-expanded="block.expanded"
            :aria-controls="`${blockIdPrefix}-${block.key}`"
            @click="emit('toggle', block.key)"
          >
            <AppIcon :name="block.expanded ? 'chevron-down' : 'chevron-right'" :size="16" class="terrain-block__chevron" />
            <span class="terrain-block__name text-label-strong">{{ block.name }}</span>
            <span class="terrain-block__meta text-label-chip">{{ block.expanded ? block.areaLabel : block.collapsedLabel }}</span>
          </button>
        </h3>
        <ul v-show="block.expanded" :id="`${blockIdPrefix}-${block.key}`" class="terrain-block__list">
          <li v-for="field in block.fields" :key="field.id">
            <TerrainFieldRow :field="field" @select="emit('select', $event)" />
          </li>
        </ul>
      </section>
    </div>
  </section>
</template>

<style scoped>
.terrain-panel {
  display: flex;
  flex-direction: column;
  gap: var(--space-14);
  padding: var(--space-18);
  border: var(--border-width-hairline) solid var(--color-border);
  border-radius: var(--radius-xl);
  background: var(--color-white);
}

.terrain-panel__header {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
}

.terrain-panel__title {
  margin: 0;
  color: var(--color-text-primary);
}

.terrain-panel__subtitle {
  margin: 0;
  color: var(--color-text-tertiary);
}

.terrain-panel__indicators {
  display: flex;
  gap: var(--space-10);
  margin: 0;
}

.terrain-panel__failures {
  display: flex;
  flex-direction: column;
  gap: var(--space-4);
  margin: 0;
  padding: 0;
  color: var(--color-status-attention);
  list-style: none;
}

.terrain-panel__missing {
  display: flex;
  gap: var(--space-10);
  align-items: center;
  padding: var(--space-8) var(--space-12);
  border-radius: var(--radius-md);
  background: var(--color-status-attention-bg);
}

.terrain-panel__missing-text {
  flex: 1;
  min-width: 0;
  margin: 0;
  color: var(--color-status-attention);
}

.terrain-panel__blocks {
  display: flex;
  flex-direction: column;
  gap: var(--space-14);
}

.terrain-panel__skeleton {
  display: flex;
  flex-direction: column;
  gap: var(--space-6);
}

.terrain-panel__skeleton-row {
  height: var(--size-control-lg);
  border-radius: var(--radius-md);
  background: var(--color-sand);
  animation: terrain-panel-pulse 1.4s var(--easing-standard) infinite;
}

@keyframes terrain-panel-pulse {
  50% {
    opacity: 0.6;
  }
}

@media (prefers-reduced-motion: reduce) {
  .terrain-panel__skeleton-row {
    animation: none;
  }
}

.terrain-block {
  display: flex;
  flex-direction: column;
  gap: var(--space-6);
}

.terrain-block__heading {
  margin: 0;
}

.terrain-block__toggle {
  display: flex;
  gap: var(--space-10);
  align-items: center;
  width: 100%;
  padding: var(--space-6) var(--space-4);
  border: 0;
  border-radius: var(--radius-sm);
  background: transparent;
  color: inherit;
  font: inherit;
  text-align: left;
  cursor: pointer;
}

.terrain-block__toggle:focus-visible {
  outline: none;
  box-shadow: var(--focus-ring);
}

.terrain-block__chevron {
  color: var(--color-text-secondary);
}

.terrain-block__name {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  color: var(--color-text-primary);
  text-overflow: ellipsis;
  white-space: nowrap;
}

.terrain-block__meta {
  color: var(--color-text-tertiary);
  white-space: nowrap;
}

.terrain-block__list {
  display: flex;
  flex-direction: column;
  gap: var(--space-6);
  margin: 0;
  padding: 0;
  list-style: none;
}
</style>
