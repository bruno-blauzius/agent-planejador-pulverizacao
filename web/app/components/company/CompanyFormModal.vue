<script setup lang="ts">
import type { SegmentOption } from '~/components/SegmentedControl.vue'
import type { CompanyFormResult } from '~/composables/useCompanyForm'
import type { CompanyListModal } from '~/composables/useCompanyListPage'
import type { CompanyPlan } from '~/types/admin'

/**
 * "Nova empresa" / "Editar empresa". No Figma frame exists: derived from the
 * invite modal (UserAccessModal, Figma 52:758) with the same shell, form
 * conventions and states. Shell over useCompanyForm; opened by the page from
 * the query (`?new=1`, `?company=<id>`). Edit keeps the CNPJ read-only and
 * shows loading / not-found while a deep-linked company is not on the list.
 * POST + novalidate + submit.prevent: data never goes to the URL.
 */
const props = defineProps<{
  modal: CompanyListModal
}>()

const emit = defineEmits<{
  close: []
  saved: [result: CompanyFormResult]
}>()

const editedCompany = computed(() => (props.modal.mode === 'edit' ? props.modal.company : null))

const {
  title,
  submitLabel,
  name,
  document: documentNumber,
  city,
  state,
  plan,
  planOptions,
  planHint,
  isDocumentLocked,
  fieldErrors,
  formError,
  touch,
  submit,
  saving,
  isLoadingCompany,
} = useCompanyForm({ mode: props.modal.mode, company: editedCompany })

const open = ref(true)
const formId = useId()

const isLoading = computed(() => isLoadingCompany.value && props.modal.mode === 'edit' && props.modal.loading)
const loadError = computed(() => (props.modal.mode === 'edit' ? props.modal.loadError : null))
const isMissing = computed(() => isLoadingCompany.value && !isLoading.value && !loadError.value)
const showForm = computed(() => !isLoadingCompany.value)

const planSegments = computed<readonly SegmentOption<CompanyPlan>[]>(() =>
  planOptions.map((option) => ({ value: option.value, label: option.title })),
)

/** Single exit path: BaseModal's own close (X, Esc, overlay) and Cancel end here, once. */
let closed = false
function close() {
  if (closed) return
  closed = true
  open.value = false
  emit('close')
}

async function onSubmit() {
  const result = await submit()
  if (!result || closed) return
  closed = true
  emit('saved', result)
}
</script>

<template>
  <BaseModal
    v-model:open="open"
    :title="showForm ? title : 'Editar empresa'"
    :description="showForm ? 'Dados da empresa cliente, dona das próprias fazendas, talhões e usuários.' : undefined"
    icon="document"
    @close="close"
  >
    <div v-if="isLoading" class="company-form__skeleton" aria-busy="true" aria-label="Carregando empresa">
      <div v-for="index in 4" :key="index" class="company-form__skeleton-row" />
    </div>

    <ToastAlert
      v-else-if="loadError"
      tone="error"
      title="Não foi possível carregar a empresa"
      :message="loadError.message"
      :dismissible="false"
    />

    <p v-else-if="isMissing" class="company-form__missing text-body-md" role="status">
      Empresa não encontrada. Ela pode ter sido excluída ou estar em outra página: volte para a lista e abra o menu da empresa de novo.
    </p>

    <form
      v-else
      :id="formId"
      class="company-form"
      method="post"
      novalidate
      :aria-busy="saving || undefined"
      @submit.prevent="onSubmit"
    >
      <TextField
        v-model="name"
        label="Nome da empresa"
        name="name"
        autocomplete="organization"
        :error="fieldErrors.name"
        required
        data-autofocus
        @blur="touch('name')"
      />
      <TextField
        v-model="documentNumber"
        label="CNPJ"
        name="document"
        inputmode="numeric"
        autocomplete="off"
        placeholder="00.000.000/0000-00"
        :readonly="isDocumentLocked"
        :hint="isDocumentLocked ? 'O CNPJ não pode ser alterado.' : undefined"
        :error="fieldErrors.document"
        required
        @blur="touch('document')"
      />

      <div class="company-form__row">
        <TextField
          v-model="city"
          label="Cidade"
          name="city"
          autocomplete="address-level2"
          :error="fieldErrors.city"
          required
          @blur="touch('city')"
        />
        <TextField
          v-model="state"
          label="UF"
          name="state"
          autocomplete="address-level1"
          :maxlength="2"
          :error="fieldErrors.state"
          required
          @blur="touch('state')"
        />
      </div>

      <div class="company-form__plan">
        <span class="text-label-field" aria-hidden="true">Plano</span>
        <SegmentedControl v-model="plan" :options="planSegments" label="Plano" block />
        <p class="company-form__hint text-body-sm">{{ planHint }}</p>
        <p v-if="fieldErrors.plan" class="company-form__error text-body-sm" role="alert">{{ fieldErrors.plan }}</p>
      </div>

      <ToastAlert
        v-if="formError"
        tone="error"
        title="Não foi possível salvar"
        :message="formError"
        :dismissible="false"
      />
    </form>

    <template #actions>
      <div class="company-form__actions">
        <BaseButton variant="secondary" block @click="close">Cancelar</BaseButton>
        <BaseButton
          v-if="showForm"
          type="submit"
          :form="formId"
          block
          :icon="modal.mode === 'create' ? 'plus' : undefined"
          :loading="saving"
        >
          {{ submitLabel }}
        </BaseButton>
      </div>
    </template>
  </BaseModal>
</template>

<style scoped>
.company-form {
  display: flex;
  flex-direction: column;
  gap: var(--space-20);
}

/* Same row as BaseModal's footer on desktop; stacked on narrow screens (below). */
.company-form__actions {
  display: flex;
  flex: 1;
  gap: var(--space-10);
  min-width: 0;
}

.company-form__row {
  display: grid;
  grid-template-columns: minmax(0, 2fr) minmax(0, 1fr);
  gap: var(--space-12);
}

.company-form__plan {
  display: flex;
  flex-direction: column;
  gap: var(--space-8);
}

.company-form__hint,
.company-form__missing {
  margin: 0;
  color: var(--color-text-secondary);
}

.company-form__error {
  margin: 0;
  color: var(--color-status-unsuitable);
}

.company-form__skeleton {
  display: flex;
  flex-direction: column;
  gap: var(--space-20);
}

.company-form__skeleton-row {
  height: var(--size-control-lg);
  border-radius: var(--radius-md);
  background: var(--color-sand);
  animation: company-form-pulse var(--duration-skeleton) var(--easing-standard) infinite;
}

@keyframes company-form-pulse {
  50% {
    opacity: var(--opacity-skeleton-dim);
  }
}

@media (prefers-reduced-motion: reduce) {
  .company-form__skeleton-row {
    animation: none;
  }
}

@media (max-width: 599px) {
  .company-form__row {
    grid-template-columns: minmax(0, 1fr);
  }

  /* Primary action on top; "Cancelar" stays first in the DOM, as on desktop. */
  .company-form__actions {
    flex-direction: column-reverse;
  }
}
</style>
