<script setup lang="ts">
import type { WizardReviewSection, WizardStepKey } from '~/types/wizard'

/**
 * Step 4 — Revisão (Figma 35:1326): one summary block per step with an
 * "Editar" link, then the confirmation checkbox. Presentation only.
 */
withDefaults(
  defineProps<{
    sections: readonly WizardReviewSection[]
    /** Error of the confirmation checkbox (shown after trying to save). */
    confirmError?: string
  }>(),
  { confirmError: undefined },
)

const confirmed = defineModel<boolean>('confirmed', { required: true })

const emit = defineEmits<{
  edit: [step: Exclude<WizardStepKey, 'review'>]
}>()

const confirmErrorId = useId()
</script>

<template>
  <div class="review-step">
    <section
      v-for="section in sections"
      :key="section.step"
      class="review-step__section"
      :aria-label="section.title"
    >
      <header class="review-step__header">
        <h3 class="review-step__title text-label-strong">{{ section.title }}</h3>
        <button type="button" class="review-step__edit text-label-field" @click="emit('edit', section.step)">
          Editar<span class="visually-hidden"> {{ section.title }}</span>
          <AppIcon name="pen" :size="14" />
        </button>
      </header>

      <dl v-if="section.rows.length > 0" class="review-step__rows">
        <div v-for="row in section.rows" :key="row.label" class="review-step__row">
          <dt class="review-step__label text-body-sm">{{ row.label }}</dt>
          <dd
            class="review-step__value text-label-field"
            :class="{ 'review-step__value--missing': row.isEmpty }"
          >
            {{ row.value }}
          </dd>
        </div>
      </dl>
      <p v-else-if="section.emptyMessage" class="review-step__empty text-body-sm">{{ section.emptyMessage }}</p>
    </section>

    <div class="review-step__confirm">
      <BaseCheckbox
        v-model="confirmed"
        class="review-step__checkbox"
        name="confirmed"
        label="Confirmo que os dados estão corretos"
        :aria-describedby="confirmError ? confirmErrorId : undefined"
        :aria-invalid="confirmError ? true : undefined"
      />
      <p v-if="confirmError" :id="confirmErrorId" class="review-step__error text-body-sm" role="alert">{{ confirmError }}</p>
    </div>
  </div>
</template>

<style scoped>
.review-step {
  display: flex;
  flex-direction: column;
  gap: var(--space-24);
}

.review-step__section {
  display: flex;
  flex-direction: column;
  gap: var(--space-12);
  padding: var(--space-18);
  border-radius: var(--radius-lg);
  background: var(--color-sand);
}

.review-step__header {
  display: flex;
  gap: var(--space-12);
  align-items: center;
}

.review-step__title {
  flex: 1;
  min-width: 0;
  margin: 0;
  color: var(--color-text-primary);
}

.review-step__edit {
  display: inline-flex;
  gap: var(--space-4);
  align-items: center;
  padding: 0;
  border: 0;
  border-radius: var(--radius-xs);
  background: none;
  color: var(--color-green-700);
  cursor: pointer;
}

.review-step__edit:hover {
  text-decoration: underline;
}

.review-step__edit:focus-visible {
  outline: none;
  box-shadow: var(--focus-ring);
}

.review-step__rows {
  display: flex;
  flex-direction: column;
  gap: var(--space-12);
  margin: 0;
}

.review-step__row {
  display: grid;
  grid-template-columns: var(--review-label-width) minmax(0, 1fr);
  gap: var(--space-12);
  align-items: start;
}

.review-step__label {
  color: var(--color-text-tertiary);
}

.review-step__value {
  margin: 0;
  color: var(--color-text-primary);
  overflow-wrap: anywhere;
}

.review-step__value--missing,
.review-step__empty {
  color: var(--color-text-tertiary);
}

.review-step__empty {
  margin: 0;
}

.review-step__confirm {
  display: flex;
  flex-direction: column;
  gap: var(--space-8);
}

/* Figma 35:1379: the confirmation reads as body text in the primary color. */
.review-step__checkbox :deep(.base-checkbox__label) {
  color: var(--color-text-primary);
}

.review-step__error {
  margin: 0;
  color: var(--color-status-unsuitable);
}

@media (max-width: 599px) {
  .review-step__row {
    grid-template-columns: minmax(0, 1fr);
    gap: var(--space-2);
  }
}
</style>
