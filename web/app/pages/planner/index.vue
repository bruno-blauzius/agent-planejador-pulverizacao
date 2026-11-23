<script setup lang="ts">
import type { IconName } from '~/components/app/AppIcon.vue'
import type { ContextBarItem } from '~/components/planner/PlannerContextBar.vue'
import type { ToolTraceItem } from '~/components/planner/PlannerToolTrace.vue'
import type { WindowMapRow } from '~/components/planner/PlannerWindowMap.vue'
import type { ConsultedSourceKind, SprayPlan } from '~/types/planner'
import type { ChatEntry, ConditionStatView } from '~/utils/planner'
import { NOTICE_TIMEOUT_MS } from '~/theme/layout'
import { hourOfClock } from '~/utils/planner'

/**
 * Planejador de pulverização (screen 16, Figma 18:2). Presentation only: the
 * selection, the conversation, the window map and the hourly conditions come
 * from usePlannerContext / usePlannerChat / usePlannerWindows. Whether a window
 * is suitable is computed by the API; nothing here derives it.
 */
definePageMeta({ permission: 'planner.use' })
useHead({ title: 'Planejador · Agentes Agro' })

const context = usePlannerContext()
const chat = usePlannerChat()
const windows = usePlannerWindows()
const confirmation = useApplicationConfirmation()
const { can } = usePermissions()

/** Example questions of the empty conversation (copy only; the agent answers). */
const EXAMPLE_QUESTIONS: readonly string[] = [
  'Quando posso aplicar fungicida essa semana?',
  'Qual a melhor janela para os talhões escolhidos?',
]
const UNAVAILABLE_HINT = 'Em breve'

const TRACE_ICONS: Record<ConsultedSourceKind, IconName> = {
  weather: 'cloud',
  conditions: 'thermometer',
  product_rules: 'book',
  operational_capacity: 'gauge',
}

const STAT_ICONS: Record<ConditionStatView['key'], IconName> = {
  delta_t: 'thermometer',
  wind: 'wind',
  rain: 'drop',
}

// ---- context picker ---------------------------------------------------------

type PickerKey = 'farm' | 'fields' | 'product'

const pickerKey = ref<PickerKey | null>(null)
const isPickerOpen = computed({
  get: () => pickerKey.value !== null,
  set: (open) => {
    if (!open) pickerKey.value = null
  },
})

const selectedFarmLabel = computed(
  () => context.farmOptions.value.find(option => option.value === context.farmId.value)?.label ?? 'Escolher',
)
const selectedProductLabel = computed(
  () => context.productOptions.find(option => option.value === context.productKind.value)?.label ?? 'Escolher',
)

const contextItems = computed<ContextBarItem[]>(() => [
  { key: 'farm', label: 'Fazenda', value: selectedFarmLabel.value },
  { key: 'fields', label: 'Talhões', value: context.selectedFieldsLabel.value || 'Escolher' },
  { key: 'product', label: 'Produto', value: selectedProductLabel.value },
])

const PICKER_TITLES: Record<PickerKey, string> = {
  farm: 'Escolher a fazenda',
  fields: 'Escolher os talhões',
  product: 'Escolher o produto',
}

function openPicker(key: string) {
  pickerKey.value = key as PickerKey
}

function onFarmPicked(farmId: string) {
  context.selectFarm(farmId)
  pickerKey.value = null
}

function onProductPicked(kind: (typeof context.productOptions)[number]['value']) {
  context.setProductKind(kind)
  pickerKey.value = null
}

// ---- states -----------------------------------------------------------------

const isForbidden = computed(() => !chat.canUse.value || chat.isForbidden.value || windows.isForbidden.value)
const isWeatherUnavailable = computed(() => chat.isWeatherUnavailable.value || windows.isWeatherUnavailable.value)
const hasConversation = computed(() => !chat.isEmpty.value)
const canAsk = computed(() => !chat.contextMissing.value && !chat.pending.value)
const placeholder = computed(() =>
  chat.contextMissing.value ? 'Escolha a fazenda e os talhões para perguntar' : 'Pergunte ao agente…',
)
const chatErrorMessage = computed(() => {
  if (chat.isAgentUnavailable.value) return 'O agente está indisponível no momento. Tente de novo em instantes.'
  if (chat.isWeatherUnavailable.value) return 'A previsão do tempo está indisponível, então o agente não consegue responder agora.'
  return chat.error.value?.message ?? ''
})
const isMapPending = computed(() => windows.pending.value && windows.map.value.isEmpty)
const isChartPending = computed(() => windows.conditionsPending.value && windows.conditions.value.isEmpty)

// ---- conversation -----------------------------------------------------------

function traceItems(entry: ChatEntry): ToolTraceItem[] {
  return entry.consulted.map((source, index) => ({
    key: `${entry.id}-${index}`,
    icon: TRACE_ICONS[source.kind],
    title: source.title,
    detail: source.detail,
  }))
}

/** The rain warning rides inside the agent bubble (Figma 18:153). */
function rainNote(entry: ChatEntry): string | undefined {
  return entry.plan?.entries.find(planEntry => planEntry.alert?.kind === 'rain_risk')?.alert?.message
}

const lastAgentEntryId = computed(() => [...chat.entries.value].reverse().find(entry => entry.role === 'agent')?.id)

function ask(text: string) {
  void chat.send(text)
}

// ---- plan -------------------------------------------------------------------

/** Other warnings (short window, interval) sit below the cards (Figma 18:195). */
const planNote = computed(() => {
  const messages = (chat.latestPlan.value?.entries ?? [])
    .flatMap(entry => (entry.alert && entry.alert.kind !== 'rain_risk' ? [entry.alert.message] : []))
  return messages.length ? [...new Set(messages)].join(' ') : undefined
})

/** Field of the plan under review in the confirmation modal (one field at a time). */
const reviewFieldId = ref<string | null>(null)

const reviewFieldChoices = computed(() => {
  const plan = chat.planView.value
  return (plan?.entries ?? []).map((entry) => {
    const registered = confirmation.isFieldRegistered(plan?.id ?? '', entry.fieldId)
    return {
      value: entry.fieldId,
      label: entry.fieldLabel,
      disabled: registered,
      hint: registered ? 'Aplicação já registrada' : undefined,
    }
  })
})

/** "Revisar e registrar plano": starts with the first field; the modal switches to the others. */
function reviewPlan() {
  const plan = chat.planView.value
  const first = plan?.entries.find(entry => !confirmation.isFieldRegistered(plan.id, entry.fieldId))
  if (!plan || !first) return
  reviewFieldId.value = first.fieldId
  void confirmation.open(plan.id, first.fieldId)
}

function reviewField(fieldId: string) {
  const plan = chat.planView.value
  if (!plan) return
  reviewFieldId.value = fieldId
  void confirmation.open(plan.id, fieldId)
}

/** Why "Revisar e registrar plano" is unavailable, when it is. */
const primaryHint = computed(() => {
  const plan = chat.planView.value
  if (!can('applications.register')) return 'Seu perfil não pode registrar aplicações'
  if (plan && plan.entries.every(entry => confirmation.isFieldRegistered(plan.id, entry.fieldId))) {
    return 'Aplicação já registrada'
  }
  return undefined
})

/** Registered: closes the modal and says so (the application is only "planned"). */
const registeredNotice = ref<{ title: string, message: string } | null>(null)

watch(() => confirmation.registered.value, (registered) => {
  if (!registered) return
  registeredNotice.value = confirmation.notice.value
  confirmation.close()
})

// The chart follows the first window of the plan the agent suggested.
watch(() => chat.latestPlan.value?.id, () => {
  const window = chat.latestPlan.value?.entries[0]?.window
  if (window) windows.selectWindow(window.date, window.startTime, window.endTime)
})

// ---- window map -------------------------------------------------------------

/** `date|hour` of the hours covered by the plan, per field. */
function plannedHours(plan: SprayPlan | null): Map<string, Set<string>> {
  const byField = new Map<string, Set<string>>()
  for (const entry of plan?.entries ?? []) {
    const hours = byField.get(entry.fieldId) ?? new Set<string>()
    const start = hourOfClock(entry.window.startTime)
    const end = hourOfClock(entry.window.endTime)
    for (let hour = start; hour < end; hour += 1) hours.add(`${entry.window.date}|${hour}`)
    byField.set(entry.fieldId, hours)
  }
  return byField
}

const mapDays = computed(() => windows.map.value.days.map(day => ({ key: day.date, label: day.label, hourLabels: day.hourLabels })))

const mapRows = computed<WindowMapRow[]>(() => {
  const planned = plannedHours(chat.latestPlan.value)
  const { days } = windows.map.value
  return windows.map.value.rows.map((row) => {
    const selected = planned.get(row.fieldId)
    return {
      key: row.fieldId,
      label: row.fieldLabel,
      // One cell per labeled slot (06, 09, 12…): the cell of that hour.
      cells: row.days.map((day, dayIndex) => (days[dayIndex]?.hourLabels ?? []).flatMap((label) => {
        const cell = day.cells.find(candidate => candidate.hour === Number(label))
        return cell ? [{ condition: cell.condition, selected: selected?.has(`${day.date}|${cell.hour}`) ?? false }] : []
      })),
    }
  })
})

// ---- hourly conditions ------------------------------------------------------

const conditionsSummary = computed(() => {
  const { title, stats } = windows.conditions.value
  return [title, ...stats.map(stat => `${stat.label}: ${stat.value}, ${stat.caption}`)].join('. ')
})
</script>

<template>
  <div class="planner">
    <ForbiddenState v-if="isForbidden" />

    <template v-else>
      <header class="planner__header">
        <div class="planner__heading">
          <p class="planner__eyebrow text-label-field">
            <AppIcon name="sparkles" :size="14" />
            Agente de pulverização
          </p>
          <h1 class="planner__title text-page-title">Planejador de janelas</h1>
        </div>
        <PlannerContextBar
          v-if="!context.isEmpty.value"
          :items="contextItems"
          :disabled="context.pending.value && context.farmOptions.value.length === 0"
          @select="openPicker"
        />
      </header>

      <EmptyState
        v-if="context.isEmpty.value"
        icon="map"
        title="Nenhuma fazenda liberada"
        description="Você ainda não tem fazendas liberadas para planejar. Fale com o administrador da sua empresa."
      />

      <template v-else>
        <ToastAlert
          v-if="context.error.value"
          tone="error"
          title="Não foi possível carregar as fazendas e talhões"
          :message="context.error.value.message"
          :dismissible="false"
        >
          <template #actions>
            <BaseButton variant="secondary" size="sm" icon="undo" @click="context.refresh()">Tentar novamente</BaseButton>
          </template>
        </ToastAlert>
        <ToastAlert
          v-if="isWeatherUnavailable"
          tone="attention"
          title="A previsão do tempo está indisponível"
          message="Sem a previsão não dá para calcular janelas. Nada foi estimado por conta própria."
          :dismissible="false"
        >
          <template #actions>
            <BaseButton variant="secondary" size="sm" icon="undo" @click="windows.refresh()">Tentar novamente</BaseButton>
          </template>
        </ToastAlert>

        <div class="planner__body">
          <section class="planner__chat" aria-labelledby="planner-chat-title">
            <header class="planner__chat-header">
              <span class="planner__chat-icon" aria-hidden="true"><AppIcon name="sparkles" :size="18" /></span>
              <div class="planner__chat-heading">
                <h2 id="planner-chat-title" class="planner__chat-title text-label-strong">Agente de pulverização</h2>
                <p class="planner__chat-status text-caption">
                  <span class="planner__status-dot" aria-hidden="true" />
                  online<template v-if="windows.map.value.updatedLabel"> · {{ windows.map.value.updatedLabel }}</template>
                </p>
              </div>
            </header>

            <div class="planner__messages">
              <div v-if="!hasConversation" class="planner__invite">
                <p class="planner__invite-text text-body-md">
                  <template v-if="chat.contextMissing.value">Escolha a fazenda e os talhões para planejar.</template>
                  <template v-else>Pergunte quando aplicar. O agente consulta a previsão, as condições e a capacidade do equipamento.</template>
                </p>
                <PlannerSuggestionChips
                  v-if="!chat.contextMissing.value"
                  :suggestions="EXAMPLE_QUESTIONS"
                  :disabled="!canAsk"
                  @select="ask"
                />
              </div>

              <PlannerChatLog
                v-else
                :pending="chat.pending.value"
                :error="chatErrorMessage"
              >
                <template v-for="entry in chat.entries.value" :key="entry.id">
                  <PlannerChatMessage v-if="entry.role === 'user'" role="user">
                    <p v-for="(paragraph, index) in entry.paragraphs" :key="index">{{ paragraph }}</p>
                  </PlannerChatMessage>
                  <template v-else>
                    <PlannerToolTrace v-if="entry.consulted.length" :items="traceItems(entry)" />
                    <PlannerChatMessage role="agent" :note="rainNote(entry)">
                      <p v-for="(paragraph, index) in entry.paragraphs" :key="index">{{ paragraph }}</p>
                    </PlannerChatMessage>
                    <PlannerSuggestionChips
                      v-if="entry.id === lastAgentEntryId && chat.suggestions.value.length"
                      :suggestions="chat.suggestions.value"
                      :disabled="!canAsk"
                      @select="ask"
                    />
                  </template>
                </template>
                <div v-if="chat.canRetry.value && !chat.pending.value" class="planner__retry">
                  <BaseButton variant="secondary" size="sm" icon="undo" @click="chat.retry()">Tentar de novo</BaseButton>
                </div>
              </PlannerChatLog>
            </div>

            <div class="planner__input">
              <PlannerChatInput
                v-model="chat.draft.value"
                :placeholder="placeholder"
                :disabled="!canAsk"
                @submit="chat.send()"
              />
            </div>
          </section>

          <div class="planner__panels">
            <PlannerPlanPanel
              v-if="chat.planView.value"
              :title="chat.planView.value.headline"
              :note="planNote"
              :actions-disabled="chat.pending.value"
              :primary-unavailable-hint="primaryHint"
              :secondary-unavailable-hint="UNAVAILABLE_HINT"
              @primary="reviewPlan"
            >
              <PlannerFieldPlanCard
                v-for="entry in chat.planView.value.entries"
                :key="entry.fieldId"
                :field-code="entry.fieldLabel"
                :window-label="entry.windowLabel"
                :detail="entry.detail"
                :condition="entry.condition"
                :short-window="entry.alertMessage !== null"
              />
            </PlannerPlanPanel>

            <!-- Forecast data is client-only: the server cannot know the pending state the client starts with. -->
            <ClientOnly>
              <section v-if="chat.contextMissing.value" class="planner__card" aria-labelledby="planner-windows-empty">
                <h2 id="planner-windows-empty" class="planner__card-title text-h4">Mapa de janelas · próximos 5 dias</h2>
                <p class="planner__empty text-body-md">Escolha a fazenda e os talhões para planejar.</p>
              </section>
              <template v-else>
                <div v-if="isMapPending" class="planner__card planner__skeleton planner__skeleton--map" aria-busy="true">
                  <span class="visually-hidden" role="status">Carregando o mapa de janelas…</span>
                </div>
                <PlannerWindowMap
                  v-else-if="!windows.map.value.isEmpty"
                  title="Mapa de janelas · próximos 5 dias"
                  :days="mapDays"
                  :rows="mapRows"
                  :active-day-key="windows.selectedDate.value"
                  @select-day="windows.selectDay"
                />
                <section v-else class="planner__card" aria-labelledby="planner-map-empty">
                  <h2 id="planner-map-empty" class="planner__card-title text-h4">Mapa de janelas · próximos 5 dias</h2>
                  <p class="planner__empty text-body-md">
                    <template v-if="windows.error.value">Dados indisponíveis no momento.</template>
                    <template v-else>Escolha a fazenda e os talhões para planejar.</template>
                  </p>
                </section>

                <div v-if="isChartPending" class="planner__card planner__skeleton planner__skeleton--chart" aria-busy="true">
                  <span class="visually-hidden" role="status">Carregando as condições hora a hora…</span>
                </div>
                <template v-else-if="!windows.conditions.value.isEmpty">
                  <PlannerConditionsChart :view="windows.conditions.value" :summary="conditionsSummary" />
                  <p v-if="!windows.conditions.value.window" class="planner__card planner__empty text-body-md" role="status">
                    Sem janela apta neste dia.
                  </p>
                  <ul v-else class="planner__stats" aria-label="Resumo da janela">
                    <li v-for="stat in windows.conditions.value.stats" :key="stat.key">
                      <PlannerConditionStat
                        :icon="STAT_ICONS[stat.key]"
                        :label="stat.label"
                        :value="stat.value"
                        :hint="stat.caption"
                      />
                    </li>
                  </ul>
                </template>
              </template>
              <template #fallback>
                <div class="planner__card planner__skeleton planner__skeleton--map" aria-busy="true">
                  <span class="visually-hidden" role="status">Carregando o mapa de janelas…</span>
                </div>
                <div class="planner__card planner__skeleton planner__skeleton--chart" aria-busy="true">
                  <span class="visually-hidden" role="status">Carregando as condições hora a hora…</span>
                </div>
              </template>
            </ClientOnly>
          </div>
        </div>
      </template>

      <PlannerConfirmationModal
        :confirmation="confirmation"
        :fields="reviewFieldChoices"
        :current-field-id="reviewFieldId"
        @select-field="reviewField"
      />

      <ToastAlert
        v-if="registeredNotice"
        floating
        tone="success"
        :title="registeredNotice.title"
        :message="registeredNotice.message"
        :timeout="NOTICE_TIMEOUT_MS"
        @dismiss="registeredNotice = null"
      />

      <BaseModal v-model:open="isPickerOpen" :title="pickerKey ? PICKER_TITLES[pickerKey] : ''" size="sm">
        <fieldset v-if="pickerKey === 'farm'" class="planner__picker">
          <legend class="visually-hidden">Fazenda</legend>
          <label v-for="option in context.farmOptions.value" :key="option.value" class="planner__option text-body-md">
            <input
              type="radio"
              name="planner-farm"
              :value="option.value"
              :checked="option.value === context.farmId.value"
              @change="onFarmPicked(option.value)"
            >
            {{ option.label }}
          </label>
        </fieldset>
        <fieldset v-else-if="pickerKey === 'product'" class="planner__picker">
          <legend class="visually-hidden">Produto</legend>
          <label v-for="option in context.productOptions" :key="option.value" class="planner__option text-body-md">
            <input
              type="radio"
              name="planner-product"
              :value="option.value"
              :checked="option.value === context.productKind.value"
              @change="onProductPicked(option.value)"
            >
            {{ option.label }}
          </label>
        </fieldset>
        <div v-else-if="pickerKey === 'fields'" class="planner__picker">
          <p v-if="context.fieldOptions.value.length === 0" class="planner__empty text-body-md">
            {{ context.pending.value ? 'Carregando os talhões…' : 'Esta fazenda ainda não tem talhões cadastrados.' }}
          </p>
          <BaseCheckbox
            v-for="option in context.fieldOptions.value"
            :key="option.value"
            size="md"
            :label="`${option.name} · ${option.areaHa.toLocaleString('pt-BR')} ha`"
            :model-value="context.fieldIds.value.includes(option.value)"
            @update:model-value="context.toggleField(option.value)"
          />
        </div>
        <template #actions>
          <BaseButton variant="secondary" @click="isPickerOpen = false">Fechar</BaseButton>
        </template>
      </BaseModal>
    </template>
  </div>
</template>

<style scoped>
.planner {
  display: flex;
  flex-direction: column;
  gap: var(--space-24);
}

.planner__header {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-16);
  align-items: flex-end;
}

.planner__heading {
  display: flex;
  flex: 1 1 var(--page-header-main-min-width);
  flex-direction: column;
  gap: var(--space-6);
  align-items: flex-start;
  min-width: 0;
}

.planner__eyebrow {
  display: inline-flex;
  gap: var(--space-8);
  align-items: center;
  margin: 0;
  padding: var(--space-6) var(--space-12);
  border-radius: var(--radius-pill);
  background: var(--color-green-900);
  color: var(--color-sprout-400);
}

.planner__title {
  margin: 0;
  color: var(--color-text-primary);
}

.planner__body {
  display: grid;
  grid-template-columns: var(--planner-chat-width) minmax(0, 1fr);
  gap: var(--space-20);
  align-items: stretch;
}

.planner__chat {
  display: flex;
  flex-direction: column;
  min-width: 0;
  overflow: hidden;
  border: var(--border-width-hairline) solid var(--color-border);
  border-radius: var(--radius-card-xl);
  background: var(--color-white);
}

.planner__chat-header {
  display: flex;
  gap: var(--space-10);
  align-items: center;
  padding: var(--space-16) var(--space-20);
  border-bottom: var(--border-width-hairline) solid var(--color-border);
}

.planner__chat-icon {
  display: grid;
  flex-shrink: 0;
  place-items: center;
  width: var(--size-planner-code-tile);
  height: var(--size-planner-code-tile);
  border-radius: var(--radius-md);
  background: var(--color-green-900);
  color: var(--color-sprout-400);
}

.planner__chat-heading {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  min-width: 0;
}

.planner__chat-title {
  margin: 0;
  color: var(--color-text-primary);
}

.planner__chat-status {
  display: flex;
  gap: var(--space-6);
  align-items: center;
  margin: 0;
  color: var(--color-text-tertiary);
}

.planner__status-dot {
  width: var(--space-8);
  height: var(--space-8);
  border-radius: var(--radius-pill);
  background: var(--color-status-suitable);
}

.planner__messages {
  display: flex;
  flex-direction: column;
  flex: 1;
  gap: var(--space-16);
  min-width: 0;
  padding: var(--space-20);
}

.planner__messages p {
  margin: 0;
}

.planner__invite {
  display: flex;
  flex-direction: column;
  gap: var(--space-12);
}

.planner__invite-text {
  color: var(--color-text-secondary);
}

.planner__retry {
  display: flex;
}

.planner__input {
  border-top: var(--border-width-hairline) solid var(--color-border);
  padding: var(--space-14);
}

.planner__panels {
  display: flex;
  flex-direction: column;
  gap: var(--space-20);
  min-width: 0;
}

.planner__card {
  display: flex;
  flex-direction: column;
  gap: var(--space-14);
  min-width: 0;
  padding: var(--space-22);
  border: var(--border-width-hairline) solid var(--color-border);
  border-radius: var(--radius-card-xl);
  background: var(--color-white);
}

.planner__card-title,
.planner__empty {
  margin: 0;
}

.planner__card-title {
  color: var(--color-text-primary);
}

.planner__empty {
  color: var(--color-text-secondary);
}

.planner__skeleton {
  animation: planner-pulse var(--duration-skeleton) var(--easing-standard) infinite;
}

.planner__skeleton--map {
  min-height: var(--planner-skeleton-map-height);
}

.planner__skeleton--chart {
  min-height: var(--planner-skeleton-chart-height);
}

.planner__stats {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: var(--space-12);
  margin: 0;
  padding: 0;
  list-style: none;
}

.planner__picker {
  display: flex;
  flex-direction: column;
  gap: var(--space-12);
  min-width: 0;
  margin: 0;
  padding: 0;
  border: 0;
}

.planner__option {
  display: flex;
  gap: var(--space-10);
  align-items: center;
  color: var(--color-text-primary);
  cursor: pointer;
}

.planner__option input {
  accent-color: var(--color-green-800);
}

@keyframes planner-pulse {
  50% {
    opacity: var(--opacity-skeleton-dim);
  }
}

@media (max-width: 1199px) {
  .planner__body {
    grid-template-columns: minmax(0, 1fr);
  }
}

@media (max-width: 899px) {
  .planner__stats {
    grid-template-columns: minmax(0, 1fr);
  }
}

@media (prefers-reduced-motion: reduce) {
  .planner__skeleton {
    animation: none;
  }
}
</style>
