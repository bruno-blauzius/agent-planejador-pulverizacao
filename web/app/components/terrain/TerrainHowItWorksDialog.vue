<script setup lang="ts">
/**
 * "Como calculamos" dialog (no Figma frame, decision 3 of the contract): the
 * source of the elevation data, the Embrapa classes and the attribution the
 * Copernicus license requires. The copy comes from the logic layer
 * (`FarmTerrainTab.methodology`), so the numbers live in one place.
 */
defineProps<{
  title: string
  paragraphs: readonly string[]
  /** "Contém dados modificados do Copernicus DEM GLO-30 © DLR e Airbus". */
  attribution: string
}>()

const open = defineModel<boolean>('open', { default: false })
</script>

<template>
  <BaseModal v-model:open="open" icon="relief" :title="title">
    <div class="terrain-how">
      <p v-for="paragraph in paragraphs" :key="paragraph" class="terrain-how__paragraph text-body-md">{{ paragraph }}</p>
      <p class="terrain-how__attribution text-caption">{{ attribution }}</p>
    </div>

    <template #actions>
      <BaseButton variant="secondary" block @click="open = false">Entendi</BaseButton>
    </template>
  </BaseModal>
</template>

<style scoped>
.terrain-how {
  display: flex;
  flex-direction: column;
  gap: var(--space-12);
}

.terrain-how__paragraph {
  margin: 0;
  color: var(--color-text-secondary);
}

.terrain-how__attribution {
  margin: 0;
  padding: var(--space-10) var(--space-12);
  border-radius: var(--radius-md);
  background: var(--color-sand);
  color: var(--color-text-secondary);
}
</style>
