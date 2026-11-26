# Progresso — Telas do Figma

- Arquivo: `MeWDfQIHok0s80GbqCKGV3` — página `1:4` (01 · Acesso e Cadastros)
- Outras páginas do arquivo: `0:1` (00 · Fundamentos — tokens), `1:5` (02 · Planejador de Pulverização — fora do escopo até confirmação)
- Etapa atual: **Concluída — 10/10 telas aprovadas; verificação final**

## Decisões (2026-11-21)

- API sem endpoints → **contrato primeiro + mocks**: endpoints descritos em `services/api/api/openapi.yaml`; o front usa portas em `app/services/` com implementação HTTP e implementação mock em memória, escolhida por `runtimeConfig.public.apiMock` em um plugin (raiz de composição). ADR proposto.
- Padrões assumidos (o usuário pode corrigir): 17:113 ignorado (versão antiga); página 1:5 fora do escopo; tokens da página 0:1 usados na Fundação.
- Mapa: usuário decidiu antecipar a biblioteca (tela 2). ADR `docs/adr/0012-biblioteca-de-mapa-no-front.md` (MapLibre GL + Terra Draw) **aceita**.
- Tela 2: created_at no FarmSummary; "Janelas aptas hoje" = "—" + "Disponível com o planejador"; perímetro + forecast_point na lista; menu do cartão só "Abrir"; toast neutro "Área de N ha cadastrada."
- Tela 3: "Part." = % em cada chip de fazenda; coluna mostra valor único/igual ou "Varia"; "Exportar" desabilitado "Em breve"; destaque só na linha recém-salva (flash com highlightId); modal da tela 4 via ?owner=new|<id>[&step=links].
- Tela 9: cores/legenda por variedade (Figma); `PUT /farms/{id}/fields/{fieldId}` adicionado ao contrato (planned) e "Editar" → /farms/{id}/fields/{fieldId}/edit (formulário da tela 10); "Nova área", "Planejar aplicação", "Apta hoje" e demais abas desabilitados sem dado inventado.
- Tela 9 (2026-11-23): abas "Visão geral" e "Proprietários" liberadas sem frame no Figma (`FarmOverviewPanel` no layout da revisão do assistente; `FarmOwnersPanel` com vínculos que abrem o modal do proprietário). Aba na URL (`?tab=`). "Aplicações" e "Janelas de pulverização" seguem desabilitadas.
- Tela 9: áreas em ordem alfabética natural (API sem campo de ordem; Figma mostra Norte primeiro); not_found preservado no SSR via ApiError.data.
- Tela 4: upload de documento desabilitado ("Envio de documentos em breve") + aviso de pendência; "Remover proprietário" oculto; "CPF/CNPJ válido" (sem consulta à Receita); papel padrão owner; documento bloqueado na edição.
- Assistente (telas 5–8): rascunho local (useState + localStorage, limpo no logout); foto/logo e "Usar o CAR" desabilitados; prévia mostra tipo de produção no lugar de "cultura principal"; área desenhada = prévia aproximada (@turf/area, ADR 0012) na tela 6; passo 2 exige município, UF e perímetro (sede opcional).
- Tela 10: estande (plantas/ha) "calculado ao salvar" (API); área de prévia rotulada como aproximada (ADR 0012); perímetro só avisa (checagem de vértices), sem bloquear; "+ Outra" abre o catálogo de culturas; safra null no cadastro; espaçamento opcional em campo único. Deps: terra-draw 1.40.0, terra-draw-maplibre-gl-adapter 1.5.0, @turf/area 7.4.0.
- Tela 6: município/UF digitados (sem geocodificação; `GET /geo/reverse` fica como pendência futura); resposta de `POST /perimeter-imports` ampliada (município, datum, talhões no arquivo — opcionais); limite de arquivo 20 MB (Figma); aviso (não bloqueia) se a sede cair fora do perímetro; "Usar minha localização" via navigator.geolocation.
- Tela 7: participações NÃO são reequilibradas ao vincular; total ≠ 100% mostra erro e bloqueia; todos entram como proprietário (sem seletor de papel); busca carrega a 1ª página ao abrir.

- Relevo dos talhões (2026-11-23): frames novos desenhados no Figma e aprovados pelo usuário, na seção `67:960` da página `1:4`:
  - 04b `67:961`: aba Relevo;
  - 04c `67:1276`: detalhe do talhão;
  - 04d `71:1086`: modal de ajuste manual;
  - 04e `67:1906`: ainda não calculado e progresso.

  Componentes locais: `Relevo · Selo de classe` (`67:2239`) e `Relevo · Linha do talhão` (`67:2240`). ADR 0021 aceito e implementado (2026-11-24; backend Go, migrações 000019-000021 e front com API real; o mock `terrainMock.ts` só no modo `NUXT_PUBLIC_API_MOCK=true`; `NUXT_PUBLIC_API_MOCK_PENDING=true` já usa a API real no relevo; conformidade do contrato conferida campo a campo, sem divergência de forma). Contrato (`getFarmTerrain`, `calculateFarmTerrain`, `getFieldTerrain`, `adjustFieldTerrain`, `removeFieldTerrainAdjustment`). Front implementado em 2026-11-24 (contrato em `.claude/contratos/tela-21.md`, aba `?tab=terrain`, mock em `terrainMock.ts`/`terrainSeed.ts`, 857 testes passando): revisão APROVADO COM RESSALVAS, os 2 🟡 (corrida de respostas e alerta de erro preso) foram corrigidos. A fidelidade visual contra o Figma não foi conferida pelo revisor (MCP sem autenticação no ambiente dele); a carga do bloco real do Copernicus segue pendente. Permissão de escrita: `registry.write` (a nota do Figma que sugeria `terrain.manage` foi superada pelo ADR).

- Formulários com dados sensíveis (senha, CPF/CNPJ, contato) usam `<form method="post" novalidate @submit.prevent>`: um envio antes da hidratação nunca coloca os dados na URL. Só a busca global (`AppTopbar`) usa GET.

## Telas

| # | Tela | node-id | Depende de | Status | Veredito | Observações |
|---|---|---|---|---|---|---|
| 1 | Login | 6:16 | — | ✅ aprovada | APROVADO (2 ciclos + ajuste method=post verificado pelo orquestrador) | Hero + formulário; layout sem barra lateral |
| 2 | Fazendas (lista) | 7:16, 35:1401 | Fundação | ✅ aprovada | APROVADO (ciclo 2) | Grade de cartões com mapa/selo; alterna grade/lista/mapa; estado "após salvar" com toast |
| 3 | Proprietários (lista) | 39:539, 39:1426 | Fundação | ✅ aprovada | APROVADO (ciclo 2) | Tabela; estado "após salvar" com alerta. 17:113 parece versão antiga (confirmar) |
| 4 | Proprietário — modal novo/editar | 39:819, 39:1134, 39:1667 | 3 | ✅ aprovada | APROVADO (ciclo 2) | Modal com abas Dados/Vínculos (segmento PF/PJ, upload); edição reaproveita |
| 5 | Nova fazenda — passo 1 (casca do assistente) | 35:355 | 2 | ✅ aprovada | APROVADO (ciclo 2) | Etapas + formulário + prévia ao vivo |
| 6 | Nova fazenda — passo 2 (localização) | 14:30, 36:472, 37:503 | 5 | ✅ aprovada | APROVADO (ciclo 2) | 3 modos: desenhar perímetro, marcar sede, importar arquivo |
| 7 | Nova fazenda — passo 3 (proprietário) | 35:696, 41:681, 41:914, 41:1176 | 5, 4 | ✅ aprovada | APROVADO (ciclo 2) | Busca, modal "novo proprietário nesta fazenda", vinculado |
| 8 | Nova fazenda — passo 4 (revisão/salvar) | 35:1048 | 5 | ✅ aprovada | APROVADO (ciclo 2) | Ao salvar, volta para Fazendas com toast |
| 9 | Fazenda — áreas e talhões | 15:56 | 2 | ✅ aprovada | APROVADO COM RESSALVAS 🟢 (ciclo 2) | Cabeçalho com selo, abas, lista de áreas + mapa com popover |
| 10 | Novo talhão (desenho + detalhes) | 16:82 | 9 | ✅ aprovada | APROVADO (2 ciclos + correção 🔴 de CSS e texto verificada pelo orquestrador) | Mapa com desenho + painel lateral de formulário |

## Componentes compartilhados (propostos)

- `AppSidebar` (logo, navegação, card de IA, usuário) e `AppTopbar` (busca, notificações, avatar) → layout `default`; layout `auth` para Login
- `PageHeader` (título, breadcrumb com chevron, selo, ações)
- `BaseButton`, `TextField`, `SegmentedControl`, `StatusBadge`, `TabsNav`
- `BaseModal` (sobreposição, cabeçalho com ícone, fechar, rodapé de ações)
- `ToastAlert` (sucesso/alerta)
- `FileDropzone` (upload em proprietário e importação de perímetro)
- `MapView` (miniatura, mapa da fazenda, desenho de polígono, marcador)
- `OwnerForm` (modal de proprietário em 4 e 7)
- `WizardSteps` + `WizardPreview` (passos 5–8)
- Ícones (home, map, layers, users, sprout, cal, sparkles, search, bell, chev, plus, x, upload, pen, check…)

## Dados compartilhados (propostos)

- Entidades: `Farm`, `Field` (talhão), `FieldArea`/agrupamento, `Owner` (+ vínculo `OwnerFarmLink`), `Crop`, sessão de usuário
- Endpoints necessários (**nenhum existe hoje em `services/api`** — só health/openapi/docs):
  - `POST /auth/login`, sessão
  - `GET/POST/PUT /farms`, `GET /farms/{id}`
  - `GET/POST /farms/{id}/fields`
  - `GET/POST/PUT /owners`, `GET/PUT /owners/{id}/links`
  - importação de perímetro (KML/GeoJSON/SHP)

## Componentes e composables criados (Fundação)

> **Lista oficial para os subagentes — reaproveitar, nunca recriar.**
> - Tema/Vuetify 4.3.4: `app/theme/{palette,vuetify,icons}.ts`, `app/plugins/vuetify.ts`, `assets/styles/vuetify-settings.scss`; cores só em `palette.ts`, CSS usa `--color-*`/`--space-*`/`--font-*`/`.text-*` (sem classes text-*/grid do Vuetify)
> - Layouts: `default` (AppSidebar `v-model:open` + AppTopbar), `auth` (AppAuthHero)
> - App: `AppIcon` (`IconName`), `AppLogo`, `AppSidebar`, `AppSidebarInsight`, `AppTopbar`, `AppTopbarChip`, `AppAuthHero`
> - Base (fachadas sobre Vuetify; telas usam estas, não o v-* equivalente): `BaseButton`, `TextField`, `SegmentedControl<T>`, `TabsNav<T>`, `StatusBadge`, `PageHeader`, `BaseModal`, `ToastAlert` (+`ToastAlertContent`; `floating`/`location`/`timeout`), `FileDropzone`, `MapView` (estático)
> - Vuetify sem fachada liberado nas telas: `v-data-table`, `v-autocomplete`, `v-menu`, `v-tooltip`, `v-date-picker` — vira fachada ao aparecer em 2+ telas
> - Lógica: plugin `api.ts` (`$api`, mock só em dev), composables `useApiServices`, `useSession`, `useAuthToken`, `useFarms`, `useFarm`, `useFarmActions`, `useFarmFields`, `useCrops`, `useOwners`, `useOwner`, `useOwnerActions`; utils `format*`, `initials`, `summarizeFarms`, `totalSharePercent`, `cropNames`, `groupFieldsByBlock`, `*_LABELS`
> - Testes: Vitest (63); `test/support/vuetify.ts` pronto, testes de componente exigem `@vitejs/plugin-vue` + `happy-dom` (não instalados)

### Lógica (logic-implementer — concluído, aguardando revisão)
- ADR `docs/adr/0010-contrato-primeiro-e-mocks-no-front.md`; `openapi.yaml` com 16 operações `x-status: planned` (+ `GET /crops`)
- `app/types/*`, `app/services/{ports,errors,http/*,mock/*}`, plugin `app/plugins/api.ts` (`$api`, mock via `NUXT_PUBLIC_API_MOCK=true`; login de teste no seed)
- Composables: `useApiServices`, `useSession`, `useFarms`, `useFarm`, `useFarmActions`, `useFarmFields`, `useCrops`, `useOwners`, `useOwner`, `useOwnerActions` (ações retornam resultado ou `null` + erro, não lançam)
- Utils: `format*` (ha, %, CPF/CNPJ, telefone, CEP, coordenadas, datas), `initials`, `summarizeFarms`, `totalSharePercent`, `cropNames`, `groupFieldsByBlock`, `*_LABELS`
- Sem dependências novas (sem Pinia: `useState`). Typecheck ok, 63 testes ok.
- A confirmar: `public.apiBase` substitui `apiBaseUrl` no browser; token em cookie não-httpOnly.

- Revisão ciclo 1 (REPROVADO, 3🟡): lógica corrigida — mock só via `import()` em `import.meta.dev` (fora do bundle de produção), cookie com `expires` = `expires_at` + adendo na ADR 0010, rota `/perimeter-imports`. Testes 63/63, go test ok.
- Vuetify: usuário escolheu adotar. ADR `docs/adr/0011-vuetify-como-biblioteca-de-componentes-do-front.md` **aceita** — migração da fundação em andamento (fachadas `Base*`/`App*` mantêm API).

### UI (figma-ui-implementer — concluído, aguardando revisão)
- Tokens: `app/assets/css/{tokens,typography,base,main}.css` (CSS vars, sem Tailwind); fontes Bricolage Grotesque + Plus Jakarta Sans (Google Fonts)
- Layouts: `default` (AppSidebar + AppTopbar), `auth` (AppAuthHero)
- `components/app/`: `AppIcon` (47 ícones, `IconName`), `AppLogo`, `AppSidebar` (+ `AppSidebarInsight`), `AppTopbar` (+ `AppTopbarChip`), `AppAuthHero`
- Base: `BaseButton`, `TextField`, `SegmentedControl<T>`, `StatusBadge` (suitable/attention/unsuitable/rain), `TabsNav<T>`, `PageHeader`, `BaseModal`, `ToastAlert`, `FileDropzone`, `MapView` (estático)
- Rotas da sidebar: `/farms`, `/fields`, `/owners`, `/planner`, `/applications`; `pages/index.vue` e `pages/login.vue` são temporárias
- Sem dependências novas. Lint e typecheck ok.
- Revisão ciclo 1: UI corrigida — tipografia como tokens `--font-*` + classes `.text-*`, novos tokens de espaço/raio/tamanho/layout, sem px soltos nos `.vue`; fallback de foco e function refs em SegmentedControl/TabsNav. Exceções justificadas: media query 1023px (auth), geometria do hero.

## Divergências de design

Migração da fundação para Vuetify 4.3.4 (ADR 0011), verificada a 1440 px:

- **ToastAlert `floating`:** o `v-snackbar` é a live region (o `v-alert` interno fica sem role); no tom `error` o conteúdo do snackbar recebe `role=alert`/`aria-live=assertive`. Agora é um `v-snackbar` fixo no canto superior direito (antes ficava no fluxo da página). É o comportamento de toast do Figma (35:1728), mas muda a posição na vitrine `/`. Novas props: `location` e `timeout`.
- **TabsNav:** o Vuetify só ativa a aba com Enter/Espaço; as setas apenas movem o foco. Antes as setas também selecionavam.
- **AppTopbar (`v-app-bar absolute`) e AppSidebar (`v-navigation-drawer`):** abaixo de 1024 px a sidebar vira gaveta temporária e a topbar ganha o botão "Abrir navegação". O scrim da gaveta mobile é o padrão do Vuetify, porque o Figma não tem layout mobile.
- **Ícones internos do Vuetify sem equivalente no Figma** (checkbox, radio, rating, ordenação) usam os paths do icon set `mdi-svg` do próprio Vuetify (`vuetify/iconsets/mdi-svg`, sem fonte MDI), espalhados em `app/theme/icons.ts` e sobrescritos pelos ícones do Figma onde há equivalente. As setas para esquerda e para cima são rotações dos chevrons do Figma.
- **`$border-radius-root`:** fica como número fixo (12px, espelhando `--radius-md`) porque o Sass faz contas com ele. A escala nomeada aponta para os tokens.
- **Hover dos botões:** o Figma não tem estado de hover; os tons usados são derivados dos tokens (como antes).

Tela 2 — Fazendas (lista):

- **Filtros** seguem o Figma (chips com contador, `FilterChips` sobre `v-chip-group`), não abas (`TabsNav`), embora o contrato traga `tabs`.
- **Miniaturas** são o mapa real (MapLibre, fundo escuro gerado de `palette.ts`) com o perímetro da fazenda; o Figma desenha talhões ilustrativos coloridos. A miniatura vira imagem após renderizar (libera o contexto WebGL); enquanto carrega, a ilustração do Figma aparece esmaecida.
- **Modo lista** não tem frame no Figma: `v-data-table` com os tokens do cartão (nome+município, área, talhões, culturas, proprietários, condição).
- **Modo mapa** não tem frame próprio: `MapView` 640 px com todos os perímetros coloridos por status e `MapControls` (zoom +/−, enquadrar; estilo do Figma 15:302); sem legenda.
- **"Janelas aptas hoje"** mostra "—" e "Disponível com o planejador" (sem dado na API). Saudação via `useGreeting` ("Olá, <nome>" no SSR; "Bom dia/Boa tarde/Boa noite" no cliente).
- Sidebar (card de IA e usuário) e chip de clima da topbar seguem ocultos: sem composable de dados ainda.
- Atribuição do mapa (OpenFreeMap/OpenMapTiles/OSM) sempre visível, inclusive nas miniaturas (exigência da ADR 0012).

Tela 3 — Proprietários (lista):

- **Chips de fazenda** mostram "Santa Rita · 60%" (contrato `farms[].text`); o Figma mostra só o nome. Com isso, várias linhas quebram em 2 linhas na coluna de 190 px.
- **"Exportar"** desabilitado com selo "Em breve". Paginação com botões anterior/próxima só quando há mais de uma página (o Figma mostra só os rótulos).
- **Menu "…"** (Editar, Vínculos) não tem frame no Figma: menu flutuante com os tokens. Clique na linha = Editar.
- **Tabela abaixo de 840 px** empilha cada linha (rótulo/valor) pelo modo mobile do `v-data-table`; o Figma não tem layout estreito.
- `DataTable` (fachada sobre `v-data-table`) e `EmptyState` criados ao aparecerem nas duas listas; a lista de fazendas passou a usá-los.

## Pendências de API

- Todos os endpoints acima estão ausentes.

## Pendências de verificação

- Mapa (MapLibre) no build de produção: worker emitido e referenciado, mas não exercitado em runtime (mock só em dev) — verificar quando a API real existir.
- 🟢 Mensagem de erro de rede real em `toApiError` deve ser amigável (no teste forçado apareceu a mensagem crua da exceção).
- Frame 35:1401 (toast após salvar) não comparado pixel a pixel.

## Execução paralela (pedido do usuário em 2026-11-21)

Três trilhas, cada uma dona dos seus arquivos. Só o orquestrador altera a tabela de status deste arquivo; subagentes reportam divergências ao orquestrador.

| Trilha | Telas (ordem) | Dona de |
|---|---|---|
| A | 3 → 4 | `pages/owners/**`, `components/owner/**` (inclui `OwnerForm`), `useOwner*`, `utils/ownerRow.ts` |
| B | 5 → 8 → 6 → 7 | `pages/farms/new*`, `components/wizard/**`, `useFarmWizard*`, `utils/wizard*` |
| C | 9 → 10 | `pages/farms/[id]/**`, `components/field/**`, **`components/map/**` e `theme/map-style.ts`** (inclui Terra Draw), `useFarmFields*`/`useFieldForm*` |

Dependências entre trilhas: B/6 usa o controle de desenho criado pela C; B/7 usa o `OwnerForm` criado pela A/4.
Arquivos compartilhados (`openapi.yaml`, `types/*`, `services/**`, mock `seed.ts`, `tokens.css`, `typography.css`, `palette.ts`, componentes `Base*`/`App*`): reler imediatamente antes de editar, mudança mínima e compatível, nunca reescrever o arquivo inteiro.
Contratos de tela em `.claude/contratos/tela-NN.md`.
Dev server único compartilhado em http://localhost:3002 (config `web-mock`); não subir outro. `pnpm build` só nas revisões.
- 🟢 Migrar "Em breve" do Login (Google) e de Proprietários ("Exportar") para `BaseButton unavailableHint` (padrão acessível criado na tela 9).
- ✅ AppTopbar a 390 px corrigida (busca recolhível); padding lateral 16 px abaixo de 600 px.
- `ReferenceError: document is not defined` no console em dev: artefato do HMR do Vite no worker do MapLibre; não ocorre em produção.
- 🟢 Nome longo do arquivo importado cortado sem `title` (PerimeterImportCard.vue:31).
- ✅ Verificação final (2026-11-21): lint, typecheck, 293/293 testes, `pnpm build` e `go test ./...` passam; senha do mock ausente do `.output`; worker do MapLibre emitido.

---

# Página 03 · Administração, Painel e Perfil (`52:214`)

- Pedido do usuário (2026-11-21): construir a página 03; página 02 continua fora do escopo.
- Etapa atual: **3 — Telas P3** (Fundação P3 aprovada; o 🟡 de permissões nas telas aprovadas é a trilha G)
- Decisões: ordem Perfil → Usuários → Convidar → Empresas → Painel; ADR 0013 (empresa como tenant, papéis com capacidades devolvidas pela API); Painel com contrato planned + mock (inclui dados do Planejador); Painel em `/` (substitui a vitrine), login redireciona conforme papel.

| # | Tela | node-id | Depende de | Status | Veredito | Observações |
|---|---|---|---|---|---|---|
| 11 | Perfil do usuário | 54:815 | Fundação P3 | ✅ aprovada | 🟢 (ressalvas só 🟢: tipagem errors em ProfilePersonal/PasswordSection, largura do help text, defaults duplicados em ProfileIdentityCard/PasswordSection, touched.delete redundante) | Identidade, dados pessoais, senha |
| 12 | Usuários (super admin) | 52:215 | Fundação P3 | ✅ aprovada | 🟢 (ciclo 2; ressalvas só 🟢: título duplicado e flag closed em UserAccessModal) | Tabela com chips de filtro e paginação |
| 13 | Convidar usuário — modal | 52:538 | 12 | ✅ aprovada | 🟢 (ciclo 2; ressalvas só 🟢: título duplicado e flag closed em UserAccessModal) | Campos, cartões de opção (papel), selects |
| 14 | Empresas (super admin) | 52:851 | Fundação P3 | ✅ aprovada | 🟢 (ciclo 2, sem achados novos; não verificado: visão de operador, 1440px dedicado, erros de rede só por testes) | 4 cartões de indicador + tabela paginada |
| 15 | Painel (administrador) | 54:535 | Fundação P3 | ✅ aprovada | 🟢 (ciclo 2, sem achados novos; não verificado: visão de operador, 1440px dedicado, erros de rede só por testes) | Indicadores, janelas × aplicações, próximas janelas, alertas do agente, atividade recente; seletor (fazenda/período?) |

## Fundação P3 (proposta)

- Papéis na sessão (super admin, administrador, …) e navegação por papel: sidebar ganha "Usuários" (admin e super admin) e "Empresas" (só super admin); guarda de rota por papel.
- Generalizar compartilhados existentes em vez de recriar: `FarmSummaryCard` → `StatCard` ("Cartão de indicador", telas 2, 14, 15); `OwnerAvatar` → `UserAvatar`; `PerimeterMethodOptions` → `OptionCard` ("Cartão de opção"); `BaseSelect` (fachada `v-select`, "Campo de seleção"); `FilterChips` e `DataTable` (paginação) já existem.
- Dados novos: `User`, `Company` (empresa/tenant), `Role`, `Invitation`, resumo do painel, perfil e troca de senha — nenhum endpoint existe; contrato `planned` (ADR 0010).

## Fundação P3 — criado
- Lógica: contrato planned (/auth/me com role/company/farm_scope/permissions; /users, /users/summary, /invitations, /companies, /companies/summary, /me/profile, /me/password, /dashboard; 403 forbidden/out_of_scope; FarmSummary.company_id); `usePermissions()` { can, permissions, role, company, farmScope, homePath, isFarmInScope }; `utils/permissions.ts`; middleware por `definePageMeta({ permission })`; login → `/` (dashboard.view) ou `/farms`. Capacidades: super_admin 8; admin dashboard.view, users.manage, registry.write, applications.register/confirm, planner.use; operador applications.register, planner.use (escopo de fazendas). Seed: 5 empresas, 9 usuários (credenciais de teste em `services/mock/seed.ts`).
- UI: AppSidebar seção Administração por `can`, rodapé com usuário → /profile; `BaseAvatar` (substitui OwnerAvatar), `StatCard` (substitui FarmSummaryCard), `OptionCard`/`OptionCardGroup`, `BaseSelect`, `ForbiddenState`; `/` = placeholder do Painel; vitrine em `/dev/components` (só dev).
- Lacunas: FarmInput sem company_id (super admin cria na operadora); dispensar alerta do agente; foto do perfil; aceite de convite.

## Execução paralela P3

| Trilha | Telas | Dona de |
|---|---|---|
| E | 12 → 13 | `pages/users/**`, `components/user/**`, `useUser*`/`useInvitation*`, `utils/user*` |
| F | 11 → 14 → 15 | `pages/profile*`, `pages/companies/**`, `pages/index.vue`, `components/{profile,company,dashboard}/**`, `useProfile*`/`useCompan*`/`useDashboard*` |
| G | permissões nas telas aprovadas + 🟢 da fundação P3 | `pages/farms/**`, `pages/owners/**`, rotas de escrita, composables de página existentes (`canWrite`), `PerimeterMethodOptions`, `BaseSelect` |

Mesmas regras de arquivos compartilhados, dev server único e sem `pnpm build` da seção anterior. Contratos em `.claude/contratos/tela-NN.md`.
- Telas 12/13: "Exportar" usuários desabilitado (sem endpoint); admin não vê chip/cartão "Super admin"; último acesso relativo ("há N h" até 3 h, depois "hoje, HH:mm"); edição de acesso reaproveita o modal de convite; fazendas liberadas em multisseleção; super admin com empresa travada na operadora. Modal de proprietário não abre sem registry.write (sem visualização só leitura por ora).
- Telas 11/14/15: foto do perfil "Em breve"; rótulo de papel neutro; "Nova empresa" desabilitado "Em breve" e menu "…" de empresa omitido (sem frame); operadora fora da lista de empresas; Painel com período fixo de 14 dias, alertas dispensáveis só na sessão, links para o Planejador "Em breve"; datas em America/Sao_Paulo.

## Registro P3 (ciclo final)
- Permissões nas telas aprovadas (ADR 0013): aprovado no ciclo 2 (403/out_of_scope na tela de Fazenda corrigido; useOwnerListPage usa isAuthenticated). 🟢: faltam testes de regressão de componentes (DataTable clickable, FieldMapPopover editable, NOTICE_TIMEOUT_MS), pois Vitest roda em node sem mount.
- Telas 12/13: divergências assumidas: avatar lg 40px (Proprietários mantém md 38); fazendas liberadas = lista de checkboxes (BaseSelect sem multiple; Figma só tem select simples); coluna Empresa mantida para admin (🟢 poderia ocultar).
- Telas 14/15: UI pronta, em revisão. Divergências: "Nova empresa" desabilitado "Em breve"; menu "…" omitido; busca não existe no Figma; "Abrir o Planejador" desabilitado; gráfico CSS sem lib; blocos em coluna única <1200px.
- Pendências transversais: sidebar em 375px aperta conteúdo (shell); sem tratamento global de 401; rota /planner inexistente.

---
# Página 02 · Planejador de Pulverização (nó 1:5) — inventário (2026-11-22, aguardando confirmação)

| # | Tela | node-id | Depende de | Status | Veredito | Observações |
|---|---|---|---|---|---|---|
| 16 | Planejador de pulverização (chat do agente + plano sugerido + mapa de janelas 5 dias + condições hora a hora) | 18:2 | Fundação P1/P3 | ✅ aprovada | 🟢 (revisão final ciclo 2) | Desktop 1440. Rota `/planner`. Contexto Fazenda/Talhões/Produto, conversa, "O agente consultou", plano por talhão, mapa de janelas, gráfico Delta T/vento |
| 17 | Confirmação humana antes de registrar (modal sobre a tela 16) | 19:8 | 16 | ✅ aprovada | 🟢 (revisão final ciclo 2) | Talhão, janela, produto, volume de calda, 6 checagens, operador/equipamento, checkbox "Revisei…" |
| 18 | Mobile · Operador (Hoje) | 20:18 | 16 | ✅ aprovada | 🟢 (revisão final ciclo 2) | 390x844. "Pode pulverizar", Delta T/vento/chuva, linha do dia, tarefa de hoje, "Iniciar aplicação", abas Hoje/Mapa/Aplicações/Perfil |

Compartilhados já existentes: AppSidebar (link Planejador já no menu), BaseModal, StatusBadge, MapView/MapLegend, ToastAlert, OptionCard, DataTable.
Compartilhados novos prováveis: bolha de chat/entrada, cartão de plano por talhão, mapa de janelas (grade hora × dia), gráfico de condições, lista de checagens, navegação inferior mobile.
Dados/endpoints novos (todos planned + mock, ADR 0010): conversa com o agente (mensagem → resposta + plano + fontes consultadas), previsão/janelas por fazenda, registro de aplicação (só após confirmação humana), tarefa do dia do operador.
Pontos que exigem decisão: ADR do protocolo de chat/plano (contrato), `/planner` ainda inexistente (hoje "Em breve"), mobile como rota própria ou layout responsivo, chat sem LLM (mock determinístico).
- CONFIRMADO pelo usuário (2026-11-22): inventário 16,17,18; chat com mock determinístico. Mobile: rota própria /today (padrão assumido, não respondido). ADR 0016 (protocolo planner) como proposto; pedir aprovação na pausa.
- Fundação P2 (ADR 0016 proposto, contrato planned, mocks, composables, 17+ componentes planner): ✅ aprovada com ressalvas 🟢 (ciclo 2). 🟢: JSDoc duplicado em useApplicationConfirmation:150; reset do chat não zera delivery.pending (erro de requisição antiga pode aparecer); id de entrada user-N por tamanho de lista; fidelidade visual com Figma ainda a checar no navegador.
- Tela 16 em andamento (UI /planner).

## Página 02 — telas adicionais (descobertas em 2026-11-22; o Figma foi atualizado)
| # | Tela | node-id | Depende de | Status | Veredito | Observações |
|---|---|---|---|---|---|---|
| 19 | Aplicações (histórico e planejadas) — `/applications` | 63:237 | Fundação P2 (tela 17 registra) | ✅ aprovada | 🟢 (revisão final ciclo 2) | Filtros Período/Talhão/Produto/Operador; tabela DATA·JANELA, TALHÃO, PRODUTO·DOSE, ÁREA, CONDIÇÕES, OPERADOR, STATUS (Planejada/Concluída/Cancelada); "Previsão ao vivo" nas planejadas; contexto clima no topo (Guaxupé · 24 °C) |
| 20 | Aplicação · detalhe (painel lateral 520px sobre a lista) | 63:627 (painel 63:890) | 19 | ✅ aprovada | 🟢 (revisão final ciclo 2) | Registrada em…, condições registradas (Delta T, vento, chuva 8h, duração), produto e calda (lote, dose, calda, carência), equipe e equipamento, confirmado por, assinatura digital |
Decisão do usuário: construir ("estão sim"). Contrato planned + mock (ADR 0016 estendido), item de menu "Aplicações" passa a ter rota.

## Registro P2 (final)
- Telas 16–20 aprovadas com ressalvas 🟢. Pendências 🟢: teste do hint "Seu perfil não pode registrar" e do startedNotice; esconder "Baixar registro (PDF)" no painel 404/403; ordenar talhões do seletor do planner; mapa de janelas com role=img (não tabela); decisão em aberto do ADR 0016 (summary: mês corrente vs período do filtro; applications.confirm/aprovação; timeout 60 s; operador/equipamento sem cadastro).
- Divergências P2: botão "Iniciar aplicação"/lg 56px vs 60; "Ajustar horários" e abas Mapa/Perfil(Perfil→/profile) sem endpoint; gráfico polyline e dois eixos; seletor de contexto em modal (Figma só chips); modal com seletor de talhão; chip de clima no cabeçalho (não na topbar); coluna "…" por linha omitida; avatar 32px vs 24; "Cancelada" vermelha (contrato diz muted); sem blur de fundo no modal.
- ADR 0016 aprovado pelo usuário em 2026-11-22 (decisões em aberto listadas no ADR seguem pendentes).

## Fase B do front — sessão real (2026-11-22)
- Login por redirecionamento ao Keycloak (`/login` sem senha na API real), sessão por cookie `HttpOnly`, middleware com `GET /auth/me` e erros 401/403 honestos, logout `POST /auth/logout`, usuários/convites/empresas/perfil nos adaptadores HTTP reais. Detalhes e lacunas em `docs/adr/0015-identidade-de-usuarios-e-keycloak.md` (Fase B).
- Mudanças visuais (mínimas): `/login` (botão "Entrar com o login Inceres" no lugar do formulário, rótulo "E-mail", aviso de acesso negado); Perfil (seção de senha vira link para a conta do SSO); `BaseButton` ganhou `href`. Aviso de convite criado sem e-mail usa o tom de atenção do toast.
- Pendente de decisão visual: não existe botão "Sair" em nenhuma tela (o logout está pronto em `useSession().logout()`).

## Tela 14 (Empresas) — UI de escrita ligada (2026-11-22)
- Ligados à lógica (`useCompanyListPage`/`useCompanyForm`): botão "Nova empresa" (`openCreate`), coluna de ações com menu "…", confirmação de Inativar/Excluir (erro dentro do diálogo), modal Nova/Editar empresa (CNPJ travado, carregando/não encontrada no deep link `?company=<id>`), toast flutuante do `notice`, linha destacada. Componentes: `components/company/CompanyRowActions.vue`, `CompanyConfirmDialog.vue`, `CompanyFormModal.vue`.
- PENDÊNCIA DE DESIGN: a página 03 do Figma (MeWDfQIHok0s80GbqCKGV3) só tem os frames Usuários, Convidar usuário (modal), Empresas, Painel e Perfil. Modal Nova/Editar empresa, menu "…" da linha e confirmações de Inativar/Excluir não têm frame; foram derivados dos padrões de Usuários (UserAccessModal, UserRowActions, UserConfirmDialog; TextField, SegmentedControl para o plano, ToastAlert). Pedir os frames ao design e ajustar se divergirem.
- Sem testes de componente: Vitest roda em node, sem mount (padrão do projeto). Verificação visual no navegador pendente.
