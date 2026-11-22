<script setup lang="ts">
/**
 * Authentication layout (Figma 6:16): brand hero on the left, form area on the
 * right (page content goes in the default slot) and the legal footer.
 */
const currentYear = new Date().getFullYear()
</script>

<template>
  <div class="auth-layout">
    <AppAuthHero class="auth-layout__hero" />
    <div class="auth-layout__panel">
      <main class="auth-layout__main">
        <slot />
      </main>
      <footer class="auth-layout__footer text-caption">
        <span>© {{ currentYear }} Agentes Agro</span>
        <span aria-hidden="true">·</span>
        <!-- Terms and privacy pages do not exist yet: plain text until they do. -->
        <span>Termos</span>
        <span aria-hidden="true">·</span>
        <span>Privacidade</span>
      </footer>
    </div>
  </div>
</template>

<style scoped>
.auth-layout {
  display: grid;
  grid-template-columns: minmax(0, 780fr) minmax(0, 644fr);
  gap: var(--space-16);
  min-height: 100vh;
  padding: var(--space-16);
  background: var(--color-sand);
}

.auth-layout__panel {
  display: flex;
  flex-direction: column;
  align-items: center;
  padding: var(--space-48) var(--space-24) var(--space-24);
}

.auth-layout__main {
  display: flex;
  flex: 1;
  flex-direction: column;
  justify-content: center;
  width: min(var(--auth-form-width), 100%);
}

.auth-layout__footer {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-4);
  width: min(var(--auth-form-width), 100%);
  padding-top: var(--space-40);
  color: var(--color-text-tertiary);
}


/* Custom properties are not allowed in media queries: below the 1024 px desktop breakpoint. */
@media (max-width: 1023px) {
  .auth-layout {
    grid-template-columns: minmax(0, 1fr);
  }

  .auth-layout__hero {
    display: none;
  }
}
</style>
