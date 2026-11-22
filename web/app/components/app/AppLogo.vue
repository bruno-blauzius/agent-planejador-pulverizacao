<script setup lang="ts">
/** Brand mark (Figma 7:18 / 6:29): sprout tile with leaf + two-line wordmark. */
</script>

<template>
  <span class="app-logo">
    <span class="app-logo__mark" aria-hidden="true">
      <AppIcon name="leaf" :size="20" />
    </span>
    <span class="app-logo__wordmark">
      <span class="app-logo__first">agentes</span>
      <span class="app-logo__second">agro</span>
    </span>
  </span>
</template>

<style scoped>
.app-logo {
  display: inline-flex;
  gap: var(--space-10);
  align-items: center;
}

.app-logo__mark {
  display: grid;
  place-items: center;
  width: var(--size-icon-tile-md);
  height: var(--size-icon-tile-md);
  border-radius: var(--radius-tile);
  background: var(--color-sprout-400);
  color: var(--color-green-950);
}

.app-logo__wordmark {
  display: flex;
  flex-direction: column;
  white-space: nowrap;
}

.app-logo__first {
  font: var(--font-logo-primary);
  color: var(--color-text-inverse);
}

.app-logo__second {
  font: var(--font-logo-secondary);
  color: var(--color-sprout-400);
}
</style>
