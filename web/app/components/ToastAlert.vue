<script setup lang="ts">
import { VSnackbar } from 'vuetify/components'
import type { IconName } from './app/AppIcon.vue'

/**
 * Alert (Figma "Alerta" 26:236) or toast (instance 35:1728).
 * Inline: facade over v-alert. `floating`: facade over v-snackbar, shown while
 * the component is mounted (the parent controls it with v-if and `dismiss`).
 * agent = proactive recommendation from the AI agent (dark variant).
 */
export type AlertTone = 'info' | 'success' | 'attention' | 'error' | 'agent'
export type ToastLocation = 'top' | 'top right' | 'top left' | 'bottom' | 'bottom right' | 'bottom left'

const props = withDefaults(
  defineProps<{
    tone?: AlertTone
    title: string
    message?: string
    dismissible?: boolean
    floating?: boolean
    icon?: IconName
    dismissLabel?: string
    /** Floating only: screen position of the toast. */
    location?: ToastLocation
    /** Floating only: auto-dismiss after N ms; -1 keeps it until dismissed. */
    timeout?: number
  }>(),
  {
    tone: 'info',
    message: undefined,
    dismissible: true,
    floating: false,
    icon: undefined,
    dismissLabel: 'Dispensar mensagem',
    location: 'top right',
    timeout: -1,
  },
)

const emit = defineEmits<{
  dismiss: []
}>()

defineSlots<{
  /** Optional actions below the message (e.g. "Tentar novamente"). */
  actions?: () => unknown
}>()

const DEFAULT_ICON: Record<AlertTone, IconName> = {
  info: 'info',
  success: 'check',
  attention: 'drop',
  error: 'alert',
  agent: 'sparkles',
}

const iconName = computed(() => props.icon ?? DEFAULT_ICON[props.tone])
const isError = computed(() => props.tone === 'error')

/**
 * Inline, the v-alert is the live region (errors interrupt, the rest are polite).
 * Floating, the v-snackbar content already is one, so the alert gets no role
 * to avoid a double announcement.
 */
const alertRole = computed(() => {
  if (props.floating) return 'none'
  return isError.value ? 'alert' : 'status'
})

// Single rendering of the alert: wrapped by v-snackbar when floating, or by a
// layout-neutral element (display: contents) when inline.
const wrapper = computed(() => (props.floating ? VSnackbar : 'div'))
const snackbarClass = `toast-alert-snackbar-${useId()}`
const wrapperProps = computed(() =>
  props.floating
    ? {
        'modelValue': true,
        'location': props.location,
        'timeout': props.timeout,
        'class': 'toast-alert-snackbar',
        'contentClass': snackbarClass,
        'onUpdate:modelValue': onSnackbarUpdate,
      }
    : { class: 'toast-alert-inline' },
)

function onSnackbarUpdate(isOpen: boolean) {
  if (!isOpen) emit('dismiss')
}

/**
 * VSnackbar hardcodes role="status"/aria-live="polite" on its content and has no
 * prop to change it; errors must be assertive, so the attributes are adjusted
 * on the rendered (teleported) element.
 */
function applySnackbarLiveRegion() {
  if (!props.floating || !import.meta.client) return
  const content = document.querySelector(`.${snackbarClass} .v-snackbar__content`)
  content?.setAttribute('role', isError.value ? 'alert' : 'status')
  content?.setAttribute('aria-live', isError.value ? 'assertive' : 'polite')
}

watch(() => [props.floating, props.tone], () => nextTick(applySnackbarLiveRegion), { immediate: true })
onMounted(() => requestAnimationFrame(applySnackbarLiveRegion))
</script>

<template>
  <component :is="wrapper" v-bind="wrapperProps">
    <v-alert
      class="alert"
      :class="[`alert--${tone}`, { 'alert--floating': floating }]"
      :role="alertRole"
      :icon="false"
      variant="flat"
    >
      <ToastAlertContent
        :tone="tone"
        :icon="iconName"
        :title="title"
        :message="message"
        :dismissible="dismissible"
        :dismiss-label="dismissLabel"
        @dismiss="emit('dismiss')"
      >
        <template v-if="$slots.actions" #actions><slot name="actions" /></template>
      </ToastAlertContent>
    </v-alert>
  </component>
</template>

<style scoped>
.toast-alert-inline {
  display: contents;
}

.alert {
  --alert-bg: var(--color-info-rain-bg);
  --alert-accent: var(--color-info-rain);

  display: flex;
  padding: var(--space-16);
  border-radius: var(--radius-lg);
  background: var(--alert-bg);
  color: var(--color-text-primary);
}

.alert :deep(> .v-alert__underlay),
.alert :deep(> .v-alert__overlay) {
  display: none;
}

.alert :deep(> .v-alert__content) {
  width: 100%;
}

.alert--floating {
  box-shadow: var(--shadow-floating);
}

.alert--success {
  --alert-bg: var(--color-status-suitable-bg);
  --alert-accent: var(--color-status-suitable);
}

.alert--attention {
  --alert-bg: var(--color-status-attention-bg);
  --alert-accent: var(--color-status-attention);
}

.alert--error {
  --alert-bg: var(--color-status-unsuitable-bg);
  --alert-accent: var(--color-status-unsuitable);
}

.alert--agent {
  --alert-bg: var(--color-green-900);
  --alert-accent: var(--color-sprout-400);
}
</style>

<style>
/*
 * v-snackbar is teleported by Vuetify, outside the scoped styles: its wrapper is
 * made transparent so the alert above is the visible toast (Figma 35:1728).
 */
.toast-alert-snackbar .v-snackbar__wrapper {
  min-width: 0;
  padding: 0;
  border-radius: var(--radius-lg);
  background: transparent;
  box-shadow: none;
  color: inherit;
}

.toast-alert-snackbar .v-snackbar__content {
  padding: 0;
}
</style>
