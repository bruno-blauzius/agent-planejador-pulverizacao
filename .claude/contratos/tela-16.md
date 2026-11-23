# Contrato — Tela 16: Planejador de pulverização (Figma 18:2)

Rota `/planner` com `definePageMeta({ permission: 'planner.use' })`. Protocolo e decisões: `docs/adr/0016-protocolo-do-planejador-no-front.md` (proposto). Lógica: `usePlannerContext`, `usePlannerChat`, `usePlannerWindows` e `utils/planner.ts`. A confirmação (modal) está em `tela-17.md`.
Tudo que é condição, duração, alerta e faixa ideal vem pronto da API; o front só formata. A conversa (e o plano) pertence a uma fazenda e a um usuário: trocar de fazenda ou de usuário, ou sair, limpa tudo (`reset`), e uma resposta que chega depois da troca é descartada. Rótulos de condição: `SPRAY_CONDITION_LABELS` (`utils/labels.ts`: Apta, Atenção, Inapta, Chuva).

## Endpoints (planned, ADR 0016)

`POST /planner/messages`, `GET /planner/window-map`, `GET /planner/conditions`. Erros: 403 `forbidden`/`out_of_scope`, 422, 502 `agent_unavailable`/`weather_unavailable`.

## `usePlannerContext()` (barra Fazenda / Talhões / Produto, estado compartilhado)

```ts
{
  farmId: ComputedRef<string | null>            // começa na fazenda principal do usuário
  fieldIds: ComputedRef<readonly string[]>      // começa vazio
  productKind: ComputedRef<ProductKind | null>  // começa 'fungicide'
  farmOptions: ComputedRef<{ value: string, label: string }[]>
  fieldOptions: ComputedRef<{ value: string, label: string /* "T3" */, name: string /* "Talhão 3" */, areaHa: number }[]>
  productOptions: { value: ProductKind, label: string }[]   // Fungicida, Inseticida, Herbicida, Outro
  selectedFieldsLabel: ComputedRef<string>      // "T3 · T7"; '' sem seleção
  requestContext: ComputedRef<PlannerRequestContext | null>  // null sem fazenda ou sem talhão
  isReady: ComputedRef<boolean>
  pending: ComputedRef<boolean>; error: ComputedRef<ApiError | null>
  isEmpty: ComputedRef<boolean>                 // nenhuma fazenda liberada
  selectFarm(farmId: string | null): void       // troca de fazenda limpa os talhões
  setFields(ids: readonly string[]): void; toggleField(id: string): void
  setProductKind(kind: ProductKind | null): void
  refresh(): Promise<void>
}
```

## `usePlannerChat(options?: { now?, context? })`

```ts
{
  entries: ComputedRef<ChatEntry[]>   // { id, role: 'user'|'agent', paragraphs: string[], createdAt, consulted: ConsultedSource[], plan: SprayPlan | null }
  conversationId: ComputedRef<string | null>
  draft: Ref<string>                  // campo "Pergunte ao agente…"
  suggestions: ComputedRef<string[]>  // chips ("E se chover na quarta?"); esvaziam ao enviar
  isEmpty: ComputedRef<boolean>
  latestPlan: ComputedRef<SprayPlan | null>
  planView: ComputedRef<PlanView | null>   // cartão "Plano sugerido pelo agente"
  canUse: ComputedRef<boolean>        // can('planner.use')
  contextMissing: ComputedRef<boolean> // escolher fazenda e ao menos um talhão antes de perguntar
  pending: Ref<boolean>; error: Ref<ApiError | null>
  canRetry: ComputedRef<boolean>
  isForbidden: ComputedRef<boolean>; isAgentUnavailable: ComputedRef<boolean>; isWeatherUnavailable: ComputedRef<boolean>
  send(text?: string): Promise<boolean>   // usa o rascunho quando sem argumento; false se vazio, sem contexto ou sem permissão
  retry(): Promise<boolean>               // repete a pergunta que falhou sem duplicar a bolha do usuário
  reset(): void
}

PlanView = { id, headline /* "2 janelas aptas cobrem os 90 ha" */, totalAreaLabel, hasAlerts,
  entries: { fieldId, fieldLabel /* "T3" */, detail /* "38,6 ha · 3h15" */, windowLabel /* "Qua 26 · 06:00 – 10:00" */, condition, alertMessage: string | null }[] }
```

Botão "Revisar e registrar plano" abre o modal com `useApplicationConfirmation().open(planView.id, entry.fieldId)` (um talhão por vez; "Ajustar horários" não tem endpoint: desabilitado "Em breve").

## `usePlannerWindows(options?: { context? })`

```ts
{
  map: ComputedRef<WindowMapView>   // { updatedLabel /* "dados de hoje 05:40" */, legend: {value,label}[], days: {date,label /* "Qua 26" */,hourLabels /* ['06','09','12','15','18'] */}[],
                                    //   rows: { fieldId, fieldLabel, days: { date, cells: { hour, condition, ariaLabel }[] }[] }[], isEmpty }
  conditions: ComputedRef<DayConditionsView>
  windows: ComputedRef<readonly SprayWindowSlot[]>
  selectedDate: ComputedRef<string | null>   // dia escolhido ou o primeiro do mapa
  selectDay(date: string): void              // limpa janela escolhida
  selectWindow(date: string, startTime: string, endTime: string): void
  pending: ComputedRef<boolean>; conditionsPending: ComputedRef<boolean>
  error: ComputedRef<ApiError | null>; isForbidden; isWeatherUnavailable; isEmpty
  refresh(): Promise<void>
}

DayConditionsView = {
  title /* "Quarta 26/11 · condições hora a hora" */,
  points: { hour, tickLabel: string | null /* "5h" a cada 3 */, deltaT, windKmh, gustKmh, condition, deltaTRatio, windRatio, gustRatio }[],   // série hora a hora
  deltaTAxis: { max, ticks: number[] /* [0,4,8,12] */ }, windAxis: { max, ticks: number[] /* [0,5,10,15,20] */ },
  idealRange: { deltaT: { min, max, minRatio, maxRatio }, windKmh: { ... } },   // faixa ideal (área sombreada)
  window: { label /* "Janela 06–10h" */, startHour, endHour } | null,           // início/fim, horas [start, end)
  stats: { key: 'delta_t'|'wind'|'rain', label, value, caption }[],              // "Delta T na janela · 2,6 – 6,2 °C · ideal 2 a 8" etc.
  isEmpty
}
```

**Escala (fonte única, a UI não recalcula).** Delta T e vento têm eixos próprios. Cada eixo vai de 0 ao maior valor (dado ou faixa ideal) arredondado para cima ao degrau (Delta T de 4 em 4, mínimo 12; vento de 5 em 5, mínimo 20). O eixo do vento é dimensionado pela maior **rajada**, então `windRatio` e `gustRatio` usam o mesmo eixo e a rajada nunca passa de 1. Todos os `*Ratio` (pontos e `idealRange`) são limitados a 0–1. Altura de barra/ponto = `ratio` (0–1) do alto do gráfico; os rótulos do eixo vêm de `deltaTAxis.ticks`/`windAxis.ticks` e os do eixo X de `tickLabel`.

## Estados

| Estado | Sinal | Texto sugerido |
|---|---|---|
| Carregando | `pending` / `conditionsPending` | esqueleto do mapa e do gráfico; bolha "digitando" enquanto `chat.pending` |
| Vazio (sem seleção) | `chat.contextMissing`, `windows.isEmpty` | "Escolha a fazenda e os talhões para planejar" |
| Conversa vazia | `chat.isEmpty` | convite com perguntas de exemplo |
| Sem janela no dia | `conditions.window === null` e `!isEmpty` | "Sem janela apta neste dia" (sem os 3 cartões) |
| Agente fora | `chat.isAgentUnavailable` | "O agente está indisponível" + "Tentar de novo" (`retry`) |
| Previsão fora | `isWeatherUnavailable` (chat, mapa, condições) | "A previsão do tempo está indisponível" + `refresh` |
| 403 | `isForbidden` | `ForbiddenState`; a rota já exige `planner.use` |
| Operador | sem ação extra | só as fazendas do escopo aparecem em `farmOptions` |

## Decisões em aberto (ADR 0016) e lacunas

Histórico não persistido; "Ajustar horários" sem endpoint; mapa idêntico entre talhões da mesma fazenda no mock; fuso fixo `America/Sao_Paulo`; sem streaming (resposta única, timeout de 60 s).
