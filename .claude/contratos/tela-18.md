# Contrato — Tela 18: Mobile · Operador "Hoje" (Figma 20:18, 390x844)

Rota própria `/today` (padrão assumido; ADR 0016, decisão em aberto 6) com `definePageMeta({ permission: 'planner.use' })`. Lógica: `useTodayTask` e `utils/planner.ts`.
O operador só vê as fazendas do seu escopo (a API aplica). Status, métricas, linha do dia e tarefa chegam prontos; o front só formata.

## Endpoints (planned)

`GET /planner/today` (`planner.use`) e `POST /planner/today/tasks/{taskId}/start` (`applications.register`). Erros: 403, 404, 409 `conflict` (já iniciada ou janela fechada), 502 `weather_unavailable`.

## `useTodayTask(options?: { now? })`

```ts
{
  greeting: ComputedRef<string>               // "Bom dia, Carlos" ("Olá, Carlos" no SSR)
  locationLabel: ComputedRef<string>          // "Faz. Santa Rita · Talhão 3"
  status: ComputedRef<TodayStatusView | null>
  metrics: ComputedRef<TodayMetricView[]>     // Delta T, Vento, Chuva 4h
  hours: ComputedRef<TodayHourView[]>         // "Hoje, hora a hora"
  task: ComputedRef<TodayTaskView | null>

  pending: ComputedRef<boolean>; error: ComputedRef<ApiError | null>
  isForbidden: ComputedRef<boolean>; isWeatherUnavailable: ComputedRef<boolean>
  isEmpty: ComputedRef<boolean>               // carregou, mas não há tarefa para hoje
  refresh(): Promise<void>

  canStart: ComputedRef<boolean>              // can('applications.register') e tarefa ainda não iniciada
  starting: Ref<boolean>; startError: Ref<ApiError | null>
  startConflict: ComputedRef<boolean>; startForbidden: ComputedRef<boolean>
  start(): Promise<boolean>                   // true quando a tarefa passou a "em andamento"
}

TodayStatusView = { condition, headline /* "Pode pulverizar" */, nowLabel /* "AGORA · 06:48" */,
  untilLabel: string | null /* "até 10:00" */, remainingLabel: string | null /* "· faltam 3h12" */,
  progressRatio /* 0–1, barra */, startLabel /* "06:00" */, endLabel /* "10:00" */ }
TodayMetricView = { kind: 'delta_t'|'wind'|'rain', label, valueLabel /* "4,2 °C" */, condition, isOk /* selo de check */ }
TodayHourView = { hour, label /* "06" */, condition, isCurrent /* borda escura */ }
TodayTaskView = { id, fieldLabel /* "T3" */, title /* "Fungicida · 38,6 ha" */, detail /* "0,5 L/ha · 4 tanques de 2.000 L" */, isStarted }
```

Nota de escala: a linha do dia usa só `condition` por hora (sem eixo). A escala do gráfico de condições (Delta T/vento/rajada) é da tela 16 (`tela-16.md`).

## Estados

| Estado | Sinal | Texto sugerido |
|---|---|---|
| Carregando | `pending` | esqueleto do card escuro, dos 3 cartões e da linha do dia |
| Sem tarefa | `isEmpty` | "Nenhuma aplicação planejada para hoje" (mostra status e condições) |
| Condição não apta | `status.condition` diferente de `suitable` | o cartão usa o tom da condição e a `headline` da API; `untilLabel`/`remainingLabel` podem ser nulos |
| Iniciando | `starting` | botão desabilitado com indicador |
| Iniciada | `task.isStarted` | botão vira "Aplicação em andamento", desabilitado |
| Conflito | `startConflict` | "Não foi possível iniciar: a aplicação já foi iniciada ou a janela fechou" + `refresh` |
| Previsão fora | `isWeatherUnavailable` | "A previsão do tempo está indisponível" + `refresh` |
| 403 | `isForbidden`, `startForbidden` | `ForbiddenState` / mensagem de permissão |

As abas Mapa, Aplicações e Perfil e o sino de alertas não têm endpoint nem tela nesta etapa (desabilitados "Em breve"). O botão central de IA da barra abre `/planner`.
