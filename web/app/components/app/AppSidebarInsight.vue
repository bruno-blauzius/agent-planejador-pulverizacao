<script setup lang="ts">
/** Agent insight card shown at the bottom of the sidebar (Figma 7:69). */
defineProps<{
  title: string
  message: string
}>()
</script>

<template>
  <section class="insight" aria-live="polite">
    <AppIcon name="sparkles" :size="18" class="insight__icon" />
    <p class="insight__title text-title-sm">{{ title }}</p>
    <p class="insight__message text-body-sm">{{ message }}</p>
  </section>
</template>

<style scoped>
.insight {
  display: flex;
  flex-direction: column;
  gap: var(--space-10);
  padding: var(--space-16);
  border-radius: var(--radius-lg);
  background: var(--color-surface-inverse-soft);
}

.insight__icon {
  color: var(--color-sprout-400);
}

.insight__title {
  color: var(--color-text-inverse);
}

.insight__message {
  color: var(--color-text-inverse-muted);
}
</style>
