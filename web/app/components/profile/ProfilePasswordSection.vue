<script setup lang="ts">
/**
 * "Alterar senha" (Figma 54:953): current password, new password and its
 * confirmation. Optional: left blank, the password stays the same. Rules and
 * messages come from the caller's composable. Presentation only.
 */
export type PasswordField = 'currentPassword' | 'newPassword' | 'confirmPassword'

withDefaults(
  defineProps<{
    /** "Última alteração: há 3 meses"; hidden when unknown. */
    lastChangedLabel?: string | null
    newPasswordHint?: string
    errors?: Partial<Record<string, string>>
  }>(),
  { lastChangedLabel: null, newPasswordHint: 'Mínimo de 8 caracteres, com letras e números.', errors: () => ({}) },
)

const currentPassword = defineModel<string>('currentPassword', { required: true })
const newPassword = defineModel<string>('newPassword', { required: true })
const confirmPassword = defineModel<string>('confirmPassword', { required: true })

const emit = defineEmits<{
  touch: [field: PasswordField]
}>()

const titleId = useId()
</script>

<template>
  <section class="profile-password" :aria-labelledby="titleId">
    <header class="profile-password__header">
      <AppIcon name="lock" :size="18" class="profile-password__icon" />
      <h2 :id="titleId" class="profile-password__title text-h4">Alterar senha</h2>
      <p v-if="lastChangedLabel" class="profile-password__meta text-body-sm">{{ lastChangedLabel }}</p>
    </header>

    <TextField
      v-model="currentPassword"
      label="Senha atual"
      name="currentPassword"
      type="password"
      autocomplete="current-password"
      :error="errors.currentPassword"
      @blur="emit('touch', 'currentPassword')"
    />

    <div class="profile-password__row">
      <TextField
        v-model="newPassword"
        label="Nova senha"
        name="newPassword"
        type="password"
        autocomplete="new-password"
        :hint="newPasswordHint"
        :error="errors.newPassword"
        @blur="emit('touch', 'newPassword')"
      />
      <TextField
        v-model="confirmPassword"
        label="Confirmar nova senha"
        name="confirmPassword"
        type="password"
        autocomplete="new-password"
        :error="errors.confirmPassword"
        @blur="emit('touch', 'confirmPassword')"
      />
    </div>
  </section>
</template>

<style scoped>
.profile-password {
  display: flex;
  flex-direction: column;
  gap: var(--space-16);
  padding: var(--space-22);
  border: var(--border-width-hairline) solid var(--color-border);
  border-radius: var(--radius-xl);
  background: var(--color-white);
}

.profile-password__header {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-10);
  align-items: center;
}

.profile-password__icon {
  color: var(--color-green-700);
}

.profile-password__title {
  flex: 1;
  margin: 0;
  color: var(--color-text-primary);
}

.profile-password__meta {
  margin: 0;
  color: var(--color-text-tertiary);
}

.profile-password__row {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: var(--space-12);
  align-items: start;
}

@media (max-width: 599px) {
  .profile-password__row {
    grid-template-columns: minmax(0, 1fr);
  }
}
</style>
