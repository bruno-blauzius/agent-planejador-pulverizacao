# Contrato — Tela 19: Aplicações (histórico e planejadas) (Figma 63:237)

Rota `/applications` (já na sidebar). Sem `permission` na rota: ler aplicações exige só estar autenticado e no escopo (ADR 0016); a API filtra por empresa e `farm_scope`. O painel de detalhe (tela 20) abre na mesma rota por `?application=<id>`.
Lógica: `useApplicationListPage`, `useCurrentWeather`, `utils/applications.ts`, rótulos em `utils/labels.ts` (`APPLICATION_STATUS_LABELS`: Planejada, Em andamento, Concluída, Cancelada). Nada é calculado no front: condições, totais e status vêm da API.

## Endpoints (planned, ADR 0016)

`GET /applications` (resposta com `items`, `facets` e `summary`; `period_days`, `farm_id`, `field_id`, `product_kind`, `operator_id`, `status`, `page`, `page_size`) e `GET /weather/current`. Erros: 401, 403 `out_of_scope`, 404, 502 `weather_unavailable` (só o chip de clima).

## `useApplicationListPage(options?: { now? })`

```ts
{
  subtitle: ComputedRef<string>             // "Novembro de 2026 · Fazenda Santa Rita" (mês de `now` + empresa)
  weatherLabel: ComputedRef<string | null>  // "Guaxupé · 24 °C · vento 7 km/h"; null carregando ou com a previsão fora
  weatherUnavailable: ComputedRef<boolean>  // o chip some/mostra "indisponível"; a lista segue

  filters: ComputedRef<{ periodDays: number, fieldId: string | null, productKind: ProductKind | null, operatorId: string | null, status: ApplicationStatus | null }>
  periodOptions: { value: number, label: string }[]                 // 7 / 30 (padrão) / 90 dias
  fieldOptions: ComputedRef<{ value: string | null, label: string /* "T5 · Talhão 5" */ }[]>      // "Todos" + facets da API
  productOptions: { value: ProductKind | null, label: string }[]    // Todos, Fungicida, Inseticida, Herbicida, Foliar, Outro
  operatorOptions: ComputedRef<{ value: string | null, label: string /* "Carlos" */ }[]>
  statusOptions: { value: ApplicationStatus | null, label: string }[]   // não existe no Figma; disponível se a UI quiser
  hasFilters: ComputedRef<boolean>          // o período padrão não conta
  setFilter(key: 'period' | 'field' | 'product' | 'operator' | 'status', value: string | number | null): void   // vai para a página 1
  clearFilters(): void

  summaryCards: ComputedRef<ApplicationSummaryCardView[]>   // os 4 cartões do topo (sempre 4); traços enquanto carrega ou em erro
  statusChips: { value: ApplicationStatus | null, label: string }[]   // chips do cabeçalho da tabela: Todas, Planejadas, Concluídas, Canceladas (use setFilter('status', chip.value))

  rows: ComputedRef<ApplicationRowView[]>   // ordem da API: planejadas (a mais próxima primeiro) e depois as passadas (a mais recente primeiro)
  page: WritableComputedRef<number>; pageSize: WritableComputedRef<number>; pageSizeOptions: { value, label }[]   // 10 (padrão), 20, 50
  pagination: ComputedRef<{ rangeLabel: string /* "1–6 de 6" */, page: number, pageCount: number } | null>
  total: ComputedRef<number>

  pending: ComputedRef<boolean>; error: ComputedRef<ApiError | null>
  isForbidden: ComputedRef<boolean>         // 403 forbidden
  isOutOfScope: ComputedRef<boolean>        // 403 out_of_scope ou 404
  isEmpty: ComputedRef<boolean>             // carregou, sem filtro e sem aplicações
  isFilterEmpty: ComputedRef<boolean>       // carregou, com filtro e sem resultado
  refresh(): Promise<void>

  canPlan: ComputedRef<boolean>             // can('planner.use') → "Planejar com o agente" (abre /planner)
  exportAction: { disabled: true, hint: 'Em breve' }   // "Exportar caderno de campo": sem endpoint

  selectedId: ComputedRef<string | null>    // ?application=
  openDetail(id: string): void              // clique na linha
  closeDetail(): void
}

ApplicationSummaryCardView = {
  key: 'month' | 'area' | 'window' | 'next',
  label: string,                   // "No mês" | "Área aplicada" | "Dentro da janela" | "Próxima" (o Figma escreve em caixa alta pelo estilo)
  value: string,                   // "12" | "412 ha" | "92%" | "Qui 26/11"; "—" quando a API não tem o dado
  caption: string | null,          // "9 concluídas · 2 planejadas · 1 cancelada" | "42% da área produtiva" | "11 de 12 com Delta T e vento ideais" | "T3 · 06:00 – 10:00 · fungicida"
  tone: 'default' | 'highlight'    // só "Dentro da janela" é o cartão escuro (Cartão de indicador com destaque)
}
// Origem: `summary` de GET /applications, do MÊS CORRENTE no escopo do usuário. Não segue período, talhão, produto,
// operador, status nem página. Contagens zeradas são omitidas da legenda; sem planejadas, "Próxima" fica "—".
// Nada é calculado no front: contagens, ha, percentuais e a próxima planejada vêm da API.

ApplicationRowView = {
  id,
  status: { value, label /* "Concluída" */, tone: 'suitable' | 'attention' | 'info' | 'muted' },
  dateLabel /* "Sáb 21/11" */, timeLabel /* "06:12 – 09:40" */,
  fieldCode /* "T5" */, fieldName /* "Talhão 5" */, fieldMeta: string | null /* "Catuaí 144 · Área Centro" */,
  productLabel /* "Inseticida" */, doseLabel /* "0,3 L/ha · calda 180 L/ha" */,
  areaLabel /* "56,3 ha" (uma casa decimal, "44,0 ha") */,
  conditions: { kind: 'values', parts: string[] /* ["4,2 °C", "6 km/h"] */ }
            | { kind: 'live', label: 'Previsão ao vivo' }
            | { kind: 'rain', label: 'Chuva 18 mm' }
            | { kind: 'none', label: '—' },
  operatorName /* "Carlos" */, operatorInitials /* "CM" */, operatorFullName
}
```

Os chips de status do Figma (Todas, Planejadas, Concluídas, Canceladas, sem contador) usam `statusChips`; o select "Status" de `statusOptions` não existe no desenho e pode ser ignorado.

Tom do selo de status: Concluída `suitable` (verde), Em andamento `attention`, Planejada `info`, Cancelada `muted` (o Figma só mostra "Concluída" em verde; os outros tons são proposta).

## Estados

| Estado | Sinal | Texto sugerido |
|---|---|---|
| Carregando | `pending` | linhas em esqueleto; cartões com "—" (`summaryCards`) |
| Vazio | `isEmpty` | "Nenhuma aplicação ainda" + botão "Planejar com o agente" (`canPlan`) |
| Filtro sem resultado | `isFilterEmpty` | "Nenhuma aplicação com esses filtros" + "Limpar filtros" (`clearFilters`) |
| Erro | `error` (nem 403 nem escopo) | mensagem da API + "Tentar de novo" (`refresh`) |
| 403 | `isForbidden` | `ForbiddenState` |
| Fora do escopo | `isOutOfScope` | "Fazenda fora do seu escopo" |
| Clima fora | `weatherUnavailable` | chip "Clima indisponível"; a tabela não é afetada |
| Operador | sem ação extra | a API já limita às fazendas liberadas; sem `planner.use` o botão do agente some |

Em largura estreita a tabela empilha as linhas (como as telas 3 e 14). Datas "Qua/Sex" do Figma são só do desenho: o rótulo é calculado da data real.

## Decisões em aberto e lacunas

Operador vê as aplicações da fazenda liberada, não só as suas; indicadores só do mês corrente; "Exportar caderno de campo" sem endpoint; filtro "Status" não está no Figma; cancelar/concluir sem endpoint (ADR 0016).
