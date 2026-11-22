<script setup lang="ts">
import type { AvatarTone } from '~/components/base/BaseAvatar.vue'
import { avatarToneFor } from '~/components/base/BaseAvatar.vue'

/**
 * Company cell of the companies table (Figma 52:975): square initials tile,
 * name and location. The tile tone is stable per company id.
 */
const props = defineProps<{
  id: string
  name: string
  initials: string
  locationLabel: string
}>()

const tone = computed<AvatarTone>(() => avatarToneFor(props.id))
</script>

<template>
  <span class="company-identity">
    <span class="company-identity__tile text-label-strong" :class="`company-identity__tile--${tone}`" aria-hidden="true">
      {{ initials }}
    </span>
    <span class="company-identity__text">
      <span class="company-identity__name text-label-strong">{{ name }}</span>
      <span class="company-identity__location text-body-sm">{{ locationLabel }}</span>
    </span>
  </span>
</template>

<style scoped>
.company-identity {
  display: flex;
  gap: var(--space-12);
  align-items: center;
  min-width: 0;
}

.company-identity__tile {
  display: grid;
  flex-shrink: 0;
  place-items: center;
  width: var(--size-file-tile);
  height: var(--size-file-tile);
  border-radius: var(--radius-tile);
  color: var(--color-text-inverse);
}

.company-identity__tile--coffee {
  background: var(--color-coffee-600);
}

.company-identity__tile--green {
  background: var(--color-green-600);
}

.company-identity__tile--earth {
  background: var(--color-coffee-700);
}

.company-identity__tile--forest {
  background: var(--color-green-700);
}

.company-identity__tile--dark {
  background: var(--color-green-800);
}

.company-identity__text {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  min-width: 0;
}

.company-identity__name {
  overflow: hidden;
  color: var(--color-text-primary);
  text-overflow: ellipsis;
  white-space: nowrap;
}

.company-identity__location {
  color: var(--color-text-tertiary);
}
</style>
