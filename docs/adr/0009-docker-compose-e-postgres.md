# ADR 0009: Docker Compose, Postgres local próximo de produção e esqueletos dos serviços

- **Status:** proposto
- **Data:** 2026-11-21

## Contexto

O monorepo precisa subir os quatro projetos (`web`, `api`, `spray-planner`, `label-advisor`) e um Postgres com um comando. A máquina de desenvolvimento já roda, na network Docker `inceres-infra`, uma infraestrutura compartilhada com Redis e Kafka. O RabbitMQ também roda localmente, mas fora dessa network. Os agentes vão precisar dessa infraestrutura no futuro.

Até aqui, `web`, `spray-planner` e `label-advisor` eram só pastas vazias, sem como fazer build de uma imagem.

## Opções

**Rede**
1. Usar a network externa `inceres-infra` (escolhida).
2. Criar uma network nova e reconectar a infraestrutura existente a ela.

**Postgres**
1. Um banco e um par de usuários por serviço (escolhida).
2. Um banco único, com um schema por serviço.
3. Usuário superadmin compartilhado: descartada, contraria "cada dado tem um único dono".

**Serviços sem código**
1. Esqueleto mínimo bootável, só com probes de saúde (escolhida).
2. Dockerfiles prontos, mas serviços desligados por profile do compose.

## Decisão

- **Compose na raiz** (`name: agentes-agro`), com duas redes:
  - `agro`: rede privada do projeto. É a única onde o Postgres é alcançável.
  - `inceres-infra` (externa): os quatro serviços de aplicação entram nela para usar Redis, Kafka e RabbitMQ.
  - Os serviços de nome genérico têm prefixo `agro-` (`agro-api`, `agro-web`, `agro-postgres`), para não colidir no DNS da rede compartilhada.
- **Postgres** `pgvector/pgvector:pg17`, configurado como produção:
  - Um banco por serviço (`api`, `label_advisor`), cada um com `<svc>_migrator` (dono, DDL) e `<svc>_app` (só DML, via default privileges).
  - `CONNECT` revogado de `PUBLIC`, então um serviço não conecta no banco do outro.
  - `scram-sha-256` em toda conexão por rede.
  - Data checksums, `wal_level = replica`, `pg_stat_statements`, timeouts de sessão presa e log de queries lentas (500 ms).
  - Porta exposta só em `127.0.0.1`, senhas apenas no `.env`.
  - As extensões (`vector`, `pg_stat_statements`) são criadas no init, porque exigem superusuário.
- **Imagens**: multi-stage e não-root. Os containers de aplicação rodam com `read_only`, `cap_drop: ALL` e `no-new-privileges`.
  - **API Go**: distroless, com um binário `healthcheck` à parte, porque a imagem não tem shell.
  - **Python**: `python:3.12-slim`, com dependências instaladas pelo `uv` a partir do `uv.lock`.
  - **Web**: `node:22-alpine`, rodando só o `.output` do Nuxt.
- **Healthchecks**: os três serviços além da API usam o mesmo padrão de probes (`/health/live`, `/health/startup`, `/health/ready`), e o `depends_on` espera `service_healthy`.
- **Dependências novas**, todas necessárias para o esqueleto bootar:
  - Python: `fastapi`, `uvicorn[standard]`, `pydantic-settings` (já previstos na Stack). Em dev: `pytest`, `httpx`, `mypy`, `ruff`.
  - Front: `nuxt`, `vue`, `vue-router` (previstos na Stack). Em dev: `typescript`, `vue-tsc`, `vitest`.
  - LangChain, LangGraph, pgvector e Alembic continuam fora até as fases que os exigem.

## Consequências

- **Para subir:** `make up` sobe tudo. É preciso que a network `inceres-infra` exista e que o `.env` tenha as senhas do Postgres (o compose falha com mensagem clara se faltarem).
- **RabbitMQ:** precisa ser conectado à `inceres-infra` com `docker network connect inceres-infra rabbitmq` para ser alcançável pelos serviços.
- **Script de init:** só roda com o volume vazio. Mudar usuários ou bancos exige `docker compose down -v`, que apaga os dados locais, ou uma migração manual.
- **TLS:** as conexões locais usam `sslmode=disable`. Em produção, o banco gerenciado deve exigir TLS (`sslmode=verify-full`).
- **Senhas:** precisam ser URL-safe, porque entram nas URLs de conexão.
- **Tuning:** o `postgresql.conf` está dimensionado para o limite de 1 GB do container e deve ser recalculado para o tamanho da instância de produção.
