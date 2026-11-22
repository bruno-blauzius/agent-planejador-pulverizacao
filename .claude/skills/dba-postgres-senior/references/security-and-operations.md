# Segurança e operação

## Papéis e privilégios (estado atual)

Definido em `infra/docker/postgres/init/01-create-service-databases.sh` (ADR 0009):

- Superusuário (`POSTGRES_USER`, padrão `agro_admin`): só administração. **Nenhum serviço o usa.**
- `<svc>_migrator`: dono do banco e do schema `public`; roda DDL/migrações.
- `<svc>_app`: `LOGIN`, `CONNECTION LIMIT 40`, `CONNECT, TEMPORARY` no banco, `USAGE` no schema; `SELECT/INSERT/UPDATE/DELETE` em tabelas, `USAGE/SELECT` em sequences, `EXECUTE` em funções, tudo por `ALTER DEFAULT PRIVILEGES FOR ROLE <migrator>`.
- `CONNECT` revogado de `PUBLIC` e `ALL` no schema `public` revogado de `PUBLIC`.

Regras:

1. **Privilégio mínimo.** Dê ao `app` só o necessário; tabela imutável (`applications` confirmadas, auditoria, eventos) pode ter `REVOKE UPDATE, DELETE` para o `app`.
2. **Leitor separado quando precisar** (relatórios, BI, suporte): role `<svc>_readonly` com `SELECT` apenas e `default_transaction_read_only = on`. Nunca use o `app` para análise manual.
3. **Nunca** `SUPERUSER`, `CREATEROLE`, `CREATEDB`, `BYPASSRLS` para roles de serviço.
4. **Funções `SECURITY DEFINER`**: evitar; se inevitável, fixe `SET search_path = pg_catalog, public` e revogue `EXECUTE` de `PUBLIC`.
5. **Senhas:** `scram-sha-256`, geradas aleatoriamente, no `.env`/secret manager, rotacionáveis (rotação sem downtime: criar nova credencial, trocar a aplicação, remover a antiga). URL-safe porque entram na URL de conexão.
6. **Rede:** o Postgres só fica na rede `agro`; porta publicada em `127.0.0.1` apenas. Em produção: sem IP público, security group restrito, TLS obrigatório (`sslmode=verify-full`; local usa `sslmode=disable` por conveniência).
7. **Auditoria:** `log_connections`/`log_disconnections` já ligados. Para trilha de DDL/DML sensível considere `pgaudit` (extensão: ADR).
8. **Imagens:** fixe a versão major (`pg17`) e atualize minors com regularidade; verifique CVEs da imagem.

Verificar privilégios efetivos:

```sql
\du+                                   -- papéis e atributos
\dp public.*                            -- privilégios por tabela
SELECT * FROM information_schema.role_table_grants WHERE grantee = 'api_app';
SELECT has_table_privilege('api_app', 'public.fields', 'DELETE');
```

## Row Level Security (quando adotada)

- `ENABLE` + `FORCE ROW LEVEL SECURITY` nas tabelas de tenant; policies `USING` e `WITH CHECK` por `current_setting('app.company_id', true)`.
- A aplicação executa `SET LOCAL app.company_id = ...` no início de **cada transação** (compatível com pool). Sem a variável, a policy deve retornar zero linhas.
- O papel `app` **não** tem `BYPASSRLS`; `migrator` e superusuário ignoram RLS, então os testes de isolamento usam o papel `app`.
- Teste: empresa A não lê, não altera e não insere dado da empresa B, inclusive por FK composta e por *joins*.
- RLS é defesa adicional; a autorização continua sendo da API (ADR 0013). Impacto de desempenho: policies simples e indexadas por `company_id`.

## Dados pessoais (LGPD)

- Dados pessoais no escopo: usuários (nome, e-mail), convites, proprietários (nome, documento, contato). Inventarie colunas e defina finalidade e retenção.
- **Minimização:** não armazene o que não usa. **Retenção:** rotina para expirar convites e anonimizar/apagar quando devido.
- **Senhas:** hash com argon2id/bcrypt na aplicação; nunca em claro, nunca em log.
- **Logs e dumps:** não registre valores de parâmetros sensíveis; mascare dados ao copiar produção para desenvolvimento (não use dado real em ambiente local).
- Documentos (CPF/CNPJ) quando persistidos: colunas dedicadas, restrição de acesso por papel, considerar criptografia em nível de aplicação para campos de alto risco.
- Direito de exclusão/portabilidade: o modelo deve permitir localizar e remover/anonimizar os dados de uma pessoa sem quebrar fatos históricos (`applications` referenciam pessoa por id, não copiam dado pessoal).

## Backup, restauração e PITR

- `wal_level = replica` já permite arquivamento de WAL e réplicas. **Backup que nunca foi restaurado não é backup.**
- Defina **RPO** (perda tolerável) e **RTO** (tempo de recuperação) com o dono do produto; deles vem a estratégia:
  - Banco gerenciado em produção: backups automáticos + PITR do provedor, retenção definida, restauração testada periodicamente em ambiente separado.
  - Self-managed: `pg_basebackup`/pgBackRest/WAL-G com arquivamento contínuo de WAL.
- Backups lógicos (`pg_dump -Fc`) servem para migração entre versões, extração parcial e ambientes pequenos; não substituem PITR em produção.
- Backups criptografados, fora da mesma conta/zona do banco, com acesso restrito.
- **Teste de restauração** documentado e agendado: restaurar, subir a aplicação contra a cópia, validar contagens e integridade.
- Dev local: `docker compose down` preserva o volume; `down -v` apaga tudo. Backup rápido local:

```bash
docker compose exec agro-postgres pg_dump -U <migrator> -Fc -d api > api.dump
```

  (Não commite dumps; `data/raw/` e `.env` também estão proibidos no git.)

## Alta disponibilidade e réplicas

- Réplica de leitura (streaming) escala leitura e serve de standby. Atenção a *replication lag*: leitura logo após escrita deve ir ao primário (read-your-writes).
- Failover automático (Patroni ou gerenciado) com teste periódico.
- Slots de replicação inativos retêm WAL e enchem o disco: monitore `pg_replication_slots`.
- A aplicação deve tolerar queda de conexão: *retry* com *backoff* em erros transitórios (`57P01`, `08006`, `40001`, `40P01`), nunca em erros de dados (`23xxx`).

## Upgrade de versão

- Minor: troca de binário, rápido; faça com regularidade.
- Major (ex.: 17 → 18): `pg_upgrade` ou replicação lógica; ensaie em cópia, valide extensões (`vector` precisa de versão compatível), reconstrua estatísticas (`ANALYZE`) e teste a aplicação e o plano das queries críticas.
- Ao subir a imagem local, o volume de dados de major antigo **não** é compatível: exige dump/restore ou `pg_upgrade`.

## Operação diária (runbook curto)

| Situação | Primeiro passo |
|---|---|
| Aplicação lenta | `pg_stat_activity` (quem roda/espera), `pg_stat_statements` (quem consome), locks bloqueantes |
| Conexões esgotadas | Quem as segura (`state`, `application_name`), transações presas, tamanho dos pools × réplicas |
| Disco cheio | WAL retido (slot inativo?), bloat, tabelas/índices maiores (`pg_total_relation_size`), logs; nunca apague arquivos de WAL manualmente |
| Deadlock | Log do Postgres mostra as duas queries; padronize ordem de acesso, encurte transações |
| Migração travada | Há lock bloqueante? `pg_blocking_pids`; cancele (`pg_cancel_backend`) antes de terminar (`pg_terminate_backend`) |
| Query descontrolada | `pg_cancel_backend(pid)`; depois corrija causa e adicione `statement_timeout` por papel |
| Réplica atrasada | Transação longa na réplica, I/O, volume de WAL; verifique `pg_stat_replication` |

Antes de qualquer ação destrutiva em ambiente compartilhado ou produção: confirme o ambiente (`SELECT current_database(), inet_server_addr();`), faça em transação com `SELECT` de conferência do `WHERE`, tenha backup verificado e peça confirmação explícita ao usuário.

## Ambiente local deste repositório

- Subir: `make up` (ou `make dev`); estado: `make ps`; logs: `make logs`.
- Acesso ao banco pelo host: `127.0.0.1:${POSTGRES_HOST_PORT:-5433}`; ou `docker compose exec agro-postgres psql -U <role> -d <db>`.
- Resetar o banco local é `docker compose down -v` (destrutivo: confirme antes). Mudança no script de init só tem efeito depois disso.
- Segredos (`POSTGRES_PASSWORD`, `*_DB_MIGRATOR_PASSWORD`, `*_DB_APP_PASSWORD`) ficam no `.env`, nunca no repositório (`.env.example` só traz nomes).
