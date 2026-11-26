<script setup lang="ts">
import type { RainGaugesResource } from '~/composables/useRainGauges'
import { RAIN_GAUGE_NAME_MAX, validateRainGaugeName } from '~/utils/fieldConditionsForm'

/**
 * "Pluviômetros da fazenda" (ADR 0022, phase 2; no Figma frame: follows the
 * project modals). Lists the farm's gauges, renames one inline and registers a
 * new one by name. Only for `registry.write` (the host checks `gauges.canWrite`;
 * the API answers 403 anyway). Saving and errors come from `gauges`
 * (useRainGauges); the name rule is `validateRainGaugeName`. Emits `created`
 * with the new gauge's id, so the dialog that opened it can select it.
 */
const props = defineProps<{
  gauges: RainGaugesResource
}>()

const emit = defineEmits<{
  close: []
  created: [gaugeId: string]
}>()

const {
  gauges: list,
  pending,
  error,
  creating,
  createErrors,
  createFormError,
  renaming,
  renameErrors,
  renameFormError,
} = props.gauges

// Errors of a previous opening must not greet this one.
props.gauges.clearCreateError()
props.gauges.clearRenameError()

const open = ref(true)
const createFormId = useId()
const newName = ref('')
const newNameError = ref<string | null>(null)
const editingId = ref<string | null>(null)
const editingName = ref('')
const editingError = ref<string | null>(null)
/** Inline rename input; it takes the focus when it opens. */
const renameInputId = useId()

const createNameError = computed(() => newNameError.value ?? createErrors.value.name ?? undefined)
const renameNameError = computed(() => editingError.value ?? renameErrors.value.name ?? undefined)

// Typing again clears the previous answer of the API and the local check.
watch(newName, () => {
  newNameError.value = null
  props.gauges.clearCreateError()
})
watch(editingName, () => {
  editingError.value = null
  props.gauges.clearRenameError()
})

const isLoading = computed(() => pending.value && list.value.length === 0)

function close() {
  open.value = false
  emit('close')
}

async function onCreate() {
  newNameError.value = validateRainGaugeName(newName.value, list.value)
  if (newNameError.value) return
  const created = await props.gauges.create({ name: newName.value.trim() })
  if (!created) return
  newName.value = ''
  emit('created', created.id)
}

async function startRename(id: string, name: string) {
  editingId.value = id
  editingName.value = name
  editingError.value = null
  props.gauges.clearRenameError()
  await nextTick()
  const input = document.getElementById(renameInputId)
  if (input instanceof HTMLInputElement) input.select()
}

function cancelRename() {
  editingId.value = null
  editingError.value = null
}

async function onRename() {
  const id = editingId.value
  if (!id) return
  editingError.value = validateRainGaugeName(editingName.value, list.value, id)
  if (editingError.value) return
  const renamed = await props.gauges.rename(id, editingName.value.trim())
  if (!renamed) return
  cancelRename()
  // Back to the item's "Renomear", where the keyboard user started.
  await nextTick()
  document.querySelector<HTMLElement>(`[data-rename-gauge="${CSS.escape(id)}"]`)?.focus()
}
</script>

<template>
  <BaseModal
    v-model:open="open"
    icon="gauge"
    title="Pluviômetros da fazenda"
    description="Cadastre os pluviômetros e depois ligue cada talhão ao seu no perfil de solo."
    @close="emit('close')"
  >
    <div class="gauge-manager">
      <p v-if="isLoading" class="gauge-manager__muted text-caption" role="status">Carregando pluviômetros…</p>
      <ToastAlert
        v-else-if="error && !list.length"
        tone="error"
        title="Não foi possível carregar os pluviômetros"
        :message="error.message"
        :dismissible="false"
      />
      <p v-else-if="!list.length" class="gauge-manager__muted text-caption">Nenhum pluviômetro cadastrado ainda.</p>

      <ul v-if="list.length" class="gauge-manager__list" aria-label="Pluviômetros cadastrados">
        <li v-for="gauge in list" :key="gauge.id" class="gauge-manager__item">
          <form
            v-if="editingId === gauge.id"
            class="gauge-manager__rename"
            method="post"
            novalidate
            @submit.prevent="onRename"
          >
            <ToastAlert
              v-if="renameFormError"
              tone="error"
              title="Não foi possível renomear"
              :message="renameFormError"
              :dismissible="false"
            />
            <TextField
              :id="renameInputId"
              v-model="editingName"
              :label="`Novo nome de ${gauge.name}`"
              name="gaugeName"
              autocomplete="off"
              :maxlength="RAIN_GAUGE_NAME_MAX"
              :error="renameNameError"
            />
            <div class="gauge-manager__row-actions">
              <BaseButton variant="ghost" size="sm" @click="cancelRename">Cancelar</BaseButton>
              <BaseButton type="submit" variant="secondary" size="sm" :loading="renaming">Salvar</BaseButton>
            </div>
          </form>
          <template v-else>
            <span class="gauge-manager__name text-label-strong">{{ gauge.name }}</span>
            <BaseButton
              variant="ghost"
              size="sm"
              icon="pen"
              :data-rename-gauge="gauge.id"
              @click="startRename(gauge.id, gauge.name)"
            >
              Renomear<span class="visually-hidden"> {{ gauge.name }}</span>
            </BaseButton>
          </template>
        </li>
      </ul>

      <!-- method="post": a submit before hydration must never put the form in the URL. -->
      <form :id="createFormId" class="gauge-manager__create" method="post" novalidate @submit.prevent="onCreate">
        <ToastAlert
          v-if="createFormError"
          tone="error"
          title="Não foi possível cadastrar o pluviômetro"
          :message="createFormError"
          :dismissible="false"
        />
        <TextField
          v-model="newName"
          label="Nome do novo pluviômetro"
          name="newGaugeName"
          autocomplete="off"
          placeholder="Ex.: Pluviômetro do talhão 4"
          :maxlength="RAIN_GAUGE_NAME_MAX"
          :data-autofocus="list.length ? undefined : true"
          :error="createNameError"
        />
        <div class="gauge-manager__row-actions">
          <BaseButton type="submit" variant="secondary" size="sm" icon="plus" :loading="creating">Cadastrar pluviômetro</BaseButton>
        </div>
      </form>
    </div>

    <template #actions>
      <BaseButton variant="ghost" block @click="close">Concluir</BaseButton>
    </template>
  </BaseModal>
</template>

<style scoped>
.gauge-manager {
  display: flex;
  flex-direction: column;
  gap: var(--space-18);
}

.gauge-manager__muted {
  margin: 0;
  color: var(--color-text-secondary);
}

.gauge-manager__list {
  display: flex;
  flex-direction: column;
  margin: 0;
  padding: 0;
  border: var(--border-width-hairline) solid var(--color-border);
  border-radius: var(--radius-md);
  list-style: none;
}

.gauge-manager__item {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-8);
  align-items: center;
  justify-content: space-between;
  min-width: 0;
  padding: var(--space-10) var(--space-14);
}

.gauge-manager__item + .gauge-manager__item {
  border-top: var(--border-width-hairline) solid var(--color-coffee-100);
}

.gauge-manager__name {
  min-width: 0;
  color: var(--color-text-primary);
  overflow-wrap: anywhere;
}

.gauge-manager__rename,
.gauge-manager__create {
  display: flex;
  flex: 1 1 100%;
  flex-direction: column;
  gap: var(--space-8);
}

.gauge-manager__row-actions {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-8);
  justify-content: flex-end;
}
</style>
