<script lang="ts">
export type AvatarTone = 'coffee' | 'green' | 'earth' | 'forest' | 'dark'
export type AvatarSize = 'sm' | 'md' | 'xl'

const TONES: readonly AvatarTone[] = ['coffee', 'green', 'earth', 'forest', 'dark']

/** Stable tone for a person: the same id always gets the same color. */
export function avatarToneFor(seed: string): AvatarTone {
  const hash = [...seed].reduce((sum, char) => sum + char.charCodeAt(0), 0)
  return TONES[hash % TONES.length] ?? 'coffee'
}
</script>

<script setup lang="ts">
/**
 * Avatar with initials (library "Avatar" 26:173: white ring, earth/green tones).
 * sm = 32 px (sidebar user, farm card, wizard), md = 38 px (owner/user tables),
 * xl = 120 px (profile identity, Figma 54:904).
 * Decorative by default: the person's name is shown next to it.
 */
const props = withDefaults(
  defineProps<{
    initials: string
    /** Picks a stable tone (e.g. the person id) when `tone` is not given. */
    seed?: string
    tone?: AvatarTone
    size?: AvatarSize
  }>(),
  { seed: undefined, tone: undefined, size: 'sm' },
)

const TEXT_CLASS: Record<AvatarSize, string> = { sm: 'text-label-badge', md: 'text-label-strong', xl: 'text-h2' }

const resolvedTone = computed<AvatarTone>(() => props.tone ?? avatarToneFor(props.seed ?? props.initials))
</script>

<template>
  <span
    class="base-avatar"
    :class="[`base-avatar--${size}`, `base-avatar--${resolvedTone}`, TEXT_CLASS[size]]"
    aria-hidden="true"
  >
    {{ initials }}
  </span>
</template>

<style scoped>
.base-avatar {
  display: grid;
  place-items: center;
  flex-shrink: 0;
  border: var(--border-width-strong) solid var(--color-white);
  border-radius: var(--radius-pill);
  color: var(--color-text-inverse);
}

.base-avatar--sm {
  width: var(--size-avatar-sm);
  height: var(--size-avatar-sm);
}

.base-avatar--md {
  width: var(--size-avatar-md);
  height: var(--size-avatar-md);
}

.base-avatar--xl {
  width: var(--size-avatar-xl);
  height: var(--size-avatar-xl);
  border-width: var(--border-width-heavy);
}

.base-avatar--coffee {
  background: var(--color-coffee-600);
}

.base-avatar--green {
  background: var(--color-green-600);
}

.base-avatar--earth {
  background: var(--color-coffee-700);
}

.base-avatar--forest {
  background: var(--color-green-700);
}

.base-avatar--dark {
  background: var(--color-green-800);
}
</style>
