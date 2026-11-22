<script setup lang="ts">
/**
 * Company status with a colored dot (Figma 52:988): Ativa (green),
 * Em avaliação · N dias (amber), Inativa (grey). Label and tone come ready.
 */
defineProps<{
  label: string
  tone: 'success' | 'warning' | 'neutral'
}>()
</script>

<template>
  <span class="company-status text-label-badge" :class="`company-status--${tone}`">
    <span class="company-status__dot" aria-hidden="true" />
    {{ label }}
  </span>
</template>

<style scoped>
.company-status {
  display: inline-flex;
  gap: var(--space-6);
  align-items: center;
  white-space: nowrap;
}

.company-status__dot {
  flex-shrink: 0;
  width: var(--size-status-dot);
  height: var(--size-status-dot);
  border-radius: var(--radius-pill);
  background: currentColor;
}

.company-status--success {
  color: var(--color-status-suitable);
}

.company-status--warning {
  color: var(--color-status-attention);
}

.company-status--neutral {
  color: var(--color-text-tertiary);
}
</style>
