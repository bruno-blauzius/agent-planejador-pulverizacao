# Contrato — Tela 13: Convidar usuário / Editar acesso (modal, Figma 52:758)

Aberto pela tela 12 (`?invite=1`, ou `?user=<id>` para editar o acesso). O conteúdo do modal → `useUserAccessForm({ user })`, em `app/composables/useUserAccessForm.ts` + `app/utils/userAccessForm.ts`.

## Decisões pendentes (padrão assumido entre parênteses)

1. **Papéis oferecidos**: "Super admin" só aparece com `users.manage_all`. O administrador vê "Administrador" e "Operador de planejamento". (Assumido: o cartão de super admin some, em vez de aparecer desabilitado.)
2. **Empresa**:
   - Com `companies.manage` (super admin): a lista é a empresa do próprio usuário (operadora) + `GET /companies`.
   - Sem `companies.manage`: só a empresa da sessão, com o select desabilitado.
   - Com o papel "Super admin", a empresa fica travada na empresa do usuário atual (operadora). (Assumido; a API valida.)
3. **Fazendas liberadas**: só valem para "Operador de planejamento". Nos outros papéis o select fica desabilitado em "Todas da empresa". Para o operador: "Todas da empresa" ou uma seleção de fazendas da empresa escolhida (multisseleção; o Figma mostra um select simples). As fazendas vêm de `GET /farms` (até 100) filtradas pela empresa.
4. **Edição de acesso**: o Figma só tem o convite. (Assumido: o mesmo modal, com título "Editar acesso", nome e e-mail somente leitura e o botão "Salvar acesso". Só os campos alterados vão no `PATCH`. Na própria conta, o papel fica desabilitado.)
5. **E-mail já cadastrado (409)**: erro no campo e-mail, "Este e-mail já tem cadastro."

## Endpoints (planned)

`POST /invitations`, `PATCH /users/{userId}`, `GET /companies` (só com `companies.manage`), `GET /farms`.

## Assinatura

```ts
// app/utils/userAccessForm.ts
export type FarmScopeMode = 'all' | 'selected'
export type UserAccessField = 'name' | 'email' | 'role' | 'companyId' | 'farmIds'
export interface UserAccessValues {
  name: string; email: string
  role: UserRole | null
  companyId: string | null
  farmScopeMode: FarmScopeMode
  farmIds: string[]
}

// app/composables/useUserAccessForm.ts
export function useUserAccessForm(options?: {
  /** Conta em edição; omitido ou null = convite. */
  user?: MaybeRefOrGetter<UserAccount | null | undefined>
}): {
  mode: ComputedRef<'invite' | 'edit'>
  texts: ComputedRef<{ eyebrow: 'USUÁRIOS', title: string, description: string, submitLabel: string }>
  //   convite: "Convidar usuário" / "A pessoa recebe um e-mail para criar a senha. O papel define o que ela pode ver e fazer." / "Enviar convite"
  //   edição:  "Editar acesso" / "O papel define o que <nome> pode ver e fazer." / "Salvar acesso"
  values: UserAccessValues                 // reactive, v-model
  identityReadonly: ComputedRef<boolean>   // edição: nome e e-mail somente leitura
  roleOptions: ComputedRef<readonly OptionCardItem<UserRole>[]> // { value, title, description, icon: 'shield'|'users'|'sprout', unavailableHint? }
  companyOptions: ComputedRef<readonly SelectItem<string>[]>
  companyLocked: ComputedRef<boolean>      // select desabilitado
  scopeOptions: ComputedRef<readonly SelectItem<FarmScopeMode>[]> // "Todas da empresa" | "Fazendas selecionadas"
  scopeLocked: ComputedRef<boolean>        // papel diferente de operador
  farmOptions: ComputedRef<readonly SelectItem<string>[]>         // fazendas da empresa escolhida (mode 'selected')
  fieldErrors: ComputedRef<Partial<Record<UserAccessField, string>>>
  formError: ComputedRef<string | null>    // 403 → "Você não tem permissão para dar esse papel ou acesso a essa empresa."; rede; genérico
  touch: (field: UserAccessField) => void
  submit: () => Promise<boolean>           // sucesso → flash { title: 'Convite enviado' | 'Acesso atualizado', description, highlightId } → a página fecha o modal e chama takeFlash()
  saving: ComputedRef<boolean>
  pending: ComputedRef<boolean>            // empresas/fazendas carregando
  loadError: ComputedRef<ApiError | null>
}
```

- Validação local: nome obrigatório; e-mail com formato válido; papel; empresa; com "Fazendas selecionadas", ao menos uma fazenda. Os erros aparecem após o blur (`touch`) ou o envio.
- Os `details` da API vão para os campos: `company_id` → companyId, `farm_scope.farm_ids` → farmIds.
- Ao trocar de empresa, as fazendas selecionadas são limpas. Ao trocar para um papel que não é operador, o escopo volta para "Todas da empresa".
- A UI fecha o modal pela página (`closeModal`) depois de `submit()` resolver `true`.
