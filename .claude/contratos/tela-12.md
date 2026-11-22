# Contrato — Tela 12: Usuários (Figma 52:215 / conteúdo 52:289)

Rota: `pages/users/index.vue` (`definePageMeta({ permission: 'users.manage' })`, já existe) → `useUserListPage()`.
Lógica: `app/composables/{useUsers,useUserActions,useUserListPage}.ts` e `app/utils/userRow.ts` (logic-implementer, trilha E). O modal da tela 13 está em `tela-13.md`.

## Decisões pendentes (padrão assumido entre parênteses)

1. **"Exportar"**: não há endpoint. (Assumido: desabilitado "Em breve", como na tela 3.)
2. **Administrador (sem `users.manage_all`)**: só vê a própria empresa e nunca vê super admins. (Assumido: some o chip "Super admin" e o cartão de papel "Super admin"; o sobretítulo vira "Administração · 4 usuários · Fazenda Santa Rita".)
3. **Contadores nos chips**: o Figma esconde. (Assumido: o view model traz `count`, vindo de `GET /users/summary`, e a UI decide se mostra.)
4. **Último acesso**: "agora" (< 1 min), "há N min" (< 1 h), "há N h" (< 3 h), "hoje, 06:48", "ontem", "há N dias" (< 30), depois "dd/mm/aaaa"; sem acesso, "—". O fuso é fixo em America/Sao_Paulo para o SSR e o cliente renderizarem o mesmo texto. (Assumido; o Figma mistura "há 2 h" e "hoje, 06:48", e os 3 h de corte são escolha minha.)
5. **Ações do "…"** (dependem do status; a API decide e responde 403):
   - Ativo: "Editar papel e acesso" e "Inativar".
   - Inativo: "Editar papel e acesso" e "Reativar".
   - Convite pendente: "Reenviar convite" e "Cancelar convite".
   - A própria linha: "Inativar" e a troca de papel ficam desabilitadas ("Você não pode alterar a própria conta"; o mock responde 403).
   - "Inativar" e "Cancelar convite" pedem confirmação.
6. **Textos dos cartões de papel**: são copy de apresentação, iguais ao Figma, e não regra. As capacidades reais vêm da sessão/API.

## Endpoints (planned, ADR 0010/0013)

`GET /users` (busca, papel, status, página), `GET /users/summary`, `PATCH /users/{userId}` (status), `POST /invitations/{id}/resend`, `DELETE /invitations/{id}`.

## Assinatura

```ts
// app/utils/userRow.ts
export type UserListFilter = 'all' | 'super_admin' | 'admin' | 'planning_operator' | 'invited'
export type UserRowActionKind = 'edit_access' | 'deactivate' | 'activate' | 'resend_invitation' | 'cancel_invitation'
export type RoleTone = 'cafe' | 'green' | 'neutral'                 // Etiqueta: Café | Verde | Neutro
export type UserStatusTone = 'suitable' | 'attention' | 'muted'     // ponto verde | âmbar | cinza

export interface UserRowAction {
  readonly kind: UserRowActionKind
  readonly label: string                 // "Editar papel e acesso", "Inativar", "Reativar", "Reenviar convite", "Cancelar convite"
  readonly danger: boolean               // Inativar, Cancelar convite
  readonly disabledHint: string | null   // não nulo = item desabilitado com dica
  readonly confirm: { title: string, description: string, confirmLabel: string } | null
}

export interface UserRowView {
  readonly id: string
  readonly name: string
  readonly email: string
  readonly initials: string              // primeiro + último nome: "Ana Lúcia Prado" → "AP" (BaseAvatar com seed = id)
  readonly roleLabel: string             // "Super admin" | "Administrador" | "Operador de planejamento"
  readonly roleTone: RoleTone
  readonly companyName: string
  readonly lastAccessLabel: string       // decisão 4
  readonly status: { value: UserStatus, label: string, tone: UserStatusTone } // "Ativo" | "Convite pendente" | "Inativo"
  readonly isSelf: boolean
  readonly highlighted: boolean          // linha recém-salva (flash)
  readonly actions: readonly UserRowAction[]
}

export interface RoleCardView {
  readonly role: UserRole
  readonly label: string; readonly tone: RoleTone
  readonly countLabel: string            // "1 usuário" | "3 usuários" | "—" (1ª carga/erro)
  readonly description: string
  readonly capabilities: readonly string[] // 3 itens com check
}

// app/composables/useUserListPage.ts
export const USER_PAGE_SIZES: readonly number[] = [10, 20, 50]
export function useUserListPage(options?: { debounceMs?: number, now?: () => Date }): {
  eyebrow: ComputedRef<string | null>            // "Administração · 9 usuários em 5 empresas"; null na 1ª carga/erro
  roleCards: ComputedRef<readonly RoleCardView[]>
  search: Ref<string>                            // ?q= após debounce
  filter: WritableComputedRef<UserListFilter>    // ?filter=; volta para a página 1
  filterChips: ComputedRef<readonly { value: UserListFilter, label: string, count: number | null }[]>
  page: WritableComputedRef<number>              // ?page=
  pageSize: WritableComputedRef<number>          // ?size= (10 | 20 | 50), "10 por página"
  pageSizeOptions: readonly { value: number, label: string }[]
  rows: ComputedRef<readonly UserRowView[]>
  pagination: ComputedRef<{ rangeLabel: string, page: number, pageCount: number } | null> // "Mostrando 1–6 de 9"
  pending, error, isEmpty, isFilterEmpty, refresh
  canInvite: ComputedRef<boolean>                // can('users.manage')
  exportAction: { disabled: true, hint: 'Em breve' }

  // Ações do menu "…"
  runAction: (row: UserRowView, kind: UserRowActionKind) => Promise<void> // a UI confirma antes quando `confirm` não é nulo
  actionPending: ComputedRef<string | null>      // id da linha em ação
  notice: ComputedRef<FlashMessage | null>       // toast: sucesso ou erro honesto (403 → "Você não tem permissão para essa ação.")
  dismissNotice: () => void

  // Modal da tela 13 (query)
  modal: ComputedRef<{ mode: 'invite' } | { mode: 'edit', user: UserAccount | null } | null> // ?invite=1 | ?user=<id>
  openInvite: () => void
  closeModal: () => void
  /** Chame no mount e quando o modal fechar: consome o flash, destaca a linha e recarrega a lista e o resumo. */
  takeFlash: () => FlashMessage | null
}
```

- `edit_access` não chama a API: abre `?user=<id>` (tela 13 em modo edição). Se o usuário não estiver na página atual, `modal.user` vem `null` e a UI mostra "Usuário não encontrado nesta página".
- `runAction` atualiza a lista e o resumo quando dá certo.

## Estados

Carregando (esqueleto, cartões com "—"), erro (alerta + `refresh`), vazio (`isEmpty`: nenhum usuário além dos filtros; CTA "Convidar usuário"), filtro sem resultado (`isFilterEmpty`) e sem permissão (a rota já barra; um 403 vira `error.code === 'forbidden'` → `ForbiddenState`).
