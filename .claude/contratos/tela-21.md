# Contrato — Tela 21: Fazenda · Relevo dos talhões (Figma 67:960: 04b 67:961, 04c 67:1276, 04d 71:1086, 04e 67:1906)

Rota: a mesma da tela 9, `pages/farms/[id]/index.vue`, aba nova **`?tab=terrain`** (as abas usam o valor em inglês na URL: `fields` é o padrão e sai da query, `overview`, `owners`; o pedido citava `?tab=relevo`, mas seguimos o padrão existente).
Query da aba: `terrainField=<fieldId>` (seleção e popover; id desconhecido lê como `null`), `terrainView=detail` (cartão 04c; sem ele, mapa). Modal de ajuste (04d) e diálogo "Como calculamos" são estado local da página.
Lógica: `app/types/terrain.ts` (feito), `app/utils/terrain.ts` (tipos feitos; funções na etapa 2), `app/services/ports.ts` (`TerrainService` feito), composables `useFarmTerrain`, `useFieldTerrain`, `useTerrainAdjustmentForm`, `useFarmTerrainTab` e mudanças em `useFarmDetailPage`/`utils/farmDetail.ts` (logic-implementer, etapa 2).
ADR: `docs/adr/0021-relevo-do-talhao.md` (aceito). Nenhuma regra de relevo no front: classe, compatibilidade de método e `adjustment_outdated` vêm da API.

Nota do backend (2026-11-24): talhão menor que cerca de `MinValidCells` x área da célula (1,8 ha com os padrões) ou sobre células sem dado termina `failed` com "dados de elevação insuficientes"; para ele só vale o ajuste manual. A tela deve oferecer o ajuste nesse caso em vez de insistir no recálculo.

## Decisões

Aceitas pelo usuário/orquestrador:

1. **Camadas do mapa**: Declividade (classe), Altitude (faixas pela altitude média do talhão, `(elevationMinM + elevationMaxM) / 2`, sobre a faixa da fazenda `summary.elevationMinM..elevationMaxM`) e Exposição (8 setores + plano). O `FieldTerrainSummary` já traz `elevation_min_m`/`elevation_max_m`.
2. **Modal de ajuste sem classe calculada no front**: mostra a classe atual (da API) e o aviso "A classe é recalculada ao salvar."
3. **"Como calculamos"**: diálogo informativo, sem frame no Figma, com a atribuição exigida pela licença: "Contém dados modificados do Copernicus DEM GLO-30 © DLR e Airbus".
4. **"← Voltar ao mapa"** no 04c: volta para `terrainView` ausente, mantendo `terrainField`.

Tomadas neste contrato (confirmar na revisão):

5. **Lista e mapa partem dos talhões** (`useFarmFields`), não só do relevo: os grupos usam `groupFieldsByBlock` (mesma ordem e chaves da aba Áreas e talhões) e cada talhão é cruzado com o `FieldTerrainSummary` pelo id. O talhão sem linha no relevo (criado depois da última leitura) aparece como "Não calculado". Polígono, rótulo "T4" e variedade ("Arara" no popover) vêm do `Field`.
6. **Média do grupo** ("162 ha · média 7,4 %"): média da declividade efetiva ponderada pela área dos talhões com relevo do grupo, a mesma fórmula que a API documenta para `summary.slope_mean_pct`. É agregação de apresentação, não regra agronômica. Se o grupo não tiver relevo: "162 ha · sem relevo". Recolhido: "102 ha · 1 talhão sem relevo" quando houver talhão sem valor efetivo, senão a mesma média.
7. **Popover busca o detalhe** do talhão selecionado (`useFieldTerrain`, o mesmo do 04c) para montar o aviso de método a partir de `method_fits`. Enquanto carrega ou se falhar, usa o texto genérico quando `hasRestrictedMethod`; sem restrição, sem aviso.
8. **Faixas de altitude**: `TERRAIN_ELEVATION_BAND_COUNT = 5` faixas iguais sobre a faixa da fazenda (rótulos "872–887 m", arredondados ao metro). Faixa de largura zero (um só talhão) vira uma faixa. Altitude é só calculada: talhão com ajuste e sem cálculo fica "Não calculado" nessa camada.
9. **Legendas**: Declividade mostra as 5 classes sempre (é uma escala), mais "Não calculado" se houver; Exposição mostra os 9 valores sempre, mais "Sem exposição" (ajuste manual, `aspect` nulo) e "Não calculado" se houver; Altitude mostra as faixas, mais "Não calculado" se houver.
10. **Badge da aba**: `count = summary.restrictedFieldCount` quando > 0; `null` com 0 ou antes de carregar (o badge é um alerta, não um total). A aba Relevo fica entre "Áreas e talhões" e "Proprietários" e é habilitada.
11. **Botões do cabeçalho na aba Relevo**: "Como calculamos" (sempre) e, só com `can('registry.write')`, "Calcular relevo" (`scope: 'missing'`) quando `calculatedFieldCount === 0`, senão "Recalcular relevo" (`scope: 'all'`). Durante o cálculo o botão fica `loading`. Talhões sem relevo com outros já calculados: faixa `missingBanner` "2 talhões sem relevo" com "Calcular" (`missing`), para não forçar recálculo de tudo.
12. **Consulta periódica**: `TERRAIN_POLL_INTERVAL_MS = 3000` (constante nomeada em `utils/terrain.ts`, sobrescrita por opção do composable nos testes). Só consulta com a aba Relevo ativa, no cliente, enquanto `progress !== null`; para ao zerar, ao trocar de aba ou ao desmontar. Ao terminar, recarrega o detalhe aberto.
13. **Mock de relevo independente dos talhões** (ADR 0010): `createMockTerrainService({ fields: FieldService, canWrite: () => boolean, now, latencyMs, overrides? })`.
    - Em `NUXT_PUBLIC_API_MOCK=true`, recebe o `fields` do mock e `overrides` do seed coerentes com as fazendas do mock: uma fazenda inteira sem cálculo (04e), um talhão forte ondulado com 62 % da área acima de 20 % (tratorizado restrito, autopropelido inadequado), um em baixada e um `failed` com motivo.
    - Em `NUXT_PUBLIC_API_MOCK_PENDING=true`, `terrain` NÃO é mais mock: a API Go implementa as 5 operações e o serviço é o HTTP real (`terrain` saiu de `PENDING_SERVICE_KEYS`).
    - Cálculo assíncrono simulado: `calculate` marca os talhões como `pending` e cria `progress`; cada `getFarm` avança um ou dois talhões até `ready` (ou `failed` no talhão de falha) e zera `progress` no fim. Classe e compatibilidade no mock usam os limites padrão do `openapi.yaml` (3/8/20/45 %, quatro limites) só para gerar dados coerentes; isso é dublê da API, não regra do front.
14. **Rótulo do modelo no 04b**: `FarmTerrain.summary` não traz o modelo de elevação, então o subtítulo usa a constante `TERRAIN_ELEVATION_MODEL_LABEL = 'Copernicus 30 m'` (fonte fixada pelo ADR 0021). No 04c o rótulo vem de `calculated.elevationModel` ("Copernicus DEM GLO-30 (30 m)"). Registrado como lacuna abaixo.

## Divergências de design

- **Rótulo do modelo no 04c**: o Figma diz "Copernicus DEM 30 m"; a tela mostra a fonte devolvida pela API ("Copernicus DEM GLO-30 (30 m)") para não reescrever o texto do servidor.
- **Média por área** e **subtítulo do 04b**: ver decisões 6 e 14.
- **Ordem das áreas**: alfabética natural, como na tela 9.

## Endpoints (openapi.yaml, tag `Terrain`, implementados na API; mock só em `NUXT_PUBLIC_API_MOCK=true`)

Conformidade (2026-11-24): os DTOs que a API produz (`terrain_dto.go`) e os do front (`terrainDto.ts`, `terrainMappers.ts`) foram comparados campo a campo: nomes snake_case, nulabilidade (`slope_p90_pct`, `aspect`, `progress`, `effective`, `calculated`, `adjustment`, `failure_reason`, `elevation_min_m/max_m` do resumo), enums, datas RFC 3339, `class_areas`, `profile`, `method_fits`, `limit_source`, `adjusted_by`, `summary.*`, 202 do cálculo com o `FarmTerrain` no corpo e 422 com `details[].field` (`origin`, `slope_mean_pct`, `slope_max_pct`, `position`, `justification`, `scope`). Sem divergência. Observação: algumas mensagens de `details` vêm em inglês do domínio (ex.: "must be between 0 and 100") e a tela as mostra como chegam.

- `GET /farms/{farmId}/terrain` (`getFarmTerrain`) → `useFarmTerrain` (lista, indicadores, mapa, progresso)
- `POST /farms/{farmId}/terrain/calculations` (`calculateFarmTerrain`, `registry.write`, 202) → `useFarmTerrain.calculate(scope)`
- `GET /farms/{farmId}/fields/{fieldId}/terrain` (`getFieldTerrain`) → `useFieldTerrain` (popover, 04c, modal)
- `PUT /farms/{farmId}/fields/{fieldId}/terrain/adjustment` (`adjustFieldTerrain`, `registry.write`) → `useFieldTerrain.adjust`
- `DELETE /farms/{farmId}/fields/{fieldId}/terrain/adjustment` (`removeFieldTerrainAdjustment`, `registry.write`) → `useFieldTerrain.removeAdjustment`
- Também: `GET /farms/{farmId}` e `GET /farms/{farmId}/fields` (já usados pela tela 9).

Lacunas (não inventadas; apontar para a API):
- `FarmTerrain.summary` sem `elevation_model` (decisão 14).
- Sem resumo por área (bloco); a média do grupo é calculada na apresentação (decisão 6).
- Erro de cálculo sem código estável (`failure_reason` é texto livre); a tela o mostra como veio.

## Assinatura

### Domínio (`app/types/terrain.ts`, criado)

```ts
export type TerrainStatus = 'not_calculated' | 'pending' | 'calculating' | 'ready' | 'failed'
export const SLOPE_CLASSES = ['flat', 'gently_undulating', 'undulating', 'strongly_undulating', 'mountainous'] as const
export type SlopeClass = (typeof SLOPE_CLASSES)[number]
export type TerrainPosition = 'hilltop' | 'slope' | 'lowland'
export const TERRAIN_ASPECTS = ['north', 'northeast', 'east', 'southeast', 'south', 'southwest', 'west', 'northwest', 'flat'] as const
export type TerrainAspect = (typeof TERRAIN_ASPECTS)[number]
export type TerrainSource = 'calculated' | 'manual'
export type TerrainAdjustmentOrigin = 'topographic_survey' | 'field_assessment'
export type SprayMethod = 'knapsack' | 'tractor_boom' | 'self_propelled' | 'airblast' | 'aircraft' | 'drone'
export type MethodFitStatus = 'compatible' | 'restricted' | 'unsuitable'
export type SlopeLimitSource = 'reference' | 'equipment'
export type TerrainCalculationScope = 'missing' | 'all'

export interface ElevationModel { source: string, resolutionM: number, version: string }
export interface TerrainMeasurement {
  slopeMeanPct: number, slopeP90Pct: number | null, slopeMaxPct: number
  slopeClass: SlopeClass, position: TerrainPosition, aspect: TerrainAspect | null
}
export interface SlopeClassArea { slopeClass: SlopeClass, areaHa: number, sharePct: number }
export interface ElevationProfilePoint { distanceM: number, elevationM: number }
export interface TerrainCalculation {
  measurement: TerrainMeasurement, elevationMinM: number, elevationMaxM: number
  classAreas: readonly SlopeClassArea[], profile: readonly ElevationProfilePoint[]
  elevationModel: ElevationModel, calculatedAt: Date
}
export interface TerrainAdjustment {
  origin: TerrainAdjustmentOrigin, measurement: TerrainMeasurement, justification: string
  adjustedBy: PersonRef /* types/planner */, adjustedAt: Date
}
export interface MethodFit {
  method: SprayMethod, slopeLimitPct: number | null, limitSource: SlopeLimitSource
  status: MethodFitStatus, areaAboveLimitPct: number | null
}
export interface FieldTerrainSummary {
  fieldId: string, fieldName: string, block: string | null, areaHa: number
  status: TerrainStatus, failureReason: string | null, source: TerrainSource | null
  measurement: TerrainMeasurement | null
  elevationMinM: number | null, elevationMaxM: number | null
  hasRestrictedMethod: boolean
}
export interface FarmTerrainSummary {
  fieldCount: number, calculatedFieldCount: number, slopeMeanPct: number | null
  restrictedFieldCount: number, elevationMinM: number | null, elevationMaxM: number | null
  calculatedAt: Date | null
}
export interface TerrainProgress { total: number, done: number }
export interface FarmTerrain { farmId: string, summary: FarmTerrainSummary, progress: TerrainProgress | null, items: readonly FieldTerrainSummary[] }
export interface FieldTerrain {
  fieldId: string, farmId: string, status: TerrainStatus, failureReason: string | null
  source: TerrainSource | null, effective: TerrainMeasurement | null
  calculated: TerrainCalculation | null, adjustment: TerrainAdjustment | null
  adjustmentOutdated: boolean, methodFits: readonly MethodFit[]
}
export interface FieldTerrainAdjustmentInput {
  origin: TerrainAdjustmentOrigin, slopeMeanPct: number, slopeMaxPct: number
  position: TerrainPosition, justification: string
}
```
(Todos os campos são `readonly` no arquivo.) DTOs snake_case vão em `app/types/terrainDto.ts` e o mapeamento em `services/http/terrainMappers.ts` (etapa 2, padrão de `plannerDto`/`plannerMappers`; datas viram `Date`).

### Porta (`app/services/ports.ts`, criada)

```ts
export interface TerrainService {
  getFarm(farmId: string): Promise<FarmTerrain>
  calculate(farmId: string, scope: TerrainCalculationScope): Promise<FarmTerrain>
  getField(farmId: string, fieldId: string): Promise<FieldTerrain>
  adjust(farmId: string, fieldId: string, input: FieldTerrainAdjustmentInput): Promise<FieldTerrain>
  removeAdjustment(farmId: string, fieldId: string): Promise<FieldTerrain>
}
// Etapa 2: ApiServices ganha `readonly terrain: TerrainService` (http, mock e pending — decisão 13).
```

### View-models (`app/utils/terrain.ts`, tipos criados)

```ts
export type TerrainMapLayer = 'slope' | 'elevation' | 'aspect'
export type TerrainClassKey = SlopeClass | 'not_calculated'
export type ElevationBandKey = `elevation-${number}`             // 0 = mais baixa
export type TerrainMapKey = TerrainClassKey | ElevationBandKey | TerrainAspect | 'no_aspect'

export interface SlopeClassBadgeView { key: TerrainClassKey, label: string }          // "Forte ondulado" | "Não calculado"
export interface TerrainIndicatorView { label: string, value: string, hint: string | null }
export interface TerrainPanelHeaderView { title: string, subtitle: string }          // "Relevo por talhão" / "Modelo de elevação Copernicus 30 m · calculado em 12/11/2026"
export interface FarmTerrainIndicatorsView { slopeMean: TerrainIndicatorView, restricted: TerrainIndicatorView, elevation: TerrainIndicatorView }
export interface TerrainFieldRowView {
  id: string, name: string
  detail: string            // "6,2 % · máx. 9,8 % · encosta" | "Calculando…" | "Falha no cálculo" | "Relevo não calculado"
  badge: SlopeClassBadgeView, colorKey: TerrainClassKey
  isAdjusted: boolean, isFailed: boolean, selected: boolean
}
export interface TerrainBlockView { key: string, name: string, areaLabel: string, collapsedLabel: string, expanded: boolean, fields: readonly TerrainFieldRowView[] }
export interface TerrainMapFeature { id: string, geometry: GeoJsonPolygon, terrainKey: TerrainMapKey, dashed: boolean, label: string }
export interface TerrainLegendItemView { key: TerrainMapKey, label: string }
export interface TerrainLegendView { layer: TerrainMapLayer, title: string, items: readonly TerrainLegendItemView[] }
export interface TerrainMapLayerOptionView { value: TerrainMapLayer, label: string, disabled: boolean }
export interface TerrainPopoverView {
  id: string, name: string, subtitle: string   // "Área Centro · Arara · 44,0 ha"
  badge: SlopeClassBadgeView
  slopeMean: TerrainIndicatorView, slopeMax: TerrainIndicatorView, relief: TerrainIndicatorView  // Média / Máxima / Desnível "69 m"
  methodWarning: string | null, canOpenDetail: boolean
}
export interface SlopeClassAreaRowView { key: SlopeClass, label: string, areaLabel: string, shareLabel: string, sharePct: number }
export interface ProfilePointView { x: number, y: number }   // 0..1; x 0 = topo; y 1 = mais alto
export interface ElevationProfileView { points: readonly ProfilePointView[], topLabel: string, baseLabel: string } // "941 m · topo" / "872 m · base (290 m)"
export interface TerrainFactRowView { label: string, value: string }     // Posição / Exposição / Baixada fria (Sim = position === 'lowland')
export interface MethodFitRowView { method: SprayMethod, label: string, limitLabel: string, status: MethodFitStatus, statusLabel: string }
export interface TerrainComparisonRowView { label: string, calculated: string, adjusted: string }
export interface FieldTerrainDetailView {
  id: string, title: string, badge: SlopeClassBadgeView, subtitle: string
  status: TerrainStatus, failureMessage: string | null
  hasAdjustment: boolean, outdatedWarning: string | null
  comparison: readonly TerrainComparisonRowView[], adjustmentNote: string | null
  indicators: { slopeMean, slopeP90, slopeMax, elevation: TerrainIndicatorView }
  classAreas: readonly SlopeClassAreaRowView[], profile: ElevationProfileView | null
  facts: readonly TerrainFactRowView[], methods: readonly MethodFitRowView[], methodsNote: string
}
export interface TerrainNotCalculatedView { title: string, items: readonly string[], calculateLabel: string | null, noPermissionMessage: string | null }
export interface TerrainProgressView { label: string, percent: number }   // "9 de 14 talhões"
export interface TerrainFailureView { fieldId: string, name: string, reason: string }
export interface TerrainMissingBannerView { label: string, calculateLabel: string | null }
export type TerrainTabState = 'loading' | 'error' | 'not_calculated' | 'calculating' | 'ready'
export type TerrainView = 'map' | 'detail'
export type FarmHeaderActionId = 'newArea' | 'newField' | 'terrainMethodology' | 'calculateTerrain' | 'recalculateTerrain'
export interface FarmHeaderActionView {
  id: FarmHeaderActionId, label: string, icon: string | null, variant: 'primary' | 'secondary'
  to: string | null, unavailableHint: string | null, loading: boolean
}
export type TerrainAdjustmentFormField = 'origin' | 'slopeMeanPct' | 'slopeMaxPct' | 'position' | 'justification'
export type TerrainAdjustmentFormErrors = Partial<Record<TerrainAdjustmentFormField, string>>
export interface TerrainAdjustmentFormValues { origin: TerrainAdjustmentOrigin, slopeMeanPct: string, slopeMaxPct: string, position: TerrainPosition | null, justification: string }
export interface TerrainAdjustmentHintsView { slopeMean: string | null, slopeMax: string | null, position: string | null, currentClass: string, classNotice: string }
export const TERRAIN_SCOPE_BY_ACTION = { calculateTerrain: 'missing', recalculateTerrain: 'all' } as const
```

Funções e constantes de `utils/terrain.ts` (etapa 2, puras e testadas): `TERRAIN_POLL_INTERVAL_MS`, `TERRAIN_ELEVATION_BAND_COUNT`, `TERRAIN_ELEVATION_MODEL_LABEL`, `TERRAIN_COPERNICUS_ATTRIBUTION`, `TERRAIN_METHODS_NOTE`, `SLOPE_CLASS_LABELS`, `TERRAIN_POSITION_LABELS`, `TERRAIN_ASPECT_LABELS`, `SPRAY_METHOD_LABELS`, `TERRAIN_ADJUSTMENT_ORIGIN_LABELS`, `TERRAIN_MAP_LAYERS` + `TERRAIN_MAP_LAYER_LABELS`; `terrainClassKey(measurement | null)`, `elevationBandOf(field, range, count)`, `terrainMapKey(layer, summary, range)`, `toTerrainBlockViews(blocks, summariesById, collapsed, selectedId)`, `toFarmTerrainIndicators(terrain | null)`, `toTerrainPanelHeader(terrain | null)`, `toTerrainMapFeatures(fields, summariesById, layer, range)`, `terrainLegend(layer, summaries, range)`, `toTerrainPopoverView(field, summary, detail | null)`, `methodFitWarning(fits)`, `toFieldTerrainDetailView(field, detail)`, `normalizeProfile(points)`, `toTerrainNotCalculatedView(fieldCount, canWrite)`, `toTerrainProgressView(progress)`, `terrainFailures(summaries)`, `terrainTabState(...)`, `emptyTerrainAdjustmentValues(detail)`, `validateTerrainAdjustment(values)`, `toTerrainAdjustmentInput(values)`, `terrainAdjustmentErrorsFromApi(details)` (`slope_mean_pct` → `slopeMeanPct` etc.), `terrainTabFrom(query)`/`terrainViewFrom(query)`.

Formatos: percentuais com uma casa ("6,2 %", espaço antes de %), altitude inteira ("872–948 m", traço meia-risca), área "44,0 ha" (`formatFieldArea`), data "12/11/2026" (`formatDate`).

### Composables (etapa 2)

```ts
// app/composables/useFarmTerrain.ts
export interface FarmTerrainResource {
  readonly terrain: Readonly<Ref<FarmTerrain | null>>
  readonly pending: ComputedRef<boolean>
  readonly error: ComputedRef<ApiError | null>
  readonly refresh: () => Promise<void>
  readonly isRunning: ComputedRef<boolean>                // progress !== null
  readonly calculate: (scope: TerrainCalculationScope) => Promise<FarmTerrain | null>  // atualiza `terrain` com a resposta 202
  readonly calculating: Readonly<Ref<boolean>>
  readonly calculateError: Readonly<Ref<ApiError | null>>
}
export function useFarmTerrain(
  farmId: MaybeRefOrGetter<string>,
  options?: { active?: MaybeRefOrGetter<boolean>, pollIntervalMs?: number, service?: TerrainService },
): FarmTerrainResource
// chave `terrain:farm:<farmId>`; com `active` false não carrega nem consulta (aba inativa).

// app/composables/useFieldTerrain.ts
export interface FieldTerrainResource {
  readonly terrain: Readonly<Ref<FieldTerrain | null>>   // null sem fieldId
  readonly pending: ComputedRef<boolean>
  readonly error: ComputedRef<ApiError | null>
  readonly refresh: () => Promise<void>
  readonly adjust: (input: FieldTerrainAdjustmentInput) => Promise<FieldTerrain | null>
  readonly adjusting: Readonly<Ref<boolean>>
  readonly adjustError: Readonly<Ref<ApiError | null>>
  readonly removeAdjustment: () => Promise<FieldTerrain | null>
  readonly removing: Readonly<Ref<boolean>>
  readonly removeError: Readonly<Ref<ApiError | null>>
}
export function useFieldTerrain(
  farmId: MaybeRefOrGetter<string>,
  fieldId: MaybeRefOrGetter<string | null>,
  options?: { onChanged?: () => Promise<void>, service?: TerrainService },  // onChanged: recarrega o relevo da fazenda
): FieldTerrainResource
// chave `terrain:field:<farmId>:<fieldId>`; sem fieldId resolve null sem chamar a API.

// app/composables/useTerrainAdjustmentForm.ts
export interface TerrainAdjustmentForm {
  readonly title: ComputedRef<string>                     // "Ajustar relevo · Talhão 4"
  readonly values: TerrainAdjustmentFormValues            // reactive, v-model
  readonly errors: ComputedRef<TerrainAdjustmentFormErrors> // local (após tentar salvar) + 422 da API por campo
  readonly formError: ComputedRef<string | null>          // erro sem campo (403, rede, 404)
  readonly hints: ComputedRef<TerrainAdjustmentHintsView>
  readonly originOptions: readonly { value: TerrainAdjustmentOrigin, label: string }[]
  readonly positionOptions: readonly { value: TerrainPosition, label: string }[]
  readonly justificationMaxLength: 1000
  readonly infoNotice: string                             // aviso informativo do 04d
  readonly saving: Readonly<Ref<boolean>>
  readonly reset: () => void                              // ao abrir: preenche com o ajuste atual, senão com o calculado
  readonly submit: () => Promise<boolean>                 // true = salvo; a página fecha o modal
}
export function useTerrainAdjustmentForm(options: {
  fieldName: MaybeRefOrGetter<string>
  field: FieldTerrainResource                             // inversão de dependência: testável com fake
}): TerrainAdjustmentForm
// Validação local só espelha o contrato: obrigatórios, 0–100, máx ≥ média, justificativa 1..1000 (trim).

// app/composables/useFarmTerrainTab.ts (montado por useFarmDetailPage)
export interface FarmTerrainTab {
  readonly state: ComputedRef<TerrainTabState>
  readonly error: ComputedRef<ApiError | null>
  readonly refresh: () => Promise<void>
  readonly header: ComputedRef<TerrainPanelHeaderView>
  readonly indicators: ComputedRef<FarmTerrainIndicatorsView>
  readonly blocks: ComputedRef<readonly TerrainBlockView[]>
  readonly toggleBlock: (key: string) => void
  readonly notCalculated: ComputedRef<TerrainNotCalculatedView | null>   // 04e (estado not_calculated, ou calculating sem nada pronto)
  readonly progress: ComputedRef<TerrainProgressView | null>
  readonly failures: ComputedRef<readonly TerrainFailureView[]>
  readonly missingBanner: ComputedRef<TerrainMissingBannerView | null>
  readonly calculate: (scope: TerrainCalculationScope) => Promise<void>
  readonly calculating: ComputedRef<boolean>              // pedido em voo ou progress != null
  readonly calculateError: Readonly<Ref<ApiError | null>>
  readonly mapLayer: Ref<TerrainMapLayer>
  readonly mapLayers: ComputedRef<readonly TerrainMapLayerOptionView[]>
  readonly mapFeatures: ComputedRef<readonly TerrainMapFeature[]>
  readonly legend: ComputedRef<TerrainLegendView>
  readonly selectedFieldId: WritableComputedRef<string | null>   // ?terrainField (replace)
  readonly selectField: (id: string | null) => void       // reabre o grupo; null também sai do 04c
  readonly popover: ComputedRef<TerrainPopoverView | null>  // só em view 'map'
  readonly view: WritableComputedRef<TerrainView>          // ?terrainView=detail (replace); 'detail' sem seleção lê 'map'
  readonly openDetail: () => void                          // "Ver relevo do talhão"
  readonly backToMap: () => void                           // "← Voltar ao mapa"
  readonly detail: ComputedRef<FieldTerrainDetailView | null>
  readonly detailPending: ComputedRef<boolean>
  readonly detailError: ComputedRef<ApiError | null>
  readonly refreshDetail: () => Promise<void>
  readonly canWrite: ComputedRef<boolean>                  // can('registry.write'): "Ajustar manualmente", "Remover ajuste", calcular
  readonly removeAdjustment: () => Promise<boolean>
  readonly removing: Readonly<Ref<boolean>>
  readonly removeError: Readonly<Ref<ApiError | null>>
  readonly adjustmentForm: TerrainAdjustmentForm           // do talhão selecionado
  readonly methodology: { readonly title: string, readonly paragraphs: readonly string[], readonly attribution: string }
}
export function useFarmTerrainTab(options: {
  farmId: MaybeRefOrGetter<string>
  active: MaybeRefOrGetter<boolean>                       // tab === 'terrain'
  fields: ReturnType<typeof useFarmFields>                // o mesmo recurso já usado pela página (sem nova chamada)
}): FarmTerrainTab
```

### Mudanças em `utils/farmDetail.ts` e `useFarmDetailPage`

```ts
export type FarmDetailTab = 'overview' | 'fields' | 'terrain' | 'owners' | 'applications' | 'windows'
// FARM_DETAIL_TABS na ordem acima; FARM_DETAIL_TAB_LABELS.terrain = 'Relevo'; ENABLED_TABS inclui 'terrain'
export function farmDetailTabs(counts: { fields: number | null, owners: number | null, terrainRestricted: number | null }): FarmDetailTabView[]
export function farmHeaderActions(tab: FarmDetailTab, ctx: {
  farmId: string, canWrite: boolean
  terrain: { calculatedFieldCount: number, running: boolean, requesting: boolean } | null
}): FarmHeaderActionView[]
// Outras abas, com canWrite: [newArea (secondary, "Em breve"), newField (primary, to newFieldRoute)] — como hoje.
// Aba terrain: [terrainMethodology (secondary, "Como calculamos")] + com canWrite
//   [calculateTerrain "Calcular relevo"] se calculatedFieldCount === 0, senão [recalculateTerrain "Recalcular relevo"].

// FarmDetailPage ganha:
readonly terrain: FarmTerrainTab
readonly headerActions: ComputedRef<readonly FarmHeaderActionView[]>
readonly runHeaderAction: (id: FarmHeaderActionId) => void
//   calculateTerrain/recalculateTerrain → terrain.calculate(TERRAIN_SCOPE_BY_ACTION[id]);
//   terrainMethodology e newArea não fazem nada aqui: a página abre o diálogo; newField usa `to`.
```
`newArea`, `newFieldTo` e `canWrite` continuam no retorno (compatibilidade da tela 10).

## Mapa (figma-ui-implementer implementa em `MapView`/`MapCanvas`/`theme/map-style.ts`)

Proposta de assinatura; se a UI preferir outro nome, avise e o logic-implementer renomeia em `TerrainMapFeature`:

```ts
// theme/map-style.ts
export type MapColorBy = 'crop' | 'status' | 'terrain'
export const TERRAIN_CLASS_COLORS: Readonly<Record<TerrainClassKey, string>>       // not_calculated = cinza
export const TERRAIN_ELEVATION_COLORS: readonly string[]                            // sequencial, índice = banda
export const TERRAIN_ASPECT_COLORS: Readonly<Record<TerrainAspect | 'no_aspect' | 'not_calculated', string>>
export function terrainPalette(layer: TerrainMapLayer): Readonly<Record<string, string>> // chave → cor (elevation-0…)
// components/map/MapView.vue — MapFeature ganha:
readonly terrainKey?: string     // colorBy="terrain": 'match' na paleta passada por prop
readonly dashed?: boolean        // contorno tracejado (line-dasharray filtrado por propriedade)
// prop nova: terrainPalette?: Readonly<Record<string, string>>
```
A barra da linha usa `TERRAIN_CLASS_COLORS[row.colorKey]`; a legenda, `terrainPalette(legend.layer)[item.key]`. Em 04e, `mapLayers[].disabled` é true e todos os polígonos vêm `not_calculated` + `dashed`.

## Estados

- **Carregando** (`state === 'loading'`): indicadores "—", esqueleto na lista, mapa com os polígonos cinza (talhões já carregados).
- **Erro** (`state === 'error'`): alerta "Não foi possível carregar o relevo" + `refresh`. Erro da fazenda continua tratado pela página (404/403).
- **Vazio** (fazenda sem talhões): o mesmo `isEmpty` da tela 9 (CTA "Desenhe o primeiro talhão").
- **Não calculado** (`state === 'not_calculated'`, 04e): `notCalculated` com a lista e o botão (com `registry.write`) ou `noPermissionMessage`; mapa cinza, camadas desabilitadas.
- **Calculando** (`state === 'calculating'`): `progress` ("9 de 14 talhões" + barra). Sem nenhum pronto, painel 04e com o progresso no lugar do botão; com algum pronto, lista normal com a barra no topo. Linhas `pending`/`calculating` mostram "Calculando…".
- **Falha por talhão**: linha com `isFailed` e selo "Não calculado"; `failures` lista nome + `failure_reason` (no 04e e acima da lista). O detalhe mostra `failureMessage`.
- **Sem permissão**: sem botões de cálculo, "Ajustar manualmente" e "Remover ajuste"; leitura completa. 403 em ação vira `formError`/`calculateError` com `forbiddenMessageFor`.
- **Detalhe** (04c): `detailPending` → esqueleto no cartão; `detailError` → alerta + `refreshDetail`; com ajuste: `comparison`, `adjustmentNote`, ação "Remover ajuste" (`removeAdjustment`); `outdatedWarning` quando a API indicar.
- **Modal** (04d): `adjustmentForm.reset()` ao abrir; `submit()` true fecha e o detalhe e a lista já vêm atualizados (o composable recarrega o relevo da fazenda via `onChanged`); erros 422 aparecem no campo.

## O que a UI consome

- Página: `tab` (`'terrain'`), `tabs` (badge da aba), `headerActions` + `runHeaderAction` (abre o diálogo "Como calculamos" localmente para `terrainMethodology`) e `terrain: FarmTerrainTab`.
- Componentes sugeridos (UI decide): `TerrainPanel` (lista, indicadores, grupos), `TerrainNotCalculatedPanel` (04e), `TerrainFieldDetail` (04c), `TerrainAdjustmentDialog` (04d), `TerrainMethodologyDialog`, `MapLegend` com `TerrainLegendView`.
- Textos fixos do diálogo e da nota de métodos vêm de `terrain.methodology` e `detail.methodsNote`; a atribuição Copernicus é `terrain.methodology.attribution`.

## Etapa 2 (lógica implementada): ajustes ao contrato

- `FarmTerrainTab` ganhou `restrictedFieldCount: ComputedRef<number | null>` (badge da aba) e `headerState` (`{ calculatedFieldCount, running, requesting } | null`, usado por `farmHeaderActions`). A página usa `page.terrain`, `page.headerActions` e `page.runHeaderAction`.
- `farmDetailTabs({ ..., terrainRestricted? })`: o campo é opcional (a tela 10 não o passa); 0 ou ausente = sem badge.
- `terrainTabFrom` não existe: a aba continua em `farmDetailTabFrom`. Existem `terrainViewFrom(value)` e `terrainFieldIdFrom(value)` (recebem o valor de `route.query.terrainView`/`terrainField`).
- `TerrainPopoverView.canOpenDetail` é `true` sempre que o talhão tem linha no relevo (também sem cálculo e com falha: o cartão 04c é por onde se ajusta um talhão sem cálculo).
- Rótulo do método `drone` é "Drone" (sem "(ARP)"), para a frase do aviso de método ficar em minúsculas sem sigla quebrada.
- `useTerrainAdjustmentForm.title`, `.errors`, `.formError` e `.hints` são `ComputedRef`: em template, só desembrulham no nível raiz. Desestruture (`const { title } = form`) ou use `form.title.value`; `<BaseModal :title="form.title">` quebra o typecheck.
- Mock: `terrainMock.ts` (serviço), `terrainSeed.ts` (mundo mock: Santa Rita calculada com Talhão 4 forte ondulado 62 % acima de 20 %, Talhão 2 com ajuste manual e Talhão 9 `failed`; Ouro Verde com a baixada plana; Boa Esperança sem cálculo; Recanto calculada). Sem seed, tudo começa `not_calculated`.
- `useFarmTerrain`/`useFieldTerrain` mostram a resposta de `calculate`/`adjust`/`removeAdjustment` até a próxima carga (`internal/latest.ts`); o ajuste também chama `onChanged` (recarrega o relevo da fazenda).
- Serviço HTTP: `createTerrainHttpService` em `services/http/services.ts`; DTOs em `types/terrainDto.ts`; mapeamento em `services/http/terrainMappers.ts`.
- Presentação aceita (revisão): a média ponderada por área de cada bloco (decisão 6) e as 5 faixas de altitude pela altitude média do talhão (decisão 8) são agregações de apresentação. Se o bloco virar fonte de decisão, o agregado deve vir da API (ADR 0021).
- `FarmTerrainTab.clearCalculateError` dispensa o alerta de erro do cálculo; o erro também some ao trocar de fazenda. Ação em voo cuja fazenda/talhão mudou não atualiza a tela; o resultado de `calculate`/`adjust` prevalece sobre um GET iniciado antes dele.
