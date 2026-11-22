# Migrações seguras

Princípio: a migração roda com o sistema em uso e a versão anterior da aplicação ainda ativa durante o deploy (rolling). Toda mudança precisa ser compatível com **duas versões da aplicação ao mesmo tempo**.

## Regras gerais

1. **Imutável depois de aplicada.** Erro em migração já executada em qualquer ambiente compartilhado se corrige com uma nova.
2. **Um arquivo, uma responsabilidade.** Misturar DDL pesado, backfill e índice no mesmo arquivo dificulta rollback e estende locks.
3. **Sempre `up` e `down`**, e teste `up` → `down` → `up` em banco descartável. Quando `down` for realmente irreversível (DROP de dado), declare isso no arquivo e exija backup verificado antes.
4. **Rode com o papel `<svc>_migrator`**, nunca com o `app` nem com o superusuário. Objetos criados pelo migrator recebem privilégios do `app` por `ALTER DEFAULT PRIVILEGES` (já configurado no init); se criar schema novo, replique os default privileges e o `GRANT USAGE`.
5. **Defina `lock_timeout` na própria migração** quando o DDL precisar de lock forte (`SET LOCAL lock_timeout = '3s';`), para falhar rápido em vez de enfileirar todas as queries atrás de si. O ambiente já tem `lock_timeout = 10s` global; não aumente.
6. **Nada de regra de negócio em migração** e nada de dados de teste. Dados de referência (domínios fixos) entram de forma idempotente (`INSERT ... ON CONFLICT DO NOTHING`).
7. **Comente o porquê** de decisões não óbvias no próprio arquivo.
8. **Extensões não são criadas em migração** (exigem superusuário): ficam em `infra/docker/postgres/init/` e no provisionamento de produção. A migração pode assumir que a extensão existe e falhar com mensagem clara se não existir.

## Mapa de locks dos DDL mais comuns

| Operação | Lock / efeito | Forma segura |
|---|---|---|
| `CREATE INDEX` | `SHARE`: bloqueia escritas | `CREATE INDEX CONCURRENTLY` (não roda em transação) |
| `DROP INDEX` | `ACCESS EXCLUSIVE` | `DROP INDEX CONCURRENTLY` |
| `REINDEX` | bloqueia | `REINDEX INDEX CONCURRENTLY` |
| `ADD COLUMN` nullable, sem default | rápido (metadado) | direto |
| `ADD COLUMN ... DEFAULT <constante>` | rápido no PG 11+ (sem reescrita) | direto; default volátil (`random()`; `now()` é estável) pode reescrever |
| `ALTER COLUMN TYPE` | reescreve a tabela com `ACCESS EXCLUSIVE` (algumas conversões não) | expand/contract: coluna nova, backfill em lotes, troca |
| `SET NOT NULL` | varre a tabela com `ACCESS EXCLUSIVE` | `ADD CONSTRAINT ... CHECK (col IS NOT NULL) NOT VALID` → `VALIDATE CONSTRAINT` → `SET NOT NULL` (PG 12+ reaproveita o check) → remove o check |
| `ADD FOREIGN KEY` | bloqueia as tabelas durante a validação | `ADD CONSTRAINT ... NOT VALID` e depois `VALIDATE CONSTRAINT` (lock leve) |
| `ADD CHECK` | varre a tabela | `NOT VALID` + `VALIDATE CONSTRAINT` |
| `ADD UNIQUE` | constrói índice com lock | `CREATE UNIQUE INDEX CONCURRENTLY` + `ADD CONSTRAINT ... UNIQUE USING INDEX` |
| `RENAME` coluna/tabela | rápido, mas quebra a versão antiga do app | expand/contract |
| `DROP COLUMN` | rápido, mas quebra quem ainda usa | só depois de nenhuma versão usar a coluna |
| `TRUNCATE` | `ACCESS EXCLUSIVE` | evite em tabela viva |

`NOT VALID` + `VALIDATE` adiciona a restrição sem varrer sob lock forte; a validação roda com lock que permite leitura e escrita.

## Expand / Contract (mudanças incompatíveis)

Ex.: renomear `fields.area` para `fields.area_hectares`.

1. **Expand (migração 1 + deploy A):** adicione a coluna nova (nullable). A aplicação passa a escrever nas duas e a ler da nova com fallback.
2. **Backfill (migração 2, ou job):** copie em lotes (ver abaixo).
3. **Constraints:** aplique `NOT NULL`/`CHECK` na coluna nova via `NOT VALID` + `VALIDATE`.
4. **Deploy B:** a aplicação só usa a coluna nova.
5. **Contract (migração 3, depois do deploy B estável):** remova a coluna antiga.

Nunca renomeie/remova no mesmo deploy em que a aplicação para de usar.

## Backfill em lotes

```sql
-- Repetir até 0 linhas afetadas, em transações curtas.
WITH batch AS (
  SELECT id FROM fields
  WHERE area_hectares IS NULL
  ORDER BY id
  LIMIT 5000
  FOR UPDATE SKIP LOCKED
)
UPDATE fields f
SET area_hectares = f.area
FROM batch
WHERE f.id = batch.id;
```

Um `UPDATE` gigante gera bloat, WAL enorme e lag de réplica. Faça pausa entre lotes e monitore. Em tabelas pequenas (desenvolvimento, primeiras fases) um `UPDATE` único é aceitável; declare a premissa de volume.

## golang-migrate (`services/api/migrations/`)

- Arquivos `NNNNNN_descricao.up.sql` / `NNNNNN_descricao.down.sql`, numeração consistente no repositório.
- O driver do Postgres executa o arquivo em uma única chamada, e vários comandos num mesmo arquivo viram uma transação implícita: **`CREATE INDEX CONCURRENTLY` deve ficar sozinho em seu arquivo, com um único comando**. Valide isso no ambiente antes de contar com o comportamento.
- A tabela `schema_migrations` guarda versão e flag `dirty`. Migração que falha no meio deixa `dirty = true`: corrija o estado (`migrate force <versão>`) só depois de entender o que foi aplicado.
- Em produção, migração roda como etapa separada do deploy (job/init), com o papel migrator, **não** no startup de cada réplica da aplicação (corrida entre réplicas e privilégio excessivo em runtime). Use advisory lock se precisar serializar.
- Linha de comando típica, a confirmar quando a Fase 3 introduzir a ferramenta: `migrate -path services/api/migrations -database "$MIGRATOR_URL" up`. Se não houver alvo no `Makefile`, crie-o em vez de improvisar e atualize o `CLAUDE.md`.

## Alembic (`services/label-advisor/alembic/`)

- Comandos: `uv run alembic revision -m "descricao"`, `uv run alembic upgrade head`, `uv run alembic downgrade -1`, `uv run alembic history`, `uv run alembic current`.
- Prefira SQL explícito em `op.execute(...)`. Para índice concorrente: `op.create_index(..., postgresql_concurrently=True)` dentro de `with op.get_context().autocommit_block():`.
- **Não use `--autogenerate` às cegas**: ele não vê tudo (CHECKs, índices parciais, tipos de vetor, ordem de colunas) e pode gerar `DROP` indevido. Revise cada operação; para tipos `vector`, escreva a migração à mão.
- O `env.py` conecta com o papel `migrator` (URL de migração separada da URL do serviço). A aplicação em runtime usa `label_advisor_app`.
- Mantenha uma única *head*; conflito de heads se resolve com `alembic merge`.
- Alembic e eventuais modelos SQLAlchemy ficam em `adapters/`; nunca os importe em `domain/` ou `application/`.

## Verificação de uma migração (faça sempre)

Use um banco descartável, isolado do volume de desenvolvimento:

```bash
docker run --rm -d --name pg-migration-test -e POSTGRES_PASSWORD=test agentes-agro/postgres:dev   # build: docker compose build agro-postgres
# aplicar up, conferir, aplicar down, aplicar up de novo (sem publicar porta:
# no Windows algumas portas altas são reservadas; use `docker exec -i pg-migration-test psql -U postgres`)
# conferir objetos e privilégios do papel app: \d+ tabela, \dp tabela, \di+ tabela
docker rm -f pg-migration-test
```

Depois, `EXPLAIN` das queries afetadas com volume sintético realista (`generate_series`) e confira o plano.

## Checklist de revisão de migração

- [ ] Compatível com a versão anterior da aplicação (rolling deploy)?
- [ ] Qual lock e por quanto tempo? Reescreve a tabela? Tamanho da tabela em produção?
- [ ] Índices em tabela existente com `CONCURRENTLY`, em arquivo/bloco sem transação?
- [ ] Constraints novas com `NOT VALID` + `VALIDATE` quando a tabela já tem dados?
- [ ] `down` existe e foi testado? Se perde dado, está declarado?
- [ ] Backfill em lotes e idempotente?
- [ ] Privilégios do papel `app` corretos nos objetos novos?
- [ ] `company_id`, FK composta e índice com `company_id` na frente?
- [ ] Nenhuma regra de negócio, nenhum dado de teste, nenhuma extensão criada aqui?
