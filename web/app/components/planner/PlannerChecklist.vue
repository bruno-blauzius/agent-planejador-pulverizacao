<script setup lang="ts">
import { CHECKLIST_STATUS_LABELS } from '~/utils/labels'

/**
 * "Checagens do agente" (Figma 19:446): the checks behind a plan, each with a
 * status icon. The status comes from the API; `passed` is the only state shown
 * in the Figma, `warning` and `failed` follow the same pattern.
 */
export type ChecklistStatus = 'passed' | 'warning' | 'failed'

export interface ChecklistItem {
  key: string
  title: string
  detail: string
  /** Defaults to `passed`. */
  status?: ChecklistStatus
}

withDefaults(
  defineProps<{
    items: readonly ChecklistItem[]
    title?: string
  }>(),
  { title: 'Checagens do agente' },
)

const STATUS_ICON = { passed: 'check', warning: 'alert', failed: 'x' } as const

const titleId = useId()
</script>

<template>
  <section class="checklist" :aria-labelledby="titleId">
    <h3 :id="titleId" class="checklist__title text-label-strong">{{ title }}</h3>
    <ul class="checklist__list">
      <li v-for="item in items" :key="item.key" class="checklist__item">
        <span class="checklist__badge" :class="`checklist__badge--${item.status ?? 'passed'}`" aria-hidden="true">
          <AppIcon :name="STATUS_ICON[item.status ?? 'passed']" :size="14" />
        </span>
        <span class="checklist__text">
          <span class="checklist__name text-label-field">
            <span class="visually-hidden">{{ CHECKLIST_STATUS_LABELS[item.status ?? 'passed'] }}: </span>{{ item.title }}
          </span>
          <span class="checklist__detail text-caption">{{ item.detail }}</span>
        </span>
      </li>
    </ul>
  </section>
</template>

<style scoped>
.checklist {
  display: flex;
  flex-direction: column;
  gap: var(--space-10);
}

.checklist__title {
  margin: 0;
  color: var(--color-text-primary);
}

.checklist__list {
  display: flex;
  flex-direction: column;
  gap: var(--space-10);
  margin: 0;
  padding: 0;
  list-style: none;
}

.checklist__item {
  display: flex;
  gap: var(--space-12);
  align-items: center;
}

.checklist__badge {
  display: inline-flex;
  flex-shrink: 0;
  align-items: center;
  justify-content: center;
  width: var(--size-planner-check-badge);
  height: var(--size-planner-check-badge);
  border-radius: var(--radius-pill);
}

.checklist__badge--passed {
  background: var(--color-status-suitable-bg);
  color: var(--color-status-suitable);
}

.checklist__badge--warning {
  background: var(--color-status-attention-bg);
  color: var(--color-status-attention);
}

.checklist__badge--failed {
  background: var(--color-status-unsuitable-bg);
  color: var(--color-status-unsuitable);
}

.checklist__text {
  display: flex;
  flex-direction: column;
  gap: 1px;
  min-width: 0;
}

.checklist__name {
  color: var(--color-text-primary);
}

.checklist__detail {
  color: var(--color-text-tertiary);
}
</style>
