<script setup lang="ts">
import type { RoleTone } from '~/utils/userRow'

/**
 * Role card of the users screen (Figma 52:318): role tag, how many users have
 * it, what the role is for and its main capabilities. Texts come ready from
 * the logic (the capabilities themselves are decided by the API, ADR 0013).
 */
defineProps<{
  label: string
  tone: RoleTone
  /** "1 usuário" / "5 usuários". */
  countLabel: string
  description: string
  capabilities: readonly string[]
}>()
</script>

<template>
  <article class="user-role-card">
    <header class="user-role-card__header">
      <h2 class="user-role-card__title"><UserRoleTag :label="label" :tone="tone" /></h2>
      <span class="user-role-card__count text-body-sm">{{ countLabel }}</span>
    </header>
    <p class="user-role-card__description text-body-sm">{{ description }}</p>
    <ul class="user-role-card__capabilities" :aria-label="`Capacidades de ${label}`">
      <li v-for="capability in capabilities" :key="capability" class="user-role-card__capability text-body-sm">
        <AppIcon name="check" :size="14" class="user-role-card__check" />
        {{ capability }}
      </li>
    </ul>
  </article>
</template>

<style scoped>
.user-role-card {
  display: flex;
  flex-direction: column;
  gap: var(--space-12);
  min-width: 0;
  padding: var(--space-20);
  border: var(--border-width-hairline) solid var(--color-border);
  border-radius: var(--radius-card-lg);
  background: var(--color-white);
}

.user-role-card__header {
  display: flex;
  gap: var(--space-8);
  align-items: center;
  justify-content: space-between;
}

.user-role-card__title {
  min-width: 0;
  margin: 0;
  font: inherit;
}

.user-role-card__count {
  flex-shrink: 0;
  color: var(--color-text-tertiary);
}

.user-role-card__description {
  margin: 0;
  color: var(--color-text-secondary);
}

.user-role-card__capabilities {
  display: flex;
  flex-direction: column;
  gap: var(--space-8);
  margin: 0;
  padding: 0;
  list-style: none;
}

.user-role-card__capability {
  display: flex;
  gap: var(--space-8);
  align-items: center;
  color: var(--color-text-primary);
}

.user-role-card__check {
  flex-shrink: 0;
  color: var(--color-status-suitable);
}
</style>
