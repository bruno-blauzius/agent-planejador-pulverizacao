<script setup lang="ts">
/**
 * Perfil do usuário (screen 11, Figma 54:815 / 54:882). Every authenticated
 * role reaches its own profile (no `permission`). Presentation only: data,
 * validation and saving come from useProfilePage.
 */
useHead({ title: 'Perfil · Agentes Agro' })

const {
  pending,
  error,
  refresh,
  identity,
  photo,
  name,
  email,
  currentPassword,
  newPassword,
  confirmPassword,
  passwordHint,
  passwordChangedLabel,
  fieldErrors,
  touch,
  isDirty,
  saving,
  save,
  discard,
  notice,
  dismissNotice,
  formError,
} = useProfilePage()

const form = ref<HTMLFormElement | null>(null)
const errorMessage = ref<HTMLElement | null>(null)

const isFirstLoad = computed(() => pending.value && !identity.value)

/** A blocked save lands focus on the reason: the general error, else the first invalid field. */
async function onSubmit() {
  if (await save()) return
  await nextTick()
  if (formError.value) {
    errorMessage.value?.focus()
    return
  }
  form.value?.querySelector<HTMLElement>('input[aria-invalid="true"]')?.focus()
}
</script>

<template>
  <div class="profile-page">
    <PageHeader eyebrow="Minha conta" title="Perfil" />

    <ToastAlert
      v-if="notice"
      :tone="notice.kind === 'success' ? 'success' : 'error'"
      :title="notice.title"
      :message="notice.description ?? undefined"
      @dismiss="dismissNotice"
    />

    <div v-if="isFirstLoad" class="profile-page__loading" role="status" aria-label="Carregando perfil">
      <span class="profile-page__skeleton profile-page__skeleton--identity" />
      <span class="profile-page__skeleton" />
    </div>

    <ToastAlert
      v-else-if="error && !identity"
      tone="error"
      title="Não foi possível carregar o seu perfil"
      :message="error.message"
      :dismissible="false"
    >
      <template #actions>
        <BaseButton variant="secondary" size="sm" @click="refresh()">Tentar novamente</BaseButton>
      </template>
    </ToastAlert>

    <div v-else-if="identity" class="profile-page__layout">
      <ProfileIdentityCard
        :name="identity.name"
        :email="identity.email"
        :initials="identity.initials"
        :seed="identity.email"
        :role-label="identity.roleLabel"
        :company-label="identity.companyName"
        :photo-unavailable-hint="photo.hint"
        :managed-title="identity.managedTitle"
        :managed-text="identity.managedText"
      />

      <!-- method="post": a submit before hydration must never put passwords in the URL. -->
      <form ref="form" method="post" novalidate class="profile-page__form" @submit.prevent="onSubmit">
        <ProfilePersonalSection
          v-model:name="name"
          :email="email"
          :errors="fieldErrors"
          @touch="touch"
        />
        <ProfilePasswordSection
          v-model:current-password="currentPassword"
          v-model:new-password="newPassword"
          v-model:confirm-password="confirmPassword"
          :last-changed-label="passwordChangedLabel"
          :new-password-hint="passwordHint"
          :errors="fieldErrors"
          @touch="touch"
        />

        <p
          v-if="formError"
          ref="errorMessage"
          class="profile-page__error text-body-sm"
          role="alert"
          tabindex="-1"
        >
          {{ formError }}
        </p>

        <div class="profile-page__actions">
          <BaseButton variant="ghost" :disabled="!isDirty || saving" @click="discard">Descartar</BaseButton>
          <BaseButton type="submit" :loading="saving" :disabled="!isDirty">Salvar alterações</BaseButton>
        </div>
      </form>
    </div>
  </div>
</template>

<style scoped>
.profile-page {
  display: flex;
  flex-direction: column;
  gap: var(--space-24);
}

.profile-page__layout {
  display: grid;
  grid-template-columns: var(--profile-identity-width) minmax(0, 1fr);
  gap: var(--space-24);
  align-items: start;
}

.profile-page__form {
  display: flex;
  flex-direction: column;
  gap: var(--space-20);
  min-width: 0;
}

.profile-page__actions {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-10);
  justify-content: flex-end;
}

.profile-page__error {
  margin: 0;
  padding: var(--space-12) var(--space-14);
  border-radius: var(--radius-md);
  background: var(--color-status-unsuitable-bg);
  color: var(--color-status-unsuitable);
  outline: none;
}

.profile-page__error:focus-visible {
  box-shadow: var(--focus-ring);
}

.profile-page__loading {
  display: grid;
  grid-template-columns: var(--profile-identity-width) minmax(0, 1fr);
  gap: var(--space-24);
}

.profile-page__skeleton {
  height: var(--profile-skeleton-height);
  border-radius: var(--radius-xl);
  background: var(--color-white);
  animation: profile-pulse var(--duration-skeleton) var(--easing-standard) infinite;
}

@keyframes profile-pulse {
  50% {
    opacity: var(--opacity-skeleton-dim);
  }
}

@media (prefers-reduced-motion: reduce) {
  .profile-page__skeleton {
    animation: none;
  }
}

@media (max-width: 959px) {
  .profile-page__layout,
  .profile-page__loading {
    grid-template-columns: minmax(0, 1fr);
  }
}

@media (max-width: 599px) {
  .profile-page__actions {
    flex-direction: column-reverse;
    align-items: stretch;
  }
}
</style>
