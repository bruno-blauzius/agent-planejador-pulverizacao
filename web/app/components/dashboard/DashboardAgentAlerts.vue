<script setup lang="ts">
import type { AlertTone } from '~/components/ToastAlert.vue'
import type { AgentAlertView } from '~/utils/dashboard'

/**
 * "Alertas do agente" (Figma 54:753): inline alerts (attention, agent
 * highlight, info) that can be dismissed. Dismissing is local to this visit
 * (no endpoint yet); the caller decides.
 */
withDefaults(
  defineProps<{
    alerts: readonly AgentAlertView[]
    /** Data still loading (nothing to show yet). */
    pending?: boolean
    /** Loading failed: never claim the block is empty. */
    failed?: boolean
  }>(),
  { pending: false, failed: false },
)

const emit = defineEmits<{
  dismiss: [id: string]
}>()

const TONE: Record<AgentAlertView['tone'], AlertTone> = {
  attention: 'attention',
  highlight: 'agent',
  info: 'info',
}

const titleId = useId()
</script>

<template>
  <section class="agent-alerts" :aria-labelledby="titleId">
    <h2 :id="titleId" class="agent-alerts__title text-h4">Alertas do agente</h2>
    <DashboardBlockStatus v-if="alerts.length === 0" :pending="pending" :failed="failed">
      Nenhum alerta no momento.
    </DashboardBlockStatus>
    <ToastAlert
      v-for="alert in alerts"
      :key="alert.id"
      :tone="TONE[alert.tone]"
      :icon="alert.tone === 'highlight' ? undefined : alert.icon"
      :title="alert.title"
      :message="alert.description"
      dismiss-label="Dispensar alerta"
      @dismiss="emit('dismiss', alert.id)"
    />
  </section>
</template>

<style scoped>
.agent-alerts {
  display: flex;
  flex-direction: column;
  gap: var(--space-16);
  min-width: 0;
  padding: var(--space-22);
  border: var(--border-width-hairline) solid var(--color-border);
  border-radius: var(--radius-xl);
  background: var(--color-white);
}

.agent-alerts__title {
  margin: 0;
  color: var(--color-text-primary);
}

</style>
