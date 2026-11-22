# Contrato — Assistente de nova fazenda (telas 5–8)

Figma `MeWDfQIHok0s80GbqCKGV3`: passo 1 `35:355`, etapas `14:125`, prévia `14:288`, passo 2 `14:30`/`36:472`/`37:503`, passo 3 `35:696`/`41:681`/`41:914`/`41:1176`, passo 4 `35:1048`.
Dono: trilha B (`useFarmWizard*`, `utils/wizard*`, `types/wizard.ts`).

## Decisões (orquestrador, 2026-11-21)

1. **Rascunho local** (`useState` + `localStorage`, chave `agro:farm-wizard-draft:v1`), gravado a cada mudança; "Rascunho salvo · há N min" reflete a gravação local. Restaurado em `onMounted`; nada é gravado antes de restaurar. **Limpo no logout**: o assistente registra `registerLogoutCleanup('farm-wizard', …)` e `useSession().logout()` roda os registros (`useLogoutCleanups`); um 401 não apaga. O rascunho guarda `userId` e o de outra pessoa é descartado ao restaurar.
2. **"Salvar e sair"**: grava o rascunho local e volta para `/farms` com toast "Rascunho salvo"; `/farms/new` retoma o rascunho.
3. **Foto/logotipo (passo 1)**: dropzone desabilitada com "Em breve" (lacuna de API; não modelada no composable).
4. **"Cultura principal" na prévia**: mostra o tipo de produção (divergência registrada; `FarmInput` não tem cultura).
5. **Área do perímetro desenhado**: prévia "≈ N ha · aproximada" com `approximateAreaHa` de `utils/geoArea.ts` (ADR 0012); a área do import e a oficial vêm da API.
6. **"Usar o CAR"**: desabilitado (sem endpoint); o nº do CAR é só armazenado.
7. **Obrigatórios do passo 2**: município, UF e perímetro; a sede marcada no mapa é opcional (vira `forecastPoint`; sem ela a API usa o centróide).

## Rotas

- `/farms/new` → passo 1 (ou o último rascunho). `?step=identification|location|owners|review`.
- `/farms/new?step=location&source=file` → passo 2 no modo "Importar arquivo" ("Importar KML" da lista).
- `?source=address|pin|file` sozinho também abre o passo 2. Erro 422 em `location.forecast_point` aparece no campo `latitude`.
- Pular para um passo cujos anteriores estão inválidos leva ao primeiro passo inválido (`replace`), exceto passos já alcançados (`furthestStep`). `?source=file` marca o passo 2 como alcançado: "Importar KML" abre direto nele com o passo 1 pendente; `save()` volta ao passo 1 se ele estiver vazio.

## Tipos (`app/types/wizard.ts`)

```ts
export type WizardStepKey = 'identification' | 'location' | 'owners' | 'review'
export type WizardStepStatus = 'complete' | 'current' | 'pending'
/** Como localizar a fazenda (passo 2, SegmentedControl). */
export type LocationMode = 'address' | 'pin' | 'file'
/** Como definir o perímetro (passo 2). 'car' fica desabilitado até haver endpoint. */
export type PerimeterMethod = 'draw' | 'car'

export interface IdentificationDraft {
  name: string
  code: string          // "Apelido / código" (vazio → null no envio)
  cooperative: string   // vazio → null
  productionType: ProductionType | null
}

export interface LocationDraft {
  mode: LocationMode
  perimeterMethod: PerimeterMethod
  postalCode: string    // como digitado; envio só com dígitos
  city: string
  state: string         // UF
  carNumber: string
  perimeter: PerimeterGeometry | null
  /** Só quando a API calculou (importação). Nunca calculado no front. */
  perimeterAreaHa: number | null
  /** Sede marcada no mapa, ou centróide devolvido pela importação. */
  forecastPoint: GeoPoint | null
  importedFileName: string | null
}

export interface WizardOwnerShare {
  ownerId: string
  name: string
  personType: PersonType
  /** Já mascarado (maskDocument) — o rascunho local nunca guarda CPF completo. */
  documentLabel: string
  role: LinkRole
  sharePercent: number | null
}

export interface FarmWizardDraft {
  version: 1
  identification: IdentificationDraft
  location: LocationDraft
  owners: WizardOwnerShare[]
  confirmed: boolean            // passo 4 "Confirmo que os dados estão corretos"
  furthestStep: WizardStepKey   // libera clique nas etapas
  savedAt: string | null        // ISO da última gravação local
}

export type WizardFieldErrors = Partial<Record<string, string>>  // chave = campo do passo
export type WizardErrors = Record<WizardStepKey, WizardFieldErrors>

export interface WizardStepView {   // componente de etapas 14:125
  key: WizardStepKey
  number: number                    // 1..4
  title: string                     // "Identificação"
  description: string               // "Nome, apelido e tipo"
  status: WizardStepStatus
  /** Pode clicar para ir até ela. */
  isReachable: boolean
}

export interface WizardPreviewRow { key: 'area'|'city'|'crop'|'forecastPoint', label: string, value: string }

export interface FarmWizardPreview { // prévia ao vivo 14:288
  name: string                      // "Nova fazenda" enquanto vazio
  rows: WizardPreviewRow[]          // Área, Município, Cultura principal, Ponto de previsão ("—" quando falta)
  perimeter: PerimeterGeometry | null
  forecastPoint: GeoPoint | null
  /** Texto do card com ícone sparkles. */
  hint: string
}
```

**Implementado** (`app/types/wizard.ts`, `app/utils/wizard.ts`, `app/utils/wizardDraft.ts`, `app/composables/useFarmWizard.ts`).

Campos com erro por passo:
- identification: `name`, `code`, `cooperative`, `productionType`
- location: `postalCode`, `city`, `state`, `carNumber`, `perimeter`
- owners: `owners` (lista vazia / total ≠ 100%), `owner:<ownerId>` (participação inválida)
- review: `confirmed`

## Composable `useFarmWizard(options?)`

```ts
export interface WizardDraftStorage {
  load(): string | null
  save(serialized: string): void
  clear(): void
}
export interface UseFarmWizardOptions {
  /** Default: localStorage no cliente; null no SSR. Testes injetam um fake. */
  storage?: WizardDraftStorage | null
  now?: () => Date
}

export function useFarmWizard(options?: UseFarmWizardOptions): {
  // Estado (v-model direto: `identification.name`, `location.city`…)
  identification: ComputedRef<IdentificationDraft>   // objeto reativo, editável
  location: ComputedRef<LocationDraft>
  owners: ComputedRef<WizardOwnerShare[]>
  confirmed: WritableComputedRef<boolean>

  // Etapas e navegação (?step=)
  currentStep: ComputedRef<WizardStepKey>
  stepNumber: ComputedRef<number>           // "PASSO N DE 4"
  steps: ComputedRef<WizardStepView[]>
  isFirstStep: ComputedRef<boolean>         // botão "Cancelar" em vez de "Voltar"
  isLastStep: ComputedRef<boolean>          // "Salvar fazenda" em vez de "Continuar"
  next(): Promise<boolean>                  // valida o passo atual; false se inválido
  back(): Promise<void>
  goTo(step: WizardStepKey): Promise<boolean> // false se não alcançável
  cancel(): Promise<void>                   // descarta o rascunho → /farms

  // Validação de apresentação (aparece após touch ou tentativa de avançar)
  fieldErrors: ComputedRef<WizardFieldErrors>   // do passo atual (locais + 422 da API)
  touch(field: string): void
  isStepValid(step: WizardStepKey): boolean

  // Passo 2 — ver seção "Passo 2 (tela 6)" (vem de useFarmWizardLocation, espalhado no retorno)

  // Passo 3 (OwnerSearch / OwnerForm da trilha A)
  addOwner(owner: OwnerSummary | Owner, sharePercent?: number | null): void
  removeOwner(ownerId: string): void
  setOwnerShare(ownerId: string, sharePercent: number | null): void
  totalShare: ComputedRef<number>               // barra "Participação total"
  shareTotalLabel: ComputedRef<string>          // "100%"

  // Prévia e revisão
  preview: ComputedRef<FarmWizardPreview>
  review: ComputedRef<WizardReviewSection[]>    // passo 4 (35:1048)
  draftSavedLabel: ComputedRef<string | null>   // "Rascunho salvo · há 2 min"
  draftSavedAt: Readonly<Ref<number | null>>    // epoch ms da última gravação (anúncio ao vivo no WizardSteps)

  // Envio
  save(): Promise<boolean>        // createFarm(status: active) → flash farmCreatedFlash → /farms
  saveAndExit(): Promise<void>    // grava rascunho local → flash → /farms
  saving: ComputedRef<boolean>
  saveError: ComputedRef<string | null>  // mensagem geral (erros por campo vão para fieldErrors)
}

export interface WizardReviewSection {
  step: Exclude<WizardStepKey, 'review'>
  title: string                    // "1 · Identificação"
  rows: { label: string, value: string, isEmpty: boolean }[]  // texto final; vazio = "Não informado" + isEmpty (estilo cinza)
  emptyMessage: string | null      // "Nenhum proprietário vinculado." quando a seção não tem linhas
}
```

### Comportamento
- Estado em `useState('farm-wizard:draft')`: sobrevive à troca de passo e à navegação do cliente. No cliente, o rascunho salvo é restaurado em `onNuxtReady` (evita divergência de hidratação) e regravado a cada mudança.
- `next()`: marca todos os campos do passo como tocados; com erro, fica no passo. Sem erro, avança (`navigateTo({ query: { step } })`) e atualiza `furthestStep`.
- `save()`: valida os 4 passos; se algum falhar, vai ao primeiro inválido. Monta `FarmInput` com `toFarmInput(draft)` (`status: 'active'`, strings vazias → `null`, CEP só dígitos, UF maiúscula). Erro 422: `details` vão para os campos do passo correspondente (`name`, `state`, `location.perimeter`, `owners[0].owner_id`…) e o assistente volta para esse passo; `save()` só resolve depois da troca de rota (a UI foca o erro em seguida). Sucesso: `useFlash().show(farmCreatedFlash(farm))`, limpa o rascunho, `navigateTo('/farms')`.
- Regras agronômicas e área nunca são calculadas aqui. Soma de participação = 100% é só aviso de UX (a API decide).

## Passo 2 (tela 6) — localização

Figma `14:30` (desenhar), `36:472` (marcar sede; lat/long + "Usar minha localização" em `36:748`), `37:503` (importar arquivo).

### Decisões (usuário/orquestrador, 2026-11-21)

1. **Município/UF digitados** (obrigatórios); aviso "Município identificado pelas coordenadas" oculto. Exceção: se o import devolver `municipality`/`state`, eles preenchem os campos ainda vazios. Pendência futura: `GET /geo/reverse?lat=&lng=` → `{ city, state }` e `GET /geo/postal-codes/{cep}` na `services/api` (plano próprio).
2. **Limite do arquivo: 20 MB** (Figma). `openapi.yaml`, mock e pré-checagem do front atualizados.
3. **`PerimeterImport` ampliado** (planned, campos opcionais): `municipality`, `state`, `datum` ("SIRGAS 2000"), `field_count`. Modelo: `city`, `state`, `datum`, `fieldCount`. O cartão do import mostra município, sistema e o banner "O arquivo já tem os talhões" quando `fieldCount > 0`.
4. **Sede fora do perímetro**: aviso que não bloqueia (`headquartersWarning`).
5. **"Usar minha localização"**: `navigator.geolocation` aprovado.

### Modos
- `location.mode` (`'address' | 'pin' | 'file'`, SegmentedControl "Por endereço / Marcar no mapa / Importar arquivo"), também por `?source=address|pin|file` (abre o passo 2 mesmo com o passo 1 pendente).
- `location.perimeterMethod` (`'draw' | 'file' | 'car'`): "Desenhar no mapa / Importar arquivo / Usar o CAR (desabilitado)". Atualizado sozinho por `setDrawnPerimeter` e `importPerimeterFile`.

### Estado novo em `LocationDraft`
`latitude`, `longitude` (texto pt-BR, v-model direto; sincronizados com `forecastPoint`), `importedFileSize`, `importedSourceFormat`, `importedCity`, `importedState`, `importedDatum`, `importedFieldCount`.

### API do passo 2 (`useFarmWizardLocation`, exposta por `useFarmWizard`)
```ts
perimeterImportAccept: string                          // ".kml,.kmz,.geojson,.json,.zip" para o FileDropzone
importPerimeterFile(file: File): Promise<boolean>      // pré-checa formato/tamanho; POST /perimeter-imports
importing: Readonly<Ref<boolean>>
importError: Readonly<Ref<ApiError | null>>
importErrorMessage: ComputedRef<string | null>         // formato, > 20 MB, sem polígono (422), rede
importSummary: ComputedRef<PerimeterImportSummary | null> // { fileName, detailLabel "48 KB · KML", areaLabel, cityLabel, forecastPointLabel, datumLabel, fieldCount }
headquartersWarning: ComputedRef<string | null>        // "A sede está fora do perímetro…" (não bloqueia)
perimeterAreaLabel: ComputedRef<string | null>         // "486,2 ha" (API) | "≈ 41,2 ha · aproximada" (desenho) | null
setDrawnPerimeter(perimeter: PerimeterGeometry | null): void // controle de desenho da trilha C (descarta o import)
clearPerimeter(): void
undoPerimeter(): void                                  // "Desfazer": volta ao perímetro antes do último desenho/import; desfazer um import também reverte a sede e o município/UF que ele preencheu
canUndoPerimeter: ComputedRef<boolean>                 // histórico só desta visita; substitui o histórico local do LocationStep
setForecastPoint(point: GeoPoint | null): void         // pino no mapa; preenche lat/long
pasteCoordinates(text: string): boolean                // "-21,30, -46,71"
locateMe(): Promise<boolean>                           // "Usar minha localização"
locating: Readonly<Ref<boolean>>
locateError: Readonly<Ref<string | null>>              // permissão negada / indisponível / falha
```
Opção nova: `useFarmWizard({ geolocation })` (fake nos testes; padrão `navigator.geolocation` no cliente).

### Regras
- Import: o centróide vira sede só se nenhuma sede foi definida; desenhar depois descarta o import.
- Validação (`fieldErrors`): `perimeter` (obrigatório; desenho com < 3 vértices → "Feche o perímetro com pelo menos 3 vértices."), `latitude`/`longitude` (opcionais; se uma for preenchida, as duas válidas e no intervalo), além de `postalCode`, `city`, `state`.
- Coordenadas digitadas válidas atualizam `forecastPoint` (e a prévia); texto inválido mantém o último ponto válido; apagar as duas zera a sede (a API usa o centróide).
- Funções puras em `app/utils/wizardLocation.ts`.

## Passo 3 (tela 7) — proprietários

Figma `35:696` (lista), `41:681` (busca), `41:914` (modal "Cadastrar e vincular"), `41:1176` (vinculada + toast).

### Decisões (usuário, 2026-11-21)
1. **Sem reequilíbrio**: vincular (busca ou "Salvar e vincular") nunca mexe na participação dos demais. O total aparece como está (ex.: 120%), e o erro "A participação total precisa fechar 100%." bloqueia "Continuar" até a pessoa ajustar. "Vincular" sugere só o que falta para 100% ao novo proprietário. O toast traz "Participação total: 120%. Ajuste para fechar 100%." quando o total não fecha.
2. **Papel**: todos entram como `owner` (sem seletor no Figma).
3. **Busca**: `useOwners` carrega a 1ª página ao abrir o assistente; as sugestões aparecem com 2+ caracteres.

### API do passo 3 (`useFarmWizardOwners`, exposta por `useFarmWizard`)
```ts
ownerSearch: Ref<string>                         // v-model da busca (debounce 300 ms; opção searchDebounceMs)
isOwnerSearchActive: ComputedRef<boolean>        // dropdown aberto (2+ caracteres)
ownerSuggestions: ComputedRef<OwnerSuggestionView[]> // { id, name, initials, detail "PF · ***.330.884-** · Sítio Boa Esperança", owner }, sem os já vinculados
ownerSearchPending / ownerSearchError
newOwnerSuggestion: ComputedRef<string | null>   // "Não achou? Cadastrar “Hel…” como novo proprietário"
newOwnerPrefillName: ComputedRef<string>         // nome para pré-preencher o OwnerForm
ownerFormPreset: ComputedRef<{ name, currentShares }> // useOwnerForm({ mode: 'wizard', presetFarm })
ownerRows: ComputedRef<WizardOwnerRowView[]>     // { ownerId, name, initials, detail ("· nova"), shareInput, isHighlighted, error }
addOwner(owner, sharePercent?)                   // "Vincular"; sugere o que falta para 100%; limpa a busca
linkRegisteredOwner({ owner, sharePercent })     // retorno de useOwnerForm({mode:'wizard'}).submit(); não mexe nos demais; destaca
removeOwner(ownerId)
setOwnerShare(ownerId, "60" | 60 | null)
totalShare, shareTotalLabel ("100%"), shareProgress (0–100), isShareComplete
ownerNotice: { title "Helena Prado cadastrada e vinculada", description "Participação total: 120%. Ajuste…" | null } | null; dismissOwnerNotice()
alertsBannerVisible; dismissAlertsBanner()       // "Avisos de janela para os proprietários"
```
Erros (`fieldErrors` do passo e `ownerRows[].error`): `owners` (nenhum proprietário / total ≠ 100%), `owner:<id>` (participação fora de 1–100%). Funções puras em `app/utils/wizardOwners.ts`.

## Funções puras (`app/utils/wizard.ts`)
`WIZARD_STEPS`, `WIZARD_STEP_META`, `parseWizardStep`, `createEmptyDraft`, `validateIdentification`, `validateLocation`, `validateOwners`, `validateReview`, `validateStep`, `firstInvalidStep`, `buildStepViews`, `buildPreview`, `buildReview`, `toFarmInput`, `mapApiErrorsToSteps`, `formatDraftSavedLabel`, `WIZARD_MESSAGES`.
`app/utils/wizardDraft.ts`: `serializeDraft`, `parseDraft` (valida `unknown`), `browserDraftStorage`.
