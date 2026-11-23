<script setup lang="ts">
import type { CompanyRowAction, CompanyRowActionKind } from '~/utils/company'

/**
 * "More options" menu of a company row. No Figma frame exists for it: derived
 * from the users menu (UserRowActions, Figma 52:425). Which entries exist comes
 * from the logic (capability `companies.manage`, status); the menu only renders
 * them and emits the chosen kind (the page confirms deactivate/delete).
 */
defineProps<{
  actions: readonly CompanyRowAction[]
  /** Company the menu refers to, for the button's accessible name. */
  companyName: string
  /** Company id, exposed on the trigger so the page can find it again to return focus. */
  companyId: string
}>()

const isOpen = ref(false)

const emit = defineEmits<{
  select: [kind: CompanyRowActionKind]
}>()
</script>

<template>
  <v-menu v-if="actions.length" v-model="isOpen" location="bottom end">
    <template #activator="{ props: menuProps }">
      <button
        type="button"
        class="company-actions__trigger"
        aria-haspopup="menu"
        :aria-expanded="isOpen"
        :data-company-actions-trigger="companyId"
        :aria-label="`Mais opções para ${companyName}`"
        :title="`Mais opções para ${companyName}`"
        v-bind="menuProps"
      >
        <AppIcon name="more" :size="18" />
      </button>
    </template>
    <ul class="company-actions__menu" :aria-label="`Ações para ${companyName}`">
      <li v-for="action in actions" :key="action.kind">
        <button
          type="button"
          class="company-actions__item text-label-field"
          :class="{ 'company-actions__item--danger': action.tone === 'danger' }"
          @click="emit('select', action.kind)"
        >
          {{ action.label }}
        </button>
      </li>
    </ul>
  </v-menu>
</template>

<style scoped>
.company-actions__trigger {
  display: grid;
  place-items: center;
  width: var(--size-icon-tile-md);
  height: var(--size-icon-tile-md);
  border: 0;
  border-radius: var(--radius-tile);
  background: transparent;
  color: var(--color-text-secondary);
  cursor: pointer;
}

.company-actions__trigger:hover {
  background: var(--color-sand);
  color: var(--color-text-primary);
}

.company-actions__trigger:focus-visible,
.company-actions__item:focus-visible {
  outline: none;
  box-shadow: var(--focus-ring);
}

.company-actions__menu {
  min-width: var(--menu-min-width);
  margin: 0;
  padding: var(--space-6);
  border-radius: var(--radius-md);
  background: var(--color-white);
  box-shadow: var(--shadow-floating);
  list-style: none;
}

.company-actions__item {
  display: block;
  width: 100%;
  padding: var(--space-8) var(--space-10);
  border: 0;
  border-radius: var(--radius-sm);
  background: transparent;
  color: var(--color-text-primary);
  text-align: left;
  cursor: pointer;
}

.company-actions__item:hover {
  background: var(--color-sand);
}

.company-actions__item--danger {
  color: var(--color-status-unsuitable);
}
</style>
