<script setup lang="ts">
/**
 * "No permission" state (ADR 0013): shown when the session lacks a capability
 * or the API answers 403. Honest message and a way back; never a blank page.
 */
withDefaults(
  defineProps<{
    title?: string
    description?: string
    /** Where the "Voltar" action goes; null hides it. */
    backTo?: string | null
    backLabel?: string
  }>(),
  {
    title: 'Você não tem permissão para acessar esta página',
    description: 'Seu perfil não inclui esta área. Se precisar dela, fale com o administrador da sua empresa.',
    backTo: '/',
    backLabel: 'Voltar ao painel',
  },
)
</script>

<template>
  <EmptyState icon="lock" :title="title" :description="description" variant="forbidden">
    <template v-if="backTo" #actions>
      <BaseButton variant="secondary" :to="backTo">{{ backLabel }}</BaseButton>
    </template>
  </EmptyState>
</template>
