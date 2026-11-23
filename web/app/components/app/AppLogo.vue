<script setup lang="ts">
/**
 * Brand mark "broto agente": sprout tile with a two-leaf seedling and an
 * agent spark, plus the two-line wordmark. Always rendered on dark surfaces
 * (sidebar, auth hero). Standalone files live in public/brand/ (logo.svg,
 * logo-inverse.svg, icon.svg); keep the paths below in sync with them.
 */
</script>

<template>
  <span class="app-logo">
    <span class="app-logo__mark" aria-hidden="true">
      <svg viewBox="0 0 64 64" width="26" height="26" focusable="false">
        <path d="M32 56V31" fill="none" stroke="currentColor" stroke-width="4.5" stroke-linecap="round" />
        <path d="M32 36C19 36 11 28 11 15C24 15 32 23 32 36Z" fill="currentColor" />
        <path d="M32 31C32 18 40 9 54 9C54 23 45 31 32 31Z" fill="currentColor" />
        <path
          class="app-logo__spark"
          d="M50 37L52.2 43.8L59 46L52.2 48.2L50 55L47.8 48.2L41 46L47.8 43.8Z"
        />
      </svg>
    </span>
    <span class="app-logo__wordmark">
      <span class="app-logo__first">agentes</span>
      <span class="app-logo__second">agro</span>
    </span>
  </span>
</template>

<style scoped>
.app-logo {
  display: inline-flex;
  gap: var(--space-10);
  align-items: center;
}

.app-logo__mark {
  display: grid;
  place-items: center;
  width: var(--size-icon-tile-md);
  height: var(--size-icon-tile-md);
  border-radius: var(--radius-tile);
  background: var(--color-sprout-400);
  color: var(--color-green-950);
}

/* The spark marks the AI agent; on the sprout tile it needs a mid green to read apart from the leaves. */
.app-logo__spark {
  fill: var(--color-green-600);
}

.app-logo__wordmark {
  display: flex;
  flex-direction: column;
  white-space: nowrap;
}

.app-logo__first {
  font: var(--font-logo-primary);
  color: var(--color-text-inverse);
}

.app-logo__second {
  font: var(--font-logo-secondary);
  color: var(--color-sprout-400);
}
</style>
