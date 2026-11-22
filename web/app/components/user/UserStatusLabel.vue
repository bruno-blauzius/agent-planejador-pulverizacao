<script setup lang="ts">
import type { UserStatus } from '~/types/admin'

/**
 * Account status with a colored dot (Figma 52:422): Ativo (green), Convite
 * pendente (amber), Inativo (grey). The label comes ready from the logic.
 */
defineProps<{
  status: UserStatus
  label: string
}>()
</script>

<template>
  <span class="user-status text-label-badge" :class="`user-status--${status}`">
    <span class="user-status__dot" aria-hidden="true" />
    {{ label }}
  </span>
</template>

<style scoped>
.user-status {
  display: inline-flex;
  gap: var(--space-6);
  align-items: center;
  white-space: nowrap;
}

.user-status__dot {
  flex-shrink: 0;
  width: var(--size-status-dot);
  height: var(--size-status-dot);
  border-radius: var(--radius-pill);
  background: currentColor;
}

.user-status--active {
  color: var(--color-status-suitable);
}

.user-status--invited {
  color: var(--color-status-attention);
}

.user-status--inactive {
  color: var(--color-text-tertiary);
}
</style>
