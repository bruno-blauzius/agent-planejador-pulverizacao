<script setup lang="ts">
import type { CompanyConfirmation } from '~/composables/useCompanyListPage'

/**
 * Confirmation of "Inativar" / "Excluir" a company. No Figma frame exists:
 * derived from UserConfirmDialog (small BaseModal, "Voltar" + danger action).
 * A failure (e.g. company with active users) shows inside the dialog, which
 * stays open. Closing is ignored by the logic while the action is pending.
 */
defineProps<{
  confirmation: CompanyConfirmation
}>()

const emit = defineEmits<{
  confirm: []
  cancel: []
}>()
</script>

<template>
  <BaseModal
    :open="true"
    :title="confirmation.title"
    :description="confirmation.description"
    icon="alert"
    size="sm"
    @close="emit('cancel')"
  >
    <ToastAlert
      v-if="confirmation.error"
      tone="error"
      title="Não foi possível concluir"
      :message="confirmation.error"
      :dismissible="false"
    />
    <template #actions>
      <BaseButton variant="secondary" :disabled="confirmation.pending" data-autofocus @click="emit('cancel')">
        {{ confirmation.cancelLabel }}
      </BaseButton>
      <BaseButton
        :variant="confirmation.tone === 'danger' ? 'danger' : 'primary'"
        :loading="confirmation.pending"
        @click="emit('confirm')"
      >
        {{ confirmation.confirmLabel }}
      </BaseButton>
    </template>
  </BaseModal>
</template>
