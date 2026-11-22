# ADR 0013: Empresas como tenant, papéis e permissões

- **Status:** aceito (2026-11-21)
- **Data:** 2026-11-21

## Contexto

A página do Figma "03 · Administração, Painel e Perfil" introduz conceitos que o sistema ainda não tem:

- **Empresas** (tela "Empresas · super admin"): cada cliente da plataforma é uma empresa, dona das próprias fazendas, talhões, proprietários e aplicações (ex.: "Fazenda Santa Rita", "Sítio Boa Esperança" como empresas clientes; "Agentes Agro" como a operadora da plataforma).
- **Usuários e papéis** (telas "Usuários" e "Convidar usuário"), com três papéis descritos no próprio design:
  - **Super admin** — gestão do sistema: empresas, usuários de todas as empresas e configurações.
  - **Administrador** — gerencia a própria empresa: cadastros (fazendas, talhões, proprietários), usuários da empresa e confirma aplicações.
  - **Operador de planejamento** — usa o Planejador e o agente e registra aplicações nas **fazendas liberadas** para ele; somente leitura nos cadastros.
- **Convites**: o usuário é convidado por e-mail e cria a própria senha; status Ativo, Convite pendente, Inativo.
- **Painel (administrador)** e **Perfil do usuário** dependem de saber quem é o usuário, a empresa e o papel.

Hoje a sessão (`GET /auth/me`) só traz o usuário, e nenhum dado é segregado por empresa. O CLAUDE.md exige ADR para mudança de contrato e de modelo.

## Opções

1. **Só papéis no front, sem empresa.** Simples, mas não representa o design (empresas, escopo por fazenda) nem dá isolamento de dados.
2. **Empresa como tenant + papéis fixos verificados por nome no front** (`if role === 'admin'`). Espalha regra de autorização pela UI e acopla o front aos nomes dos papéis.
3. **Empresa como tenant + papéis com permissões (capacidades) devolvidas pela API** (escolhida).

## Decisão

### Modelo

- `Company` é o **tenant**: toda fazenda, talhão, proprietário, vínculo e aplicação pertence a exatamente uma empresa.
- `User` pertence a uma empresa e tem **um papel**: `super_admin`, `admin` ou `planning_operator`. O super admin pertence à empresa operadora da plataforma e enxerga todas as empresas.
- O operador de planejamento tem um **escopo de fazendas**: todas as fazendas da empresa ou uma lista explícita (campo "Fazendas liberadas" do convite).
- `Invitation` registra convites pendentes (e-mail, papel, empresa, escopo, expiração). Aceitar o convite cria/ativa o usuário; a definição de senha acontece fora do front autenticado (link do e-mail).

### Autorização: a API decide, o front só apresenta

- **A API é a única fonte de autorização.** Todo endpoint filtra os dados pela empresa do token e aplica o papel e o escopo de fazendas. Super admin pode filtrar por empresa (`company_id`) nos endpoints de administração.
- `GET /auth/me` passa a devolver `role`, `company { id, name }`, `farm_scope` (`all` ou lista de ids) e **`permissions`**: lista de capacidades, por exemplo `companies.manage`, `users.manage`, `users.manage_all`, `registry.write`, `applications.register`, `applications.confirm`, `planner.use`, `dashboard.view`.
- **O front verifica capacidades, nunca nomes de papel**, e apenas para apresentação: itens de menu, botões, rotas e mensagens. Esconder um botão não é segurança; uma ação proibida que chegue à API recebe `403` e a UI mostra erro honesto.
- No front, um composable único (`usePermissions()` → `can(permission)`) lê as capacidades da sessão; a navegação e um middleware de rota (`definePageMeta({ permission: 'users.manage' })`) usam esse composable. Nenhum componente calcula permissão a partir do papel.
- O mock deriva as capacidades do papel com a mesma tabela que a API terá, documentada no contrato.

### Contrato (planned, ADR 0010)

- Sessão: `GET /auth/me` ampliado (acima).
- Usuários: `GET /users` (busca, papel, status, página), `PATCH /users/{userId}` (papel, empresa, escopo, status), `GET /users/summary` (contagem por papel).
- Convites: `POST /invitations`, `POST /invitations/{id}/resend`, `DELETE /invitations/{id}`.
- Empresas (super admin): `GET /companies` (busca, status, página), `GET /companies/summary`, `POST /companies`, `PATCH /companies/{companyId}`.
- Perfil: `GET /me/profile`, `PATCH /me/profile`, `POST /me/password` (senha atual + nova).
- Painel: `GET /dashboard` com indicadores, janelas × aplicações, próximas janelas, alertas do agente e atividade recente. Os blocos que vêm do Planejador ficam descritos no contrato e servidos pelo mock até o Planejador existir.
- Erros: `403 forbidden` com código próprio para "sem permissão" e "fora do escopo".

## Consequências

**Positivas**
- Isolamento de dados por empresa definido desde o contrato, antes da implementação Go.
- A UI não conhece regras de autorização: mudar o que um papel pode fazer é mudança só na API.
- Telas novas e existentes usam o mesmo `can()`; testes do front usam sessões falsas com capacidades diferentes.

**Negativas / custos**
- **Telas já aprovadas precisam respeitar as capacidades** (ex.: operador não vê "Nova fazenda", "Novo proprietário", "Novo talhão", "Editar"): Fazendas, Proprietários (lista e modal), Fazenda: áreas e talhões, Novo/editar talhão e o assistente de nova fazenda. São mais de 3 telas aprovadas, o que exige confirmação do usuário antes de alterar.
- A implementação Go precisará de filtro por empresa em todas as consultas (ex.: coluna `company_id` + escopo no repositório) e testes de isolamento; isso fica para a tarefa da API.
- Mais estados de erro na UI (`403`).

## Pendências

- Fluxo de aceite de convite e definição de senha (página pública) — não está no Figma; fica fora desta entrega.
- Se um usuário poderá pertencer a mais de uma empresa (hoje: não).
