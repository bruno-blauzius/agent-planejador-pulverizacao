<script setup lang="ts">
/**
 * "Dados pessoais" (Figma 54:935): the user edits the full name; the e-mail is
 * managed by an administrator and shown disabled. Presentation only.
 */
withDefaults(
  defineProps<{
    email: string
    errors?: Partial<Record<string, string>>
  }>(),
  { errors: () => ({}) },
)

const name = defineModel<string>('name', { required: true })

const emit = defineEmits<{
  touch: [field: 'name']
}>()

const titleId = useId()
</script>

<template>
  <section class="profile-section" :aria-labelledby="titleId">
    <h2 :id="titleId" class="profile-section__title text-h4">Dados pessoais</h2>
    <TextField
      v-model="name"
      label="Nome completo"
      name="name"
      autocomplete="name"
      required
      :error="errors.name"
      @blur="emit('touch', 'name')"
    />
    <TextField
      :model-value="email"
      label="E-mail"
      name="email"
      type="email"
      autocomplete="email"
      disabled
    />
  </section>
</template>

<style scoped>
.profile-section {
  display: flex;
  flex-direction: column;
  gap: var(--space-16);
  padding: var(--space-22);
  border: var(--border-width-hairline) solid var(--color-border);
  border-radius: var(--radius-xl);
  background: var(--color-white);
}

.profile-section__title {
  margin: 0;
  color: var(--color-text-primary);
}
</style>
