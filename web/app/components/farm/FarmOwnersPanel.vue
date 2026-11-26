<script setup lang="ts">
import type { FarmOwnerRowView } from '~/utils/farmDetail'

/**
 * "Proprietários" tab of the farm screen. No Figma frame: the links of the
 * farm in the card style of the screen, each opening the owner modal
 * (screen 4) where data, links and documents are edited.
 */
withDefaults(
  defineProps<{
    owners: readonly FarmOwnerRowView[]
    loading?: boolean
  }>(),
  { loading: false },
)

const TONES = ['coffee', 'green', 'dark'] as const
</script>

<template>
  <section class="farm-owners" aria-label="Proprietários da fazenda" :aria-busy="loading || undefined">
    <div v-if="loading" class="farm-owners__skeleton" aria-label="Carregando proprietários">
      <div v-for="index in 3" :key="index" class="farm-owners__skeleton-row" />
    </div>
    <EmptyState
      v-else-if="!owners.length"
      icon="users"
      title="Nenhum proprietário vinculado"
      description="Vincule proprietários na tela de Proprietários para assinar o registro de aplicações."
    />
    <ul v-else class="farm-owners__list">
      <li v-for="(owner, index) in owners" :key="owner.ownerId">
        <NuxtLink :to="owner.to" class="farm-owners__row">
          <BaseAvatar :initials="owner.initials" :tone="TONES[index % TONES.length]" />
          <span class="farm-owners__identity">
            <span class="farm-owners__name text-label-strong">{{ owner.name }}</span>
            <span class="farm-owners__role text-caption">{{ owner.roleLabel }}</span>
          </span>
          <span v-if="owner.shareLabel" class="farm-owners__share text-label-field">
            {{ owner.shareLabel }}<span class="visually-hidden"> de participação</span>
          </span>
          <AppIcon name="chevron-right" :size="16" class="farm-owners__chevron" aria-hidden="true" />
        </NuxtLink>
      </li>
    </ul>
  </section>
</template>

<style scoped>
.farm-owners {
  max-width: 720px;
  padding: var(--space-18);
  border: var(--border-width-hairline) solid var(--color-border);
  border-radius: var(--radius-xl);
  background: var(--color-white);
}

.farm-owners__list {
  display: flex;
  flex-direction: column;
  gap: var(--space-8);
  margin: 0;
  padding: 0;
  list-style: none;
}

.farm-owners__row {
  display: flex;
  gap: var(--space-12);
  align-items: center;
  padding: var(--space-12);
  border-radius: var(--radius-lg);
  color: inherit;
  text-decoration: none;
}

.farm-owners__row:hover {
  background: var(--color-sand);
}

.farm-owners__row:focus-visible {
  outline: none;
  box-shadow: var(--focus-ring);
}

.farm-owners__identity {
  display: flex;
  flex: 1;
  flex-direction: column;
  min-width: 0;
}

.farm-owners__name {
  overflow: hidden;
  color: var(--color-text-primary);
  text-overflow: ellipsis;
  white-space: nowrap;
}

.farm-owners__role {
  color: var(--color-text-tertiary);
}

.farm-owners__share {
  color: var(--color-text-primary);
}

.farm-owners__chevron {
  color: var(--color-text-tertiary);
}

.farm-owners__skeleton {
  display: flex;
  flex-direction: column;
  gap: var(--space-8);
}

.farm-owners__skeleton-row {
  height: 56px;
  border-radius: var(--radius-lg);
  background: var(--color-sand);
}
</style>
