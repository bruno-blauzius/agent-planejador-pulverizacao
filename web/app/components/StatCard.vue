<script setup lang="ts">
/**
 * Stat card ("Cartão de indicador", library 31:177; instances 52:949, 54:635,
 * farm list 7:118): overline label, big value and an optional detail line.
 * `highlight` = the dark card tied to the agent. `size="lg"` is the larger
 * value of the farm list (36 px); the library default is `md` (H2, 32 px).
 */
withDefaults(
  defineProps<{
    label: string
    value: string
    caption?: string | null
    highlight?: boolean
    size?: 'md' | 'lg'
  }>(),
  { caption: null, highlight: false, size: 'md' },
)
</script>

<template>
  <section class="stat-card" :class="{ 'stat-card--highlight': highlight }" :aria-label="label">
    <p class="stat-card__label text-overline" aria-hidden="true">{{ label }}</p>
    <p class="stat-card__value" :class="size === 'lg' ? 'text-page-title-sm' : 'text-h2'">{{ value }}</p>
    <p v-if="caption" class="stat-card__caption" :class="size === 'lg' ? 'text-caption-md' : 'text-body-sm'">
      {{ caption }}
    </p>
  </section>
</template>

<style scoped>
.stat-card {
  display: flex;
  flex-direction: column;
  gap: var(--space-8);
  min-width: 0;
  padding: var(--space-22);
  border: var(--border-width-hairline) solid var(--color-border);
  border-radius: var(--radius-card-lg);
  background: var(--color-white);
}

.stat-card__label {
  color: var(--color-text-tertiary);
}

.stat-card__value {
  overflow: hidden;
  color: var(--color-text-primary);
  text-overflow: ellipsis;
  white-space: nowrap;
}

.stat-card__caption {
  color: var(--color-text-secondary);
}

.stat-card--highlight {
  border-color: var(--color-green-900);
  background: var(--color-green-900);
}

.stat-card--highlight .stat-card__label {
  color: var(--color-sprout-400);
}

.stat-card--highlight .stat-card__value {
  color: var(--color-text-inverse);
}

.stat-card--highlight .stat-card__caption {
  color: var(--color-text-inverse-body);
}
</style>
