<script setup lang="ts">
import previewMapUrl from '~/assets/images/wizard-preview-map.svg'
import type { PerimeterGeometry } from '~/types/geo'

/**
 * Live preview of the farm being registered (Figma "Prévia ao vivo" 35:573 / 14:288).
 * Presentation only: rows and texts come ready from the wizard composable.
 * Without a perimeter the Figma illustration is shown; with one, the real map.
 */
export interface WizardPreviewItem {
  key: string
  label: string
  value: string
}

withDefaults(
  defineProps<{
    name: string
    rows: readonly WizardPreviewItem[]
    perimeter?: PerimeterGeometry | null
    /** Text of the sparkles card. Hidden when empty. */
    hint?: string
  }>(),
  { perimeter: null, hint: undefined },
)

const titleId = useId()
</script>

<template>
  <aside class="wizard-preview" :aria-labelledby="titleId">
    <div class="wizard-preview__map">
      <MapView
        v-if="perimeter"
        :perimeter="perimeter"
        alt="Perímetro da fazenda no mapa"
        height="var(--wizard-preview-map-height)"
      />
      <img
        v-else
        :src="previewMapUrl"
        alt=""
        width="300"
        height="160"
        class="wizard-preview__illustration"
      >
    </div>

    <div class="wizard-preview__body">
      <p class="wizard-preview__eyebrow text-overline">Prévia ao vivo</p>
      <h2 :id="titleId" class="wizard-preview__name">{{ name }}</h2>

      <dl class="wizard-preview__rows">
        <div v-for="row in rows" :key="row.key" class="wizard-preview__row">
          <dt class="wizard-preview__label text-caption-md">{{ row.label }}</dt>
          <dd class="wizard-preview__value text-label-meta">{{ row.value }}</dd>
        </div>
      </dl>

      <div v-if="hint" class="wizard-preview__hint">
        <AppIcon name="sparkles" :size="16" class="wizard-preview__hint-icon" />
        <p class="wizard-preview__hint-text text-body-sm">{{ hint }}</p>
      </div>
    </div>
  </aside>
</template>

<style scoped>
.wizard-preview {
  display: flex;
  flex-direction: column;
  overflow: hidden;
  border-radius: var(--radius-xl);
  background: var(--color-green-900);
  color: var(--color-text-inverse);
}

.wizard-preview__map {
  height: var(--wizard-preview-map-height);
  overflow: hidden;
}

/* The Figma frame is wider than the card and clipped on the right. */
.wizard-preview__illustration {
  display: block;
  width: 100%;
  height: var(--wizard-preview-map-height);
  object-fit: cover;
  object-position: left top;
}

.wizard-preview__body {
  display: flex;
  flex-direction: column;
  gap: var(--space-16);
  padding: var(--space-22);
}

.wizard-preview__eyebrow {
  margin: 0;
  color: var(--color-sprout-400);
}

.wizard-preview__name {
  margin: 0;
  font: var(--font-h3);
  overflow-wrap: anywhere;
}

.wizard-preview__rows {
  display: flex;
  flex-direction: column;
  gap: var(--space-16);
  margin: 0;
}

.wizard-preview__row {
  display: flex;
  gap: var(--space-8);
  align-items: flex-start;
}

.wizard-preview__label {
  flex: 1;
  min-width: 0;
  color: var(--color-text-inverse-caption);
}

.wizard-preview__value {
  margin: 0;
  text-align: right;
}

.wizard-preview__hint {
  display: flex;
  flex-direction: column;
  gap: var(--space-8);
  padding: var(--space-14);
  border-radius: var(--radius-tile-lg);
  background: var(--color-sprout-tint);
}

.wizard-preview__hint-icon {
  color: var(--color-sprout-400);
}

.wizard-preview__hint-text {
  margin: 0;
  color: var(--color-text-inverse-note);
}
</style>
