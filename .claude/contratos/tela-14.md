# Contrato — Tela 14: Empresas (super admin)

Figma `MeWDfQIHok0s80GbqCKGV3`: tela `52:851`, conteúdo `52:925`. Rota `/companies`, `definePageMeta({ permission: 'companies.manage' })`.
Dono: trilha F (`useCompan*`, `utils/company.ts`). Endpoints (planned, ADR 0013): `GET /companies` (busca, status, página), `GET /companies/summary`.

## Decisões pendentes (usuário)

1. **"Nova empresa" e o menu "…" da linha** não têm frame no Figma. Proposta: botão visível só com `companies.manage`, abrindo "Em breve" até haver design; o contrato já tem `POST /companies` e `PATCH /companies/{id}` (editar, plano, ativar/inativar) para quando o modal existir.
2. **Busca**: o frame não tem campo de busca próprio (só a busca global da topbar). Proposta: campo "Buscar empresa ou CNPJ" acima da tabela, ligado a `?q=` (o endpoint já aceita `search`). Pode ficar oculto se o usuário preferir seguir o Figma à risca.
3. **Variação "+18% vs. outubro"**: calculada no front a partir de `applicationsThisMonth` e `applicationsPreviousMonth` do resumo (apresentação). Mês anterior = mês do relógio do usuário − 1. Sem mês anterior (0), mostra "Primeiro mês com aplicações".
4. A empresa operadora (Agentes Agro) não aparece na lista, porque o mock lista só clientes. Confirmar que a API fará o mesmo.

## Composable `useCompanyListPage(options?)`

```ts
export interface UseCompanyListPageOptions { debounceMs?: number; now?: () => Date }

export function useCompanyListPage(options?): {
  // Indicadores (GET /companies/summary)
  statCards: ComputedRef<CompanyStatCardView[]>   // 4 cartões, "—" enquanto carrega ou com erro
  summaryError: ComputedRef<ApiError | null>

  // Tabela (GET /companies)
  search: Ref<string>                    // ?q= (debounce 300 ms; volta à página 1)
  rows: ComputedRef<CompanyRowView[]>
  pending: ComputedRef<boolean>          // skeleton das linhas
  error: ComputedRef<ApiError | null>    // erro honesto + refresh()
  isForbidden: ComputedRef<boolean>      // 403 em qualquer das duas chamadas → ForbiddenState
  isEmpty: ComputedRef<boolean>          // sem empresas e sem busca
  isEmptySearch: ComputedRef<boolean>    // busca sem resultado ("Nenhuma empresa para “x”")
  clearSearch(): void
  refresh(): Promise<void>

  // Paginação (?page=&size=, replace)
  pagination: ComputedRef<CompanyPaginationView>
  page: WritableComputedRef<number>
  pageSize: WritableComputedRef<number>  // 10 | 20 | 50 ("10 por página")
  pageSizeOptions: readonly { value: number, label: string }[]

  canCreate: ComputedRef<boolean>        // can('companies.manage')
}

export interface CompanyStatCardView {
  key: 'companies' | 'farms' | 'users' | 'applications'
  label: string        // "Empresas ativas" | "Fazendas no sistema" | "Usuários" | "Aplicações no mês"
  value: string        // "3" | "7" | "9" | "31"
  caption: string | null // "1 em avaliação" | "1.284 ha monitorados" | "1 convite pendente" | "+18% vs. outubro"
  highlight: boolean   // só "Aplicações no mês"
}

export interface CompanyRowView {
  id: string
  name: string               // "Fazenda Santa Rita"
  initials: string           // "SR"
  locationLabel: string      // "Guaxupé · MG"
  documentLabel: string      // "12.345.678/0001-90"
  plan: { value: CompanyPlan, label: string }      // "Pro" | "Básico" | "Avaliação"
  farmCountLabel: string     // "2"
  userCountLabel: string     // "4"
  status: { value: CompanyStatus, label: string, tone: 'success' | 'warning' | 'neutral' }
                             // "Ativa" | "Em avaliação · 9 dias" | "Inativa"
}

export interface CompanyPaginationView {
  rangeLabel: string   // "Mostrando 1–4 de 4" ("Nenhuma empresa" com total 0)
  page: number
  pageCount: number
  canPrevious: boolean
  canNext: boolean
}
```

### Comportamento
- Filtros na query (`?q=&page=&size=`, `replace`); mudar busca ou tamanho volta à página 1; `?page=` além da última vai para a última.
- 403 (`forbidden`) → `isForbidden` (a rota já é protegida pelo middleware; isto cobre sessão desatualizada).
- "Em avaliação · N dias": dias até `trialEndsAt` (arredondado para cima, mínimo 0, "· último dia" quando 0).
- Funções puras em `app/utils/company.ts`.
