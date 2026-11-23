<script setup lang="ts">
import type { IconName } from '../app/AppIcon.vue'

/**
 * "O agente consultou" (Figma 18:112): what the agent looked at before
 * answering. The status of each source comes ready from the logic.
 */
export interface ToolTraceItem {
  key: string
  icon: IconName
  title: string
  detail: string
  /** Defaults to `done`. */
  status?: 'done' | 'running' | 'failed'
}

withDefaults(
  defineProps<{
    items: readonly ToolTraceItem[]
    title?: string
  }>(),
  { title: 'O agente consultou' },
)

const STATUS_TEXT = { done: 'consultado', running: 'consultando', failed: 'falhou' } as const
const titleId = useId()
</script>

<template>
  <section class="tool-trace" :aria-labelledby="titleId">
    <h3 :id="titleId" class="tool-trace__title text-label-micro">{{ title }}</h3>
    <ul class="tool-trace__list" aria-live="polite" :aria-busy="items.some(item => item.status === 'running') || undefined">
      <li v-for="item in items" :key="item.key" class="tool-trace__item">
        <span class="tool-trace__icon" aria-hidden="true">
          <AppIcon :name="item.icon" :size="15" />
        </span>
        <span class="tool-trace__text">
          <span class="tool-trace__name text-label-chip">{{ item.title }}</span>
          <span class="tool-trace__detail text-caption-xs">{{ item.detail }}</span>
        </span>
        <span class="tool-trace__status" :class="`tool-trace__status--${item.status ?? 'done'}`">
          <span class="visually-hidden">{{ STATUS_TEXT[item.status ?? 'done'] }}</span>
          <span v-if="(item.status ?? 'done') === 'running'" class="tool-trace__spinner" aria-hidden="true" />
          <AppIcon v-else :name="item.status === 'failed' ? 'alert' : 'check'" :size="16" />
        </span>
      </li>
    </ul>
  </section>
</template>

<style scoped>
.tool-trace {
  display: flex;
  flex-direction: column;
  gap: var(--space-8);
  padding: var(--space-14);
  border-radius: var(--radius-lg);
  background: var(--color-sand);
}

.tool-trace__title {
  margin: 0;
  color: var(--color-text-tertiary);
  letter-spacing: var(--tracking-overline);
  text-transform: uppercase;
}

.tool-trace__list {
  display: flex;
  flex-direction: column;
  gap: var(--space-8);
  margin: 0;
  padding: 0;
  list-style: none;
}

.tool-trace__item {
  display: flex;
  gap: var(--space-10);
  align-items: center;
}

.tool-trace__icon {
  display: inline-flex;
  flex-shrink: 0;
  align-items: center;
  justify-content: center;
  width: var(--size-planner-trace-icon);
  height: var(--size-planner-trace-icon);
  border-radius: var(--radius-sm);
  background: var(--color-white);
  color: var(--color-green-700);
}

.tool-trace__text {
  display: flex;
  flex: 1 1 auto;
  flex-direction: column;
  min-width: 0;
}

.tool-trace__name {
  color: var(--color-text-primary);
}

.tool-trace__detail {
  color: var(--color-text-tertiary);
}

.tool-trace__status {
  display: inline-flex;
  flex-shrink: 0;
}

.tool-trace__status--done {
  color: var(--color-status-suitable);
}

.tool-trace__status--running {
  color: var(--color-text-tertiary);
}

.tool-trace__status--failed {
  color: var(--color-status-unsuitable);
}

.tool-trace__spinner {
  width: var(--size-spinner);
  height: var(--size-spinner);
  border: var(--border-width-strong) solid currentColor;
  border-right-color: transparent;
  border-radius: var(--radius-pill);
  animation: tool-trace-spin var(--duration-spinner) linear infinite;
}

@keyframes tool-trace-spin {
  to {
    transform: rotate(360deg);
  }
}

@media (prefers-reduced-motion: reduce) {
  .tool-trace__spinner {
    animation-duration: 2s;
  }
}
</style>
