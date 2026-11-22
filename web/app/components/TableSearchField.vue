<script setup lang="ts">
/**
 * Search box inside a table card toolbar (Figma 39:635, 52:377): sand box,
 * 40 px, search icon. Filters as the user types (v-model); a GET-free form so
 * Enter never navigates.
 */
defineProps<{
  /** Visible placeholder and accessible name. */
  label: string
}>()

const model = defineModel<string>({ default: '' })
const inputId = useId()
</script>

<template>
  <form class="table-search" role="search" @submit.prevent>
    <label :for="inputId" class="visually-hidden">{{ label }}</label>
    <AppIcon name="search" :size="16" class="table-search__icon" />
    <input
      :id="inputId"
      v-model="model"
      type="search"
      class="table-search__input text-body-md"
      :placeholder="label"
      autocomplete="off"
    >
  </form>
</template>

<style scoped>
.table-search {
  display: flex;
  flex: 1 1 var(--page-header-main-min-width);
  gap: var(--space-8);
  align-items: center;
  min-width: 0;
  height: var(--size-search-inline);
  padding: 0 var(--space-12);
  border-radius: var(--radius-tile);
  background: var(--color-sand);
}

.table-search:focus-within {
  box-shadow: var(--focus-ring);
}

.table-search__icon {
  flex-shrink: 0;
  color: var(--color-text-secondary);
}

.table-search__input {
  flex: 1;
  min-width: 0;
  padding: 0;
  border: 0;
  background: transparent;
  color: var(--color-text-primary);
  outline: none;
}

.table-search__input::placeholder {
  color: var(--color-text-tertiary);
}
</style>
