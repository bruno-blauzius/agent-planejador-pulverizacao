<script setup lang="ts">
import type { DataTableHeader } from '~/components/DataTable.vue'
import type { FilterChipItem } from '~/components/FilterChips.vue'
import type { ApplicationRowView } from '~/utils/applications'

/**
 * Aplicações (screens 19 and 20, Figma 63:237 and 63:890). Any signed-in user
 * may read: the API filters by company and farm scope, so the route needs no
 * extra capability. Presentation only: rows, filters, weather and the detail
 * panel come from useApplicationListPage / useApplicationDetail.
 */
useHead({ title: 'Aplicações · Agentes Agro' })

const {
  subtitle,
  weatherLabel,
  weatherUnavailable,
  filters,
  periodOptions,
  fieldOptions,
  productOptions,
  operatorOptions,
  statusChips,
  summaryCards,
  hasFilters,
  setFilter,
  clearFilters,
  rows,
  page,
  pageSize,
  pageSizeOptions,
  pagination,
  pending,
  error,
  isForbidden,
  isOutOfScope,
  isEmpty,
  isFilterEmpty,
  refresh,
  canPlan,
  exportAction,
  selectedId,
  openDetail,
  closeDetail,
} = useApplicationListPage()

const detail = useApplicationDetail(selectedId)

const HEADERS: readonly DataTableHeader[] = [
  { title: 'Data · Janela', key: 'dateLabel', width: 'var(--application-table-date-width)' },
  { title: 'Talhão', key: 'fieldCode', width: 'var(--application-table-field-width)' },
  { title: 'Produto · Dose', key: 'productLabel', width: 'var(--application-table-product-width)' },
  { title: 'Área', key: 'areaLabel', width: 'var(--application-table-area-width)' },
  { title: 'Condições', key: 'conditions', width: 'var(--application-table-conditions-width)' },
  { title: 'Operador', key: 'operatorName', width: 'var(--application-table-operator-width)' },
  { title: 'Status', key: 'status', width: 'var(--application-table-status-width)' },
]

const ALL_STATUSES = 'all'

const statusChipItems = computed<readonly FilterChipItem[]>(() =>
  statusChips.map(chip => ({ value: chip.value ?? ALL_STATUSES, label: chip.label })),
)
const statusFilter = computed({
  get: () => filters.value.status ?? ALL_STATUSES,
  set: (value: string) => setFilter('status', value === ALL_STATUSES ? null : value),
})

const perPageValues = computed(() => pageSizeOptions.map(option => option.value))
const isFirstLoad = computed(() => pending.value && rows.value.length === 0)
const showTable = computed(() => !isEmpty.value && !isOutOfScope.value && !(error.value && rows.value.length === 0))
const isSheetOpen = computed({
  get: () => selectedId.value !== null,
  set: (open: boolean) => {
    if (!open) closeDetail()
  },
})

/*
 * The dialog has no activator, so focus is returned by hand: to the row button
 * of the closed application, or to the page title when that row is gone.
 */
watch(selectedId, async (current, previous) => {
  if (current || !previous) return
  await nextTick()
  const row = document.querySelector<HTMLElement>(`[data-application-id="${CSS.escape(previous)}"]`)
  if (row) {
    row.focus()
    return
  }
  const title = document.querySelector<HTMLElement>('.applications h1')
  title?.setAttribute('tabindex', '-1')
  title?.focus()
})

function repeat() {
  const path = detail.planRepeat()
  if (path) void navigateTo(path)
}

function onRowClick(row: ApplicationRowView) {
  openDetail(row.id)
}
</script>

<template>
  <div class="applications">
    <ForbiddenState v-if="isForbidden" />

    <template v-else>
      <PageHeader :eyebrow="subtitle" title="Aplicações">
        <template #actions>
          <AppTopbarChip v-if="weatherLabel" :label="weatherLabel" />
          <AppTopbarChip v-else-if="weatherUnavailable" label="Clima indisponível" />
          <BaseButton class="applications__action" variant="ghost" :unavailable-hint="exportAction.hint">Exportar caderno de campo</BaseButton>
          <BaseButton v-if="canPlan" class="applications__action" variant="accent" icon="plus" to="/planner">Planejar com o agente</BaseButton>
        </template>
      </PageHeader>

      <ToastAlert
        v-if="error && !isOutOfScope"
        tone="error"
        title="Não foi possível carregar as aplicações"
        :message="error.message"
        :dismissible="false"
      >
        <template #actions>
          <BaseButton variant="secondary" size="sm" icon="undo" @click="refresh()">Tentar de novo</BaseButton>
        </template>
      </ToastAlert>

      <section v-if="!isOutOfScope" class="applications__stats" aria-label="Resumo" :aria-busy="pending || undefined">
        <StatCard
          v-for="card in summaryCards"
          :key="card.key"
          :label="card.label"
          :value="card.value"
          :caption="card.caption"
          :highlight="card.tone === 'highlight'"
        />
      </section>

      <EmptyState
        v-if="isOutOfScope"
        variant="forbidden"
        icon="map"
        title="Fazenda fora do seu escopo"
        description="Esta fazenda não existe ou não está liberada para o seu perfil."
      />

      <EmptyState
        v-else-if="isEmpty"
        icon="calendar"
        title="Nenhuma aplicação ainda"
        description="Planeje uma janela com o agente e registre a primeira aplicação."
      >
        <template v-if="canPlan" #actions>
          <BaseButton variant="accent" icon="plus" to="/planner">Planejar com o agente</BaseButton>
        </template>
      </EmptyState>

      <DataTable
        v-else-if="showTable"
        :headers="HEADERS"
        :items="rows"
        label="Aplicações"
        clickable
        :highlighted-id="selectedId"
        :aria-busy="isFirstLoad || undefined"
        @row-click="onRowClick"
      >
        <template #toolbar>
          <div class="applications__toolbar">
            <ApplicationFilterSelect
              label="Período"
              :options="periodOptions"
              :model-value="filters.periodDays"
              @update:model-value="setFilter('period', $event)"
            />
            <ApplicationFilterSelect
              label="Talhão"
              :options="fieldOptions"
              :model-value="filters.fieldId"
              @update:model-value="setFilter('field', $event)"
            />
            <ApplicationFilterSelect
              label="Produto"
              :options="productOptions"
              :model-value="filters.productKind"
              @update:model-value="setFilter('product', $event)"
            />
            <ApplicationFilterSelect
              label="Operador"
              :options="operatorOptions"
              :model-value="filters.operatorId"
              @update:model-value="setFilter('operator', $event)"
            />
            <FilterChips v-model="statusFilter" :items="statusChipItems" label="Filtrar por status" size="sm" />
          </div>
        </template>

        <template #[`item.dateLabel`]="{ item }">
          <span class="applications__date">
            <button
              type="button"
              class="applications__open text-label-strong"
              :data-application-id="item.id"
              @click.stop="openDetail(item.id)"
            >
              {{ item.dateLabel }}
              <span class="visually-hidden">, {{ item.fieldCode }}, {{ item.productLabel }}: abrir detalhes</span>
            </button>
            <span class="applications__time text-body-sm">{{ item.timeLabel }}</span>
          </span>
        </template>
        <template #[`item.fieldCode`]="{ item }">
          <span class="applications__field">
            <span
              class="applications__tile text-label-strong"
              :class="{ 'applications__tile--info': item.status.tone === 'info' }"
              aria-hidden="true"
            >{{ item.fieldCode }}</span>
            <span class="applications__field-text">
              <span class="text-label-strong">{{ item.fieldName }}</span>
              <span v-if="item.fieldMeta" class="applications__meta text-caption">{{ item.fieldMeta }}</span>
            </span>
          </span>
        </template>
        <template #[`item.productLabel`]="{ item }">
          <span class="applications__product">
            <span class="text-label-strong">{{ item.productLabel }}</span>
            <span class="applications__meta text-caption">{{ item.doseLabel }}</span>
          </span>
        </template>
        <template #[`item.areaLabel`]="{ item }">
          <span class="applications__area text-label-strong">{{ item.areaLabel }}</span>
        </template>
        <template #[`item.conditions`]="{ item }">
          <ApplicationConditions :conditions="item.conditions" />
        </template>
        <template #[`item.operatorName`]="{ item }">
          <span class="applications__operator text-body-sm" :title="item.operatorFullName">
            <BaseAvatar :initials="item.operatorInitials" :seed="item.operatorFullName" size="sm" />
            {{ item.operatorName }}
          </span>
        </template>
        <template #[`item.status`]="{ item }">
          <ApplicationStatusPill :tone="item.status.tone" :label="item.status.label" />
        </template>
        <template #[`no-data`]>
          <div class="applications__no-data text-body-md">
            <template v-if="isFirstLoad">Carregando aplicações…</template>
            <template v-else-if="isFilterEmpty">
              <p>Nenhuma aplicação com esses filtros.</p>
              <BaseButton v-if="hasFilters" variant="secondary" size="sm" @click="clearFilters()">Limpar filtros</BaseButton>
            </template>
            <template v-else>Nenhuma aplicação.</template>
          </div>
        </template>

        <template v-if="pagination" #footer>
          <TablePagination
            v-model:page="page"
            v-model:per-page="pageSize"
            :summary="`Mostrando ${pagination.rangeLabel}`"
            :page-count="pagination.pageCount"
            :per-page-options="perPageValues"
            label="Paginação de aplicações"
          />
        </template>
      </DataTable>
    </template>

    <ApplicationDetailPanel
      v-model:open="isSheetOpen"
      :view="detail.view.value"
      :pending="detail.pending.value"
      :error="detail.error.value"
      :is-forbidden="detail.isForbidden.value"
      :is-not-found="detail.isNotFound.value"
      :can-repeat="detail.canRepeat.value"
      :download-hint="detail.downloadAction.hint"
      @retry="detail.refresh()"
      @repeat="repeat"
    />
  </div>
</template>

<style scoped>
.applications {
  display: flex;
  flex-direction: column;
  gap: var(--space-24);
}

.applications__stats {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: var(--space-16);
}

.applications__toolbar {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-10);
  align-items: center;
  min-width: 0;
}

.applications__date,
.applications__field-text,
.applications__product {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  min-width: 0;
}

.applications__open {
  padding: 0;
  border: 0;
  border-radius: var(--radius-xs);
  background: transparent;
  color: var(--color-text-primary);
  text-align: start;
  cursor: pointer;
}

.applications__open:focus-visible {
  outline: none;
  box-shadow: var(--focus-ring);
}

.applications__time,
.applications__meta {
  color: var(--color-text-secondary);
}

.applications__field {
  display: flex;
  gap: var(--space-10);
  align-items: center;
  min-width: 0;
}

.applications__tile {
  display: grid;
  flex-shrink: 0;
  place-items: center;
  width: var(--size-icon-tile-md);
  height: var(--size-icon-tile-md);
  border-radius: var(--radius-tile);
  background: var(--color-green-100);
  color: var(--color-green-800);
}

.applications__tile--info {
  background: var(--color-info-rain-bg);
  color: var(--color-info-rain);
}

.applications__operator {
  display: inline-flex;
  gap: var(--space-8);
  align-items: center;
  color: var(--color-text-secondary);
}

.applications__no-data {
  display: flex;
  flex-direction: column;
  gap: var(--space-12);
  align-items: center;
  padding: var(--space-24);
  color: var(--color-text-secondary);
  text-align: center;
}

.applications__no-data p {
  margin: 0;
}

@media (max-width: 1199px) {
  .applications__stats {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}

@media (max-width: 599px) {
  /* Full-width actions line up with the cards below instead of leaving ragged edges. */
  .applications__action {
    width: 100%;
  }

  .applications__stats {
    grid-template-columns: minmax(0, 1fr);
  }
}
</style>
