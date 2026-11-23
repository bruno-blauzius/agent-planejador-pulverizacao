<script setup lang="ts">
/**
 * Login (Figma 6:16, form 6:76). Presentation only: state, validation, the
 * request and the post-login redirect live in useLoginForm.
 * Real API (ADR 0015): no password here; the button leaves for the SSO Inceres
 * login. The e-mail/password form is the dev mock only.
 */
definePageMeta({ layout: 'auth', public: true })
useHead({ title: 'Entrar · Agentes Agro' })

const {
  kind,
  ssoUrl,
  accessError,
  identifier,
  password,
  rememberMe,
  fieldErrors,
  formError,
  pending,
  submit,
  touch,
} = useLoginForm()

const isForgotPasswordOpen = ref(false)
</script>

<template>
  <div class="login">
    <header class="login__header">
      <h1 class="login__title text-page-title">Bem-vindo de volta</h1>
      <p class="login__subtitle text-body-base">
        Entre para acompanhar suas fazendas e janelas de aplicação.
      </p>
    </header>

    <ToastAlert
      v-if="accessError"
      tone="error"
      :title="accessError.title"
      :message="accessError.message"
      :dismissible="false"
    />

    <div v-if="kind === 'sso'" class="login__sso">
      <BaseButton size="lg-compact" block :href="ssoUrl ?? undefined">
        Entrar com o login Inceres
      </BaseButton>
      <p class="login__caption text-body-sm">
        Você vai para a página de login da Inceres e volta para o Agentes Agro em seguida.
      </p>
    </div>

    <!-- method="post": a submit before hydration must never put the password in the URL. -->
    <form v-else class="login__form" method="post" novalidate @submit.prevent="submit()">
      <div class="login__fields">
        <TextField
          v-model="identifier"
          label="E-mail"
          name="identifier"
          placeholder="voce@fazenda.com.br"
          autocomplete="username"
          :error="fieldErrors.identifier"
          @blur="touch('identifier')"
        />
        <TextField
          v-model="password"
          label="Senha"
          name="password"
          type="password"
          placeholder="••••••••••"
          autocomplete="current-password"
          :error="fieldErrors.password"
          @blur="touch('password')"
        />
        <div class="login__options">
          <BaseCheckbox v-model="rememberMe" label="Manter conectado" name="remember" />
          <button type="button" class="login__forgot text-label-strong" @click="isForgotPasswordOpen = true">
            Esqueci minha senha
          </button>
        </div>
      </div>

      <ToastAlert
        v-if="formError"
        tone="error"
        title="Não foi possível entrar"
        :message="formError"
        :dismissible="false"
      />

      <BaseButton type="submit" size="lg-compact" block :loading="pending">
        Entrar
      </BaseButton>
    </form>

    <div class="login__divider" role="separator" aria-label="ou">
      <span class="login__divider-line" />
      <span class="login__divider-text text-caption-md" aria-hidden="true">ou</span>
      <span class="login__divider-line" />
    </div>

    <div class="login__google">
      <BaseButton variant="secondary" size="lg-compact" block unavailable-hint="Em breve" class="login__google-button">
        <span class="login__google-mark" aria-hidden="true">G</span>
        Entrar com o Google
        <span class="login__soon text-label-badge" aria-hidden="true">Em breve</span>
      </BaseButton>
      <p class="login__caption text-body-sm">
        Use a conta Google vinculada ao seu cadastro na cooperativa.
      </p>
    </div>

    <p class="login__first-access text-body-md">
      Primeiro acesso?
      <strong class="login__first-access-strong text-label-strong">Fale com seu técnico de campo</strong>
    </p>

    <BaseModal
      v-model:open="isForgotPasswordOpen"
      size="sm"
      icon="lock"
      title="Esqueci minha senha"
      description="Fale com seu técnico de campo para redefinir a senha."
    >
      <template #actions>
        <BaseButton block @click="isForgotPasswordOpen = false">Entendi</BaseButton>
      </template>
    </BaseModal>
  </div>
</template>

<style scoped>
/* Figma 6:76: 440 px column, 28 px between blocks, 18 px between fields. */
.login {
  display: flex;
  flex-direction: column;
  gap: var(--space-28);
  width: 100%;
}

.login__header {
  display: flex;
  flex-direction: column;
  gap: var(--space-8);
}

.login__title {
  color: var(--color-text-primary);
}

.login__subtitle {
  color: var(--color-text-secondary);
}

.login__form {
  display: flex;
  flex-direction: column;
  gap: var(--space-28);
}

.login__sso {
  display: flex;
  flex-direction: column;
  gap: var(--space-16);
}

.login__fields {
  display: flex;
  flex-direction: column;
  gap: var(--space-18);
}

.login__options {
  display: flex;
  gap: var(--space-12);
  align-items: center;
  justify-content: space-between;
}

.login__forgot {
  padding: 0;
  border: 0;
  border-radius: var(--radius-xs);
  background: transparent;
  color: var(--color-green-600);
  cursor: pointer;
}

.login__forgot:hover {
  text-decoration: underline;
}

.login__forgot:focus-visible {
  outline: none;
  box-shadow: var(--focus-ring);
}

.login__divider {
  display: flex;
  gap: var(--space-12);
  align-items: center;
}

.login__divider-line {
  flex: 1;
  height: var(--border-width-hairline);
  background: var(--color-border);
}

.login__divider-text {
  color: var(--color-text-tertiary);
}

.login__google {
  display: flex;
  flex-direction: column;
  gap: var(--space-28);
}

.login__google-button {
  position: relative;
}

/* Neutral "G" placeholder as in the Figma (34:360 asks for the official logo later). */
.login__google-mark {
  position: absolute;
  top: 50%;
  left: var(--space-16);
  display: grid;
  place-items: center;
  width: var(--size-brand-mark);
  height: var(--size-brand-mark);
  border: var(--border-width-hairline) solid var(--color-border);
  border-radius: var(--radius-pill);
  background: var(--color-white);
  color: var(--color-info-rain);
  font: var(--font-brand-mark);
  transform: translateY(-50%);
}

.login__soon {
  margin-left: var(--space-8);
  padding: var(--space-2) var(--space-8);
  border-radius: var(--radius-pill);
  background: var(--color-sand);
  color: var(--color-text-tertiary);
}

.login__caption {
  color: var(--color-text-tertiary);
  text-align: center;
}

.login__first-access {
  color: var(--color-text-secondary);
  text-align: center;
}

.login__first-access-strong {
  margin-left: var(--space-4);
  color: var(--color-green-700);
}
</style>
