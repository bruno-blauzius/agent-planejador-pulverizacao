<script setup lang="ts">
/**
 * Mobile layout of the operator view (Figma 20:18): no sidebar or top bar.
 * A phone-width column on the sand background; on wider screens it stays
 * centered at that width. The page owns its header and bottom navigation.
 */
</script>

<template>
  <div class="mobile-shell">
    <a href="#main-content" class="mobile-shell__skip">Pular para o conteúdo</a>
    <main id="main-content" class="mobile-shell__main" tabindex="-1">
      <slot />
    </main>
  </div>
</template>

<style scoped>
.mobile-shell {
  display: flex;
  justify-content: center;
  min-height: 100vh;
  min-height: 100dvh;
  background: var(--color-sand);
}

.mobile-shell__main {
  display: flex;
  flex-direction: column;
  width: min(var(--planner-phone-width), 100%);
  background: var(--color-sand);
}

.mobile-shell__main:focus {
  outline: none;
}

.mobile-shell__skip {
  position: absolute;
  top: var(--space-8);
  left: var(--space-8);
  z-index: var(--z-skip-link);
  padding: var(--space-8) var(--space-12);
  border-radius: var(--radius-sm);
  background: var(--color-white);
  transform: translateY(-200%);
}

.mobile-shell__skip:focus {
  transform: none;
}
</style>
