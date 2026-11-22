<script setup lang="ts">
import { toRaw } from 'vue'
import { useDisplay } from 'vuetify'
import { NOTICE_TIMEOUT_MS } from '~/theme/layout'
import type { FlashMessage } from '~/types/flash'
import { USER_ROLE_LABELS } from '~/utils/labels'
import { PERMISSION_DENIED_FLASH } from '~/utils/permissions'

/**
 * App shell (Figma frame 7:16) on the Vuetify layout system: navigation drawer
 * (AppSidebar), app bar (AppTopbar) and v-main. Below the mobile breakpoint the
 * drawer becomes temporary and the top bar shows a button to open it.
 * Insight and weather data will be injected through composables from the logic
 * layer; until then those optional parts of the shell stay hidden.
 */
const { mobile } = useDisplay()
const { user } = useSession()
const { can, role, company } = usePermissions()
const flash = useFlash()

/** Footer of the sidebar: name and "Papel · Empresa" (Figma 52:281). */
const sidebarUser = computed(() => {
  if (!user.value || !role.value) return undefined
  const subtitle = [USER_ROLE_LABELS[role.value], company.value?.name].filter(Boolean).join(' · ')
  return { id: user.value.id, name: user.value.name, subtitle }
})

/*
 * "Acesso não permitido" (route middleware) is shown by the shell, not by the
 * landing page: the redirect may land on the page already open, which does not
 * remount. Other flash messages stay with the pages that consume them.
 */
const accessDenied = ref<FlashMessage | null>(null)

function showAccessDenied(message: FlashMessage | null) {
  if (!message || toRaw(message) !== PERMISSION_DENIED_FLASH) return
  accessDenied.value = flash.consume()
}

watch(() => flash.message.value, showAccessDenied)
onMounted(() => showAccessDenied(flash.message.value))

// Only used below the mobile breakpoint; on desktop the drawer is permanent.
const isNavigationOpen = ref(false)

function toggleNavigation() {
  isNavigationOpen.value = !isNavigationOpen.value
}
</script>

<template>
  <div class="app-shell">
    <a href="#main-content" class="app-shell__skip">Pular para o conteúdo</a>
    <AppSidebar v-model:open="isNavigationOpen" :can="can" :user="sidebarUser" />
    <AppTopbar :show-navigation-toggle="mobile" @toggle-navigation="toggleNavigation" />
    <v-main id="main-content" class="app-shell__main" tabindex="-1">
      <div class="app-shell__content">
        <slot />
      </div>
    </v-main>
    <ToastAlert
      v-if="accessDenied"
      floating
      :tone="accessDenied.kind"
      :title="accessDenied.title"
      :message="accessDenied.description"
      :timeout="NOTICE_TIMEOUT_MS"
      @dismiss="accessDenied = null"
    />
  </div>
</template>

<style scoped>
.app-shell {
  min-height: 100vh;
}

.app-shell__main:focus {
  outline: none;
}

.app-shell__content {
  display: flex;
  flex-direction: column;
  gap: var(--space-28);
  padding: var(--space-28) var(--content-padding-x) var(--space-40);
}

.app-shell__skip {
  position: absolute;
  top: var(--space-8);
  left: var(--space-8);
  z-index: var(--z-skip-link);
  padding: var(--space-8) var(--space-12);
  border-radius: var(--radius-sm);
  background: var(--color-white);
  transform: translateY(-200%);
}

.app-shell__skip:focus {
  transform: none;
}
</style>
