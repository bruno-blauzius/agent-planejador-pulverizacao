<script setup lang="ts">
import type { FarmCardView } from '~/utils/farmCard'

/**
 * Farm card of the list (Figma 7:173): perimeter thumbnail with status badge
 * and menu, name, location, info chips, owners and the agent headline.
 * The whole card opens the farm (stretched link on the title).
 */
defineProps<{
  farm: FarmCardView
}>()
</script>

<template>
  <article class="farm-card">
    <MapView
      class="farm-card__map"
      variant="thumbnail"
      :height="'var(--farm-card-map-height)'"
      :perimeter="farm.perimeter"
      :alt="`Perímetro da ${farm.name}`"
    >
      <StatusBadge
        v-if="farm.condition"
        class="farm-card__badge"
        :tone="farm.condition.value"
        :label="farm.condition.label"
      />
      <v-menu location="bottom end">
        <template #activator="{ props: menuProps }">
          <button
            type="button"
            class="farm-card__menu"
            :aria-label="`Mais opções da ${farm.name}`"
            v-bind="menuProps"
          >
            <AppIcon name="more" :size="18" />
          </button>
        </template>
        <nav class="farm-card__menu-list" :aria-label="`Opções da ${farm.name}`">
          <NuxtLink :to="farm.to" class="farm-card__menu-item text-label-meta">Abrir</NuxtLink>
        </nav>
      </v-menu>
    </MapView>

    <div class="farm-card__body">
      <div class="farm-card__heading">
        <div class="farm-card__titles">
          <h3 class="farm-card__name text-title-md">
            <NuxtLink :to="farm.to" class="farm-card__link">{{ farm.name }}</NuxtLink>
          </h3>
          <p class="farm-card__location text-caption-md">
            <AppIcon name="pin" :size="14" />
            {{ farm.locationLabel }}
          </p>
        </div>
        <FarmOwnerAvatars :owners="farm.owners" />
      </div>

      <ul class="farm-card__chips">
        <li class="farm-card__chip text-label-chip">{{ farm.areaLabel }}</li>
        <li class="farm-card__chip text-label-chip">{{ farm.fieldCountLabel }}</li>
        <li v-if="farm.cropLabel" class="farm-card__chip text-label-chip">{{ farm.cropLabel }}</li>
      </ul>

      <div class="farm-card__footer">
        <template v-if="farm.headline">
          <AppIcon name="sparkles" :size="16" class="farm-card__sparkles" />
          <span class="farm-card__headline text-label-meta">{{ farm.headline }}</span>
        </template>
        <span v-else class="farm-card__headline farm-card__headline--empty text-caption-md">Sem previsão</span>
        <span class="farm-card__open text-label-meta-strong" aria-hidden="true">
          Abrir
          <AppIcon name="chevron-right" :size="16" />
        </span>
      </div>
    </div>
  </article>
</template>

<style scoped>
.farm-card {
  position: relative;
  display: flex;
  flex-direction: column;
  overflow: hidden;
  border: var(--border-width-hairline) solid var(--color-border);
  border-radius: var(--radius-xl);
  background: var(--color-white);
  transition: box-shadow var(--duration-fast) var(--easing-standard);
}

.farm-card:hover {
  box-shadow: var(--shadow-card);
}

.farm-card:focus-within {
  box-shadow: var(--focus-ring);
}

.farm-card__badge {
  position: absolute;
  top: var(--space-16);
  left: var(--space-16);
}

.farm-card__menu {
  position: absolute;
  top: var(--space-14);
  right: var(--space-16);
  z-index: 1;
  display: grid;
  place-items: center;
  width: var(--size-icon-tile-md);
  height: var(--size-icon-tile-md);
  border: 0;
  border-radius: var(--radius-pill);
  background: var(--color-surface-over-map);
  color: var(--color-text-secondary);
  cursor: pointer;
}

.farm-card__menu:focus-visible {
  outline: none;
  box-shadow: var(--focus-ring);
}

.farm-card__menu-list {
  display: flex;
  flex-direction: column;
  min-width: var(--menu-min-width);
  padding: var(--space-4);
  border-radius: var(--radius-md);
  background: var(--color-white);
  box-shadow: var(--shadow-floating);
}

.farm-card__menu-item {
  padding: var(--space-10) var(--space-12);
  border-radius: var(--radius-sm);
  color: var(--color-text-primary);
  text-decoration: none;
}

.farm-card__menu-item:hover,
.farm-card__menu-item:focus-visible {
  outline: none;
  background: var(--color-sand);
}

.farm-card__body {
  display: flex;
  flex-direction: column;
  gap: var(--space-14);
  padding: var(--space-22);
}

.farm-card__heading {
  display: flex;
  gap: var(--space-8);
  align-items: center;
}

.farm-card__titles {
  display: flex;
  flex: 1;
  flex-direction: column;
  gap: var(--space-4);
  min-width: 0;
}

.farm-card__name {
  overflow: hidden;
  color: var(--color-text-primary);
  text-overflow: ellipsis;
  white-space: nowrap;
}

.farm-card__link {
  color: inherit;
  text-decoration: none;
  outline: none;
}

/* Stretched link: the whole card is clickable; the menu sits above it. */
.farm-card__link::after {
  content: '';
  position: absolute;
  inset: 0;
}

.farm-card__location {
  display: flex;
  gap: var(--space-6);
  align-items: center;
  color: var(--color-text-secondary);
}

.farm-card__chips {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-8);
  margin: 0;
  padding: 0;
  list-style: none;
}

.farm-card__chip {
  padding: var(--padding-badge-y) var(--space-10);
  border-radius: var(--radius-sm);
  background: var(--color-sand);
  color: var(--color-text-secondary);
}

.farm-card__footer {
  display: flex;
  gap: var(--space-8);
  align-items: center;
  padding-top: var(--space-14);
  border-top: var(--border-width-hairline) solid var(--color-border);
}

.farm-card__sparkles {
  color: var(--color-green-700);
}

.farm-card__headline {
  flex: 1;
  min-width: 0;
  color: var(--color-green-700);
}

.farm-card__headline--empty {
  color: var(--color-text-tertiary);
}

.farm-card__open {
  display: inline-flex;
  gap: var(--space-8);
  align-items: center;
  color: var(--color-green-800);
}
</style>
