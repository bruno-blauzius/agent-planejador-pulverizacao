<script setup lang="ts">
import type { SprayCondition } from '~/types/spray'

/**
 * Plan line of one field (Figma 18:177), meant for the dark plan panel. The
 * window label, duration and condition come ready from the API; `shortWindow`
 * flags a window shorter than the work needs (Figma 18:191, the "⚠").
 */
withDefaults(
  defineProps<{
    /** Field code shown in the tile, e.g. "T3". */
    fieldCode: string
    /** e.g. "Qua · 26/11 · 06:00 – 10:00". */
    windowLabel: string
    /** e.g. "38,6 ha · 3h15". */
    detail: string
    condition?: SprayCondition
    shortWindow?: boolean
  }>(),
  { condition: 'suitable', shortWindow: false },
)
</script>

<template>
  <article class="field-plan-card">
    <span class="field-plan-card__code text-title-sm" aria-hidden="true">{{ fieldCode }}</span>
    <div class="field-plan-card__text">
      <h3 class="field-plan-card__window text-label-button">
        <span class="visually-hidden">Talhão {{ fieldCode }}: </span>{{ windowLabel }}
      </h3>
      <p class="field-plan-card__detail text-body-sm">
        {{ detail }}
        <template v-if="shortWindow">
          <AppIcon name="alert" :size="14" class="field-plan-card__alert" />
          <span class="visually-hidden">janela curta para o trabalho</span>
        </template>
      </p>
    </div>
    <StatusBadge :tone="condition" />
  </article>
</template>

<style scoped>
.field-plan-card {
  display: flex;
  gap: var(--space-14);
  align-items: center;
  padding: var(--space-14);
  border-radius: var(--radius-lg);
  background: var(--color-surface-inverse-glass);
}

.field-plan-card__code {
  display: inline-flex;
  flex-shrink: 0;
  align-items: center;
  justify-content: center;
  width: var(--size-planner-code-tile);
  height: var(--size-planner-code-tile);
  border-radius: var(--radius-md);
  background: var(--color-sprout-400);
  color: var(--color-green-950);
}

.field-plan-card__text {
  display: flex;
  flex: 1 1 auto;
  flex-direction: column;
  gap: var(--space-2);
  min-width: 0;
}

.field-plan-card__window {
  margin: 0;
  color: var(--color-text-inverse);
  white-space: pre-wrap;
}

.field-plan-card__detail {
  display: flex;
  gap: var(--space-4);
  align-items: center;
  margin: 0;
  color: var(--color-text-inverse-muted);
}

.field-plan-card__alert {
  color: var(--color-status-attention);
}
</style>
