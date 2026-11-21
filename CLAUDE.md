# CLAUDE.md

Guia para o Claude Code trabalhar neste repositório. Leia este arquivo inteiro antes de qualquer tarefa. Visão geral, diagramas e roadmap estão no [README.md](./README.md).

## Contexto

Monorepo de estudo de agentes de IA para o agro, mantido por um arquiteto de software que trabalha com Clean Architecture, SOLID, padrões de projeto e Clean Code. Arquitetura não é opcional aqui: uma solução que funciona mas viola as regras abaixo não está pronta.

- `services/spray-planner` — agente Python (LangGraph) que planeja janelas de pulverização
- `services/label-advisor` — agente RAG Python que responde sobre bulas e boletins técnicos
- `services/api` — API em Go: BFF do front, dona dos dados de talhões e aplicações
- `web` — front-end em Nuxt + Vue
- `docs/projetos/` — o problema, as fases e a definição de pronto de cada agente

## Idioma

- Converse, explique e escreva documentação em **português do Brasil**.
- Código, identificadores, nomes de arquivos e mensagens de log em **inglês**. Use o glossário no final deste arquivo para traduzir termos do domínio de forma consistente.
- Commits: Conventional Commits, descrição em português, escopo do serviço. Ex.: `feat(spray-planner): adiciona regra de rajada de vento`.

---

## Regras de arquitetura (obrigatórias)

### Todos os serviços

- **Regra de dependência:** dependências apontam sempre para dentro (`adapters` → `application` → `domain`). Nunca o contrário.
- **Composição em um único lugar:** dependências são montadas apenas na raiz de composição (`main.py` no Python, `cmd/api/main.go` no Go). Nada de instanciar adaptadores dentro de casos de uso.
- **Injeção pelo construtor.** Sem singletons, sem estado global, sem service locator.
- **Cada dado tem um único dono.** Serviços conversam por HTTP com contrato; nunca acessam o banco de outro serviço.
- **Valores de negócio são configuração**, não números mágicos (limites de Delta T, vento, chuva, top-k da busca, etc.).

### Serviços Python (`spray-planner`, `label-advisor`)

Camadas em `src/<pacote>/`:

| Camada | Permitido importar | Proibido importar |
|---|---|---|
| `domain/` | biblioteca padrão (`dataclasses`, `enum`, `datetime`, `decimal`) | qualquer framework ou biblioteca externa, incluindo Pydantic |
| `application/` | `domain`, biblioteca padrão | LangChain, LangGraph, FastAPI, httpx, SQLAlchemy, Pydantic |
| `adapters/` | tudo acima e bibliotecas externas | — |

- Entidades e objetos de valor do domínio são `@dataclass(frozen=True)` sempre que possível; objetos de valor validam a si mesmos no construtor.
- Portas são `typing.Protocol`, pequenas e específicas (ISP). Prefira `FieldReader` e `ApplicationWriter` a um `FieldRepository` genérico.
- Um caso de uso por classe, com um único método público `execute`. Entrada e saída por DTOs (`dataclass`) da camada `application`.
- Pydantic é usado apenas na borda: schemas do FastAPI, configurações (`pydantic-settings`) e validação de saída do LLM.

### Regras específicas de agentes

Estas regras existem porque agentes falham de forma silenciosa. Siga à risca:

1. **LangChain e LangGraph vivem apenas em `adapters/`.** O agente é um adaptador de entrada (`adapters/inbound/agent/`); o LLM usado para gerar respostas no RAG é um adaptador de saída (`adapters/outbound/llm/`).
2. **Regra determinística nunca vai para o prompt.** Delta T, vento, chuva, intervalo entre aplicações e capacidade operacional são calculados no `domain` e testados sem LLM. O prompt não deve pedir ao modelo para fazer contas.
3. **Ferramentas são finas:** validam a entrada, chamam um caso de uso, serializam a saída. Nenhuma regra de negócio dentro da função da ferramenta.
4. **Saída de ferramenta é compacta.** Devolva janelas agrupadas e resumos, nunca a previsão horária crua de vários dias.
5. **Nenhuma ferramenta executa SQL ou código livre** gerado pelo modelo.
6. **Ferramentas que escrevem dados exigem interrupção para aprovação humana** (interrupt do LangGraph).
7. **Prompts e descrições de ferramentas ficam em arquivos versionados** em `adapters/inbound/agent/prompts/`, não em strings espalhadas pelo código.
8. **Toda resposta do `label-advisor` cita documento e página.** Se os trechos recuperados não sustentam a resposta, o agente recusa. Nunca relaxe essa regra para melhorar o placar.
9. **Falhas externas geram resposta honesta.** API de clima ou serviço fora do ar resulta em erro tratado e informado ao usuário, nunca em resposta inventada.

### Serviço Go (`services/api`)

- Layout: `cmd/api` (composição), `internal/domain`, `internal/usecase`, `internal/adapter/{http,postgres,agentclient}`, `internal/platform`.
- **Interfaces são declaradas no pacote que as consome** (em `usecase`), não no pacote que as implementa.
- `context.Context` é sempre o primeiro parâmetro de funções que fazem I/O.
- Erros são retornados e embrulhados com `fmt.Errorf("...: %w", err)`; erros de domínio são valores sentinela ou tipos próprios, checados com `errors.Is`/`errors.As`. Sem `panic` fora da inicialização.
- Roteamento com `net/http` da biblioteca padrão. Acesso a dados com pgx + sqlc; migrações com golang-migrate.
- Logs estruturados com `log/slog`.
- Probes de Kubernetes ficam em /health/live, /health/startup e /health/ready; liveness nunca consulta dependências.
- O contrato em `api/openapi.yaml` é atualizado **junto** com qualquer mudança de endpoint.
- Endpoints consumidos pelos agentes são somente leitura, exceto o de registro de aplicação, que só é chamado após aprovação humana.

### Front-end (`web`)

- Nuxt 4, Vue 3 com `<script setup lang="ts">` e Composition API. TypeScript em modo strict.
- Componentes não fazem chamadas HTTP. Chamadas ficam em `app/services/`, consumidas por composables ou stores Pinia.
- Componentes de apresentação recebem props e emitem eventos; lógica reutilizável vai para `composables/`.
- O front fala **apenas** com `services/api`. Nunca chame os serviços Python diretamente.

---

## Clean Code e padrões de projeto

- Nomes revelam intenção; nada de abreviações obscuras (`fld`, `wthr`, `tmp2`).
- Funções pequenas, com um nível de abstração e retorno antecipado em vez de `if` aninhado.
- Composição em vez de herança para reutilizar comportamento.
- Comentários explicam **por quê**, não **o quê**. Docstrings em APIs públicas de cada camada.
- Não crie abstração para um único uso futuro hipotético. Uma interface existe quando há uma porta real ou quando ela permite testar.

Padrões já adotados, use-os em vez de inventar alternativas:

| Padrão | Onde |
|---|---|
| Ports & Adapters | Fronteira aplicação/infraestrutura em todos os serviços |
| Specification | Regras de aptidão do `spray-planner` (`DeltaTRule`, `WindRule`, `RainRule`), combináveis |
| Strategy | Chunking e extração de PDF no `label-advisor` |
| Repository | Persistência de talhões e aplicações na `api` |
| Composition Root | `main.py` / `cmd/api/main.go` |

---

## Testes e avaliação

- **Domain:** testes unitários puros, sem mocks, cobrindo casos de borda (ex.: Delta T no limite exato da faixa).
- **Application:** testes com **fakes** das portas (classes simples em memória), não com `unittest.mock`. Fakes ficam em `tests/fakes/`.
- **Adapters:** testes de integração em `tests/integration/`. Chamadas HTTP externas usam respostas gravadas, nunca a rede real.
- **Testes unitários nunca chamam um LLM real.**
- **Evals** (`evals/`) usam LLM real, têm custo e rodam separadamente com `make eval`. Rode-os ao alterar prompts, ferramentas, chunking ou recuperação, e reporte o placar antes e depois.
- **Nunca altere um dataset de avaliação para fazer o agente passar.** Se um caso parecer errado, aponte e pergunte.
- Go: testes orientados a tabela (`table-driven`), com `t.Run` por caso.
- Front: Vitest para composables e stores.

---

## Comandos

Na raiz:

```bash
make up       # docker-compose com Postgres (pgvector) e serviços
make test     # testes de todos os serviços
make lint     # lint e checagem de tipos de todos os serviços
make eval     # avaliações dos agentes (LLM real)
make hooks    # instala pre-commit e pre-push (rode uma vez após clonar)
make security # gitleaks, bandit, pip-audit, govulncheck em todo o repositório
make audit    # CVEs no uv.lock de cada serviço Python
```

Serviços Python (dentro de `services/spray-planner` ou `services/label-advisor`):

```bash
uv sync
uv run pytest tests/unit
uv run pytest tests/integration
uv run ruff check . && uv run ruff format --check .
uv run mypy src
uv run python evals/run_evals.py
uv run alembic upgrade head          # apenas label-advisor
```

API Go (`services/api`):

```bash
go test ./...
golangci-lint run
sqlc generate
go run ./cmd/api
```

Front (`web`):

```bash
pnpm install
pnpm dev
pnpm test
pnpm lint
```

> Se um comando listado ainda não existir no repositório, crie o alvo correspondente (no `Makefile`, `pyproject.toml` ou `package.json`) em vez de improvisar um comando diferente, e atualize este arquivo.

---

## Fluxo de trabalho esperado

1. **Antes de implementar,** leia o documento do projeto em `docs/projetos/` e identifique em que fase a tarefa se encaixa. Não adiante trabalho de fases futuras sem pedido explícito.
2. **Proponha um plano curto** (arquivos a criar/alterar, camada de cada um) e aguarde confirmação em tarefas que toquem mais de uma camada ou mais de um serviço.
3. **Decisão arquitetural nova** (dependência nova, mudança de contrato, nova camada, troca de biblioteca) exige um ADR proposto em `docs/adr/NNNN-titulo.md` com contexto, opções, decisão e consequências. Pergunte antes de seguir.
4. **Implemente de dentro para fora:** domínio e testes primeiro, depois caso de uso, depois adaptadores.
5. **Antes de concluir,** rode lint, checagem de tipos e testes do serviço afetado. Informe o resultado.
6. Mudanças pequenas e focadas. Não reformate nem refatore arquivos fora do escopo da tarefa; se notar um problema, aponte-o.

## Nunca faça

- Importar framework no `domain` ou na `application`
- Colocar regra de negócio em prompt, ferramenta, handler HTTP ou componente Vue
- Acessar o banco de outro serviço
- Commitar `.env`, chaves de API ou PDFs de `data/raw/`
- Adicionar dependência sem justificar no plano
- Silenciar erro (`except: pass`, `_ = err`) ou capturar exceção genérica sem relançar com contexto
- Desabilitar regras de lint ou de tipo para fazer o código passar
- Usar `git commit --no-verify` ou remover um hook do pre-commit para fazer o commit passar

## Definição de pronto de uma tarefa

- [ ] Respeita a regra de dependência e as regras da camada
- [ ] Testes novos ou ajustados, passando
- [ ] Lint e checagem de tipos sem erros
- [ ] Evals rodados, se a mudança afeta o comportamento do agente
- [ ] ADR criado, se houve decisão arquitetural
- [ ] Contrato OpenAPI atualizado, se mudou endpoint
- [ ] Documentação atualizada, se mudou comportamento ou comando

---

## Glossário do domínio

| Português | Código (inglês) | Observação |
|---|---|---|
| Talhão | `Field` | Unidade de área de manejo dentro da fazenda |
| Cultura | `Crop` | Soja, milho, café… |
| Safrinha | `SecondSeason` | Segunda safra, geralmente milho após soja |
| Defensivo | `CropProtectionProduct` | Inseticida, fungicida, herbicida |
| Princípio ativo | `ActiveIngredient` | |
| Nome comercial | `TradeName` | Vários nomes comerciais podem ter o mesmo princípio ativo |
| Bula | `ProductLabel` | Documento oficial do defensivo |
| Alvo / praga | `Target` / `Pest` | |
| Dose | `Dosage` | Sempre com unidade explícita |
| Carência | `PreHarvestInterval` | Dias entre a última aplicação e a colheita |
| Intervalo de reentrada | `ReEntryInterval` | Tempo até pessoas poderem entrar na área |
| Pulverização / aplicação | `Application` | Registro de uma aplicação em um talhão |
| Janela de pulverização | `SprayWindow` | Período contínuo apto para aplicar |
| Delta T | `DeltaT` | Diferença entre temperatura de bulbo seco e úmido, em °C |
| Calda | `SprayMix` | |
| Deriva | `Drift` | |
| Rendimento operacional | `OperationalCapacity` | Hectares por hora do equipamento |
| Boletim técnico | `TechnicalBulletin` | Publicações da Embrapa e fundações de pesquisa |