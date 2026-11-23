<script setup lang="ts">
/**
 * Component showcase for development only (/dev/components): renders the base
 * components with placeholder content. Not available in production builds.
 */
import type { DayConditionsView } from '~/utils/planner'

if (!import.meta.dev) throw createError({ statusCode: 404, statusMessage: 'Página não encontrada' })

useHead({ title: 'Componentes · Agentes Agro' })

type Choice = 'first' | 'second'
type Tab = 'overview' | 'details' | 'history'

const choice = ref<Choice>('first')
const tab = ref<Tab>('overview')
const text = ref('')
const isModalOpen = ref(false)
const isToastVisible = ref(true)
const selectedFileName = ref<string>()

const choiceOptions = [
  { value: 'first', label: 'Opção A' },
  { value: 'second', label: 'Opção B' },
] as const

const tabItems = [
  { value: 'overview', label: 'Visão geral' },
  { value: 'details', label: 'Detalhes', count: 0 },
  { value: 'history', label: 'Histórico' },
] as const

type Role = 'admin' | 'operator'
const role = ref<Role | null>('admin')
const company = ref<string | null>(null)

const roleOptions = [
  { value: 'admin', title: 'Opção A', description: 'Descrição da opção A.', icon: 'shield' },
  { value: 'operator', title: 'Opção B', description: 'Descrição da opção B.', icon: 'sprout' },
] as const

const companyItems = [
  { value: 'a', label: 'Item A' },
  { value: 'b', label: 'Item B' },
] as const

function onFilesSelected(files: File[]) {
  selectedFileName.value = files[0]?.name
}

/* Spray planner showcase (Figma page 02): placeholder data, same texts as the frames. */
const plannerDraft = ref('')
const plannerLastSent = ref<string>()
const plannerPending = ref(false)
const plannerError = ref<string>()
const plannerNavKey = ref('today')

const plannerContext = [
  { key: 'farm', label: 'Fazenda', value: 'Santa Rita' },
  { key: 'fields', label: 'Talhões', value: 'T3 · T7' },
  { key: 'product', label: 'Produto', value: 'Fungicida' },
] as const

const plannerTrace = [
  { key: 'forecast', icon: 'cloud', title: 'Previsão do tempo', detail: 'Open-Meteo · 5 dias, hora a hora' },
  { key: 'conditions', icon: 'thermometer', title: 'Condições de aplicação', detail: 'Delta T, vento e chuva por hora' },
  { key: 'rules', icon: 'book', title: 'Regras do produto', detail: 'Carência e intervalo · Consultor de Bulas' },
  { key: 'capacity', icon: 'gauge', title: 'Rendimento operacional', detail: 'Pulverizador 2.000 L · 12 ha/h' },
] as const

const plannerSuggestions = ['E se chover na quarta?', 'Comparar com sábado', 'Só o talhão 3'] as const

const plannerMapDays = ['Qua 26', 'Qui 27', 'Sex 28', 'Sáb 29', 'Dom 30'].map(label => ({
  key: label,
  label,
  hourLabels: ['06', '09', '12', '15', '18'],
}))

/** s = Apta, a = Atenção, u = Inapta, r = Chuva; * = parte do plano sugerido. */
function plannerCells(codes: string) {
  const byCode = { s: 'suitable', a: 'attention', u: 'unsuitable', r: 'rain' } as const
  return codes.split(' ').map(day =>
    [...day.matchAll(/[saur]\*?/g)].map(([code]) => ({
      condition: byCode[code!.charAt(0) as keyof typeof byCode],
      selected: code!.endsWith('*'),
    })),
  )
}

const plannerMapRows = [
  { key: 't3', label: 'T3', cells: plannerCells('s*s*aus rrras ssaua sauus sssas') },
  { key: 't7', label: 'T7', cells: plannerCells('sauua rrrra s*s*uua aauus ssaas') },
]

/* Same shape as the logic's DayConditionsView (utils/planner.ts). */
const DELTA_T_MAX = 12
const WIND_MAX = 20
const plannerDeltaT = [1.6, 2.6, 3.4, 4.1, 5.1, 6.2, 7.2, 8.5, 9.6, 10.3, 10.1, 9.2, 7.9, 6.4, 5.2, 3.1]
const plannerWind = [1.5, 2.0, 2.5, 3.0, 3.5, 4.0, 5.0, 6.0, 6.5, 7.0, 6.5, 5.5, 4.5, 3.5, 2.5, 2.0]

const plannerConditions: DayConditionsView = {
  title: 'Quarta 26/11 · condições hora a hora',
  points: plannerDeltaT.map((deltaT, index) => ({
    hour: index + 5,
    tickLabel: index % 3 === 0 ? `${index + 5}h` : null,
    deltaT,
    windKmh: plannerWind[index]!,
    gustKmh: plannerWind[index]! + 3,
    condition: 'suitable',
    deltaTRatio: deltaT / DELTA_T_MAX,
    windRatio: plannerWind[index]! / WIND_MAX,
    gustRatio: (plannerWind[index]! + 3) / WIND_MAX,
  })),
  deltaTAxis: { max: DELTA_T_MAX, ticks: [0, 4, 8, 12] },
  windAxis: { max: WIND_MAX, ticks: [0, 5, 10, 15, 20] },
  idealRange: {
    deltaT: { min: 2, max: 8, minRatio: 2 / DELTA_T_MAX, maxRatio: 8 / DELTA_T_MAX },
    windKmh: { min: 3, max: 10, minRatio: 3 / WIND_MAX, maxRatio: 10 / WIND_MAX },
  },
  window: { label: 'Janela 06–10h', startHour: 6, endHour: 10 },
  stats: [
    { key: 'delta_t', label: 'Delta T na janela', value: '2,6 – 6,2 °C', caption: 'ideal 2 a 8' },
    { key: 'wind', label: 'Vento médio', value: '6 km/h', caption: 'rajada máx. 11' },
    { key: 'rain', label: 'Chuva até 18h', value: '0 mm', caption: 'próx. chuva qui 03h' },
  ],
  isEmpty: false,
}

const PLANNER_STAT_ICONS = { delta_t: 'thermometer', wind: 'wind', rain: 'drop' } as const

const plannerChecks = [
  { key: 'delta-t', title: 'Delta T dentro da faixa', detail: '2,6 a 6,2 °C (ideal 2 a 8 °C)' },
  { key: 'wind', title: 'Vento adequado', detail: '4 a 8 km/h, rajada máxima 11 km/h' },
  { key: 'rain', title: 'Sem chuva após a aplicação', detail: '8h livres; próxima chuva quinta 03h' },
  { key: 'interval', title: 'Intervalo desde a última aplicação', detail: '21 dias (mínimo do produto: 14 dias)', status: 'warning' },
  { key: 'harvest', title: 'Carência até a colheita', detail: 'Colheita prevista mai/2027 · carência de 30 dias respeitada', status: 'failed' },
] as const

const plannerHours = Array.from({ length: 13 }, (_, index) => ({
  label: String(index + 6).padStart(2, '0'),
  condition: (['suitable', 'suitable', 'suitable', 'suitable', 'attention', 'attention', 'unsuitable', 'unsuitable', 'unsuitable', 'attention', 'attention', 'suitable', 'suitable'] as const)[index]!,
  current: index === 0,
}))

const plannerNavItems = [
  { key: 'today', label: 'Hoje', icon: 'home' },
  { key: 'map', label: 'Mapa', icon: 'map', unavailableHint: 'Em breve' },
  { key: 'applications', label: 'Aplicações', icon: 'calendar' },
  { key: 'profile', label: 'Perfil', icon: 'user' },
] as const

function onPlannerSubmit(text: string) {
  plannerLastSent.value = text
  plannerDraft.value = ''
}
</script>

<template>
  <div class="page">
    <PageHeader title="Componentes" eyebrow="Desenvolvimento" subtitle="Página temporária para validar o layout e os componentes base.">
      <template #actions>
        <BaseButton variant="ghost">Secundária</BaseButton>
        <BaseButton icon="plus" @click="isModalOpen = true">Abrir modal</BaseButton>
      </template>
    </PageHeader>

    <ToastAlert
      v-if="isToastVisible"
      tone="success"
      title="Layout carregado"
      message="Componentes base renderizados com os tokens do Figma."
      floating
      @dismiss="isToastVisible = false"
    />

    <section class="showcase" aria-label="Componentes base">
      <div class="showcase__row">
        <BaseButton variant="primary" icon="plus">Primário</BaseButton>
        <BaseButton variant="accent" icon="sparkles">Acento</BaseButton>
        <BaseButton variant="secondary" icon="plus">Secundário</BaseButton>
        <BaseButton variant="ghost" icon="plus">Fantasma</BaseButton>
        <BaseButton variant="danger" icon="trash">Perigo</BaseButton>
        <BaseButton size="sm" disabled icon="plus">Desabilitado</BaseButton>
      </div>

      <div class="showcase__row">
        <StatusBadge tone="suitable" />
        <StatusBadge tone="attention" />
        <StatusBadge tone="unsuitable" />
        <StatusBadge tone="rain" />
        <SegmentedControl v-model="choice" :options="choiceOptions" label="Exemplo de seleção" />
      </div>

      <TabsNav v-model="tab" :items="tabItems" label="Exemplo de abas" />

      <div class="showcase__grid">
        <TextField v-model="text" label="Rótulo" placeholder="Digite aqui" icon="mail" hint="Texto de ajuda" />
        <TextField label="Com erro" placeholder="Digite aqui" error="Mensagem de erro" />
        <TextField label="Desabilitado" placeholder="Digite aqui" disabled />
      </div>

      <FileDropzone
        title="Documento (opcional)"
        description="PDF ou imagem"
        :file-name="selectedFileName"
        @select="onFilesSelected"
      />

      <MapView alt="Ilustração estática de talhões" rounded>
        <StatusBadge tone="suitable" class="showcase__map-badge" />
      </MapView>
    </section>

    <section class="showcase" aria-label="Componentes da página 03">
      <div class="showcase__grid">
        <StatCard label="Indicador" value="12" caption="Detalhe do indicador" />
        <StatCard label="Indicador do agente" value="3" caption="Detalhe" highlight />
      </div>
      <div class="showcase__grid">
        <OptionCardGroup v-model="role" :options="roleOptions" label="Cartões de opção" />
        <BaseSelect v-model="company" label="Campo de seleção" :items="companyItems" />
      </div>
      <div class="showcase__row">
        <BaseAvatar initials="AB" seed="a" />
        <BaseAvatar initials="CD" seed="b" size="md" />
      </div>
      <ForbiddenState />
    </section>

    <section class="showcase" aria-label="Componentes do Planejador (página 02)">
      <h2 class="text-h3">Planejador · desktop (18:2)</h2>
      <PlannerContextBar :items="plannerContext" />

      <div class="planner-demo">
        <div class="planner-demo__chat">
          <PlannerChatLog :pending="plannerPending" :error="plannerError">
          <PlannerChatMessage role="user">
            Preciso aplicar fungicida nos talhões 3 e 7 essa semana. Quando faço?
          </PlannerChatMessage>
          <PlannerToolTrace :items="plannerTrace" />
          <PlannerChatMessage
            role="agent"
            note="Quinta tem 18 mm de chuva prevista. Evite: o produto precisa de 2h sem chuva para não lavar."
          >
            <p>
              <strong>A melhor janela é quarta (26/11), das 06h às 10h:</strong>
              Delta T entre 3 e 6 °C, vento de 4 a 8 km/h e nada de chuva até as 18h.
            </p>
            <p>
              Os dois talhões somam 90 ha e o pulverizador faz 12 ha/h, ou seja, cerca de 7h30 de trabalho.
              Como a janela tem 4h, sugiro dividir: T3 na quarta e T7 na sexta, no mesmo horário.
            </p>
          </PlannerChatMessage>
          </PlannerChatLog>
          <PlannerSuggestionChips :suggestions="plannerSuggestions" @select="onPlannerSubmit" />
          <p v-if="plannerLastSent" class="text-caption" role="status">Enviado: {{ plannerLastSent }}</p>
          <PlannerChatInput v-model="plannerDraft" @submit="onPlannerSubmit" />
          <div class="showcase__row">
            <BaseButton size="sm" variant="secondary" @click="plannerPending = !plannerPending">Alternar espera</BaseButton>
            <BaseButton
              size="sm"
              variant="secondary"
              @click="plannerError = plannerError ? undefined : 'Não foi possível consultar a previsão agora. Tente novamente em instantes.'"
            >
              Alternar erro
            </BaseButton>
          </div>
        </div>

        <div class="planner-demo__panels">
          <PlannerPlanPanel
            title="2 janelas aptas cobrem os 90 ha"
            note="T7 precisa de 4h20 e a janela tem 4h. O agente sugere começar 05:40 ou deixar 3 ha para sábado."
          >
            <PlannerFieldPlanCard field-code="T3" window-label="Qua · 26/11  ·  06:00 – 10:00" detail="38,6 ha · 3h15" />
            <PlannerFieldPlanCard field-code="T7" window-label="Sex · 28/11  ·  06:00 – 10:00" detail="51,4 ha · 4h20" short-window />
          </PlannerPlanPanel>

          <PlannerWindowMap title="Mapa de janelas · próximos 5 dias" :days="plannerMapDays" :rows="plannerMapRows" />

          <div class="planner-demo__chart">
            <PlannerConditionsChart
              :view="plannerConditions"
              summary="Delta T sobe de 1,6 °C às 5h até 10,3 °C às 14h; a janela de 06h às 10h fica dentro da faixa ideal de 2 a 8 °C."
            />
            <div class="planner-demo__stats">
              <PlannerConditionStat
                v-for="stat in plannerConditions.stats"
                :key="stat.key"
                :icon="PLANNER_STAT_ICONS[stat.key]"
                :label="stat.label"
                :value="stat.value"
                :hint="stat.caption"
              />
            </div>
          </div>
        </div>
      </div>

      <h2 class="text-h3">Confirmação · checagens (19:446)</h2>
      <div class="planner-demo__checklist">
        <PlannerChecklist :items="plannerChecks" />
      </div>

      <h2 class="text-h3">Planejador · mobile (20:18)</h2>
      <div class="phone">
        <div class="phone__scroll">
          <PlannerNowCard
            eyebrow="Agora · 06:48"
            headline="Pode&#10;pulverizar"
            until="até 10:00"
            remaining="· faltam 3h12"
            :progress="0.3"
            start-label="06:00"
            end-label="10:00"
          />
          <div class="phone__metrics">
            <PlannerMetricTile icon="thermometer" value="4,2 °C" label="Delta T" />
            <PlannerMetricTile icon="wind" value="6 km/h" label="Vento" />
            <PlannerMetricTile icon="drop" value="0 mm" label="Chuva 4h" />
          </div>
          <PlannerHourStrip title="Hoje, hora a hora" :hours="plannerHours" />
          <PlannerTaskCard field-code="T3" title="Fungicida · 38,6 ha" detail="0,5 L/ha · 4 tanques de 2.000 L" />
        </div>
        <PlannerBottomNav :items="plannerNavItems" :active-key="plannerNavKey" @select="plannerNavKey = $event" />
      </div>
    </section>

    <BaseModal
      v-model:open="isModalOpen"
      eyebrow="Exemplo"
      title="Título do modal"
      description="Subtítulo explicando a ação."
      icon="user"
    >
      <TextField v-model="text" label="Campo" placeholder="Digite aqui" data-autofocus />
      <template #actions>
        <BaseButton variant="secondary" block @click="isModalOpen = false">Cancelar</BaseButton>
        <BaseButton block @click="isModalOpen = false">Continuar</BaseButton>
      </template>
    </BaseModal>
  </div>
</template>

<style scoped>
.page {
  display: flex;
  flex-direction: column;
  gap: var(--space-28);
}

.showcase {
  display: flex;
  flex-direction: column;
  gap: var(--space-24);
}

.showcase__row {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-12);
  align-items: center;
}

.showcase__grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(var(--grid-min-column), 1fr));
  gap: var(--space-16);
}

.planner-demo {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-20);
  align-items: flex-start;
}

.planner-demo__chat {
  display: flex;
  flex: 0 1 var(--planner-chat-width);
  flex-direction: column;
  gap: var(--space-16);
  min-width: 0;
  padding: var(--space-20);
  border: var(--border-width-hairline) solid var(--color-border);
  border-radius: var(--radius-card-xl);
  background: var(--color-white);
}

.planner-demo__panels {
  display: flex;
  flex: 1 1 var(--planner-map-min-width);
  flex-direction: column;
  gap: var(--space-20);
  min-width: 0;
}

.planner-demo__chart {
  display: flex;
  flex-direction: column;
  gap: var(--space-12);
  padding: var(--space-22);
  border: var(--border-width-hairline) solid var(--color-border);
  border-radius: var(--radius-card-xl);
  background: var(--color-white);
}

.planner-demo__stats {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-12);
}

.planner-demo__checklist {
  max-width: var(--modal-width-md);
}

/* 390 px phone frame of the mobile operator view (Figma 20:18). */
.phone {
  display: flex;
  flex-direction: column;
  width: 100%;
  max-width: var(--planner-phone-width);
  overflow: hidden;
  border: var(--border-width-hairline) solid var(--color-border);
  border-radius: var(--radius-2xl);
  background: var(--color-sand);
}

.phone__scroll {
  display: flex;
  flex-direction: column;
  gap: var(--space-12);
  padding: var(--space-16) var(--space-18);
}

.phone__metrics {
  display: flex;
  gap: var(--space-10);
}

.showcase__map-badge {
  position: absolute;
  top: var(--space-16);
  left: var(--space-16);
}
</style>
