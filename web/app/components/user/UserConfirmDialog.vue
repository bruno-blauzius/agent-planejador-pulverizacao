<script setup lang="ts">
/**
 * Confirmation of a destructive row action ("Inativar", "Cancelar convite"):
 * small BaseModal with the texts that come with the action.
 */
defineProps<{
  title: string
  description: string
  confirmLabel: string
  loading?: boolean
}>()

const open = defineModel<boolean>('open', { default: false })

const emit = defineEmits<{
  confirm: []
}>()
</script>

<template>
  <BaseModal v-model:open="open" :title="title" :description="description" icon="alert" size="sm">
    <template #actions>
      <BaseButton variant="secondary" @click="open = false">Voltar</BaseButton>
      <BaseButton variant="danger" :loading="loading" @click="emit('confirm')">{{ confirmLabel }}</BaseButton>
    </template>
  </BaseModal>
</template>
