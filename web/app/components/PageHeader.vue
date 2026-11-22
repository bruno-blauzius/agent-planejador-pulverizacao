<script setup lang="ts">
/**
 * Screen header (Figma 7:103, 15:143, 14:117): optional breadcrumb or eyebrow,
 * title with optional badge slot, optional subtitle and an actions slot.
 * Screens with a breadcrumb use the smaller title (36 px); the others 40 px.
 */
export interface BreadcrumbItem {
  label: string
  to?: string
}

const props = withDefaults(
  defineProps<{
    title: string
    eyebrow?: string
    subtitle?: string
    breadcrumbs?: readonly BreadcrumbItem[]
  }>(),
  { eyebrow: undefined, subtitle: undefined, breadcrumbs: () => [] },
)

defineSlots<{
  /** Inline badge next to the title (e.g. StatusBadge). */
  badge?: () => unknown
  /** Action buttons aligned to the bottom-right. */
  actions?: () => unknown
}>()

const hasBreadcrumbs = computed(() => props.breadcrumbs.length > 0)
const titleClass = computed(() => (hasBreadcrumbs.value ? 'text-page-title-sm' : 'text-page-title'))

function isLast(index: number): boolean {
  return index === props.breadcrumbs.length - 1
}
</script>

<template>
  <header class="page-header">
    <div class="page-header__main">
      <nav v-if="hasBreadcrumbs" aria-label="Trilha de navegação">
        <ol class="page-header__breadcrumbs">
          <li v-for="(crumb, index) in breadcrumbs" :key="`${index}-${crumb.label}`" class="page-header__crumb text-label-meta">
            <NuxtLink
              v-if="crumb.to && !isLast(index)"
              :to="crumb.to"
              class="page-header__crumb-link"
            >
              {{ crumb.label }}
            </NuxtLink>
            <span
              v-else
              class="page-header__crumb-text"
              :class="{ 'page-header__crumb-text--current': isLast(index) }"
              :aria-current="isLast(index) ? 'page' : undefined"
            >
              {{ crumb.label }}
            </span>
            <AppIcon
              v-if="!isLast(index)"
              name="chevron-right"
              :size="14"
              class="page-header__separator"
            />
          </li>
        </ol>
      </nav>
      <p v-else-if="eyebrow" class="page-header__eyebrow text-label-md">{{ eyebrow }}</p>

      <div class="page-header__title-row">
        <h1 class="page-header__title" :class="titleClass">{{ title }}</h1>
        <slot name="badge" />
      </div>

      <p v-if="subtitle" class="page-header__subtitle text-body-md">{{ subtitle }}</p>
    </div>

    <div v-if="$slots.actions" class="page-header__actions">
      <slot name="actions" />
    </div>
  </header>
</template>

<style scoped>
.page-header {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-16);
  align-items: flex-end;
}

.page-header__main {
  display: flex;
  flex: 1 1 var(--page-header-main-min-width);
  flex-direction: column;
  gap: var(--space-8);
  min-width: 0;
}

.page-header__breadcrumbs {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-6);
  align-items: center;
  margin: 0;
  padding: 0;
  list-style: none;
}

.page-header__crumb {
  display: inline-flex;
  gap: var(--space-6);
  align-items: center;
}

.page-header__crumb-link,
.page-header__crumb-text {
  color: var(--color-text-tertiary);
  text-decoration: none;
}

.page-header__crumb-link:hover {
  color: var(--color-text-secondary);
  text-decoration: underline;
}

.page-header__crumb-text--current {
  color: var(--color-green-700);
}

.page-header__separator {
  color: var(--color-text-tertiary);
}

.page-header__eyebrow {
  color: var(--color-coffee-600);
}

.page-header__title-row {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-12);
  align-items: center;
}

.page-header__title {
  margin: 0;
  color: var(--color-text-primary);
}

.page-header__subtitle {
  color: var(--color-text-secondary);
}

.page-header__actions {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-16);
  align-items: center;
}
</style>
