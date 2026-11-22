<script setup lang="ts">
/**
 * Status pill (Figma "Selo de status", 2:199). The tone is visual only;
 * which tone applies to a spray window comes ready from the API.
 * suitable = Apta · attention = Atenção · unsuitable = Inapta · rain = Chuva.
 */
export type StatusTone = 'suitable' | 'attention' | 'unsuitable' | 'rain'

const props = withDefaults(
  defineProps<{
    tone: StatusTone
    /** Overrides the default label of the tone. */
    label?: string
  }>(),
  { label: undefined },
)

const DEFAULT_LABEL: Record<StatusTone, string> = {
  suitable: 'Apta',
  attention: 'Atenção',
  unsuitable: 'Inapta',
  rain: 'Chuva',
}

const text = computed(() => props.label ?? DEFAULT_LABEL[props.tone])
</script>

<template>
  <!-- Facade over v-chip (ADR 0011): static, non-interactive pill. -->
  <v-chip
    class="status-badge text-label-badge"
    :class="`status-badge--${tone}`"
    variant="flat"
    :ripple="false"
    label
  >
    <span class="status-badge__dot" aria-hidden="true" />
    {{ text }}
  </v-chip>
</template>

<style scoped>
.status-badge {
  display: inline-flex;
  gap: var(--space-6);
  align-items: center;
  height: auto;
  padding: var(--padding-badge-y) var(--space-12) var(--padding-badge-y) var(--space-10);
  border-radius: var(--radius-pill);
  letter-spacing: 0;
  white-space: nowrap;
}

.status-badge :deep(.v-chip__overlay),
.status-badge :deep(.v-chip__underlay) {
  display: none;
}

.status-badge :deep(.v-chip__content) {
  gap: var(--space-6);
}

.status-badge__dot {
  width: var(--size-status-dot);
  height: var(--size-status-dot);
  border-radius: var(--radius-pill);
  background: currentColor;
}

.status-badge--suitable {
  background: var(--color-status-suitable-bg);
  color: var(--color-status-suitable);
}

.status-badge--attention {
  background: var(--color-status-attention-bg);
  color: var(--color-status-attention);
}

.status-badge--unsuitable {
  background: var(--color-status-unsuitable-bg);
  color: var(--color-status-unsuitable);
}

.status-badge--rain {
  background: var(--color-info-rain-bg);
  color: var(--color-info-rain);
}
</style>
