<script setup lang="ts">
import type { UserListModal } from '~/composables/useUserListPage'
import type { UserRole } from '~/types/auth'
import type { FarmScopeMode } from '~/utils/userAccessForm'

/**
 * "Convidar usuário" / "Editar acesso" (Figma 52:758). Modal shell over
 * useUserAccessForm, opened by the users page from the query (`?invite=1`,
 * `?user=<id>`). Roles come as option cards, company and farm scope as selects;
 * which options exist and what is locked comes from the logic (ADR 0013).
 * POST + novalidate + submit.prevent: personal data never goes to the URL.
 */
const props = defineProps<{
  modal: UserListModal
}>()

const emit = defineEmits<{
  close: []
}>()

const editedUser = computed(() => (props.modal.mode === 'edit' ? props.modal.user : null))
const isLoadingUser = computed(() => props.modal.mode === 'edit' && props.modal.user === null && props.modal.loading)
const isMissingUser = computed(() => props.modal.mode === 'edit' && props.modal.user === null && !props.modal.loading)

const {
  texts,
  values,
  identityReadonly,
  roleOptions,
  companyOptions,
  companyLocked,
  scopeOptions,
  scopeLocked,
  farmOptions,
  farmsUnavailable,
  fieldErrors,
  formError,
  touch,
  submit,
  saving,
  pending,
  loadError,
} = useUserAccessForm({ user: editedUser })

const open = ref(true)
const formId = useId()

const role = computed<UserRole | null>({
  get: () => values.role,
  set: (value) => {
    values.role = value
    touch('role')
  },
})
const companyId = computed<string | null>({
  get: () => values.companyId,
  set: (value) => {
    values.companyId = value
    touch('companyId')
  },
})
const scopeMode = computed<FarmScopeMode | null>({
  get: () => values.farmScopeMode,
  set: (value) => {
    if (value) values.farmScopeMode = value
  },
})

function isFarmSelected(id: string): boolean {
  return values.farmIds.includes(id)
}

function toggleFarm(id: string, selected: boolean) {
  values.farmIds = selected ? [...values.farmIds, id] : values.farmIds.filter((farmId) => farmId !== id)
  touch('farmIds')
}

/** Single exit path: BaseModal's own close (X, Esc, overlay), Cancel and a saved form all end here, once. */
let closed = false
function close() {
  if (closed) return
  closed = true
  open.value = false
  emit('close')
}

async function onSubmit() {
  if (await submit()) close()
}
</script>

<template>
  <BaseModal
    v-model:open="open"
    :eyebrow="texts.eyebrow"
    :title="isMissingUser || isLoadingUser ? 'Editar acesso' : texts.title"
    :description="isMissingUser || isLoadingUser ? undefined : texts.description"
    icon="mail"
    @close="close"
  >
    <div v-if="isLoadingUser" class="user-access__skeleton" aria-busy="true" aria-label="Carregando usuário">
      <div v-for="index in 4" :key="index" class="user-access__skeleton-row" />
    </div>

    <p v-else-if="isMissingUser" class="user-access__missing text-body-md">
      Usuário não encontrado nesta página. Volte para a lista e abra o menu do usuário de novo.
    </p>

    <form
      v-else
      :id="formId"
      class="user-access"
      method="post"
      novalidate
      :aria-busy="pending || undefined"
      @submit.prevent="onSubmit"
    >
      <div class="user-access__row">
        <TextField
          v-model="values.name"
          label="Nome"
          name="name"
          autocomplete="name"
          :readonly="identityReadonly"
          :error="fieldErrors.name"
          required
          @blur="touch('name')"
        />
        <TextField
          v-model="values.email"
          label="E-mail"
          name="email"
          type="email"
          autocomplete="email"
          :readonly="identityReadonly"
          :error="fieldErrors.email"
          required
          @blur="touch('email')"
        />
      </div>

      <div class="user-access__roles">
        <OptionCardGroup v-model="role" :options="roleOptions" label="Papel no sistema" name="role" />
        <p v-if="fieldErrors.role" class="user-access__error text-body-sm" role="alert">{{ fieldErrors.role }}</p>
      </div>

      <div class="user-access__row">
        <BaseSelect
          v-model="companyId"
          label="Empresa"
          name="companyId"
          :items="companyOptions"
          :disabled="companyLocked"
          :error="fieldErrors.companyId"
          required
          @blur="touch('companyId')"
        />
        <BaseSelect
          v-model="scopeMode"
          label="Fazendas liberadas"
          name="farmScope"
          :items="scopeOptions"
          :disabled="scopeLocked"
        />
      </div>
      <p v-if="farmsUnavailable && !scopeLocked && values.farmScopeMode !== 'selected'" class="user-access__hint text-body-sm">
        Fazendas ainda não disponíveis: o acesso vale para todas da empresa.
      </p>

      <fieldset v-if="!scopeLocked && values.farmScopeMode === 'selected'" class="user-access__farms">
        <legend class="user-access__farms-legend text-label-field">Fazendas desta pessoa</legend>
        <p v-if="!farmOptions.length" class="user-access__hint text-body-sm">
          {{ pending ? 'Carregando fazendas…' : farmsUnavailable ? 'Fazendas ainda não disponíveis.' : 'Nenhuma fazenda cadastrada nesta empresa.' }}
        </p>
        <div v-else class="user-access__farm-list">
          <BaseCheckbox
            v-for="farm in farmOptions"
            :key="farm.value"
            :model-value="isFarmSelected(farm.value)"
            :label="farm.label"
            name="farmIds"
            size="md"
            @update:model-value="toggleFarm(farm.value, $event)"
          />
        </div>
        <p v-if="fieldErrors.farmIds" class="user-access__error text-body-sm" role="alert">{{ fieldErrors.farmIds }}</p>
      </fieldset>

      <ToastAlert
        v-if="loadError"
        tone="error"
        title="Não foi possível carregar empresas e fazendas"
        :message="loadError.message"
        :dismissible="false"
      />
      <ToastAlert
        v-if="formError"
        tone="error"
        title="Não foi possível salvar"
        :message="formError"
        :dismissible="false"
      />
    </form>

    <template #actions>
      <BaseButton variant="secondary" block @click="close">Cancelar</BaseButton>
      <BaseButton
        v-if="!isMissingUser && !isLoadingUser"
        type="submit"
        :form="formId"
        block
        :icon="modal.mode === 'invite' ? 'plus' : undefined"
        :loading="saving"
      >
        {{ texts.submitLabel }}
      </BaseButton>
    </template>
  </BaseModal>
</template>

<style scoped>
.user-access {
  display: flex;
  flex-direction: column;
  gap: var(--space-20);
}

.user-access__row {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: var(--space-12);
}

.user-access__roles,
.user-access__farms {
  display: flex;
  flex-direction: column;
  gap: var(--space-8);
}

.user-access__farms {
  margin: 0;
  padding: 0;
  border: 0;
}

.user-access__farms-legend {
  margin-bottom: var(--space-8);
  padding: 0;
  color: var(--color-text-primary);
}

.user-access__farm-list {
  display: grid;
  max-height: var(--user-access-farm-list-max-height);
  padding-right: var(--space-4);
  overflow-y: auto;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: var(--space-8) var(--space-16);
}

.user-access__error {
  color: var(--color-status-unsuitable);
}

.user-access__skeleton {
  display: flex;
  flex-direction: column;
  gap: var(--space-20);
}

.user-access__skeleton-row {
  height: var(--size-control-lg);
  border-radius: var(--radius-md);
  background: var(--color-sand);
  animation: user-access-pulse var(--duration-skeleton) var(--easing-standard) infinite;
}

@keyframes user-access-pulse {
  50% {
    opacity: var(--opacity-skeleton-dim);
  }
}

@media (prefers-reduced-motion: reduce) {
  .user-access__skeleton-row {
    animation: none;
  }
}

.user-access__hint,
.user-access__missing {
  color: var(--color-text-secondary);
}

@media (max-width: 599px) {
  .user-access__row,
  .user-access__farm-list {
    grid-template-columns: minmax(0, 1fr);
  }
}
</style>
