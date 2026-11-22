# Contrato — Tela 9: Fazenda · Áreas e talhões (Figma 15:56 / conteúdo 15:126)

Rota: `pages/farms/[id]/index.vue` (ou `[id].vue`) → `useFarmDetailPage(() => route.params.id)`.
Lógica: `app/composables/useFarmDetailPage.ts` + `app/utils/farmDetail.ts` (logic-implementer, trilha C).

## Decisões (fechadas pelo usuário/orquestrador em 2026-11-21)

1. **Cores e legenda por VARIEDADE**, como no Figma: a chave de cor é cultura + variedade (só a cultura quando não há variedade). O índice é estável e segue a ordem de primeira aparição na lista agrupada; é o mesmo na faixa da linha (`FieldRowView.colorIndex`), no polígono (`mapFeatures[].cropIndex`, prop atual do `MapView`) e na legenda. Rótulo da legenda: o nome da variedade sem encurtar ("Catuaí 144"; sem variedade, "Milho safrinha"). `colorBy: 'status'` continua. Se a UI renomear `MapFeature.cropIndex` para `colorIndex`, avise que o logic-implementer renomeia o campo em `FieldMapFeature`.
2. **"Nova área"**: desabilitado, "Em breve" (`newArea`).
3. **"Editar" talhão**: `PUT /farms/{farmId}/fields/{fieldId}` (planned) + `useFarmFields(id).updateField(fieldId, input)`. No popover, `editFieldTo` = `/farms/{id}/fields/{fieldId}/edit` (a tela 10 reaproveita o formulário).
4. **"Planejar aplicação"** e **"Apta hoje · 06:00–09:30"**: desabilitados, "Disponível com o planejador"; a faixa mostra "Janela de hoje disponível com o planejador". Nunca inventar horário.
5. **Abas** sem tela ficam visíveis e desabilitadas; "Proprietários" mostra a contagem; "Aplicações" fica sem contagem.
6. **Ordem**: a API não tem campo de ordem, então `groupFieldsByBlock` ordena as áreas em ordem alfabética natural (pt-BR: Centro, Norte, Sul) e os talhões pelo nome natural ("Talhão 2" antes de "Talhão 10"). **"Sem área"** para `block = null`, por último; todas as áreas abertas ao entrar; selecionar pelo mapa reabre a área.
7. **"Produtiva"** = soma das áreas dos talhões.

## Divergências de design

- **Ordem das áreas**: o Figma mostra Norte, Centro, Sul; a tela usa ordem alfabética natural (Centro, Norte, Sul), porque a API não tem campo de ordem. Aceita pelo usuário/orquestrador em 2026-11-21.

## Endpoints (openapi.yaml, `x-status: planned`, mock em dev)

- `GET /farms/{farmId}` → `useFarm` (cabeçalho, selo, proprietários, perímetro)
- `GET /farms/{farmId}/fields` → `useFarmFields` (lista, mapa, popover)
- `PUT /farms/{farmId}/fields/{fieldId}` → `useFarmFields.updateField` (tela 10, modo edição)
- Lacunas: aplicações por fazenda, janela do dia por talhão (spray-planner), áreas como entidade.

## Assinatura

```ts
// app/utils/farmDetail.ts
export type FarmDetailTab = 'overview' | 'fields' | 'owners' | 'applications' | 'windows'
export type FarmMapMode = 'crop' | 'status' | 'satellite'

export interface ConditionView { readonly value: SprayCondition, readonly label: string } // "Apta"...

export interface FarmDetailTabView {
  readonly value: FarmDetailTab
  readonly label: string            // "Áreas e talhões"
  readonly count: number | null     // null = sem contador
  readonly disabled: boolean        // só 'fields' habilitada
}

export interface FieldsSummaryView {
  readonly blockCount: string       // "3"   (rótulo "Áreas")
  readonly fieldCount: string       // "9"   (rótulo "Talhões")
  readonly productiveAreaLabel: string // "412 ha" (rótulo "Produtiva")
}                                   // "—" nos três durante a 1ª carga e no erro

export interface FieldRowView {
  readonly id: string
  readonly name: string             // "Talhão 3"
  readonly shortLabel: string       // "T3" (etiqueta no mapa)
  readonly subtitle: string         // "Catuaí 144 · 38,6 ha" | "Milho safrinha · 47,7 ha"
  readonly condition: ConditionView | null
  readonly colorIndex: number       // índice em MAP_FIELD_COLORS (use % length); mesmo do mapa e legenda
  readonly selected: boolean        // destaque 15:216
}

export interface FieldBlockView {
  readonly key: string              // estável; use no v-for e em toggleBlock
  readonly name: string             // "Área Norte" | "Sem área"
  readonly areaLabel: string        // "162 ha" (aberta)
  readonly collapsedLabel: string   // "102 ha · 2 talhões" (recolhida)
  readonly expanded: boolean
  readonly fields: readonly FieldRowView[]
}

export type MapLegendView =
  | { readonly kind: 'crop', readonly items: readonly { colorIndex: number, label: string }[] }
  | { readonly kind: 'status', readonly items: readonly ConditionView[] }  // só condições presentes

export interface FieldPopoverView {
  readonly id: string
  readonly name: string             // "Talhão 3"
  readonly subtitle: string         // "Área Norte · Catuaí 144" (sem área: só a variedade/cultura)
  readonly areaLabel: string        // "38,6 ha"
  readonly plantingLabel: string    // "mar/2019" | "—"
  readonly lastApplicationLabel: string // "há 12 dias" | "hoje" | "ontem" | "—"
  readonly condition: ConditionView | null
  /** Sempre null hoje (spray-planner). UI: "Janela de hoje disponível com o planejador". */
  readonly todayWindowLabel: null
  readonly editFieldTo: string       // "/farms/<id>/fields/<fieldId>/edit"
  readonly planApplication: DisabledAction // { disabled: true, hint: 'Disponível com o planejador' }
}

export interface DisabledAction { readonly disabled: true, readonly hint: string }

// app/composables/useFarmDetailPage.ts
export interface FarmDetailPage {
  readonly header: ComputedRef<{
    readonly title: string                          // nome da fazenda; "" até carregar
    readonly breadcrumbs: readonly { label: string, to?: string }[] // [Fazendas → /farms, <nome>]
    readonly condition: ConditionView | null        // selo ao lado do título (farm.sprayOutlook)
  }>
  readonly tab: WritableComputedRef<FarmDetailTab>  // fica em 'fields'; ignora abas desabilitadas
  readonly tabs: ComputedRef<readonly FarmDetailTabView[]>
  readonly summary: ComputedRef<FieldsSummaryView>
  readonly blocks: ComputedRef<readonly FieldBlockView[]>
  readonly toggleBlock: (key: string) => void

  readonly selectedFieldId: WritableComputedRef<string | null> // ?field=<id> (replace); id desconhecido → null
  readonly selectField: (id: string | null) => void // seleciona e reabre a área do talhão
  readonly popover: ComputedRef<FieldPopoverView | null>

  readonly mapMode: Ref<FarmMapMode>                // SegmentedControl Cultura/Status/Satélite
  readonly mapColorBy: ComputedRef<'crop' | 'status'>     // → MapView colorBy
  readonly mapBasemap: ComputedRef<'vector' | 'satellite'> // → MapView basemap (satélite mantém cor por cultura)
  readonly mapFeatures: ComputedRef<readonly MapFeature[]> // { id, geometry, status, cropIndex, label: "T1" }
  readonly mapPerimeter: ComputedRef<PerimeterGeometry | null>
  readonly mapLabels: ComputedRef<readonly { id: string, label: string }[]> // "T1"… (se a UI desenhar etiquetas)
  readonly legend: ComputedRef<MapLegendView>

  readonly newFieldTo: ComputedRef<string>          // "/farms/<id>/fields/new" (botão "Novo talhão")
  readonly addFieldToBlock: ComputedRef<{ label: string, to: string }>
  //   "Adicionar talhão à Área Norte" → "/farms/<id>/fields/new?block=%C3%81rea%20Norte"
  //   área = a do talhão selecionado, senão a primeira; sem áreas: "Adicionar talhão" sem query
  readonly newArea: DisabledAction                  // "Nova área" (decisão 2)

  readonly pending: ComputedRef<boolean>            // fazenda ou talhões carregando
  readonly error: ComputedRef<ApiError | null>      // erro da fazenda, senão dos talhões
  readonly isNotFound: ComputedRef<boolean>         // fazenda inexistente (404): tela "Fazenda não encontrada"
  readonly isEmpty: ComputedRef<boolean>            // fazenda sem talhões: CTA "Desenhe o primeiro talhão" → newFieldTo
  readonly refresh: () => Promise<void>
}
export function useFarmDetailPage(farmId: MaybeRefOrGetter<string>, options?: { now?: () => Date }): FarmDetailPage
```

Rótulos prontos em `utils/farmDetail.ts` (auto-importados): `FARM_DETAIL_TAB_LABELS`, `FARM_MAP_MODES` + `FARM_MAP_MODE_LABELS` (opções do SegmentedControl), `NO_BLOCK_LABEL`. Cor da faixa da linha e da legenda: `MAP_FIELD_COLORS[colorIndex % MAP_FIELD_COLORS.length]` (`theme/map-style.ts`). Status implementado e testado (`test/unit/{utils/farmDetail,composables/useFarmDetailPage}.test.ts`).

## Estados

- **Carregando** (`pending` e sem dados): título vazio, resumo "—", esqueleto na lista e o mapa com a ilustração.
- **Erro** (`error`, não 404): alerta "Não foi possível carregar a fazenda" + `refresh`.
- **Não encontrada** (`isNotFound`): mensagem e link para `/farms`.
- **Vazio** (`isEmpty`): resumo com 0, lista com CTA para `newFieldTo`, mapa só com o perímetro.
- **Sucesso**: o clique na linha ou no polígono (`MapView @select`) chama `selectField(id)`; o X do popover chama `selectField(null)`.

## Para a tela 10

- Rota `/farms/{id}/fields/new`, com a query opcional `block` (área pré-preenchida).
- Edição: rota `/farms/{id}/fields/{fieldId}/edit`; dados iniciais vêm de `useFarmFields(id).fields` (não há `GET` de um talhão); salvar com `updateField(fieldId, input)` (`updating`, `updateError` com `details` por campo).
- Ao salvar: `createField`/`updateField` e voltar para `/farms/{id}?field=<id>`.
