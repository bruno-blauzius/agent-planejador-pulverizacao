# ADR 0008: API Go com stdlib primeiro; probes de Kubernetes e Swagger embutido

- **Status:** proposto
- **Data:** 2026-11-21

## Contexto

A `services/api` está sendo inicializada. Nesta etapa ela só precisa subir, responder probes e publicar o contrato. Ainda não há banco, agentes consumindo nem regra de negócio. Adicionar pgx, sqlc e golang-migrate agora seria dependência sem uso, contra a regra de não adicionar dependência sem justificativa.

Também precisamos operar a API em Kubernetes, o que exige semântica correta de probes e shutdown gracioso, e dar ao front e aos agentes uma documentação navegável do `api/openapi.yaml`.

## Opções

1. **Framework HTTP e bibliotecas de log/config desde já** (chi, zerolog, viper). Mais conveniência, mas dependências novas sem necessidade e conflito com o CLAUDE.md (`net/http` e `log/slog`).
2. **Só biblioteca padrão** (escolhida): `net/http`, `log/slog`, `os.LookupEnv`, `embed`.
3. **Adiantar pgx, sqlc e golang-migrate.** Antecipa trabalho da Fase 3 sem ter tabelas nem casos de uso.

## Decisão

Iniciar a API **somente com a biblioteca padrão**. Configuração por variáveis de ambiente lidas com `os.LookupEnv`:

| Variável | Padrão |
|---|---|
| `API_PORT` | `8080` |
| `API_LOG_LEVEL` | `info` |
| `API_READ_HEADER_TIMEOUT` | `5s` |
| `API_SHUTDOWN_TIMEOUT` | `15s` |
| `API_PROBE_TIMEOUT` | `2s` |
| `API_DOCS_ENABLED` | `true` |

**Probes** (detalhes em `docs/arquitetura/probes-kubernetes.md`):

- `GET /health/live`: liveness. Nunca consulta dependências; só indica que o processo responde.
- `GET /health/startup`: startup. Fica indisponível até a inicialização terminar.
- `GET /health/ready`: readiness. Fica indisponível ao iniciar, durante o shutdown e quando uma dependência falha (limitada por `API_PROBE_TIMEOUT`).

**Shutdown gracioso:** ao receber SIGTERM/SIGINT, chama `BeginShutdown` (readiness passa a falhar para o Kubernetes retirar o pod do balanceamento) e depois `http.Server.Shutdown`, limitado por `API_SHUTDOWN_TIMEOUT`.

**Documentação:** `GET /openapi.yaml` serve o contrato embutido no binário com `go:embed`; `GET /docs` serve uma Swagger UI apontando para ele. A UI carrega seus assets do CDN unpkg, então depende de rede externa do navegador. Por isso `API_DOCS_ENABLED=false` desliga as duas rotas em produção.

**pgx, sqlc e golang-migrate ficam para a Fase 3**, cada decisão com ADR próprio quando a persistência for introduzida.

## Consequências

- Nenhuma dependência de terceiros e nenhum `go.sum` por enquanto; o `Dockerfile` tolera a ausência dele.
- Configuração manual com `os.LookupEnv` exige validar e converter valores com erro claro; se crescer, reavaliar com novo ADR.
- O `openapi.yaml` precisa ficar dentro do módulo para ser embutido, e deve ser atualizado junto com qualquer endpoint.
- A Swagger UI via CDN expõe o navegador a um recurso externo; em produção, desligue-a ou substitua por assets locais.
- A dependência de readiness em bancos e serviços entra na Fase 3, reaproveitando o contrato de probes.
