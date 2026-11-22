<script setup lang="ts">
import type { DataTableHeader } from '~/components/DataTable.vue'
import type { FilterChipItem } from '~/components/FilterChips.vue'
import type { UserListFilter, UserRowAction, UserRowView } from '~/utils/userRow'

/**
 * Usuários (Figma 52:215, content 52:289). Gated by capability (ADR 0013);
 * the route middleware applies `permission`. Presentation only: data, filters,
 * row actions and the invite modal state come from useUserListPage.
 */
definePageMeta({ permission: 'users.manage' })
useHead({ title: 'Usuários · Agentes Agro' })

const {
  eyebrow,
  roleCards,
  search,
  filter,
  filterChips,
  page,
  pageSize,
  pageSizeOptions,
  rows,
  pagination,
  pending,
  error,
  isEmpty,
  isFilterEmpty,
  refresh,
  canInvite,
  exportAction,
  runAction,
  actionPending,
  notice,
  dismissNotice,
  modal,
  openInvite,
  closeModal,
  takeFlash,
} = useUserListPage()

const HEADERS: readonly DataTableHeader[] = [
  { title: 'Usuário', key: 'name' },
  { title: 'Papel', key: 'role', width: 'var(--user-table-role-width)' },
  { title: 'Empresa', key: 'companyName', width: 'var(--user-table-company-width)' },
  { title: 'Último acesso', key: 'lastAccessLabel', width: 'var(--user-table-access-width)' },
  { title: 'Status', key: 'status', width: 'var(--user-table-status-width)' },
  { title: 'Ações', key: 'actions', width: 'var(--size-icon-tile-md)', srOnly: true, align: 'end' },
]

const pendingConfirm = ref<{ row: UserRowView, action: UserRowAction } | null>(null)

const isFirstLoad = computed(() => pending.value && rows.value.length === 0)
const isForbidden = computed(() => error.value?.code === 'forbidden')
const highlightedId = computed(() => rows.value.find((row) => row.highlighted)?.id ?? null)
const chipItems = computed<readonly FilterChipItem<UserListFilter>[]>(() =>
  filterChips.value.map((chip) => ({ value: chip.value, label: chip.label })),
)
const perPageValues = computed(() => pageSizeOptions.map((option) => option.value))
const isConfirmOpen = computed({
  get: () => pendingConfirm.value !== null,
  set: (open: boolean) => {
    if (!open) pendingConfirm.value = null
  },
})

function onAction(row: UserRowView, action: UserRowAction) {
  if (action.confirm) {
    pendingConfirm.value = { row, action }
    return
  }
  void runAction(row, action.kind)
}

async function confirmAction() {
  const current = pendingConfirm.value
  if (!current) return
  await runAction(current.row, current.action.kind)
  pendingConfirm.value = null
}

onMounted(() => takeFlash())

// The modal (screen 13) saves and closes by clearing the query: show its flash.
watch(modal, (current, previous) => {
  if (previous && !current) takeFlash()
})
</script>

<template>
  <div class="users">
    <ForbiddenState v-if="isForbidden" />

    <template v-else>
      <PageHeader :eyebrow="eyebrow ?? 'Administração'" title="Usuários">
        <template #actions>
          <BaseButton variant="ghost" :unavailable-hint="exportAction.hint">Exportar</BaseButton>
          <BaseButton v-if="canInvite" icon="plus" @click="openInvite()">Convidar usuário</BaseButton>
        </template>
      </PageHeader>

      <section class="users__roles" aria-label="Papéis">
        <UserRoleCard
          v-for="card in roleCards"
          :key="card.role"
          :label="card.label"
          :tone="card.tone"
          :count-label="card.countLabel"
          :description="card.description"
          :capabilities="card.capabilities"
        />
      </section>

      <ToastAlert
        v-if="error"
        tone="error"
        title="Não foi possível carregar os usuários"
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
        title="Nenhum usuário convidado"
        description="Convide a equipe para planejar e registrar aplicações nas fazendas."
      >
        <template v-if="canInvite" #actions>
          <BaseButton icon="plus" @click="openInvite()">Convide o primeiro usuário</BaseButton>
        </template>
      </EmptyState>

      <DataTable
        v-else
        :headers="HEADERS"
        :items="rows"
        label="Usuários"
        :highlighted-id="highlightedId"
        :aria-busy="isFirstLoad || undefined"
      >
        <template #toolbar>
          <TableSearchField v-model="search" label="Filtrar por nome, e-mail ou empresa" />
          <FilterChips v-model="filter" :items="chipItems" label="Filtrar por papel ou status" />
        </template>

        <template #[`item.name`]="{ item }">
          <UserIdentity :id="item.id" :name="item.name" :email="item.email" :initials="item.initials" />
        </template>
        <template #[`item.role`]="{ item }">
          <UserRoleTag :label="item.roleLabel" :tone="item.roleTone" />
        </template>
        <template #[`item.companyName`]="{ item }">
          <span class="users__company">{{ item.companyName }}</span>
        </template>
        <template #[`item.lastAccessLabel`]="{ item }">
          <span class="users__access text-body-sm">{{ item.lastAccessLabel }}</span>
        </template>
        <template #[`item.status`]="{ item }">
          <UserStatusLabel :status="item.status.value" :label="item.status.label" />
        </template>
        <template #[`item.actions`]="{ item }">
          <span v-if="actionPending === item.id" class="users__spinner" role="status" aria-label="Aplicando ação" />
          <UserRowActions v-else :actions="item.actions" :user-name="item.name" @select="onAction(item, $event)" />
        </template>
        <template #[`no-data`]>
          <p class="users__no-data text-body-md">
            {{ isFirstLoad ? 'Carregando usuários…' : isFilterEmpty ? 'Nenhum usuário com esse filtro.' : 'Nenhum usuário.' }}
          </p>
        </template>

        <template v-if="pagination" #footer>
          <TablePagination
            v-model:page="page"
            v-model:per-page="pageSize"
            :summary="pagination.rangeLabel"
            :page-count="pagination.pageCount"
            :per-page-options="perPageValues"
            label="Paginação de usuários"
          />
        </template>
      </DataTable>
    </template>

    <UserConfirmDialog
      v-if="pendingConfirm?.action.confirm"
      v-model:open="isConfirmOpen"
      :title="pendingConfirm.action.confirm.title"
      :description="pendingConfirm.action.confirm.description"
      :confirm-label="pendingConfirm.action.confirm.confirmLabel"
      :loading="actionPending === pendingConfirm.row.id"
      @confirm="confirmAction"
    />

    <UserAccessModal v-if="modal" :modal="modal" @close="closeModal()" />

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
.users {
  display: flex;
  flex-direction: column;
  gap: var(--space-24);
}

/* Figma 52:317: equal role cards, 16 px apart (three for the super admin, two for an admin). */
.users__roles {
  display: grid;
  grid-auto-columns: minmax(0, 1fr);
  grid-auto-flow: column;
  gap: var(--space-16);
}

.users__company {
  color: var(--color-text-primary);
}

.users__access {
  color: var(--color-text-secondary);
}

.users__no-data {
  padding: var(--space-24);
  color: var(--color-text-secondary);
  text-align: center;
}

.users__spinner {
  display: inline-block;
  width: var(--size-spinner);
  height: var(--size-spinner);
  border: var(--border-width-strong) solid var(--color-text-tertiary);
  border-right-color: transparent;
  border-radius: var(--radius-pill);
  animation: users-spin var(--duration-spinner) linear infinite;
}

@keyframes users-spin {
  to {
    transform: rotate(360deg);
  }
}

@media (max-width: 1199px) {
  .users__roles {
    grid-auto-flow: row;
  }
}
</style>
