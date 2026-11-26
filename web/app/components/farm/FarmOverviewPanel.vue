<script setup lang="ts">
import type { FarmFactSectionView } from '~/utils/farmDetail'

/**
 * "Visão geral" tab of the farm screen. No Figma frame: the registry data in
 * the layout of the wizard review (35:1326), one card per group. Presentation
 * only; "Editar" belongs to the farm edit flow, not offered here yet.
 */
withDefaults(
  defineProps<{
    sections: readonly FarmFactSectionView[]
    loading?: boolean
  }>(),
  { loading: false },
)
</script>

<template>
  <div class="farm-overview" :aria-busy="loading || undefined">
    <div v-if="loading" class="farm-overview__skeleton" aria-label="Carregando dados da fazenda">
      <div v-for="index in 3" :key="index" class="farm-overview__skeleton-card" />
    </div>
    <section
      v-for="section in sections"
      v-else
      :key="section.title"
      class="farm-overview__section"
      :aria-label="section.title"
    >
      <h2 class="farm-overview__title text-label-strong">{{ section.title }}</h2>
      <dl class="farm-overview__facts">
        <div v-for="item in section.facts" :key="item.label" class="farm-overview__fact">
          <dt class="farm-overview__label text-body-sm">{{ item.label }}</dt>
          <dd class="farm-overview__value text-label-field" :class="{ 'farm-overview__value--empty': item.isEmpty }">
            {{ item.value }}
          </dd>
        </div>
      </dl>
    </section>
  </div>
</template>

<style scoped>
.farm-overview,
.farm-overview__skeleton {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(min(100%, 320px), 1fr));
  gap: var(--space-20);
  align-items: start;
}

.farm-overview__section {
  display: flex;
  flex-direction: column;
  gap: var(--space-14);
  padding: var(--space-18);
  border: var(--border-width-hairline) solid var(--color-border);
  border-radius: var(--radius-xl);
  background: var(--color-white);
}

.farm-overview__title {
  margin: 0;
  color: var(--color-text-primary);
}

.farm-overview__facts {
  display: flex;
  flex-direction: column;
  gap: var(--space-12);
  margin: 0;
}

.farm-overview__fact {
  display: grid;
  grid-template-columns: var(--review-label-width) minmax(0, 1fr);
  gap: var(--space-12);
  align-items: start;
}

.farm-overview__label {
  color: var(--color-text-tertiary);
}

.farm-overview__value {
  margin: 0;
  color: var(--color-text-primary);
  overflow-wrap: anywhere;
}

.farm-overview__value--empty {
  color: var(--color-text-tertiary);
}

.farm-overview__skeleton-card {
  height: 200px;
  border-radius: var(--radius-xl);
  background: var(--color-sand);
}
</style>
