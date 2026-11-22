<script setup lang="ts">
/**
 * Imported perimeter file (Figma 37:791): file name and details, "Válido" badge,
 * remove button and the facts the API returned, one tile each.
 * Presentation only: texts come ready from the wizard composable.
 */
export interface ImportFact {
  label: string
  value: string
}

defineProps<{
  fileName: string
  /** "48 KB · KML". */
  detail: string
  facts: readonly ImportFact[]
}>()

const emit = defineEmits<{
  remove: []
}>()
</script>

<template>
  <section class="import-card" :aria-label="`Arquivo importado: ${fileName}`">
    <div class="import-card__header">
      <span class="import-card__icon" aria-hidden="true">
        <AppIcon name="document" :size="20" />
      </span>
      <div class="import-card__file">
        <p class="import-card__name text-label-strong">{{ fileName }}</p>
        <p class="import-card__detail text-body-sm">{{ detail }}</p>
      </div>
      <span class="import-card__badge text-label-badge">
        <AppIcon name="check" :size="14" />
        Válido
      </span>
      <button
        type="button"
        class="import-card__remove"
        :aria-label="`Remover ${fileName}`"
        :title="`Remover ${fileName}`"
        @click="emit('remove')"
      >
        <AppIcon name="trash" :size="18" />
      </button>
    </div>

    <dl class="import-card__facts">
      <div v-for="fact in facts" :key="fact.label" class="import-card__fact">
        <dt class="import-card__fact-label text-body-sm">{{ fact.label }}</dt>
        <dd class="import-card__fact-value text-label-field">{{ fact.value }}</dd>
      </div>
    </dl>
  </section>
</template>

<style scoped>
.import-card {
  display: flex;
  flex-direction: column;
  gap: var(--space-12);
  padding: var(--space-16);
  border: var(--border-width-hairline) solid var(--color-border);
  border-radius: var(--radius-tile-lg);
}

.import-card__header {
  display: flex;
  gap: var(--space-12);
  align-items: center;
  padding-bottom: var(--space-12);
  border-bottom: var(--border-width-hairline) solid var(--color-border);
}

.import-card__icon {
  display: grid;
  flex-shrink: 0;
  place-items: center;
  width: var(--size-file-tile);
  height: var(--size-file-tile);
  border-radius: var(--radius-tile);
  background: var(--color-sand);
  color: var(--color-text-secondary);
}

.import-card__file {
  display: flex;
  flex: 1;
  flex-direction: column;
  gap: var(--space-2);
  min-width: 0;
}

.import-card__name,
.import-card__detail {
  margin: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.import-card__name {
  color: var(--color-text-primary);
}

.import-card__detail {
  color: var(--color-text-tertiary);
}

.import-card__badge {
  display: inline-flex;
  flex-shrink: 0;
  gap: var(--space-6);
  align-items: center;
  padding: var(--padding-badge-y) var(--space-10);
  border-radius: var(--radius-pill);
  background: var(--color-status-suitable-bg);
  color: var(--color-status-suitable);
}

.import-card__remove {
  display: grid;
  flex-shrink: 0;
  place-items: center;
  padding: var(--space-4);
  border: 0;
  border-radius: var(--radius-xs);
  background: none;
  color: var(--color-text-secondary);
  cursor: pointer;
}

.import-card__remove:hover {
  color: var(--color-status-unsuitable);
}

.import-card__remove:focus-visible {
  outline: none;
  box-shadow: var(--focus-ring);
}

.import-card__facts {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: var(--space-8);
  margin: 0;
}

.import-card__fact {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  min-width: 0;
  padding: var(--space-10);
  border-radius: var(--radius-tile);
  background: var(--color-sand);
}

.import-card__fact-label {
  color: var(--color-text-tertiary);
}

.import-card__fact-value {
  margin: 0;
  color: var(--color-text-primary);
  overflow-wrap: anywhere;
}
@media (max-width: 599px) {
  .import-card__facts {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}
</style>
