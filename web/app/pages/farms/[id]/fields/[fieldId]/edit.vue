<script setup lang="ts">
/** "Editar talhão": the same screen as "Novo talhão" (Figma 16:82), filled with the field. */
definePageMeta({ permission: 'registry.write' })
const route = useRoute()
const farmId = computed(() => String(route.params.id))
const fieldId = computed(() => String(route.params.fieldId))
</script>

<template>
  <FieldFormScreen :farm-id="farmId" :field-id="fieldId" />
</template>
