<script setup lang="ts">
import type { DataTableHeader } from '~/components/DataTable.vue'

/**
 * Empresas (screen 14, Figma 52:851 / 52:925). Gated by capability (ADR 0013);
 * the route middleware applies `permission`. Presentation only: indicators,
 * table, search and pagination come from useCompanyListPage.
 * "Nova empresa" has no frame yet (unavailable); the row "…" menu is omitted.
 */
definePageMeta({ permission: 'companies.manage' })
useHead({ title: 'Empresas · Agentes Agro' })

const {
  statCards,
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
} = useCompanyListPage()

const UNAVAILABLE_HINT = 'Em breve'

const HEADERS: readonly DataTableHeader[] = [
  { title: 'Empresa', key: 'name' },
  { title: 'CNPJ', key: 'documentLabel', width: 'var(--company-table-document-width)' },
  { title: 'Plano', key: 'plan', width: 'var(--company-table-plan-width)' },
  { title: 'Fazendas', key: 'farmCountLabel', width: 'var(--company-table-count-width)' },
  { title: 'Usuários', key: 'userCountLabel', width: 'var(--company-table-count-width)' },
  { title: 'Status', key: 'status', width: 'var(--company-table-status-width)' },
]

const isFirstLoad = computed(() => pending.value && rows.value.length === 0)
const perPageValues = computed(() => pageSizeOptions.map((option) => option.value))
</script>

<template>
  <div class="companies">
    <ForbiddenState v-if="isForbidden" />

    <template v-else>
      <PageHeader eyebrow="Administração · gestão do sistema" title="Empresas">
        <template v-if="canCreate" #actions>
          <BaseButton icon="plus" :unavailable-hint="UNAVAILABLE_HINT">Nova empresa</BaseButton>
        </template>
      </PageHeader>

      <section class="companies__stats" aria-label="Indicadores">
        <StatCard
          v-for="card in statCards"
          :key="card.key"
          :label="card.label"
          :value="card.value"
          :caption="card.caption"
          :highlight="card.highlight"
        />
      </section>

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
      />

      <DataTable
        v-else
        :headers="HEADERS"
        :items="rows"
        label="Empresas"
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

.companies__document {
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
