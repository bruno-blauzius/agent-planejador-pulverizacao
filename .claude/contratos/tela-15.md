# Contrato — Tela 15: Painel (administrador)

Figma `MeWDfQIHok0s80GbqCKGV3`: tela `54:535`, conteúdo `54:602`. Rota `/` com `definePageMeta({ permission: 'dashboard.view' })`.
Dono: trilha F (`useDashboard*`, `utils/dashboard.ts`). Endpoint (planned, ADR 0013): `GET /dashboard?farm_id=&period_days=`. Janelas, alertas e o resumo do gráfico vêm do Planejador; até ele existir, o mock os serve pelo mesmo contrato (decidido).

## Decisões pendentes (usuário)

1. **Seletor**: o Figma só tem "Fazenda" ("Todas (4)"); o período fica fixo em **14 dias** (título "últimos 14 dias"). Proposta: sem seletor de período; `?farm=<id>` na URL.
2. **Dispensar alerta (×)**: não há endpoint. Proposta: dispensa local, só nesta sessão do navegador (some até recarregar); pendência de API `POST /dashboard/alerts/{id}/dismiss`.
3. **Escala do gráfico**: horas aptas e número de aplicações são unidades diferentes no mesmo eixo (como no Figma). Proposta: cada série com a própria escala (altura relativa ao máximo da série), com `ariaLabel` por dia com os valores reais.
4. **"Ver todas"** (próximas janelas) e **"Abrir o Planejador"** apontam para `/planner`, que ainda não existe (página 02 fora do escopo). Proposta: links visíveis só com `planner.use`; a rota mostra "Em breve" até lá.
5. **Fuso das datas**: linha de data, "Hoje/Amanhã" e horários da atividade usam um fuso fixo de exibição (`DISPLAY_TIME_ZONE = America/Sao_Paulo`, hora local das fazendas atuais), igual no SSR e no cliente (sem divergência de hidratação). Depois do "·", a fazenda selecionada ou, em "Todas", o nome da empresa. Pendência: fuso por fazenda vindo da API.

## Composable `useDashboardPage(options?)`

```ts
export interface UseDashboardPageOptions { now?: () => Date }

export function useDashboardPage(options?): {
  greeting: ComputedRef<string>          // "Bom dia, Marina" (useGreeting: "Olá, Marina" no SSR)
  dateLine: ComputedRef<string>          // "Sábado, 21 de novembro · Fazenda Santa Rita" (fuso fixo; referência = generatedAt do painel)

  // Seletor "Fazenda" (?farm=, replace)
  farmOptions: ComputedRef<{ value: string | null, label: string }[]>  // "Todas (4)", "Fazenda Santa Rita"…
  selectedFarmId: WritableComputedRef<string | null>
  canOpenPlanner: ComputedRef<boolean>   // can('planner.use') → botão "Abrir o Planejador" e "Ver todas"
  plannerPath: '/planner'

  // Estado (GET /dashboard)
  pending: ComputedRef<boolean>
  error: ComputedRef<ApiError | null>
  isForbidden: ComputedRef<boolean>      // 403 forbidden
  isOutOfScope: ComputedRef<boolean>     // 403 out_of_scope ou 404 (?farm= não liberada ou de outra empresa) → "Fazenda fora do seu escopo" + voltar a "Todas"
  refresh(): Promise<void>

  statCards: ComputedRef<DashboardStatCardView[]>   // 4 cartões ("—" ao carregar/erro)
  chart: ComputedRef<DashboardChartView>
  upcomingWindows: ComputedRef<UpcomingWindowView[]>
  alerts: ComputedRef<AgentAlertView[]>  // sem os dispensados
  dismissAlert(id: string): void
  activity: ComputedRef<ActivityView[]>
}

export interface DashboardStatCardView {
  key: 'windows' | 'applications' | 'area' | 'pending'
  label: string     // "Janelas aptas hoje" | "Aplicações no mês" | "Área pulverizada" | "Pendências"
  value: string     // "3" | "12" | "412 ha" | "2"
  caption: string | null // "T3 · T5 · T7 · próxima às 06:00" | "4 confirmadas esta semana" | "de 982 ha produtivos · 42%" | "1 carência vence amanhã"
  highlight: boolean     // só "Janelas aptas hoje"
}

export interface DashboardChartView {
  title: string                 // "Janelas aptas × aplicações · últimos 14 dias"
  legend: { key: 'hours' | 'applications', label: string }[]  // "Horas aptas", "Aplicações"
  days: {
    date: string                // ISO
    label: string               // "8/11"
    showLabel: boolean          // primeiro, do meio e último (como o Figma)
    hoursRatio: number          // 0–1 (altura da barra clara)
    applicationsRatio: number   // 0–1 (altura da barra escura)
    ariaLabel: string           // "8/11: 6 h aptas, 2 aplicações"
  }[]
  summary: string | null        // texto do Planejador ("Nos últimos 14 dias houve 69 h…")
  isEmpty: boolean              // todos os dias zerados
}

export interface UpcomingWindowView {
  key: string
  fieldLabel: string            // "T3"
  whenLabel: string             // "Hoje · 06:00 – 09:30" | "Amanhã · 06:00 – 11:00" | "Qui · —"
  detail: string | null         // "Catuaí 144 · fungicida"
  condition: { value: SprayCondition, label: string }  // StatusBadge
  isHighlighted: boolean        // apta: selo do talhão em verde
}

export interface AgentAlertView {
  id: string
  title: string
  description: string
  tone: 'attention' | 'highlight' | 'info'
  icon: 'drop' | 'rain' | 'info'      // carência | chuva | atualização/outros
}

export interface ActivityView {
  id: string
  initials: string              // "CM"; "IA" para o agente
  isAgent: boolean
  actorName: string             // "Carlos Mendes" | "Agente"
  description: string           // "registrou aplicação no T3 · 38,6 ha"
  timeLabel: string             // "hoje, 10:12" | "ontem, 17:40" | "18/11, 09:00"
}
```

### Comportamento
- `?farm=` inválido/fora do escopo: 403 `out_of_scope` → `isOutOfScope`; `selectedFarmId = null` volta a "Todas".
- Datas relativas ("Hoje", "Amanhã", "hoje, 10:12") são calculadas contra `generatedAt` do painel no fuso fixo de exibição: mesmo texto no SSR e no cliente.
- Nenhum número é calculado aqui além de formatação e da porcentagem de área (`sprayed / productive`), que é apresentação.
- Funções puras em `app/utils/dashboard.ts`.
