<script setup lang="ts">
import type { TerrainFailureView, TerrainNotCalculatedView, TerrainProgressView } from '~/utils/terrain'

/**
 * "Relevo ainda não calculado" card (Figma 72:1136): what will be calculated,
 * the calculate button and, while it runs, the progress. Without `registry.write`
 * the button gives way to an honest message. Failures per field are listed with
 * the reason the API gave. Presentation only.
 */
withDefaults(defineProps<{
  view: TerrainNotCalculatedView
  /** Set while a calculation runs: the button is replaced by the progress. */
  progress?: TerrainProgressView | null
  failures?: readonly TerrainFailureView[]
  /** Request in flight (button in loading state). */
  calculating?: boolean
  /** Message of a failed calculate request. */
  error?: string | null
}>(), { progress: null, failures: () => [], calculating: false, error: null })

const emit = defineEmits<{
  calculate: []
}>()

/** Fixed copy of the Figma (72:1142 and 72:1170); the model is fixed by ADR 0021. */
const DESCRIPTION
  = 'Calculamos o relevo de cada talhão a partir do perímetro cadastrado e do modelo digital de elevação Copernicus (30 m). É feito uma vez e refeito quando o perímetro muda.'
const DURATION_NOTE = 'Leva cerca de 1 minuto. Você pode sair desta tela; avisamos quando terminar.'

const titleId = useId()
</script>

<template>
  <section class="terrain-empty" :aria-labelledby="titleId">
    <span class="terrain-empty__icon" aria-hidden="true"><AppIcon name="relief" :size="26" /></span>
    <h2 :id="titleId" class="terrain-empty__title text-h3">{{ view.title }}</h2>
    <p class="terrain-empty__description text-body-md">{{ DESCRIPTION }}</p>

    <div class="terrain-empty__list-box">
      <p class="terrain-empty__list-title text-label-meta-strong">Para cada talhão</p>
      <ul class="terrain-empty__list">
        <li v-for="item in view.items" :key="item" class="terrain-empty__item text-label-chip">
          <AppIcon name="check" :size="16" class="terrain-empty__check" />
          {{ item }}
        </li>
      </ul>
    </div>

    <ToastAlert v-if="error" tone="error" title="Não foi possível iniciar o cálculo" :message="error" :dismissible="false" />

    <template v-if="!progress">
      <BaseButton v-if="view.calculateLabel" block :loading="calculating" @click="emit('calculate')">
        {{ view.calculateLabel }}
      </BaseButton>
      <p v-else-if="view.noPermissionMessage" class="terrain-empty__no-permission text-label-chip" role="status">
        {{ view.noPermissionMessage }}
      </p>
      <p v-if="view.calculateLabel" class="terrain-empty__note text-caption">{{ DURATION_NOTE }}</p>
    </template>

    <div v-if="progress || failures.length" class="terrain-empty__state">
      <TerrainProgressBar v-if="progress" :progress="progress" label="Estado durante o cálculo" />
      <ul v-if="failures.length" class="terrain-empty__failures" aria-label="Talhões com falha no cálculo">
        <li v-for="failure in failures" :key="failure.fieldId" class="text-caption-xs">
          <strong>{{ failure.name }}:</strong> {{ failure.reason }}
        </li>
      </ul>
    </div>
  </section>
</template>

<style scoped>
.terrain-empty {
  display: flex;
  flex-direction: column;
  gap: var(--space-16);
  padding: var(--space-24);
  border: var(--border-width-hairline) solid var(--color-border);
  border-radius: var(--radius-xl);
  background: var(--color-white);
}

.terrain-empty__icon {
  display: grid;
  place-items: center;
  width: var(--size-icon-tile-xl);
  height: var(--size-icon-tile-xl);
  border-radius: var(--radius-tile-lg);
  background: var(--color-green-100);
  color: var(--color-green-700);
}

.terrain-empty__title {
  margin: 0;
  color: var(--color-text-primary);
}

.terrain-empty__description {
  margin: 0;
  color: var(--color-text-secondary);
}

.terrain-empty__list-box {
  display: flex;
  flex-direction: column;
  gap: var(--space-10);
  padding: var(--space-16);
  border-radius: var(--radius-tile-lg);
  background: var(--color-sand);
}

.terrain-empty__list-title {
  margin: 0;
  color: var(--color-text-primary);
}

.terrain-empty__list {
  display: flex;
  flex-direction: column;
  gap: var(--space-10);
  margin: 0;
  padding: 0;
  list-style: none;
}

.terrain-empty__item {
  display: flex;
  gap: var(--space-8);
  align-items: center;
  color: var(--color-text-primary);
}

.terrain-empty__check {
  flex-shrink: 0;
  color: var(--color-green-700);
}

.terrain-empty__note {
  margin: 0;
  color: var(--color-text-tertiary);
}

.terrain-empty__no-permission {
  margin: 0;
  padding: var(--space-12) var(--space-14);
  border-radius: var(--radius-md);
  background: var(--color-status-attention-bg);
  color: var(--color-coffee-700);
}

.terrain-empty__state {
  display: flex;
  flex-direction: column;
  gap: var(--space-8);
  padding: var(--space-14);
  border: var(--border-width-control) dashed var(--color-border);
  border-radius: var(--radius-tile-lg);
}

.terrain-empty__failures {
  display: flex;
  flex-direction: column;
  gap: var(--space-4);
  margin: 0;
  padding: 0;
  color: var(--color-status-attention);
  list-style: none;
}
</style>
