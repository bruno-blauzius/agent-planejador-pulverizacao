<script setup lang="ts">
import { useDisplay } from 'vuetify'
import { px, SIDEBAR_WIDTH } from '~/theme/layout'
import type { Permission } from '~/types/auth'
import type { IconName } from './AppIcon.vue'

/** Logged-in user shown in the footer (Figma 52:281). Presentation-only shape. */
interface SidebarUser {
  name: string
  /** "Super admin · Agentes Agro" (role and company, ready to display). */
  subtitle: string
  /** Stable avatar tone (e.g. the user id). */
  id?: string
}

interface NavItem {
  label: string
  to: string
  icon: IconName
  badge?: string
  /** Capability that shows the item (ADR 0013: never the role name). */
  permission?: Permission
}

interface NavSection {
  id: string
  label: string
  items: readonly NavItem[]
}

const props = withDefaults(
  defineProps<{
    user?: SidebarUser
    /**
     * Capability check from usePermissions().can (ADR 0013). Items with a
     * `permission` are hidden while it is not given; visibility is only
     * presentation, the API enforces authorization.
     */
    can?: (permission: Permission) => boolean
    /** Destination of the user footer (profile, Figma 54:815). */
    profileTo?: string
  }>(),
  { user: undefined, can: undefined, profileTo: '/profile' },
)

/**
 * Visibility of the drawer on small screens. On desktop it is permanent; below
 * the mobile breakpoint (theme/vuetify.ts) it becomes a temporary drawer
 * controlled by this model.
 */
const open = defineModel<boolean>('open', { default: false })

const { mobile } = useDisplay()

// Width from theme/layout.ts: a number for the drawer, a CSS variable for the styles.
const drawerStyle = { '--sidebar-width': px(SIDEBAR_WIDTH) }

defineSlots<{
  /** Agent insight card above the user footer (Figma 7:69). Use `AppSidebarInsight`. */
  insight?: () => unknown
}>()

// Navigation is app structure, not business data, so it lives with the component.
const SECTIONS: readonly NavSection[] = [
  {
    id: 'operation',
    label: 'Operação',
    items: [
      { label: 'Painel', to: '/', icon: 'home', permission: 'dashboard.view' },
      { label: 'Fazendas', to: '/farms', icon: 'map' },
      { label: 'Proprietários', to: '/owners', icon: 'users' },
      { label: 'Planejador', to: '/planner', icon: 'sprout', badge: 'IA' },
      { label: 'Aplicações', to: '/applications', icon: 'calendar' },
    ],
  },
  {
    // Figma 52:266.
    id: 'administration',
    label: 'Administração',
    items: [
      { label: 'Usuários', to: '/users', icon: 'shield', permission: 'users.manage' },
      { label: 'Empresas', to: '/companies', icon: 'document', permission: 'companies.manage' },
    ],
  },
]

const route = useRoute()

function isAllowed(item: NavItem): boolean {
  if (!item.permission) return true
  return props.can?.(item.permission) ?? false
}

/** Sections with at least one allowed item; empty sections disappear with their title. */
const sections = computed(() =>
  SECTIONS.map((section) => ({ ...section, items: section.items.filter(isAllowed) }))
    .filter((section) => section.items.length > 0),
)

function isActive(to: string): boolean {
  if (to === '/') return route.path === '/'
  return route.path === to || route.path.startsWith(`${to}/`)
}

function initialsOf(name: string): string {
  return name
    .split(/\s+/)
    .filter(Boolean)
    .map((part) => part[0])
    .slice(0, 2)
    .join('')
    .toUpperCase()
}
</script>

<template>
  <!-- Facade over v-navigation-drawer (ADR 0011): responsive collapse comes from Vuetify. -->
  <v-navigation-drawer
    :model-value="mobile ? open : true"
    :permanent="!mobile"
    tag="aside"
    class="sidebar-drawer"
    :width="SIDEBAR_WIDTH"
    :style="drawerStyle"
    aria-label="Navegação principal"
    @update:model-value="open = $event"
  >
    <div class="sidebar">
      <NuxtLink to="/" class="sidebar__brand" aria-label="Agentes Agro, ir para o painel">
        <AppLogo />
      </NuxtLink>

      <nav
        v-for="section in sections"
        :key="section.id"
        class="sidebar__nav"
        :aria-labelledby="`sidebar-section-${section.id}`"
      >
        <p :id="`sidebar-section-${section.id}`" class="sidebar__section text-overline">{{ section.label }}</p>
        <ul class="sidebar__list">
          <li v-for="item in section.items" :key="item.to">
            <NuxtLink
              :to="item.to"
              class="sidebar__item"
              :class="{ 'sidebar__item--active': isActive(item.to) }"
              :aria-current="isActive(item.to) ? 'page' : undefined"
            >
              <AppIcon :name="item.icon" :size="20" />
              <span class="sidebar__label">{{ item.label }}</span>
              <span v-if="item.badge" class="sidebar__badge text-label-micro">{{ item.badge }}</span>
            </NuxtLink>
          </li>
        </ul>
      </nav>

      <div class="sidebar__spacer" />

      <slot name="insight" />

      <!-- Current user (Figma 52:281): opens the profile. -->
      <NuxtLink
        v-if="user"
        :to="profileTo"
        class="sidebar__user"
        :class="{ 'sidebar__user--active': isActive(profileTo) }"
        :aria-label="`Meu perfil: ${user.name}, ${user.subtitle}`"
      >
        <BaseAvatar :initials="initialsOf(user.name)" :seed="user.id ?? user.name" tone="coffee" />
        <span class="sidebar__user-text" aria-hidden="true">
          <span class="sidebar__user-name text-label-meta">{{ user.name }}</span>
          <span class="sidebar__user-subtitle text-body-sm">{{ user.subtitle }}</span>
        </span>
        <AppIcon name="chevron-right" :size="16" class="sidebar__user-chevron" />
      </NuxtLink>
    </div>
  </v-navigation-drawer>
</template>

<style scoped>
.sidebar-drawer {
  border: 0;
  background: var(--color-green-900);
}

.sidebar {
  display: flex;
  flex-direction: column;
  gap: var(--space-4);
  width: var(--sidebar-width);
  height: 100%;
  padding: var(--space-28) var(--space-16) var(--space-20);
  background: var(--color-green-900);
  color: var(--color-text-inverse-strong);
}

.sidebar__brand {
  align-self: flex-start;
  margin: 0 var(--space-8) var(--space-28);
  border-radius: var(--radius-md);
  text-decoration: none;
}

.sidebar__nav + .sidebar__nav .sidebar__section {
  padding-top: var(--space-8);
}

.sidebar__section {
  padding: 0 var(--space-12) var(--space-8);
  color: var(--color-text-inverse-faint);
  letter-spacing: var(--tracking-overline-wide);
}

.sidebar__list {
  display: flex;
  flex-direction: column;
  gap: var(--space-4);
  margin: 0;
  padding: 0;
  list-style: none;
}

.sidebar__item {
  display: flex;
  gap: var(--space-12);
  align-items: center;
  padding: var(--padding-nav-item-y) var(--space-12);
  border-radius: var(--radius-md);
  font: var(--font-nav-item);
  color: var(--color-text-inverse-strong);
  text-decoration: none;
  transition: background-color var(--duration-fast) var(--easing-standard);
}

.sidebar__item:hover {
  background: var(--color-surface-inverse-soft);
}

.sidebar__item:focus-visible {
  outline: var(--border-width-strong) solid var(--color-sprout-400);
  outline-offset: var(--space-2);
}

.sidebar__item--active,
.sidebar__item--active:hover {
  background: var(--color-sprout-400);
  color: var(--color-green-950);
  font: var(--font-label-strong);
}

.sidebar__label {
  flex: 1;
  min-width: 0;
}

.sidebar__badge {
  padding: var(--space-2) var(--space-8);
  border-radius: var(--radius-pill);
  background: var(--color-sprout-badge);
  color: var(--color-sprout-400);
}

.sidebar__item--active .sidebar__badge {
  background: var(--color-green-950);
}

.sidebar__spacer {
  flex: 1;
}

.sidebar__user {
  display: flex;
  gap: var(--space-10);
  align-items: center;
  padding: var(--space-12);
  border-radius: var(--radius-tile-lg);
  background: var(--color-surface-inverse-soft);
  color: var(--color-text-inverse-strong);
  text-decoration: none;
}

.sidebar__user:hover,
.sidebar__user--active {
  background: var(--color-surface-inverse-hover);
}

.sidebar__user:focus-visible {
  outline: var(--border-width-strong) solid var(--color-sprout-400);
  outline-offset: var(--space-2);
}

.sidebar__user-text {
  display: flex;
  flex: 1;
  flex-direction: column;
  gap: var(--space-2);
  min-width: 0;
  white-space: nowrap;
}

.sidebar__user-name {
  overflow: hidden;
  color: var(--color-text-inverse);
  text-overflow: ellipsis;
}

.sidebar__user-subtitle {
  overflow: hidden;
  color: var(--color-text-inverse-subtle);
  text-overflow: ellipsis;
}

.sidebar__user-chevron {
  color: var(--color-text-inverse-strong);
}
</style>
