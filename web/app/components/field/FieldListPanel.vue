<script setup lang="ts">
import type { FieldBlockView, FieldsSummaryView } from '~/utils/farmDetail'

/**
 * "Lista de áreas" card of the farm detail (Figma 15:183): summary tiles,
 * collapsible blocks with their fields and the dashed "add field" link.
 * Presentation only: selection and expansion state come from the parent.
 */
withDefaults(defineProps<{
  summary: FieldsSummaryView
  blocks: readonly FieldBlockView[]
  /** Dashed "add field" link; omit for users without registry.write (ADR 0013). */
  addField?: { readonly label: string, readonly to: string } | null
  /** First load: skeleton rows instead of the blocks. */
  loading?: boolean
  /** Farm without fields: call to action instead of the blocks. */
  empty?: boolean
}>(), { addField: null, loading: false, empty: false })

const emit = defineEmits<{
  select: [id: string]
  toggle: [key: string]
}>()

const rootRef = ref<HTMLElement | null>(null)

/** Moves focus to a field row (e.g. back from the map popover). */
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
  <section ref="rootRef" class="field-panel" aria-label="Áreas e talhões" :aria-busy="loading || undefined">
    <dl class="field-panel__summary">
      <FieldMetricTile label="Áreas" :value="summary.blockCount" />
      <FieldMetricTile label="Talhões" :value="summary.fieldCount" />
      <FieldMetricTile label="Produtiva" :value="summary.productiveAreaLabel" />
    </dl>

    <div v-if="loading" class="field-panel__skeleton" aria-label="Carregando talhões">
      <div v-for="index in 5" :key="index" class="field-panel__skeleton-row" />
    </div>

    <div v-else-if="empty" class="field-panel__empty">
      <span class="field-panel__empty-icon" aria-hidden="true"><AppIcon name="pen" :size="20" /></span>
      <p class="field-panel__empty-title text-label-strong">Nenhum talhão cadastrado</p>
      <p v-if="addField" class="field-panel__empty-text text-caption">Desenhe o primeiro talhão no mapa para acompanhar cultura e condição de aplicação.</p>
      <p v-else class="field-panel__empty-text text-caption">Os talhões desta fazenda aparecem aqui quando o administrador da sua empresa cadastrá-los.</p>
    </div>

    <div v-else class="field-panel__blocks">
      <FieldBlockGroup
        v-for="block in blocks"
        :key="block.key"
        :block="block"
        @toggle="emit('toggle', $event)"
        @select="emit('select', $event)"
      />
    </div>

    <NuxtLink v-if="addField && !loading" :to="addField.to" class="field-panel__add text-label-meta-strong">
      <AppIcon name="plus" :size="16" />
      {{ empty ? 'Desenhe o primeiro talhão' : addField.label }}
    </NuxtLink>
  </section>
</template>

<style scoped>
.field-panel {
  display: flex;
  flex-direction: column;
  gap: var(--space-14);
  padding: var(--space-18);
  border: var(--border-width-hairline) solid var(--color-border);
  border-radius: var(--radius-xl);
  background: var(--color-white);
}

.field-panel__summary {
  display: flex;
  gap: var(--space-10);
  margin: 0;
}

.field-panel__blocks {
  display: flex;
  flex-direction: column;
  gap: var(--space-14);
}

.field-panel__add {
  display: flex;
  gap: var(--space-8);
  align-items: center;
  justify-content: center;
  padding: var(--space-12);
  border: var(--border-width-control) dashed var(--color-border);
  border-radius: var(--radius-md);
  color: var(--color-green-700);
  text-decoration: none;
}

.field-panel__add:hover {
  border-color: var(--color-green-700);
  background: var(--color-green-100);
}

.field-panel__add:focus-visible {
  outline: none;
  box-shadow: var(--focus-ring);
}

.field-panel__empty {
  display: flex;
  flex-direction: column;
  gap: var(--space-6);
  align-items: center;
  padding: var(--space-24) var(--space-12);
  text-align: center;
}

.field-panel__empty-icon {
  display: grid;
  place-items: center;
  width: var(--size-icon-tile-lg);
  height: var(--size-icon-tile-lg);
  margin-bottom: var(--space-6);
  border-radius: var(--radius-tile-lg);
  background: var(--color-green-100);
  color: var(--color-green-800);
}

.field-panel__empty-title {
  color: var(--color-text-primary);
}

.field-panel__empty-text {
  color: var(--color-text-secondary);
}

.field-panel__skeleton {
  display: flex;
  flex-direction: column;
  gap: var(--space-6);
}

.field-panel__skeleton-row {
  height: var(--size-control-lg);
  border-radius: var(--radius-md);
  background: var(--color-sand);
  animation: field-panel-pulse 1.4s var(--easing-standard) infinite;
}

@keyframes field-panel-pulse {
  50% {
    opacity: 0.6;
  }
}

@media (prefers-reduced-motion: reduce) {
  .field-panel__skeleton-row {
    animation: none;
  }
}
</style>
