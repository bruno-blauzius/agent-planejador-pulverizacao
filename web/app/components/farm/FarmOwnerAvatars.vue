<script setup lang="ts">
/**
 * Overlapping owner initials (Figma 7:198). Colors cycle through the Figma
 * avatar tones; the full names are exposed to assistive tech.
 */
const props = withDefaults(
  defineProps<{
    owners: readonly { readonly initials: string, readonly name: string }[]
    /** Avatars shown before collapsing into "+N". */
    max?: number
  }>(),
  { max: 3 },
)

const TONES = ['coffee', 'green', 'dark'] as const

const visible = computed(() => props.owners.slice(0, props.max))
const hiddenCount = computed(() => Math.max(props.owners.length - props.max, 0))
const accessibleLabel = computed(() => `Proprietários: ${props.owners.map((owner) => owner.name).join(', ')}`)
</script>

<template>
  <span v-if="owners.length" class="owner-avatars" role="img" :aria-label="accessibleLabel">
    <BaseAvatar
      v-for="(owner, index) in visible"
      :key="`${index}-${owner.name}`"
      class="owner-avatars__item"
      :initials="owner.initials"
      :tone="TONES[index % TONES.length]"
      :title="owner.name"
    />
    <span v-if="hiddenCount" class="owner-avatars__item owner-avatars__item--more text-label-badge" aria-hidden="true">
      +{{ hiddenCount }}
    </span>
  </span>
</template>

<style scoped>
.owner-avatars {
  display: inline-flex;
  flex-shrink: 0;
}

.owner-avatars__item {
  display: grid;
  place-items: center;
  width: var(--size-avatar-sm);
  height: var(--size-avatar-sm);
  border: var(--border-width-strong) solid var(--color-white);
  border-radius: var(--radius-pill);
  color: var(--color-text-inverse);
}

/* Figma overlap: each avatar slides 8 px under the next one. */
.owner-avatars__item + .owner-avatars__item {
  margin-left: calc(-1 * var(--space-8));
}

.owner-avatars__item--more {
  background: var(--color-sand);
  color: var(--color-text-secondary);
}
</style>
