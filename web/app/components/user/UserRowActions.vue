<script setup lang="ts">
import type { UserRowAction } from '~/utils/userRow'

/**
 * "More options" menu of a user row (Figma 52:425). Which actions exist and
 * whether they are enabled comes from the logic (capabilities, ADR 0013); the
 * menu only renders them and emits the chosen action (the page confirms it
 * when the action asks for confirmation).
 */
defineProps<{
  actions: readonly UserRowAction[]
  /** Person the menu refers to, for the button's accessible name. */
  userName: string
}>()

const emit = defineEmits<{
  select: [action: UserRowAction]
}>()
</script>

<template>
  <v-menu v-if="actions.length" location="bottom end">
    <template #activator="{ props: menuProps }">
      <button
        type="button"
        class="user-actions__trigger"
        :aria-label="`Mais opções para ${userName}`"
        :title="`Mais opções para ${userName}`"
        v-bind="menuProps"
      >
        <AppIcon name="more" :size="18" />
      </button>
    </template>
    <ul class="user-actions__menu" :aria-label="`Ações para ${userName}`">
      <li v-for="action in actions" :key="action.kind">
        <button
          type="button"
          class="user-actions__item text-label-field"
          :class="{ 'user-actions__item--danger': action.danger }"
          :aria-disabled="action.disabledHint ? 'true' : undefined"
          :title="action.disabledHint ?? undefined"
          @click="!action.disabledHint && emit('select', action)"
        >
          {{ action.label }}
          <span v-if="action.disabledHint" class="user-actions__hint text-caption">{{ action.disabledHint }}</span>
        </button>
      </li>
    </ul>
  </v-menu>
</template>

<style scoped>
.user-actions__trigger {
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

.user-actions__trigger:hover {
  background: var(--color-sand);
  color: var(--color-text-primary);
}

.user-actions__trigger:focus-visible,
.user-actions__item:focus-visible {
  outline: none;
  box-shadow: var(--focus-ring);
}

.user-actions__menu {
  min-width: var(--menu-min-width);
  margin: 0;
  padding: var(--space-6);
  border-radius: var(--radius-md);
  background: var(--color-white);
  box-shadow: var(--shadow-floating);
  list-style: none;
}

.user-actions__item {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  width: 100%;
  padding: var(--space-8) var(--space-10);
  border: 0;
  border-radius: var(--radius-sm);
  background: transparent;
  color: var(--color-text-primary);
  text-align: left;
  cursor: pointer;
}

.user-actions__item:hover:not([aria-disabled='true']) {
  background: var(--color-sand);
}

.user-actions__item[aria-disabled='true'] {
  color: var(--color-text-tertiary);
  cursor: not-allowed;
}

.user-actions__item--danger {
  color: var(--color-status-unsuitable);
}

.user-actions__hint {
  color: var(--color-text-tertiary);
}
</style>
