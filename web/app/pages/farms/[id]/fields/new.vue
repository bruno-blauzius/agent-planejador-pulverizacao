<script setup lang="ts">
/** "Novo talhão" (Figma 16:82). `?block=` pre-fills the area (from screen 9). */
definePageMeta({ permission: 'registry.write' })
const route = useRoute()
const farmId = computed(() => String(route.params.id))
</script>

<template>
  <FieldFormScreen :farm-id="farmId" />
</template>
