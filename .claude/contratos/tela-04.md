# Tela 4: modal de proprietário (novo e edição)

Frames do Figma: 39:819 (novo · Dados), 39:1134 (novo · Vínculos), 39:1667 (editar · Helena Prado) e 41:1107 (assistente, passo 3: "Cadastrar e vincular"). Trilha A.

## Decisões aprovadas (2026-11-21)

1. Upload de RG/CNH: botão desabilitado com "Envio de documentos em breve", mais o aviso de pendência.
2. "Remover proprietário": oculto até existir `DELETE /owners/{id}`.
3. Validação local dos dígitos: o texto de ajuda passa a ser "CPF válido" / "CNPJ válido" (não "validado na Receita").
4. Papel `owner` por padrão; a UI só mostra o seletor de papel se o Figma mostrar.
5. WhatsApp obrigatório com os alertas ligados. A regra é checada no passo Vínculos e no envio, com o erro em `fieldErrors.windowAlerts` (junto ao toggle), nunca no passo Dados. No assistente, que não tem toggle, os alertas acompanham a presença do WhatsApp.
6. O composable expõe todos os campos; a UI mostra os do Figma de cada modo.

## Decisões propostas (histórico)

1. **Upload de documento (RG/CNH):** não há endpoint. Proposta: a UI mostra o bloco do Figma com o botão **desabilitado**, o texto "Envio de documentos em breve" e o aviso de pendência (`pendingDocumentNotice`). O arquivo não é guardado no front e o formulário não finge que enviou. Endpoint futuro: `POST /owners/{id}/documents` (multipart).
2. **"Remover proprietário" (edição):** não há endpoint. Proposta: ocultar o botão até existir `DELETE /owners/{id}`.
3. **Dígitos verificadores de CPF/CNPJ:** proposta: validar no front, para retorno imediato; a API continua sendo a autoridade (422/409). O texto "CPF validado na Receita" do Figma promete uma consulta externa que não existe. Proposta: exibir "CPF válido" só depois de passar a validação local.
4. **Papel no vínculo:** o Figma só tem checkbox e participação. Proposta: o padrão é `owner`; o modelo aceita `tenant` e `legal_representative` (participação nula), e a UI decide se mostra um seletor.
5. **Alertas no WhatsApp:** com o toggle ligado (padrão do Figma), o WhatsApp passa a ser obrigatório. Proposta: erro no campo "Informe o WhatsApp para receber os alertas.".
6. **Campos da edição:** o Figma de edição mostra só nome, documento bloqueado e WhatsApp. Proposta: o composable expõe todos os campos (nascimento e e-mail também), e a UI escolhe quais mostrar. O documento fica bloqueado na edição e é exibido mascarado.

## Composable

```ts
type OwnerFormMode = 'create' | 'edit' | 'wizard'
type OwnerFormStep = 'data' | 'links'
type OwnerFormField = 'name' | 'document' | 'birthDate' | 'phone' | 'email' | 'wizardShare' | 'windowAlerts'

interface OwnerFormOptions {
  mode: OwnerFormMode
  ownerId?: string                                   // obrigatório em 'edit'
  /** Só em 'wizard': fazenda em cadastro e as participações atuais dela (estado do assistente). */
  presetFarm?: { name: string, currentShares: readonly { name: string, sharePercent: number }[] }
}

interface OwnerLinkRowView {
  farmId: string; farmName: string
  selected: boolean; role: LinkRole
  share: string                                      // texto do input, ex. "50"
  othersShare: number                                // soma dos OUTROS proprietários da fazenda (da lista de fazendas)
  totalShare: number                                 // othersShare + a deste proprietário
  totalLabel: string                                 // "Sítio Boa Esperança · participação total" + "100%"
  error: string | null                               // participação inválida ou total acima de 100%
}

function useOwnerForm(options: OwnerFormOptions): {
  step: Readonly<Ref<OwnerFormStep>>
  personType: WritableComputedRef<PersonType>        // segmento PF/PJ (bloqueado na edição)
  name: WritableComputedRef<string>
  document: WritableComputedRef<string>              // máscara progressiva de CPF/CNPJ conforme personType
  birthDate: WritableComputedRef<string>             // "dd/mm/aaaa" com máscara; enviado como ISO (só PF)
  phone: WritableComputedRef<string>                 // "(35) 99812-4470"
  email: WritableComputedRef<string>
  windowAlertsEnabled: WritableComputedRef<boolean>                  // toggle "Alertas de janela no WhatsApp" (padrão true)
  isDocumentLocked: ComputedRef<boolean>             // edição
  labels: ComputedRef<{ name: string, document: string }>  // "Nome completo"/"Razão social", "CPF"/"CNPJ"
  fieldErrors: ComputedRef<Partial<Record<OwnerFormField, string>>>
  documentHint: ComputedRef<string | null>           // "CPF válido" / "CNPJ válido" (decisão 3)
  formError: ComputedRef<string | null>              // erro da API sem campo; 409 vai para o campo document
  touch(field: OwnerFormField): void
  pendingDocumentNotice: ComputedRef<{ title: string, description: string } | null>  // edição com hasPendingDocuments
  // vínculos (create/edit)
  links: ComputedRef<readonly OwnerLinkRowView[]>
  toggleFarm(farmId: string, selected: boolean): void
  setShare(farmId: string, share: string): void      // marca a participação como tocada
  touchShare(farmId: string): void                   // no blur; o erro de participação só aparece depois de tocar ou enviar
  setRole(farmId: string, role: LinkRole): void
  linksSummary: ComputedRef<string>                  // "Vínculos: Sítio Boa Esperança · 50%" | "Sem vínculos"
  // assistente
  wizardShare: Ref<string>
  wizardShareContext: ComputedRef<{ currentLabel: string, hint: string | null } | null>
    // "Hoje: Bruno 60% · Marina 40%" / "Com 20% para Helena, ajuste os demais para fechar 100%."
  // navegação e envio
  goToLinks(): boolean                               // valida Dados; false mantém no passo
  backToData(): void
  submit(): Promise<OwnerFormResult | null>
  loading: ComputedRef<boolean>                      // edição: carregando owner e vínculos
  linksLoading: ComputedRef<boolean>                 // lista de fazendas da etapa Vínculos
  loadError: ComputedRef<ApiError | null>
  saving: ComputedRef<boolean>
}

type OwnerFormResult =
  | { mode: 'create' | 'edit', owner: Owner, links: readonly OwnerFarmLink[] }
  | { mode: 'wizard', owner: Owner, sharePercent: number }
```

## Comportamento

- **Create:** `submit()` valida os dois passos, chama `createOwner` e depois `replaceLinks`. Se o vínculo falhar depois de o proprietário ter sido criado, o form guarda o `id`, e o próximo `submit()` vira `updateOwner` + `replaceLinks`, sem duplicar (evita 409). No sucesso, chama `useFlash().show(ownerSavedFlash(owner, links, 'created'))`. A página fecha o modal (`closeModal`) e chama `takeFlash()` da lista.
- **Edit:** carrega `useOwner(ownerId)`, preenche os campos e chama `updateOwner` (com o documento original) + `replaceLinks`. O flash usa `'updated'`. "Editar vínculos" chama `goToLinks()`.
- **Wizard:** só passo Dados mais o campo `wizardShare`. Chama apenas `createOwner`, **sem flash e sem `replaceLinks`**: o vínculo é enviado depois pelo assistente em `POST /farms` (`owners[]`). Retorna `{ owner, sharePercent }`.
- **Soma no front, só para exibir:** `totalShare` alimenta a barra. Total acima de 100% bloqueia o envio com erro na linha; total abaixo de 100% é só aviso. A regra final é da API (422 com `links[i].share_percent`, mapeado para a linha).
- **Fazendas da etapa Vínculos:** `useFarms({ pageSize: 100 })`; `othersShare` vem de `FarmSummary.owners`, excluindo o próprio proprietário.
- **Mensagens em pt-BR:**
  - Nome: "Informe o nome completo." / "Informe a razão social."
  - Documento: "Informe o CPF." / "CPF inválido." (o mesmo para CNPJ)
  - Nascimento: "Data inválida." / "A data não pode estar no futuro."
  - WhatsApp: "Telefone inválido." / "Informe o WhatsApp para receber os alertas."
  - E-mail: "E-mail inválido."
  - Participação: "Informe a participação." / "Use um valor entre 0 e 100." / "A participação total da fazenda passaria de 100%."
