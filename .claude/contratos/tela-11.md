# Contrato — Tela 11: Perfil do usuário

Figma `MeWDfQIHok0s80GbqCKGV3`: tela `54:815`, conteúdo `54:882`. Rota `/profile` (qualquer papel autenticado; sem `permission`).
Dono: trilha F (`useProfile*`, `utils/profile.ts`). Endpoints (planned, ADR 0013): `GET /me/profile`, `PATCH /me/profile` (só nome), `POST /me/password`.

## Decisões pendentes (usuário)

1. **Foto do perfil** ("Trocar foto" / "Remover" e o selo de upload no avatar): **não há endpoint**. Proposta: botões desabilitados com "Em breve"; o avatar mostra as iniciais.
2. **Rótulo do papel com gênero** ("Administradora" no Figma): o front não sabe o gênero. Proposta: rótulo neutro de `USER_ROLE_LABELS` ("Administrador", "Super admin", "Operador de planejamento").
3. **Um botão salva tudo** ("Salvar alterações"): grava o nome se mudou e troca a senha se algum campo de senha foi preenchido, nessa ordem. Se o nome salvar e a senha falhar, o aviso diz que o nome foi salvo e a senha mostra os erros. Confirmar.
4. **Força da senha**: só apresentação (mínimo de 8, letras e números, como diz o Figma e o contrato). A API decide (422 `new_password`).

## Composable `useProfilePage(options?)`

```ts
export interface UseProfilePageOptions { now?: () => Date }

export function useProfilePage(options?): {
  // carga (GET /me/profile, useAsyncData 'profile:me')
  pending: ComputedRef<boolean>          // skeleton
  error: ComputedRef<ApiError | null>    // erro honesto + "Tentar novamente"
  refresh(): Promise<void>

  identity: ComputedRef<ProfileIdentityView | null>   // cartão da esquerda
  photo: { readonly isAvailable: false, readonly hint: 'Em breve' }

  // Dados pessoais
  name: Ref<string>                      // v-model "Nome completo"
  email: ComputedRef<string>             // campo desabilitado

  // Alterar senha
  currentPassword: Ref<string>
  newPassword: Ref<string>
  confirmPassword: Ref<string>
  passwordRules: ComputedRef<PasswordRuleView[]>  // checklist de apresentação
  passwordHint: string                   // "Mínimo de 8 caracteres, com letras e números."
  passwordChangedLabel: ComputedRef<string | null> // "Última alteração: há 3 meses"; null se nunca

  // Validação (aparece após touch ou ao salvar; 422 da API por campo)
  fieldErrors: ComputedRef<ProfileFieldErrors>   // name | currentPassword | newPassword | confirmPassword
  touch(field: ProfileField): void

  // Ações
  isDirty: ComputedRef<boolean>          // habilita "Salvar alterações" e "Descartar"
  saving: Readonly<Ref<boolean>>
  save(): Promise<boolean>
  discard(): void                        // volta ao perfil carregado e limpa senhas/erros
  notice: Readonly<Ref<ProfileNotice | null>>   // toast na própria tela
  dismissNotice(): void
  formError: ComputedRef<string | null>  // falha geral (rede, 500…)
}

export type ProfileField = 'name' | 'currentPassword' | 'newPassword' | 'confirmPassword'
export type ProfileFieldErrors = Partial<Record<ProfileField, string>>

export interface ProfileIdentityView {
  name: string                // "Marina Rezende"
  initials: string            // "MR"
  email: string
  roleLabel: string           // "Administrador"
  companyName: string         // "Fazenda Santa Rita"
  managedTitle: string        // "Gerenciado pelo administrador"
  managedText: string         // "E-mail, papel e empresa só podem ser alterados por um administrador."
}

export interface PasswordRuleView { key: 'length' | 'letter' | 'number', label: string, met: boolean }

export interface ProfileNotice { kind: 'success' | 'error', title: string, description: string | null }
```

### Comportamento
- `name` começa com o nome carregado. Erros locais: nome vazio; senha atual obrigatória quando há nova senha; nova senha fora das regras; confirmação diferente. Seção de senha vazia não é validada nem enviada.
- `save()`: valida; `PATCH /me/profile` se o nome mudou (e atualiza a sessão, para a barra lateral mostrar o nome novo); `POST /me/password` se houver senha. 422 → `fieldErrors` (`name`, `current_password` → `currentPassword`, `new_password` → `newPassword`). Sucesso: aviso "Perfil atualizado" / "Senha alterada" e campos de senha limpos.
- Sem 403: todo papel acessa o próprio perfil. 401 segue o fluxo da sessão.
- Funções puras em `app/utils/profile.ts`.
