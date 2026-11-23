<script setup lang="ts">
import { clampPage, pageSlots } from '~/utils/pagination'

/**
 * Table footer pagination (library "Paginação" 56:221, "Item de paginação"
 * 56:220): summary on the left, items-per-page menu and numbered pages on the
 * right. With a single page the summary stays and the links are hidden (Figma
 * note). Goes in DataTable's `#footer` slot.
 */
const props = withDefaults(
  defineProps<{
    /** Ready-made text, e.g. "Mostrando 1–6 de 9". */
    summary: string
    pageCount: number
    /** Options of the items-per-page menu; omit to hide it. */
    perPageOptions?: readonly number[]
    /** Accessible name of the navigation, e.g. "Paginação de usuários". */
    label?: string
  }>(),
  { perPageOptions: () => [], label: 'Paginação' },
)

const page = defineModel<number>('page', { required: true })
const perPage = defineModel<number>('perPage', { default: 10 })

const entries = computed(() => pageSlots(page.value, props.pageCount))

const hasPages = computed(() => props.pageCount > 1)

function goTo(value: number) {
  page.value = clampPage(value, props.pageCount)
}
</script>

<template>
  <div class="table-pagination">
    <p class="table-pagination__summary text-body-sm" aria-live="polite">{{ summary }}</p>

    <v-menu v-if="perPageOptions.length" location="top end">
      <template #activator="{ props: menuProps }">
        <button type="button" class="table-pagination__per-page text-label-badge" v-bind="menuProps">
          {{ perPage }} por página
          <AppIcon name="chevron-down" :size="14" />
        </button>
      </template>
      <ul class="table-pagination__menu" aria-label="Itens por página">
        <li v-for="option in perPageOptions" :key="option">
          <button
            type="button"
            class="table-pagination__option text-label-field"
            :aria-current="option === perPage ? 'true' : undefined"
            @click="perPage = option"
          >
            {{ option }} por página
          </button>
        </li>
      </ul>
    </v-menu>

    <nav v-if="hasPages" class="table-pagination__pages" :aria-label="label">
      <button
        type="button"
        class="table-pagination__item"
        aria-label="Página anterior"
        :disabled="page <= 1"
        @click="goTo(page - 1)"
      >
        <AppIcon name="chevron-right" :size="16" class="table-pagination__previous" />
      </button>
      <template v-for="entry in entries" :key="entry.kind === 'page' ? entry.value : entry.key">
        <span v-if="entry.kind === 'gap'" class="table-pagination__gap text-label-field" aria-hidden="true">…</span>
        <button
          v-else
          type="button"
          class="table-pagination__item text-label-field"
          :class="{ 'table-pagination__item--current': entry.value === page }"
          :aria-current="entry.value === page ? 'page' : undefined"
          :aria-label="`Página ${entry.value}`"
          @click="goTo(entry.value)"
        >
          {{ entry.value }}
        </button>
      </template>
      <button
        type="button"
        class="table-pagination__item"
        aria-label="Próxima página"
        :disabled="page >= pageCount"
        @click="goTo(page + 1)"
      >
        <AppIcon name="chevron-right" :size="16" />
      </button>
    </nav>
  </div>
</template>

<style scoped>
.table-pagination {
  display: flex;
  flex: 1;
  flex-wrap: wrap;
  gap: var(--space-12);
  align-items: center;
}

.table-pagination__summary {
  flex: 1 1 auto;
  margin: 0;
  color: var(--color-text-tertiary);
}

.table-pagination__per-page {
  display: inline-flex;
  gap: var(--space-6);
  align-items: center;
  padding: var(--space-8) var(--space-10);
  border: var(--border-width-hairline) solid var(--color-border);
  border-radius: var(--radius-sm);
  background: var(--color-white);
  color: var(--color-text-secondary);
  cursor: pointer;
}

.table-pagination__menu {
  min-width: var(--menu-min-width);
  margin: 0;
  padding: var(--space-6);
  border-radius: var(--radius-md);
  background: var(--color-white);
  box-shadow: var(--shadow-floating);
  list-style: none;
}

.table-pagination__option {
  width: 100%;
  padding: var(--space-8) var(--space-10);
  border: 0;
  border-radius: var(--radius-sm);
  background: transparent;
  color: var(--color-text-primary);
  text-align: left;
  cursor: pointer;
}

.table-pagination__option:hover,
.table-pagination__option[aria-current='true'] {
  background: var(--color-sand);
}

.table-pagination__pages {
  display: flex;
  gap: var(--space-6);
  align-items: center;
}

.table-pagination__item {
  display: grid;
  place-items: center;
  width: var(--size-icon-tile-md);
  height: var(--size-icon-tile-md);
  border: var(--border-width-hairline) solid var(--color-border);
  border-radius: var(--radius-sm);
  background: var(--color-white);
  color: var(--color-text-secondary);
  cursor: pointer;
}

.table-pagination__item:hover:not(:disabled, .table-pagination__item--current) {
  background: var(--color-sand);
}

.table-pagination__item:disabled {
  color: var(--color-text-tertiary);
  cursor: not-allowed;
}

.table-pagination__item--current {
  border-color: var(--color-green-800);
  background: var(--color-green-800);
  color: var(--color-text-inverse);
}

.table-pagination__per-page:focus-visible,
.table-pagination__item:focus-visible,
.table-pagination__option:focus-visible {
  outline: none;
  box-shadow: var(--focus-ring);
}

.table-pagination__previous {
  transform: rotate(180deg);
}

.table-pagination__gap {
  display: grid;
  place-items: center;
  width: var(--size-icon-tile-md);
  color: var(--color-text-tertiary);
}
</style>
