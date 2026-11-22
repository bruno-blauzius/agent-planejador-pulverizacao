<script setup lang="ts" generic="T extends { id: string }">
/**
 * Data table card (Figma 39:633; also the farm list mode). Facade over
 * v-data-table (ADR 0011: used in two screens). Card with optional toolbar
 * (`#toolbar`) and footer (`#footer`), sand header with overline labels,
 * 1 px row separators and an optional highlighted row.
 * Cell slots are forwarded as in Vuetify: `#item.<key>="{ item }"`.
 */
export interface DataTableHeader {
  readonly title: string
  readonly key: string
  readonly width?: string
  readonly align?: 'start' | 'end' | 'center'
  /** Header text only for assistive tech (e.g. an actions column). */
  readonly srOnly?: boolean
}

const props = withDefaults(
  defineProps<{
    headers: readonly DataTableHeader[]
    items: readonly T[]
    /** Accessible name of the table. */
    label: string
    /** Row that gets the "just saved" background (Figma 39:1426). */
    highlightedId?: string | null
    /** Makes rows clickable (emits `row-click`). Keep a link/button in the row for keyboard users. */
    clickable?: boolean
  }>(),
  { highlightedId: null, clickable: false },
)

const emit = defineEmits<{
  'row-click': [item: T]
}>()

const tableHeaders = computed(() =>
  props.headers.map((header) => ({ ...header, sortable: false })),
)

const srOnlyHeaders = computed(() => props.headers.filter((header) => header.srOnly))

function rowProps({ item }: { item: T }) {
  return { class: { 'data-table__row--highlighted': item.id === props.highlightedId } }
}

function onRowClick(_event: Event, row: { item: T }) {
  emit('row-click', row.item)
}

// Vuetify marks rows as clickable (pointer cursor) whenever the listener exists: bind it only when clickable.
const rowListeners = computed(() => (props.clickable ? { 'onClick:row': onRowClick } : {}))
</script>

<template>
  <section class="data-table" :class="{ 'data-table--clickable': clickable }" :aria-label="label">
    <div v-if="$slots.toolbar" class="data-table__toolbar">
      <slot name="toolbar" />
    </div>
    <v-data-table
      class="data-table__table"
      :headers="tableHeaders"
      :items="[...items]"
      item-value="id"
      :items-per-page="-1"
      :row-props="rowProps"
      hide-default-footer
      mobile-breakpoint="md"
      v-bind="rowListeners"
    >
      <template v-for="(_, name) in $slots" #[name]="slotProps">
        <slot :name="name" v-bind="slotProps ?? {}" />
      </template>
      <template v-for="header in srOnlyHeaders" :key="header.key" #[`header.${header.key}`]>
        <span class="visually-hidden">{{ header.title }}</span>
      </template>
    </v-data-table>
    <div v-if="$slots.footer" class="data-table__footer text-body-sm">
      <slot name="footer" />
    </div>
  </section>
</template>

<style scoped>
.data-table {
  overflow: hidden;
  border: var(--border-width-hairline) solid var(--color-border);
  border-radius: var(--radius-xl);
  background: var(--color-white);
}

.data-table__toolbar {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-10);
  align-items: center;
  padding: var(--space-16);
}

.data-table__table {
  background: transparent;
}

.data-table__table :deep(thead th) {
  height: auto;
  padding: var(--space-12) var(--space-20);
  border-bottom: 0;
  background: var(--color-sand);
  color: var(--color-text-tertiary);
  font: var(--font-overline);
  letter-spacing: var(--tracking-overline-table);
  text-transform: uppercase;
}

.data-table__table :deep(tbody td) {
  height: auto;
  padding: var(--space-14) var(--space-20);
  border-top: var(--border-width-hairline) solid var(--color-border);
  border-bottom: 0;
  color: var(--color-text-secondary);
  font: var(--font-body-md);
}

.data-table__table :deep(tbody tr:hover) {
  background: var(--color-sand);
}

.data-table__table :deep(tbody tr.data-table__row--highlighted) {
  background: var(--color-green-100);
}

/* Below the md breakpoint each row is stacked: label above a left-aligned, full-width value. */
.data-table__table :deep(.v-data-table__tr--mobile > td) {
  display: flex;
  flex-direction: column;
  gap: var(--space-4);
  align-items: stretch;
  min-height: 0;
  padding: var(--space-6) var(--space-20);
  border-top: 0;
  text-align: start;
}

.data-table__table :deep(.v-data-table__tr--mobile .v-data-table__td-value) {
  text-align: start;
}

.data-table__table :deep(.v-data-table__tr--mobile > td:first-child) {
  padding-top: var(--space-14);
  border-top: var(--border-width-hairline) solid var(--color-border);
}

.data-table__table :deep(.v-data-table__td-title) {
  color: var(--color-text-tertiary);
  font: var(--font-overline);
  letter-spacing: var(--tracking-overline-table);
  text-transform: uppercase;
}

.data-table--clickable .data-table__table :deep(tbody tr) {
  cursor: pointer;
}

.data-table__footer {
  display: flex;
  gap: var(--space-12);
  align-items: center;
  justify-content: space-between;
  padding: var(--space-14) var(--space-20);
  border-top: var(--border-width-hairline) solid var(--color-border);
  color: var(--color-text-tertiary);
}
</style>
