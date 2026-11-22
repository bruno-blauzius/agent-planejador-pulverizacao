<script setup lang="ts">
/** Context chip of the top bar (Figma 7:96), e.g. local weather summary. */
defineProps<{
  label: string
}>()

defineSlots<{
  /** Leading glyph (e.g. weather symbol). */
  leading?: () => unknown
}>()
</script>

<template>
  <div class="topbar-chip">
    <span v-if="$slots.leading" class="topbar-chip__leading text-body-lg" aria-hidden="true">
      <slot name="leading" />
    </span>
    <span class="topbar-chip__label text-label-meta">{{ label }}</span>
  </div>
</template>

<style scoped>
.topbar-chip {
  display: inline-flex;
  gap: var(--space-8);
  align-items: center;
  height: var(--size-topbar-control);
  padding: 0 var(--space-14);
  border: var(--border-width-hairline) solid var(--color-border);
  border-radius: var(--radius-md);
  background: var(--color-white);
  white-space: nowrap;
}

.topbar-chip__leading {
  display: inline-flex;
  color: var(--color-coffee-600);
}

.topbar-chip__label {
  color: var(--color-text-secondary);
}
</style>
