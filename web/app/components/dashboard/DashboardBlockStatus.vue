<script setup lang="ts">
/**
 * Placeholder of a dashboard block without items. Loading shows skeleton
 * lines (aria-busy), a failure says the data is unavailable, and only data
 * that loaded successfully may say the block is empty (the default slot).
 */
withDefaults(
  defineProps<{
    pending?: boolean
    failed?: boolean
  }>(),
  { pending: false, failed: false },
)

defineSlots<{
  /** Empty message, shown only when the data loaded. */
  default?: () => unknown
}>()

const SKELETON_LINES = 3
</script>

<template>
  <div v-if="pending" class="block-status block-status--loading" aria-busy="true">
    <span class="visually-hidden" role="status">Carregando…</span>
    <span v-for="line in SKELETON_LINES" :key="line" class="block-status__skeleton" aria-hidden="true" />
  </div>
  <p v-else-if="failed" class="block-status text-body-md" role="status">Dados indisponíveis no momento.</p>
  <p v-else class="block-status text-body-md"><slot /></p>
</template>

<style scoped>
.block-status {
  margin: 0;
  color: var(--color-text-secondary);
}

.block-status--loading {
  display: flex;
  flex-direction: column;
  gap: var(--space-12);
}

.block-status__skeleton {
  height: var(--size-file-tile);
  border-radius: var(--radius-md);
  background: var(--color-sand);
  animation: block-status-pulse var(--duration-skeleton) var(--easing-standard) infinite;
}

@keyframes block-status-pulse {
  50% {
    opacity: var(--opacity-skeleton-dim);
  }
}

@media (prefers-reduced-motion: reduce) {
  .block-status__skeleton {
    animation: none;
  }
}
</style>
