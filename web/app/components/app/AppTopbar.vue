<script setup lang="ts">
import { APP_BAR_HEIGHT, px, TOPBAR_HEIGHT } from '~/theme/layout'

/**
 * Top bar of the app shell (Figma 7:87): global search with ⌘K hint,
 * a context slot (e.g. local weather chip), extra actions and notifications.
 * Phones (no Figma frame): the search collapses into an icon button that
 * expands over the bar, the ⌘K hint and the context chips are hidden.
 */
const props = withDefaults(
  defineProps<{
    searchPlaceholder?: string
    hasUnreadNotifications?: boolean
    /** Shows the menu button that opens the navigation drawer (small screens). */
    showNavigationToggle?: boolean
  }>(),
  {
    searchPlaceholder: 'Buscar fazenda, talhão ou proprietário…',
    hasUnreadNotifications: false,
    showNavigationToggle: false,
  },
)

const search = defineModel<string>('search', { default: '' })

const emit = defineEmits<{
  submit: [query: string]
  'open-notifications': []
  'toggle-navigation': []
}>()

defineSlots<{
  /** Context chip next to the actions (Figma 7:96, weather). Use `AppTopbarChip`. */
  context?: () => unknown
  /** Extra action buttons before the notification bell. */
  actions?: () => unknown
}>()

// Heights from theme/layout.ts: a number for v-app-bar, a CSS variable for the styles.
const barStyle = { '--topbar-height': px(TOPBAR_HEIGHT) }

const searchInput = ref<HTMLInputElement | null>(null)
/** Phones only (CSS decides when the collapsed state applies). */
const isSearchExpanded = ref(false)

async function expandSearch() {
  isSearchExpanded.value = true
  await nextTick()
  searchInput.value?.focus()
}

function onSearchFocusOut(event: FocusEvent) {
  const form = event.currentTarget as HTMLElement
  // Collapse when focus leaves the form and nothing was typed.
  if (!form.contains(event.relatedTarget as Node | null) && !search.value) isSearchExpanded.value = false
}

const notificationsLabel = computed(() =>
  props.hasUnreadNotifications ? 'Notificações, há itens não lidos' : 'Notificações',
)

function onSubmit() {
  emit('submit', search.value.trim())
}

function onGlobalKeydown(event: KeyboardEvent) {
  const isShortcut = (event.metaKey || event.ctrlKey) && event.key.toLowerCase() === 'k'
  if (!isShortcut) return
  event.preventDefault()
  searchInput.value?.focus()
}

onMounted(() => window.addEventListener('keydown', onGlobalKeydown))
onBeforeUnmount(() => window.removeEventListener('keydown', onGlobalKeydown))
</script>

<template>
  <!--
    Facade over v-app-bar (ADR 0011). `absolute`: the bar scrolls away with the
    page, as in the Figma, instead of sticking to the top.
  -->
  <v-app-bar
    class="topbar-bar"
    :height="APP_BAR_HEIGHT"
    :style="barStyle"
    absolute
    flat
    color="transparent"
  >
    <div class="topbar" :class="{ 'topbar--search-expanded': isSearchExpanded }">
      <button
        v-if="showNavigationToggle"
        type="button"
        class="topbar__icon-button"
        aria-label="Abrir navegação"
        @click="emit('toggle-navigation')"
      >
        <AppIcon name="list" :size="20" />
      </button>

      <button
        type="button"
        class="topbar__icon-button topbar__search-toggle"
        aria-label="Abrir busca"
        :aria-expanded="isSearchExpanded"
        aria-controls="global-search-form"
        @click="expandSearch"
      >
        <AppIcon name="search" :size="20" />
      </button>

      <form
        id="global-search-form"
        class="topbar__search"
        role="search"
        @submit.prevent="onSubmit"
        @focusout="onSearchFocusOut"
      >
        <label for="global-search" class="visually-hidden">Busca global</label>
        <AppIcon name="search" :size="18" class="topbar__search-icon" />
        <input
          id="global-search"
          ref="searchInput"
          v-model="search"
          type="search"
          class="topbar__search-input text-body-md"
          :placeholder="searchPlaceholder"
          autocomplete="off"
          aria-keyshortcuts="Control+K Meta+K"
        >
        <kbd class="topbar__shortcut text-label-shortcut" aria-hidden="true">⌘K</kbd>
      </form>

      <div class="topbar__spacer" />

      <div v-if="$slots.context" class="topbar__context">
        <slot name="context" />
      </div>
      <slot name="actions" />

      <button
        type="button"
        class="topbar__icon-button"
        :aria-label="notificationsLabel"
        @click="emit('open-notifications')"
      >
        <AppIcon name="bell" :size="20" />
        <span v-if="hasUnreadNotifications" class="topbar__dot" aria-hidden="true" />
      </button>
    </div>
  </v-app-bar>
</template>

<style scoped>
.topbar-bar :deep(.v-toolbar__content) {
  align-items: flex-end;
  padding: 0 var(--content-padding-x);
}

.topbar {
  display: flex;
  flex: 1;
  min-width: 0;
  gap: var(--space-12);
  align-items: center;
  min-height: var(--topbar-height);
}

.topbar__search {
  display: flex;
  gap: var(--space-10);
  align-items: center;
  width: var(--topbar-search-width);
  max-width: 100%;
  height: var(--size-topbar-control);
  padding: 0 var(--space-16);
  border: var(--border-width-hairline) solid var(--color-border);
  border-radius: var(--radius-md);
  background: var(--color-white);
}

.topbar__search:focus-within {
  border-color: var(--color-green-700);
  box-shadow: var(--focus-ring);
}

.topbar__search-icon {
  color: var(--color-text-secondary);
}

.topbar__search-input {
  flex: 1;
  min-width: 0;
  padding: 0;
  border: 0;
  background: transparent;
  color: var(--color-text-primary);
  outline: none;
}

.topbar__search-input::placeholder {
  color: var(--color-text-tertiary);
}

.topbar__shortcut {
  padding: var(--space-2) var(--space-6);
  border-radius: var(--radius-xs);
  background: var(--color-sand);
  color: var(--color-text-tertiary);
}

.topbar__spacer {
  flex: 1;
}

.topbar__icon-button {
  position: relative;
  display: grid;
  place-items: center;
  width: var(--size-topbar-control);
  height: var(--size-topbar-control);
  flex-shrink: 0;
  border: var(--border-width-hairline) solid var(--color-border);
  border-radius: var(--radius-md);
  background: var(--color-white);
  color: var(--color-text-secondary);
  cursor: pointer;
}

.topbar__icon-button:hover {
  background: var(--color-sand);
}

.topbar__context {
  display: contents;
}

/* Desktop: the toggle does not exist visually. */
.topbar__search-toggle {
  display: none;
}

@media (max-width: 599px) {
  .topbar {
    gap: var(--space-8);
  }

  .topbar__shortcut,
  .topbar__context {
    display: none;
  }

  .topbar__search-toggle {
    display: grid;
  }

  .topbar__search {
    display: none;
  }

  .topbar--search-expanded .topbar__search {
    display: flex;
    flex: 1;
    width: auto;
  }

  .topbar--search-expanded .topbar__search-toggle,
  .topbar--search-expanded .topbar__spacer {
    display: none;
  }
}

.topbar__dot {
  position: absolute;
  top: var(--notification-dot-top);
  right: var(--notification-dot-right);
  width: var(--size-status-dot);
  height: var(--size-status-dot);
  border: var(--border-width-strong) solid var(--color-white);
  border-radius: var(--radius-pill);
  background: var(--color-status-unsuitable);
}
</style>
