# Desempenho, índices e tuning

Regra de ouro: **meça antes de mudar** e **meça depois**. Sem plano de execução ou estatística, é palpite.

## Fluxo de diagnóstico de query lenta

1. **Ache o culpado** pelo custo total, não pela query mais lenta isolada (`pg_stat_statements` já está ativo em todos os bancos):

```sql
SELECT queryid,
       calls,
       round(total_exec_time::numeric, 1)  AS total_ms,
       round(mean_exec_time::numeric, 2)   AS mean_ms,
       round((100 * total_exec_time / sum(total_exec_time) OVER ())::numeric, 1) AS pct,
       rows,
       left(query, 120) AS query
FROM pg_stat_statements
WHERE dbid = (SELECT oid FROM pg_database WHERE datname = current_database())
ORDER BY total_exec_time DESC
LIMIT 20;
```

2. **Reproduza com dados realistas** (volume e distribuição de produção; `generate_series` para sintético). Plano em tabela de 100 linhas não prova nada.
3. **Leia o plano:**

```sql
EXPLAIN (ANALYZE, BUFFERS, VERBOSE, SETTINGS) <query>;
```

   `ANALYZE` executa de verdade: para `INSERT/UPDATE/DELETE`, envolva em `BEGIN; ... ROLLBACK;`. Procure:
   - **Estimativa × real** muito diferente (fator ≥ 10): estatística ruim → `ANALYZE`, `CREATE STATISTICS` (colunas correlacionadas), aumentar `default_statistics_target` na coluna.
   - **`Seq Scan`** em tabela grande com filtro seletivo: falta índice ou ele não serve (função na coluna, tipo diferente, ordem de colunas).
   - **`Rows Removed by Filter`** alto: índice não cobre o filtro.
   - **`Sort Method: external merge`/`Disk`**: `work_mem` insuficiente ou índice que entregue ordenado.
   - **`Buffers: shared read`** alto: dado frio; `hit` alto = cache. Compare `shared hit` × `read`.
   - **Nested Loop** com muitas voltas (`loops=`) em `Index Scan`: N+1 disfarçado; considere reescrever o JOIN ou índice melhor.
   - **`Heap Fetches`** alto em `Index Only Scan`: visibility map desatualizado → VACUUM.
4. **Mude uma coisa por vez**, reexecute e compare o plano e o tempo.
5. **Registre** o antes/depois na entrega.

## Índices

| Tipo | Quando usar | Observações |
|---|---|---|
| B-tree | Igualdade, faixa, ordenação, `LIKE 'abc%'` (com collation `C`/`text_pattern_ops`) | Padrão. Ordem das colunas importa: igualdade primeiro, faixa/ordenação por último |
| Parcial (`WHERE ...`) | Subconjunto pequeno e consultado (`status = 'pending'`, `deleted_at IS NULL`) | Menor e mais rápido; a query precisa conter o predicado |
| Cobrindo (`INCLUDE (...)`) | Habilitar `Index Only Scan` sem inflar a chave | Só vale se o visibility map estiver em dia |
| Por expressão | `lower(email)`, `(payload->>'x')` | A query deve usar a mesma expressão |
| GIN | `jsonb @>`, arrays, `tsvector` (busca textual) | Escrita mais cara; use `fastupdate`/`gin_pending_list_limit` com consciência |
| GiST | Intervalos (`tstzrange`), geometria (PostGIS), `EXCLUDE` | Exige `btree_gist` para escalar misto |
| BRIN | Tabelas enormes append-only com correlação física (tempo) | Minúsculo; inútil em dado sem correlação |
| HNSW / IVFFlat | Busca vetorial (pgvector) | Ver [pgvector-rag.md](pgvector-rag.md) |

Regras:

- **Toda FK tem índice no lado filho** (o Postgres não cria sozinho). Em tabelas de tenant: `(company_id, <fk>)`.
- **`company_id` na frente** em índices de tabelas de tenant, para consultas sempre filtradas pelo tenant.
- **Não indexe por reflexo.** Cada índice custa escrita, espaço e VACUUM. Remova os não usados (`idx_scan = 0` por um ciclo de negócio inteiro) e os redundantes (prefixo de outro índice).
- **Um índice composto bem pensado vence vários simples.**
- Em tabela existente: `CREATE INDEX CONCURRENTLY` (ver [migrations.md](migrations.md)). Se falhar, deixa índice `INVALID`: `DROP` e refaça.

Achar candidatos e desperdício:

```sql
-- índices nunca usados (reset de estatística recente invalida a conclusão)
SELECT s.schemaname, s.relname, s.indexrelname, s.idx_scan, pg_size_pretty(pg_relation_size(s.indexrelid)) AS size
FROM pg_stat_user_indexes s
JOIN pg_index i ON i.indexrelid = s.indexrelid
WHERE s.idx_scan = 0 AND NOT i.indisunique AND NOT i.indisprimary
ORDER BY pg_relation_size(s.indexrelid) DESC;

-- FKs sem índice no lado filho
SELECT c.conrelid::regclass AS tabela, c.conname, pg_get_constraintdef(c.oid) AS definicao
FROM pg_constraint c
WHERE c.contype = 'f'
  AND NOT EXISTS (
    SELECT 1 FROM pg_index i
    WHERE i.indrelid = c.conrelid
      AND (i.indkey::int2[])[0:cardinality(c.conkey)-1] @> c.conkey::int2[]
      AND c.conkey::int2[] @> (i.indkey::int2[])[0:cardinality(c.conkey)-1]
  );
```

## Padrões de query

- **Paginação por chave (keyset)**, não `OFFSET` grande: `WHERE (created_at, id) < ($1, $2) ORDER BY created_at DESC, id DESC LIMIT $3` com índice `(company_id, created_at DESC, id DESC)`.
- **Evite `SELECT *`.** Em `sqlc`, liste as colunas: o contrato fica explícito e `Index Only Scan` fica possível.
- **N+1:** resolva com `JOIN`, `WHERE id = ANY($1)` ou agregação (`json_agg`) — e prove com o log/contador de queries do teste.
- **`COUNT(*)` exato em tabela grande é caro.** Use contagem aproximada (`reltuples`) ou materialize quando a UI aceitar.
- **`EXISTS` em vez de `IN (subquery)`/`COUNT(*) > 0`** para teste de existência.
- **Funções sobre coluna indexada** desativam o índice (`WHERE date(created_at) = ...`): reescreva como faixa (`created_at >= $1 AND created_at < $2`).
- **Tipos coerentes** nos parâmetros (comparar `bigint` com `numeric`/`text` impede índice).
- **Transações curtas.** Nada de chamada HTTP/LLM dentro de transação aberta (`idle_in_transaction_session_timeout = 60s` vai matar a sessão). Escreva depois da chamada externa.
- **Idempotência e upsert:** `INSERT ... ON CONFLICT (...) DO UPDATE/NOTHING` com a constraint certa.
- **Ordem de locks consistente** entre transações para evitar deadlock; `SELECT ... FOR UPDATE SKIP LOCKED` para filas.
- **Isolamento:** `READ COMMITTED` (padrão) basta na maioria; use `SERIALIZABLE` com retry ou `FOR UPDATE` quando houver invariantes entre linhas (ex.: sobreposição de janelas/aplicações no mesmo talhão — prefira `EXCLUDE` constraint).

## VACUUM, autovacuum e bloat

- O MVCC deixa versões mortas; o autovacuum as limpa e atualiza estatísticas e visibility map. O ambiente já loga autovacuum acima de 1s (`log_autovacuum_min_duration`).
- Sinais de problema: `n_dead_tup` alto, tabela/índice crescendo sem dados, `Heap Fetches` altos, transações longas segurando o horizonte (`xmin`).

```sql
SELECT relname, n_live_tup, n_dead_tup,
       round(100.0 * n_dead_tup / NULLIF(n_live_tup + n_dead_tup, 0), 1) AS dead_pct,
       last_autovacuum, last_autoanalyze
FROM pg_stat_user_tables
ORDER BY n_dead_tup DESC
LIMIT 20;

-- transações longas / paradas
SELECT pid, usename, state, now() - xact_start AS xact_age, wait_event_type, wait_event, left(query, 100) AS query
FROM pg_stat_activity
WHERE xact_start IS NOT NULL
ORDER BY xact_start
LIMIT 20;
```

- Tabelas com muita atualização: ajuste **por tabela** (`ALTER TABLE ... SET (autovacuum_vacuum_scale_factor = 0.02, autovacuum_analyze_scale_factor = 0.01)`), não global.
- Atualizações frequentes em colunas não indexadas se beneficiam de HOT; deixe `fillfactor` menor (ex.: 85–90) em tabelas muito atualizadas.
- Recupere bloat com `REINDEX CONCURRENTLY` ou `pg_repack` (extensão: ADR). **`VACUUM FULL` trava a tabela**: só em janela de manutenção.

## Conexões e pool

- Limites atuais: `max_connections = 100`; papel `*_app` com `CONNECTION LIMIT 40`. **O somatório de `max_conns` do pool de todas as réplicas do serviço deve ficar abaixo do limite do papel** (com folga para migração, `psql` e monitoramento).
- `pgxpool` (Go): defina `MaxConns`, `MinConns`, `MaxConnLifetime`, `MaxConnIdleTime`, `HealthCheckPeriod`, a partir da carga e das réplicas, como configuração. Mais conexões não é mais desempenho: com poucos cores, um pool pequeno costuma ser mais rápido.
- Python: pool no adaptador (psycopg/SQLAlchemy), dimensionado do mesmo modo; o acesso do agente é de baixa concorrência.
- Se entrar PgBouncer em modo transação (ADR): sem `SET` de sessão, sem advisory locks de sessão, cuidado com *prepared statements* (use o modo do driver compatível) e `SET LOCAL` dentro da transação.
- **Readiness** da aplicação verifica o banco; **liveness nunca** (regra de probes do `CLAUDE.md`).
- Aplicação usa `application_name` identificável no DSN (aparece no `log_line_prefix` e em `pg_stat_activity`).

## Configuração (`infra/docker/postgres/postgresql.conf`)

O arquivo é dimensionado para ~1 GB (container). Em produção recalcule pela RAM e pelo número de conexões:

| Parâmetro | Regra prática |
|---|---|
| `shared_buffers` | ~25% da RAM |
| `effective_cache_size` | ~50–75% da RAM (dica ao planejador, não aloca) |
| `work_mem` | por operação e por conexão: `RAM disponível / (max_connections × operações por query)`; suba por sessão/role para consultas pesadas, não global |
| `maintenance_work_mem` | maior para criar índice e VACUUM (inclui build de HNSW) |
| `random_page_cost` | 1.1 em SSD |
| `max_wal_size` / `checkpoint_completion_target` | evitar checkpoints frequentes; 0.9 |
| `idle_in_transaction_session_timeout`, `lock_timeout`, `statement_timeout` | defina `statement_timeout` por papel/serviço (ex.: `ALTER ROLE api_app SET statement_timeout = '30s'`), não global |

Mudança de parâmetro: registre o motivo, o valor anterior e a métrica que prova o ganho. Parâmetros que exigem restart (`shared_buffers`, `shared_preload_libraries`) vão para janela planejada.

## Observabilidade mínima

- **`pg_stat_statements`** (já ativo), `pg_stat_activity`, `pg_stat_user_tables/indexes`, `pg_locks`, `pg_stat_database` (taxa de *rollbacks*, deadlocks, `temp_files`).
- **Cache hit ratio** por tabela/índice; `temp_files`/`temp_bytes` indicam `work_mem` curto.
- **Locks bloqueantes:**

```sql
SELECT blocked.pid AS blocked_pid, left(blocked.query, 80) AS blocked_query,
       blocking.pid AS blocking_pid, left(blocking.query, 80) AS blocking_query
FROM pg_stat_activity blocked
JOIN LATERAL unnest(pg_blocking_pids(blocked.pid)) AS bp(pid) ON true
JOIN pg_stat_activity blocking ON blocking.pid = bp.pid;
```

- Logs já registram queries > 500 ms, esperas de lock, arquivos temporários e conexões. Alerte em: conexões próximas do limite, replicação com lag, autovacuum atrasado, crescimento anormal de disco, transações > N minutos, deadlocks.
- Cada serviço expõe métricas de pool (em uso, ociosas, espera) e latência de query.
