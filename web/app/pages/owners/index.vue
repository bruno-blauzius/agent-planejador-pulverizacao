<script setup lang="ts">
import type { FlashMessage } from '~/types/flash'
import type { DataTableHeader } from '~/components/DataTable.vue'
import type { FilterChipItem } from '~/components/FilterChips.vue'
import type { OwnerPersonFilter } from '~/composables/useOwnerListPage'

/**
 * "Proprietários" (Figma 39:539; after saving: 39:1426 with alert 39:1651).
 * Presentation only: filters, pagination, rows and modal routing come from
 * useOwnerListPage. The owner modal (screen 4) opens through `?owner=`.
 */
useHead({ title: 'Proprietários · Agentes Agro' })

const {
  search,
  personType,
  page,
  rows,
  eyebrow,
  pagination,
  pending,
  error,
  isEmpty,
  isFilterEmpty,
  refresh,
  modalOwnerId,
  modalStep,
  closeModal,
  openNew,
  openEdit,
  openLinks,
  takeFlash,
  canWrite,
} = useOwnerListPage()

const alert = ref<FlashMessage | null>(null)

const PERSON_FILTERS: readonly FilterChipItem<OwnerPersonFilter>[] = [
  { value: 'all', label: 'Todos' },
  { value: 'individual', label: 'PF' },
  { value: 'company', label: 'PJ' },
]

// Column widths from the Figma table (39:646).
const ALL_HEADERS: readonly DataTableHeader[] = [
  { title: 'Proprietário', key: 'name' },
  { title: 'Fazendas', key: 'farms', width: 'var(--owner-table-farms-width)' },
  { title: 'Part.', key: 'shareLabel', width: 'var(--owner-table-share-width)' },
  { title: 'Ações', key: 'actions', align: 'end', width: 'var(--size-icon-tile-md)', srOnly: true },
]

// Without registry.write the row has no actions ("Editar" and "Vínculos" are edits): drop the column.
const headers = computed(() => (canWrite.value ? ALL_HEADERS : ALL_HEADERS.filter((header) => header.key !== 'actions')))

const isFirstLoad = computed(() => pending.value && rows.value.length === 0)
const highlightedId = computed(() => rows.value.find((row) => row.isHighlighted)?.id ?? null)

function showFlash() {
  const message = takeFlash()
  if (message) alert.value = message
}

onMounted(showFlash)

const isNewOwner = computed(() => modalOwnerId.value === 'new')

/** Saved in the modal: close it (clears `?owner=`), then show the alert and highlight the row. */
function onOwnerSaved() {
  closeModal()
  showFlash()
}
</script>

<template>
  <div class="owners">
    <PageHeader :eyebrow="eyebrow ?? undefined" title="Proprietários">
      <template #actions>
        <BaseButton variant="ghost" unavailable-hint="Em breve">
          Exportar
          <span class="owners__soon text-label-badge" aria-hidden="true">Em breve</span>
        </BaseButton>
        <BaseButton v-if="canWrite" icon="plus" @click="openNew()">Novo proprietário</BaseButton>
      </template>
    </PageHeader>

    <ToastAlert
      v-if="error"
      tone="error"
      title="Não foi possível carregar os proprietários"
      :message="error.message"
      :dismissible="false"
    >
      <template #actions>
        <BaseButton variant="secondary" size="sm" icon="undo" @click="refresh()">Tentar novamente</BaseButton>
      </template>
    </ToastAlert>

    <EmptyState
      v-else-if="isEmpty"
      icon="users"
      title="Nenhum proprietário cadastrado"
      :description="canWrite
        ? 'Cadastre as pessoas e empresas donas das fazendas para registrar a participação de cada uma.'
        : 'Os proprietários das fazendas liberadas para você aparecem aqui quando o administrador da sua empresa cadastrá-los.'"
    >
      <template v-if="canWrite" #actions>
        <BaseButton icon="plus" @click="openNew()">Cadastrar primeiro proprietário</BaseButton>
      </template>
    </EmptyState>

    <DataTable
      v-else
      :headers="headers"
      :items="isFirstLoad || isFilterEmpty ? [] : rows"
      :highlighted-id="highlightedId"
      label="Proprietários"
      :clickable="canWrite"
      @row-click="openEdit($event.id)"
    >
      <template #toolbar>
        <form class="owners__search" role="search" @submit.prevent>
          <label for="owner-search" class="visually-hidden">Filtrar por nome ou CPF/CNPJ</label>
          <AppIcon name="search" :size="16" class="owners__search-icon" />
          <input
            id="owner-search"
            v-model="search"
            type="search"
            name="q"
            class="owners__search-input text-caption-md"
            placeholder="Filtrar por nome ou CPF/CNPJ"
            autocomplete="off"
          >
        </form>
        <FilterChips v-model="personType" :items="PERSON_FILTERS" label="Tipo de pessoa" size="sm" />
      </template>

      <template #[`item.name`]="{ item }">
        <div class="owners__person">
          <BaseAvatar :seed="item.id" :initials="item.initials" size="md" />
          <div class="owners__person-text">
            <div class="owners__person-line">
              <button v-if="canWrite" type="button" class="owners__name text-label-strong" @click.stop="openEdit(item.id)">
                {{ item.name }}
              </button>
              <span v-else class="owners__name owners__name--static text-label-strong">{{ item.name }}</span>
              <span v-if="item.hasPendingDocuments" class="owners__pending text-label-micro">Docs pendentes</span>
            </div>
            <span class="owners__document text-body-sm">{{ item.documentLabel }}</span>
          </div>
        </div>
      </template>

      <template #[`item.farms`]="{ item }">
        <ul class="owners__farms">
          <li v-for="farm in item.farms" :key="farm.id" class="owners__farm text-label-tag">{{ farm.text }}</li>
        </ul>
      </template>

      <template #[`item.shareLabel`]="{ item }">
        <span class="owners__share text-label-meta-strong" :class="`owners__share--${item.shareTone}`">
          {{ item.shareLabel }}
        </span>
      </template>

      <template v-if="canWrite" #[`item.actions`]="{ item }">
        <v-menu location="bottom end">
          <template #activator="{ props: menuProps }">
            <button
              type="button"
              class="owners__menu"
              :aria-label="`Mais opções de ${item.name}`"
              v-bind="menuProps"
              @click.stop
            >
              <AppIcon name="more" :size="18" />
            </button>
          </template>
          <div class="owners__menu-list" role="menu" :aria-label="`Opções de ${item.name}`">
            <button type="button" role="menuitem" class="owners__menu-item text-label-meta" @click="openEdit(item.id)">
              <AppIcon name="pen" :size="16" /> Editar
            </button>
            <button type="button" role="menuitem" class="owners__menu-item text-label-meta" @click="openLinks(item.id)">
              <AppIcon name="map" :size="16" /> Vínculos
            </button>
          </div>
        </v-menu>
      </template>

      <template #[`no-data`]>
        <div v-if="isFirstLoad" class="owners__skeleton" role="status" aria-label="Carregando proprietários">
          <div v-for="index in 5" :key="index" class="owners__skeleton-row">
            <span class="owners__skeleton-avatar" />
            <span class="owners__skeleton-line" />
          </div>
        </div>
        <p v-else class="owners__no-results text-body-md">Nenhum proprietário encontrado para esse filtro.</p>
      </template>

      <template v-if="pagination" #footer>
        <span>{{ pagination.showingLabel }}</span>
        <span class="owners__pages">
          <button
            v-if="pagination.pageCount > 1"
            type="button"
            class="owners__page-button"
            aria-label="Página anterior"
            :disabled="pagination.page <= 1"
            @click="page = pagination.page - 1"
          >
            <AppIcon name="chevron-right" :size="16" class="owners__page-prev" />
          </button>
          <span class="owners__page-label text-label-chip">{{ pagination.pageLabel }}</span>
          <button
            v-if="pagination.pageCount > 1"
            type="button"
            class="owners__page-button"
            aria-label="Próxima página"
            :disabled="pagination.page >= pagination.pageCount"
            @click="page = pagination.page + 1"
          >
            <AppIcon name="chevron-right" :size="16" />
          </button>
        </span>
      </template>
    </DataTable>

    <OwnerModal
      v-if="modalOwnerId"
      :key="`${modalOwnerId}-${modalStep}`"
      :mode="isNewOwner ? 'create' : 'edit'"
      :owner-id="isNewOwner ? undefined : modalOwnerId"
      :initial-step="modalStep"
      @close="closeModal()"
      @saved="onOwnerSaved"
    />

    <ToastAlert
      v-if="alert"
      floating
      :tone="alert.kind"
      :title="alert.title"
      :message="alert.description"
      @dismiss="alert = null"
    />
  </div>
</template>

<style scoped>
.owners {
  display: flex;
  flex-direction: column;
  gap: var(--space-24);
}

.owners__soon {
  margin-left: var(--space-8);
  padding: var(--space-2) var(--space-8);
  border-radius: var(--radius-pill);
  background: var(--color-sand);
  color: var(--color-text-tertiary);
}

/* Figma 39:635: sand search box, 40 px, inside the table card. */
.owners__search {
  display: flex;
  flex: 1 1 var(--page-header-main-min-width);
  gap: var(--space-8);
  align-items: center;
  height: var(--size-search-inline);
  padding: 0 var(--space-12);
  border-radius: var(--radius-tile);
  background: var(--color-sand);
}

.owners__search:focus-within {
  box-shadow: var(--focus-ring);
}

.owners__search-icon {
  color: var(--color-text-secondary);
}

.owners__search-input {
  flex: 1;
  min-width: 0;
  padding: 0;
  border: 0;
  background: transparent;
  color: var(--color-text-primary);
  outline: none;
}

.owners__search-input::placeholder {
  color: var(--color-text-tertiary);
}

.owners__person {
  display: flex;
  gap: var(--space-12);
  align-items: center;
  min-width: 0;
}

.owners__person-text {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  min-width: 0;
}

.owners__person-line {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-6);
  align-items: center;
}

.owners__name {
  padding: 0;
  border: 0;
  border-radius: var(--radius-xs);
  background: transparent;
  color: var(--color-text-primary);
  text-align: left;
  cursor: pointer;
}

.owners__name--static {
  cursor: default;
}

.owners__name:focus-visible {
  outline: none;
  box-shadow: var(--focus-ring);
}

.owners__pending {
  padding: var(--space-2) var(--space-8);
  border-radius: var(--radius-pill);
  background: var(--color-status-attention-bg);
  color: var(--color-status-attention);
}

.owners__document {
  color: var(--color-text-tertiary);
  white-space: nowrap;
}

.owners__farms {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-6);
  margin: 0;
  padding: 0;
  list-style: none;
}

.owners__farm {
  padding: var(--space-4) var(--space-8);
  border-radius: var(--radius-sm);
  background: var(--color-sand);
  color: var(--color-text-secondary);
}

.owners__share {
  color: var(--color-text-primary);
}

/* Tenant / legal representative in the earth tone (Figma 39:755); "Varia"/"—" muted. */
.owners__share--muted {
  color: var(--color-text-secondary);
}

.owners__share--role {
  color: var(--color-coffee-700);
}

.owners__menu {
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

.owners__menu:hover {
  background: var(--color-sand);
}

.owners__menu:focus-visible {
  outline: none;
  box-shadow: var(--focus-ring);
}

.owners__menu-list {
  display: flex;
  flex-direction: column;
  min-width: var(--menu-min-width);
  padding: var(--space-4);
  border-radius: var(--radius-md);
  background: var(--color-white);
  box-shadow: var(--shadow-floating);
}

.owners__menu-item {
  display: flex;
  gap: var(--space-8);
  align-items: center;
  padding: var(--space-10) var(--space-12);
  border: 0;
  border-radius: var(--radius-sm);
  background: transparent;
  color: var(--color-text-primary);
  text-align: left;
  cursor: pointer;
}

.owners__menu-item:hover,
.owners__menu-item:focus-visible {
  outline: none;
  background: var(--color-sand);
}

.owners__pages {
  display: inline-flex;
  gap: var(--space-8);
  align-items: center;
}

.owners__page-label {
  color: var(--color-text-secondary);
}

.owners__page-button {
  display: grid;
  place-items: center;
  width: var(--size-control-sm);
  height: var(--size-control-sm);
  border: var(--border-width-hairline) solid var(--color-border);
  border-radius: var(--radius-sm);
  background: var(--color-white);
  color: var(--color-text-secondary);
  cursor: pointer;
}

.owners__page-button:disabled {
  color: var(--color-text-tertiary);
  cursor: not-allowed;
}

.owners__page-prev {
  transform: rotate(180deg);
}

.owners__skeleton {
  display: flex;
  flex-direction: column;
  gap: var(--space-14);
  padding: var(--space-14) 0;
}

.owners__skeleton-row {
  display: flex;
  gap: var(--space-12);
  align-items: center;
}

.owners__skeleton-avatar,
.owners__skeleton-line {
  background: var(--color-sand);
  animation: owners-pulse var(--duration-skeleton) var(--easing-standard) infinite;
}

.owners__skeleton-avatar {
  width: var(--size-avatar-md);
  height: var(--size-avatar-md);
  border-radius: var(--radius-pill);
}

.owners__skeleton-line {
  width: var(--skeleton-line-width);
  height: var(--space-14);
  border-radius: var(--radius-pill);
}

@keyframes owners-pulse {
  50% {
    opacity: var(--opacity-skeleton-dim);
  }
}

@media (prefers-reduced-motion: reduce) {
  .owners__skeleton-avatar,
  .owners__skeleton-line {
    animation: none;
  }
}

.owners__no-results {
  padding: var(--space-24) 0;
  color: var(--color-text-secondary);
  text-align: center;
}

</style>
