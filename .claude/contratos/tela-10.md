# Contrato — Tela 10: Novo talhão / Editar talhão (Figma 16:82)

Rotas:
- `pages/farms/[id]/fields/new.vue` → `useFieldForm({ farmId: () => route.params.id })`. Query opcional `?block=` pré-preenche a área (vem do "Adicionar talhão à Área X" da tela 9).
- `pages/farms/[id]/fields/[fieldId]/edit.vue` → `useFieldForm({ farmId, fieldId })`, com o mesmo formulário.

Lógica: `app/composables/useFieldForm.ts`, `app/utils/fieldForm.ts` e `app/utils/geoArea.ts` (`approximateAreaHa`, compartilhado com a trilha B). Dependências (ADR 0012) instaladas: `terra-draw` 1.40.0, `terra-draw-maplibre-gl-adapter` 1.5.0 e `@turf/area` 7.4.0. Terra Draw só pode ser importado em `components/map/`.

## Decisões pendentes (padrão assumido entre parênteses)

1. **"≈ 4.080 plantas/ha · usado no cálculo de calda"**: o estande é calculado pela API (`plants_per_ha`). (Assumido: no cadastro novo, a ajuda diz "Estande calculado ao salvar"; na edição, mostra o valor da API: "≈ 4.082 plantas/ha · usado no cálculo de calda". O front não faz a conta.)
2. **Rótulo da área de prévia**: a ADR 0012 exige marcar a prévia como aproximada. (Assumido: valor "41,2 ha" com a legenda "aproximados, calculados a partir do desenho", que diverge do texto do Figma. Na edição, sem mexer no desenho, aparece a área da API com a legenda "área do cadastro".)
3. **Perímetro**: só um aviso, sem bloquear. A checagem é simples: algum vértice fora do perímetro da fazenda. Uma aresta que atravessa um perímetro côncavo não é detectada. Quem valida é a API.
4. **"+ Outra" (cultura)**: não há endpoint para criar cultura. (Assumido: abre a lista das outras culturas do catálogo `GET /crops`, hoje "Milho safrinha". Os chips fixos são Café arábica, Café conilon, Soja, Milho e Trigo, como no Figma. Se a cultura escolhida não for fixa, ela aparece como chip selecionado.)
5. **Safra (`season`)**: não aparece no Figma. (Assumido: é enviada como `null` no cadastro; na edição, o valor existente é mantido.)
6. **Espaçamento**: um campo só, "3,5 × 0,7" (entre linhas × entre plantas, em metros), opcional. Aceita `×`, `x`, `X` ou `*`, vírgula ou ponto.

## Endpoints

- `GET /farms/{farmId}` (cabeçalho, perímetro), `GET /farms/{farmId}/fields` (contexto do mapa, nome sugerido, dados da edição), `GET /crops`
- `POST /farms/{farmId}/fields` (cadastro) e `PUT /farms/{farmId}/fields/{fieldId}` (edição, planned). Não existe `GET` de um talhão só: a edição procura o talhão na lista.

## Assinatura

```ts
// app/utils/geoArea.ts
export function approximateAreaHa(geometry: GeoJsonPolygon | GeoJsonMultiPolygon): number // 1 casa decimal; prévia, nunca a área oficial
export function vertexCount(polygon: GeoJsonPolygon | null): number           // anel externo sem o ponto de fechamento
export function isDrawablePolygon(polygon: GeoJsonPolygon | null): polygon is GeoJsonPolygon // >= 3 vértices
export function verticesInside(polygon: GeoJsonPolygon, perimeter: PerimeterGeometry): boolean

// app/utils/fieldForm.ts
export type FieldFormStep = 'draw' | 'details'
export type FieldFormField = 'name' | 'block' | 'cropCode' | 'variety' | 'plantingDate' | 'spacing' | 'polygon'
export type FieldFormErrors = Partial<Record<FieldFormField, string>>
export interface FieldFormValues {
  name: string; block: string; cropCode: string | null; variety: string
  plantingDate: string   // ISO "YYYY-MM-DD" ou "" (o TextField/v-date-picker cuida da máscara dd/mm/aaaa)
  spacing: string        // "3,5 × 0,7" ou ""
}
export interface CropChipView { code: string, label: string, icon: string, selected: boolean } // icon: "☕", "🌱", "🌽", "🌾", "🌿"

// app/composables/useFieldForm.ts
export function useFieldForm(options: {
  farmId: MaybeRefOrGetter<string>
  fieldId?: MaybeRefOrGetter<string | null | undefined>
}): {
  mode: ComputedRef<'create' | 'edit'>
  header: ComputedRef<{ title: string, breadcrumbs: { label: string, to?: string }[], condition: ConditionView | null }>
  //   [Fazendas → /farms, <fazenda> → /farms/<id>, "Novo talhão" | "<nome do talhão>"]
  tabs: ComputedRef<readonly FarmDetailTabView[]>   // mesmas abas da tela 9 ("Áreas e talhões" ativa)
  eyebrow: ComputedRef<string>                      // "NOVO TALHÃO · ÁREA SUL" | "NOVO TALHÃO" | "EDITAR TALHÃO · ÁREA SUL"
  panelTitle: string                                // "Detalhes do talhão"

  values: FieldFormValues                           // reactive; v-model direto nos campos
  namePlaceholder: ComputedRef<string>              // "Talhão 10" (próximo número livre)
  cropChips: ComputedRef<{ featured: CropChipView[], others: CropChipView[] }>
  selectCrop: (code: string) => void
  standHint: ComputedRef<string>                    // decisão 1

  polygon: WritableComputedRef<GeoJsonPolygon | null> // v-model do controle de desenho; cada set entra no histórico
  undo: () => void
  canUndo: ComputedRef<boolean>
  clearDrawing: () => void
  vertexCount: ComputedRef<number>
  drawHint: ComputedRef<string>  // "Clique no mapa para marcar os vértices · feche no primeiro ponto" | "Arraste os vértices para ajustar · 5 pontos"
  areaPreview: ComputedRef<{ label: string, caption: string, approximate: boolean }> // "41,2 ha" / "—"
  perimeterWarning: ComputedRef<string | null>      // decisão 3

  step: Ref<FieldFormStep>                          // 'draw' no cadastro, 'details' na edição; vai sozinho para 'details' quando o 1º desenho fecha e volta para 'draw' quando o desenho deixa de ser válido (desfazer, limpar)
  steps: ComputedRef<readonly { value: FieldFormStep, label: string, done: boolean, active: boolean }[]> // "Desenho" / "2 · Detalhes"
  goToDraw: () => void                              // "Ajustar"
  goToDetails: () => boolean                        // false (e erro em polygon) sem desenho válido

  mapPerimeter: ComputedRef<PerimeterGeometry | null>
  contextFeatures: ComputedRef<readonly FieldMapFeature[]> // { id, geometry, status, cropIndex, label: "T1" }; outros talhões (o editado fica de fora), cor por variedade
  mapMode: Ref<FarmMapMode>; mapColorBy: ComputedRef<'crop' | 'status'>; mapBasemap: ComputedRef<'vector' | 'satellite'>

  fieldErrors: ComputedRef<FieldFormErrors>         // locais após touch/submit + `details` da API (crop_code → cropCode; row/plant_spacing_m → spacing)
  formError: ComputedRef<string | null>             // erro geral (rede, 404, 500), sem texto cru
  touch: (field: FieldFormField) => void
  submit: () => Promise<boolean>                    // cria/atualiza → flash → navega para /farms/<id>?field=<id>
  saving: ComputedRef<boolean>
  cancelTo: ComputedRef<string>                     // "/farms/<id>" | "/farms/<id>?field=<fieldId>"

  pending: ComputedRef<boolean>                     // fazenda, talhões ou culturas carregando
  error: ComputedRef<ApiError | null>               // erro de carga
  isNotFound: ComputedRef<boolean>                  // fazenda inexistente ou (edição) talhão que não está na fazenda
}
```

## Comportamento

- **Desenho:** o controle (Terra Draw, `components/map/`) emite `update:modelValue` só ao **terminar** um traço ou arraste, nunca a cada movimento do mouse, para o histórico do "Desfazer" fazer sentido. Recebe `polygon` como valor inicial na edição.
- **Flash após salvar:** `{ kind: 'success', title: 'Talhão 9 cadastrado' | 'Talhão 9 atualizado', description: 'Área de 41,2 ha.', highlightId: <id> }`. A área do flash é a da API. **A tela 9 precisa consumir o flash** (`useFlash().consume()`) e mostrar o `ToastAlert`.
- **Edição:** o formulário é preenchido uma vez, quando o talhão aparece na lista, e o histórico do desenho começa vazio.
- **Estados:** carregando (esqueleto no painel, mapa com ilustração), erro de carga (alerta + voltar), não encontrado (mensagem + link para `/farms/<id>`), salvando (`saving` desabilita "Salvar talhão").
