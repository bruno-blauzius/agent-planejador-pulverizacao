<script setup lang="ts">
import type { ActivityView } from '~/utils/dashboard'

/**
 * "Atividade recente" (Figma 54:789): who did what and when. The agent gets
 * the dark avatar with "IA".
 */
defineProps<{
  activity: readonly ActivityView[]
}>()

const titleId = useId()
</script>

<template>
  <section class="activity" :aria-labelledby="titleId">
    <h2 :id="titleId" class="activity__title text-h4">Atividade recente</h2>
    <p v-if="activity.length === 0" class="activity__empty text-body-md">Nenhuma atividade recente.</p>
    <ul v-else class="activity__list">
      <li v-for="entry in activity" :key="entry.id" class="activity__item">
        <BaseAvatar :initials="entry.initials" :seed="entry.actorName" :tone="entry.isAgent ? 'dark' : undefined" />
        <span class="activity__text">
          <span class="activity__line text-body-sm">
            <strong class="activity__actor">{{ entry.actorName }}</strong> {{ entry.description }}
          </span>
          <span class="activity__time text-body-sm">{{ entry.timeLabel }}</span>
        </span>
      </li>
    </ul>
  </section>
</template>

<style scoped>
.activity {
  display: flex;
  flex-direction: column;
  gap: var(--space-16);
  padding: var(--space-22);
  border: var(--border-width-hairline) solid var(--color-border);
  border-radius: var(--radius-xl);
  background: var(--color-white);
}

.activity__title {
  margin: 0;
  color: var(--color-text-primary);
}

.activity__list {
  display: flex;
  flex-direction: column;
  gap: var(--space-16);
  margin: 0;
  padding: 0;
  list-style: none;
}

.activity__item {
  display: flex;
  gap: var(--space-10);
  align-items: flex-start;
}

.activity__text {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  min-width: 0;
}

.activity__line {
  color: var(--color-text-secondary);
}

/* The actor name is bold inside the sentence (Figma 54:795). */
.activity__actor {
  font-weight: 700;
}

.activity__time {
  color: var(--color-text-tertiary);
}

.activity__empty {
  margin: 0;
  color: var(--color-text-secondary);
}
</style>
