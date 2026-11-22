<script setup lang="ts">
import type { UpcomingWindowView } from '~/utils/dashboard'

/**
 * "Próximas janelas" (Figma 54:703): field tile (green when suitable), when,
 * detail and the condition badge. "Ver todas" goes to the planner; while the
 * planner does not exist it is shown as unavailable.
 */
withDefaults(
  defineProps<{
    windows: readonly UpcomingWindowView[]
    /** Shows "Ver todas" (the user can use the planner). */
    showSeeAll?: boolean
    seeAllUnavailableHint?: string
  }>(),
  { showSeeAll: false, seeAllUnavailableHint: 'Em breve' },
)

const titleId = useId()
</script>

<template>
  <section class="upcoming" :aria-labelledby="titleId">
    <header class="upcoming__header">
      <h2 :id="titleId" class="upcoming__title text-h4">Próximas janelas</h2>
      <BaseButton v-if="showSeeAll" variant="ghost" size="sm" :unavailable-hint="seeAllUnavailableHint">
        Ver todas
      </BaseButton>
    </header>

    <p v-if="windows.length === 0" class="upcoming__empty text-body-md">Nenhuma janela prevista para os próximos dias.</p>
    <ul v-else class="upcoming__list">
      <li v-for="item in windows" :key="item.key" class="upcoming__item">
        <span class="upcoming__field text-label-strong" :class="{ 'upcoming__field--highlight': item.isHighlighted }">
          {{ item.fieldLabel }}
        </span>
        <span class="upcoming__text">
          <span class="upcoming__when text-label-field">{{ item.whenLabel }}</span>
          <span v-if="item.detail" class="upcoming__detail text-body-sm">{{ item.detail }}</span>
        </span>
        <StatusBadge :tone="item.condition.value" :label="item.condition.label" />
      </li>
    </ul>
  </section>
</template>

<style scoped>
.upcoming {
  display: flex;
  flex-direction: column;
  gap: var(--space-16);
  padding: var(--space-22);
  border: var(--border-width-hairline) solid var(--color-border);
  border-radius: var(--radius-xl);
  background: var(--color-white);
}

.upcoming__header {
  display: flex;
  gap: var(--space-8);
  align-items: center;
}

.upcoming__title {
  flex: 1;
  margin: 0;
  color: var(--color-text-primary);
}

.upcoming__list {
  display: flex;
  flex-direction: column;
  gap: var(--space-16);
  margin: 0;
  padding: 0;
  list-style: none;
}

.upcoming__item {
  display: flex;
  gap: var(--space-12);
  align-items: center;
  padding: var(--space-4) 0;
}

.upcoming__field {
  display: grid;
  flex-shrink: 0;
  place-items: center;
  width: var(--size-file-tile);
  height: var(--size-file-tile);
  border-radius: var(--radius-tile);
  background: var(--color-sand);
  color: var(--color-text-secondary);
}

.upcoming__field--highlight {
  background: var(--color-sprout-400);
  color: var(--color-green-950);
}

.upcoming__text {
  display: flex;
  flex: 1;
  flex-direction: column;
  gap: var(--space-2);
  min-width: 0;
}

.upcoming__when {
  color: var(--color-text-primary);
}

.upcoming__detail {
  color: var(--color-text-tertiary);
}

.upcoming__empty {
  margin: 0;
  color: var(--color-text-secondary);
}
</style>
