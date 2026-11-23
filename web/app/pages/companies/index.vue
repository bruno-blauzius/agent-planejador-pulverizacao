<script setup lang="ts">
import type { DataTableHeader } from '~/components/DataTable.vue'

/**
 * Empresas (screen 14, Figma 52:851 / 52:925). Gated by capability (ADR 0013);
 * the route middleware applies `permission`. Presentation only: indicators,
 * table, search and pagination come from useCompanyListPage.
 * The "Nova/Editar empresa" modal, the row "…" menu and the confirmations have
 * no Figma frame: derived from the Usuários patterns (see progresso-telas.md).
 */
definePageMeta({ permission: 'companies.manage' })
useHead({ title: 'Empresas · Agentes Agro' })

const {
  statCards,
  summaryError,
  summaryPending,
  search,
  rows,
  pending,
  error,
  isForbidden,
  isEmpty,
  isEmptySearch,
  clearSearch,
  refresh,
  pagination,
  page,
  pageSize,
  pageSizeOptions,
  canCreate,
  runAction,
  actionPending,
  confirmation,
  confirmAction,
  cancelConfirmation,
  notice,
  dismissNotice,
  modal,
  openCreate,
  closeModal,
  handleSaved,
  takeFlash,
} = useCompanyListPage()

const BASE_HEADERS: readonly DataTableHeader[] = [
  { title: 'Empresa', key: 'name' },
  { title: 'CNPJ', key: 'documentLabel', width: 'var(--company-table-document-width)' },
  { title: 'Plano', key: 'plan', width: 'var(--company-table-plan-width)' },
  { title: 'Fazendas', key: 'farmCountLabel', width: 'var(--company-table-count-width)' },
  { title: 'Usuários', key: 'userCountLabel', width: 'var(--company-table-count-width)' },
  { title: 'Status', key: 'status', width: 'var(--company-table-status-width)' },
]
const ACTIONS_HEADER: DataTableHeader = {
  title: 'Ações',
  key: 'actions',
  width: 'var(--size-icon-tile-md)',
  srOnly: true,
  align: 'end',
}

// The actions column only exists when the user can manage companies.
const headers = computed<readonly DataTableHeader[]>(() => (canCreate.value ? [...BASE_HEADERS, ACTIONS_HEADER] : BASE_HEADERS))
const highlightedId = computed(() => rows.value.find((row) => row.highlighted)?.id ?? null)
const modalKey = computed(() => (modal.value?.mode === 'edit' ? `edit:${modal.value.companyId}` : 'create'))

const isFirstLoad = computed(() => pending.value && rows.value.length === 0)
const perPageValues = computed(() => pageSizeOptions.map((option) => option.value))

/*
 * Focus return (WCAG 2.4.3). The confirmation and the form modal are opened from
 * a menu item (which unmounts with the menu) or from "Nova empresa": remember the
 * originating control and give it the focus back when the dialog closes. If the
 * control is gone (row deleted), fall back to the search field, then the title.
 */
let focusOrigin: { element: HTMLElement | null, companyId: string | null } | null = null
const pageRoot = ref<HTMLElement | null>(null)

function rememberFocusOrigin(companyId: string | null = null) {
  const active = document.activeElement
  focusOrigin = { element: active instanceof HTMLElement && active !== document.body ? active : null, companyId }
}

function resolveFocusTarget() {
  const origin = focusOrigin
  focusOrigin = null
  const root = pageRoot.value
  if (!root) return
  const byCompany = origin?.companyId
    ? root.querySelector<HTMLElement>(`[data-company-actions-trigger="${CSS.escape(origin.companyId)}"]`)
    : null
  const target =
    (origin?.element?.isConnected ? origin.element : null)
    ?? byCompany
    ?? root.querySelector<HTMLElement>('input[type="search"]')
  if (target) return target.focus()
  const heading = root.querySelector<HTMLElement>('h1')
  if (!heading) return
  heading.tabIndex = -1
  heading.focus()
}

function onRowAction(item: { id: string }, kind: Parameters<typeof runAction>[1]) {
  // The menu item is already focused/active when the click lands; the trigger is its owner.
  rememberFocusOrigin(item.id)
  return runAction(item, kind)
}

function onOpenCreate() {
  rememberFocusOrigin()
  openCreate()
}

const isDialogOpen = computed(() => Boolean(confirmation.value) || Boolean(modal.value))
/*
 * While the action runs, the row shows a spinner instead of the "…" button: wait
 * for it to finish before looking the button up (a deleted row is then gone).
 */
function restoreFocus() {
  if (!actionPending.value) return resolveFocusTarget()
  const stop = watch(actionPending, (pendingId) => {
    if (pendingId) return
    stop()
    setTimeout(resolveFocusTarget, 100)
  })
}

watch(isDialogOpen, (open, wasOpen) => {
  // After BaseModal's own restore (timers) and the dialog leaving the DOM.
  if (wasOpen && !open) setTimeout(restoreFocus, 50)
})

// A message left by another screen (flash) is shown once the page mounts.
onMounted(() => takeFlash())
</script>

<template>
  <div ref="pageRoot" class="companies">
    <ForbiddenState v-if="isForbidden" />

    <template v-else>
      <PageHeader eyebrow="Administração · gestão do sistema" title="Empresas">
        <template v-if="canCreate" #actions>
          <BaseButton icon="plus" @click="onOpenCreate()">Nova empresa</BaseButton>
        </template>
      </PageHeader>

      <section class="companies__stats" aria-label="Indicadores" :aria-busy="summaryPending || undefined">
        <StatCard
          v-for="card in statCards"
          :key="card.key"
          :label="card.label"
          :value="card.value"
          :caption="card.caption"
          :highlight="card.highlight"
        />
      </section>

      <!-- Indicators failed but the table may be fine: a discreet notice, cards keep "—". -->
      <p v-if="summaryError && !error" class="companies__summary-error text-body-sm" role="status">
        Não foi possível carregar os indicadores.
        <BaseButton variant="ghost" size="sm" icon="undo" @click="refresh()">Tentar novamente</BaseButton>
      </p>

      <!-- When the list fails, the search stays usable (retry with another term). -->
      <div v-if="error" class="companies__toolbar">
        <TableSearchField v-model="search" label="Buscar empresa ou CNPJ" />
      </div>

      <ToastAlert
        v-if="error"
        tone="error"
        title="Não foi possível carregar as empresas"
        :message="error.message"
        :dismissible="false"
      >
        <template #actions>
          <BaseButton variant="secondary" size="sm" icon="undo" @click="refresh()">Tentar novamente</BaseButton>
        </template>
      </ToastAlert>

      <EmptyState
        v-else-if="isEmpty"
        icon="document"
        title="Nenhuma empresa cliente"
        description="As empresas clientes da plataforma aparecem aqui assim que forem cadastradas."
      >
        <template v-if="canCreate" #actions>
          <BaseButton icon="plus" @click="onOpenCreate()">Cadastrar a primeira empresa</BaseButton>
        </template>
      </EmptyState>

      <DataTable
        v-else
        :headers="headers"
        :items="rows"
        label="Empresas"
        :highlighted-id="highlightedId"
        :aria-busy="isFirstLoad || undefined"
      >
        <template #toolbar>
          <TableSearchField v-model="search" label="Buscar empresa ou CNPJ" />
        </template>

        <template #[`item.name`]="{ item }">
          <CompanyIdentity :id="item.id" :name="item.name" :initials="item.initials" :location-label="item.locationLabel" />
        </template>
        <template #[`item.documentLabel`]="{ item }">
          <span class="companies__document text-body-sm">{{ item.documentLabel }}</span>
        </template>
        <template #[`item.plan`]="{ item }">
          <CompanyPlanTag :plan="item.plan.value" :label="item.plan.label" />
        </template>
        <template #[`item.farmCountLabel`]="{ item }">
          <span class="companies__count text-label-strong">{{ item.farmCountLabel }}</span>
        </template>
        <template #[`item.userCountLabel`]="{ item }">
          <span class="companies__count text-label-strong">{{ item.userCountLabel }}</span>
        </template>
        <template #[`item.status`]="{ item }">
          <CompanyStatusLabel :label="item.status.label" :tone="item.status.tone" />
        </template>
        <template #[`item.actions`]="{ item }">
          <span v-if="actionPending === item.id" class="companies__spinner" role="status" aria-label="Aplicando ação" />
          <CompanyRowActions
            v-else
            :actions="item.actions"
            :company-name="item.name"
            :company-id="item.id"
            @select="onRowAction(item, $event)"
          />
        </template>
        <template #[`no-data`]>
          <div class="companies__no-data">
            <p class="text-body-md">
              {{ isFirstLoad ? 'Carregando empresas…' : isEmptySearch ? `Nenhuma empresa para “${search}”.` : 'Nenhuma empresa.' }}
            </p>
            <BaseButton v-if="isEmptySearch" variant="secondary" size="sm" @click="clearSearch()">Limpar busca</BaseButton>
          </div>
        </template>

        <template #footer>
          <TablePagination
            v-model:page="page"
            v-model:per-page="pageSize"
            :summary="pagination.rangeLabel"
            :page-count="pagination.pageCount"
            :per-page-options="perPageValues"
            label="Paginação de empresas"
          />
        </template>
      </DataTable>
    </template>

    <CompanyConfirmDialog
      v-if="confirmation"
      :confirmation="confirmation"
      @confirm="confirmAction()"
      @cancel="cancelConfirmation()"
    />

    <CompanyFormModal
      v-if="modal"
      :key="modalKey"
      :modal="modal"
      @close="closeModal()"
      @saved="handleSaved"
    />

    <ToastAlert
      v-if="notice"
      floating
      :tone="notice.kind"
      :title="notice.title"
      :message="notice.description"
      @dismiss="dismissNotice()"
    />
  </div>
</template>

<style scoped>
.companies {
  display: flex;
  flex-direction: column;
  gap: var(--space-24);
}

/* Figma 52:948: four equal indicator cards, 16 px apart. */
.companies__stats {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: var(--space-16);
}

.companies__toolbar {
  display: flex;
  padding: var(--space-12);
  border: var(--border-width-hairline) solid var(--color-border);
  border-radius: var(--radius-xl);
  background: var(--color-white);
}

.companies__summary-error {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-8);
  align-items: center;
  margin: 0;
  color: var(--color-text-secondary);
}

.companies__document {
  white-space: nowrap;
  color: var(--color-text-secondary);
}

.companies__count {
  color: var(--color-text-primary);
}

.companies__no-data {
  display: flex;
  flex-direction: column;
  gap: var(--space-12);
  align-items: center;
  padding: var(--space-24);
  color: var(--color-text-secondary);
  text-align: center;
}

.companies__spinner {
  display: inline-block;
  width: var(--size-spinner);
  height: var(--size-spinner);
  border: var(--border-width-strong) solid var(--color-text-tertiary);
  border-right-color: transparent;
  border-radius: var(--radius-pill);
  animation: companies-spin var(--duration-spinner) linear infinite;
}

@keyframes companies-spin {
  to {
    transform: rotate(360deg);
  }
}

@media (prefers-reduced-motion: reduce) {
  .companies__spinner {
    animation: none;
  }
}

@media (max-width: 1199px) {
  .companies__stats {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}

@media (max-width: 599px) {
  .companies__stats {
    grid-template-columns: minmax(0, 1fr);
  }
}
</style>
