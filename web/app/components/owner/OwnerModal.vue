<script setup lang="ts">
import type { OwnerFormOptions, OwnerFormResult, OwnerFormStep } from '~/composables/useOwnerForm'

/**
 * Owner modal (screen 4; Figma 39:1045 new · Dados, 39:1360 Vínculos,
 * 39:1893 edit). BaseModal + OwnerForm; the form object comes from useOwnerForm.
 * Mount it with a `key` per owner so each opening starts a fresh form.
 * `wizard` (Figma 41:1107, new-farm step 3): "Cadastrar e vincular", Dados only
 * plus the share in the farm being registered (`presetFarm`).
 */
const props = withDefaults(
  defineProps<{
    mode: 'create' | 'edit' | 'wizard'
    ownerId?: string
    /** Opens directly on "Vínculos" (row menu "Vínculos"). */
    initialStep?: OwnerFormStep
    /** Only in 'wizard': farm being registered and its current shares. */
    presetFarm?: OwnerFormOptions['presetFarm']
    /** Prefills the name (wizard: text typed in the owner search). */
    initialName?: string
  }>(),
  { ownerId: undefined, initialStep: 'data', presetFarm: undefined, initialName: undefined },
)

const emit = defineEmits<{
  close: []
  saved: [result: OwnerFormResult]
}>()

const form = useOwnerForm({ mode: props.mode, ownerId: props.ownerId, presetFarm: props.presetFarm })
const { step, name, loading, loadError, saving, goToLinks, backToData, submit } = form
if (props.initialName) name.value = props.initialName

const open = ref(true)
const modalRef = ref<{ focusTitle: () => void } | null>(null)
const formId = useId()

const isCreate = computed(() => props.mode === 'create')
const isWizard = computed(() => props.mode === 'wizard')
const eyebrow = computed(() =>
  isWizard.value && props.presetFarm ? `Novo proprietário · ${props.presetFarm.name}` : 'Proprietário',
)
const ownerName = computed(() => name.value.trim())

const title = computed(() => {
  if (step.value === 'links') return ownerName.value ? `Vínculos de ${ownerName.value}` : 'Vínculos'
  if (isWizard.value) return 'Cadastrar e vincular'
  if (isCreate.value) return 'Novo proprietário'
  return ownerName.value ? `Editar · ${ownerName.value}` : 'Editar proprietário'
})

const description = computed(() => {
  if (step.value === 'links') return 'Em quais fazendas responde e com qual participação?'
  if (isWizard.value) return 'Os dados ficam salvos no cadastro geral; o vínculo com esta fazenda é feito aqui mesmo.'
  return isCreate.value
    ? 'Quem é a pessoa ou empresa? Validamos o CPF ou CNPJ na hora.'
    : 'Dados cadastrais e documentos.'
})

/** Primary action of the current step: create · Dados continues; everything else saves. */
const isContinueStep = computed(() => isCreate.value && step.value === 'data')
const primaryLabel = computed(() => {
  if (isContinueStep.value) return 'Continuar'
  if (isWizard.value) return 'Salvar e vincular'
  return isCreate.value ? 'Salvar proprietário' : 'Salvar alterações'
})

async function onSubmit() {
  if (isContinueStep.value) {
    goToLinks()
    return
  }
  const result = await submit()
  if (result) emit('saved', result)
}

function onSecondary() {
  if (step.value === 'links') backToData()
  else close()
}

function close() {
  open.value = false
  emit('close')
}

// A step change replaces the whole body: move focus to the new title instead of the body.
watch(step, () => modalRef.value?.focusTitle())

// "Vínculos" from the row menu: wait for the owner to load (edit), then switch.
watch(loading, (isLoading) => {
  if (!isLoading && props.initialStep === 'links' && step.value === 'data') goToLinks()
}, { immediate: true })
</script>

<template>
  <BaseModal
    ref="modalRef"
    v-model:open="open"
    :eyebrow="eyebrow"
    icon="user"
    :title="title"
    :description="description"
    @close="emit('close')"
  >
    <OwnerStepIndicator v-if="isCreate || step === 'links'" :step="step" />

    <div v-if="loading" class="owner-modal__loading" role="status" aria-label="Carregando proprietário">
      <span v-for="index in 3" :key="index" class="owner-modal__skeleton" />
    </div>
    <ToastAlert
      v-else-if="loadError"
      tone="error"
      title="Não foi possível carregar o proprietário"
      :message="loadError.message"
      :dismissible="false"
    />
    <!-- method="post": a submit before hydration must never put personal data in the URL. -->
    <form v-else :id="formId" method="post" novalidate @submit.prevent="onSubmit">
      <OwnerForm :form="form" :mode="mode" :farm-name="presetFarm?.name" />
    </form>

    <template #actions>
      <BaseButton :variant="step === 'links' || isCreate || isWizard ? 'secondary' : 'ghost'" block @click="onSecondary">
        {{ step === 'links' ? 'Voltar' : 'Cancelar' }}
      </BaseButton>
      <BaseButton type="submit" :form="formId" block :loading="saving" :disabled="loading || Boolean(loadError)">
        {{ primaryLabel }}
      </BaseButton>
    </template>
  </BaseModal>
</template>

<style scoped>
.owner-modal__loading {
  display: flex;
  flex-direction: column;
  gap: var(--space-20);
}

.owner-modal__skeleton {
  height: var(--size-control-md);
  border-radius: var(--radius-md);
  background: var(--color-sand);
  animation: owner-modal-pulse var(--duration-skeleton) var(--easing-standard) infinite;
}

@keyframes owner-modal-pulse {
  50% {
    opacity: var(--opacity-skeleton-dim);
  }
}

@media (prefers-reduced-motion: reduce) {
  .owner-modal__skeleton {
    animation: none;
  }
}
</style>
