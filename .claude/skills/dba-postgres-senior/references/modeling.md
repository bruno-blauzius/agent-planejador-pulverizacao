# Modelagem de dados no PostgreSQL

Exemplos usam o vocabulário do projeto. São ponto de partida, não schema aprovado: o schema real nasce do documento da fase em `docs/projetos/` e do contrato em `services/api/api/openapi.yaml`.

## Tipos: escolhas padrão

| Dado | Use | Evite | Motivo |
|---|---|---|---|
| Chave primária | `bigint GENERATED ALWAYS AS IDENTITY` ou `uuid` (`gen_random_uuid()`) | `serial`, `int` | `serial` é legado; `int` estoura. UUID quando o id é exposto na API/URL ou gerado fora do banco |
| Instante no tempo | `timestamptz` (armazenado em UTC) | `timestamp` sem zona | O servidor roda em UTC; `timestamp` perde o fuso e gera bugs de horário/janela |
| Data civil (ex.: data da safra) | `date` | `timestamptz` | Não tem hora nem fuso |
| Quantidade/dose/área/dinheiro | `numeric(p,s)` | `float`, `real` | Sem erro de arredondamento. Dose sempre com unidade em coluna própria (glossário: "Dose sempre com unidade explícita") |
| Medida física observada (Delta T, vento) | `numeric` ou `double precision` | — | Se for só dado observado/calculado, `double precision` é aceitável |
| Texto | `text` + `CHECK (length(...))` quando precisar limitar | `varchar(n)` por hábito | Mesmo desempenho, sem migração para aumentar limite |
| Booleano | `boolean NOT NULL DEFAULT false` | `smallint`, `char(1)` | — |
| Conjunto fechado | `text` + `CHECK (col IN (...))` ou tabela de domínio com FK | `ENUM` nativo | `ENUM` é difícil de alterar/remover valor; `CHECK` muda com migração leve |
| Payload semiestruturado | `jsonb` | `json` | Indexável; use só para dado realmente variável, nunca para fugir de modelar colunas |
| Período (janela de pulverização) | `tstzrange` + `EXCLUDE USING gist` | duas colunas sem regra | Impede sobreposição pelo banco (requer `btree_gist` → ADR de extensão) |
| Geometria do talhão | Ver "Geoespacial" abaixo | — | Decisão arquitetural |
| E-mail | `UNIQUE (company_id, lower(email))` ou `citext` | comparação case-sensitive | `citext` é extensão (ADR); índice em `lower(email)` não exige extensão |

## Regras de modelagem

1. **Normalize até 3FN** e desnormalize só com evidência de desempenho medida. Colunas derivadas (ex.: "área total da fazenda") são calculadas ou mantidas pela aplicação com teste; evite triggers.
2. **Toda coluna que não pode ser nula é `NOT NULL`.** Nulo significa "desconhecido", não "vazio". Defaults explícitos.
3. **Constraints nomeadas:** `pk_<tabela>`, `fk_<tabela>_<referencia>`, `uq_<tabela>_<colunas>`, `ck_<tabela>_<regra>`, `ix_<tabela>_<colunas>`. Facilita migração e mensagem de erro.
4. **FKs com ação explícita** (`ON DELETE RESTRICT` por padrão; `CASCADE` só para composição verdadeira, como itens de um agregado). Sempre indexe a coluna da FK no lado filho.
5. **Invariantes físicas e de integridade no banco** são bem-vindas: `CHECK (area_hectares > 0)`, `CHECK (dose_amount > 0)`, unicidade de `(company_id, name)`. **Política agronômica** (limites de Delta T, vento, chuva) não entra em `CHECK`: é configuração e regra de `domain`.
6. **Soft delete** só quando há requisito (auditoria, restauração). Use `deleted_at timestamptz` + índice único parcial (`WHERE deleted_at IS NULL`) e lembre que todo filtro precisa dele. Para aplicações já registradas, prefira estados (`status`) a apagar.
7. **Colunas de auditoria** em tabelas de negócio: `created_at timestamptz NOT NULL DEFAULT now()`, `updated_at` mantido pela aplicação (ou trigger mínimo e genérico, sem regra de negócio), e `created_by`/`updated_by` quando o requisito pedir.
8. **Concorrência otimista** em registros editados por várias pessoas: coluna `version integer NOT NULL DEFAULT 1` e `UPDATE ... WHERE id = $1 AND version = $2`, devolvendo conflito quando 0 linhas.
9. **Idempotência** em escrita vinda de agente ou fila: coluna/constraint única de chave de idempotência (a aplicação registrada após aprovação humana não pode duplicar se a chamada for repetida).
10. **Nomes:** tabelas no plural (`fields`, `applications`), colunas `snake_case`, FK como `<entidade>_id`, booleanos como `is_`/`has_`, instantes terminando em `_at`, datas em `_on`.

## Multi-tenancy (ADR 0013)

- Toda tabela de negócio da API tem `company_id <tipo do id> NOT NULL REFERENCES companies(id)`.
- **Filhos herdam o tenant de forma verificável.** Para impedir que um talhão da empresa A aponte para fazenda da empresa B, use FK composta: `UNIQUE (company_id, id)` no pai e `FOREIGN KEY (company_id, farm_id) REFERENCES farms (company_id, id)` no filho. O banco passa a recusar vazamento entre tenants mesmo com bug na aplicação.
- Índices e unicidades começam por `company_id`: `UNIQUE (company_id, name)`, `INDEX (company_id, farm_id)`.
- Escopo de fazendas do operador de planejamento: tabela de vínculo `user_farm_access (company_id, user_id, farm_id)` com PK composta, em vez de array em coluna.
- **RLS** (defesa em profundidade): `ALTER TABLE ... ENABLE ROW LEVEL SECURITY` com policy por `current_setting('app.company_id')`. Pontos de atenção: o dono da tabela (`migrator`) ignora RLS a menos que `FORCE ROW LEVEL SECURITY`; com pool de conexões, use `SET LOCAL` dentro da transação (nunca `SET` de sessão); teste que sem a variável definida nada é retornado. Adotar RLS exige ADR e teste de isolamento.
- Usuários, convites e papéis (`super_admin`, `admin`, `planning_operator`) são dados da `api`; o agente recebe o contexto pela API, não pelo banco.

## Geoespacial (talhões no mapa)

O front usa MapLibre (ADR 0012) e precisará de geometria de talhões e fazendas. O PostGIS já está na imagem e habilitado no banco `api` (ADR 0014); as opções de uso são:

1. **`jsonb` com GeoJSON** — sem recurso espacial; serve para armazenar e devolver à API; sem índice espacial nem cálculos geométricos. Suficiente se o banco só guarda e entrega.
2. **PostGIS** (`geometry(MultiPolygon, 4326)`, índice GiST, `ST_Area(geography)`, `ST_Intersects`, `ST_GeomFromGeoJSON`) — correto se houver consulta espacial (talhão que contém um ponto, sobreposição, área real). Já disponível; o papel `app` o usa sem privilégio extra.
3. **Colunas de centro (`latitude`/`longitude`) + GeoJSON só para exibição** — pragmático para a primeira fase.

Recomende a opção mais simples que atenda às consultas reais. Com PostGIS: SRID 4326 (WGS84), `geometry(MultiPolygon, 4326) NOT NULL`, índice GiST (`CREATE INDEX CONCURRENTLY ... USING gist`) e `CHECK (ST_IsValid(boundary))`. Para hectares, calcule com `ST_Area(boundary::geography) / 10000` (esferoide) ou projete para um SRID métrico da região (ex.: SIRGAS 2000 / Brazil Polyconic, 5880); os dois diferem um pouco, então escolha um e valide em teste. A API entrega GeoJSON ao front (`ST_AsGeoJSON`), que é o que o MapLibre consome.

## Histórico e dados que mudam

- Registro de aplicação (`applications`) é um **fato**: imutável depois de confirmado. Correções criam novo registro ou estado, preservando o original e quem aprovou.
- Para histórico de alterações de cadastro, tabela de eventos/auditoria append-only (`entity`, `entity_id`, `actor_id`, `changed_at`, `diff jsonb`) é preferível a versionar linhas, salvo requisito de consulta "como era em X".
- Dados de clima são lidos da API de clima; guarde no banco só o que for necessário (ex.: snapshot da janela recomendada que embasou uma aplicação), com a fonte e o instante da leitura.

## Particionamento e volume

- Não particione "por precaução". Considere quando uma tabela passar de dezenas de GB e houver chave natural de poda (tempo) ou necessidade de descarte barato por período (`DROP PARTITION`).
- Use `PARTITION BY RANGE (created_at)` em tabelas append-only (eventos, auditoria, logs de agente). A PK/unicidade precisa incluir a chave de partição. Crie partições futuras por rotina, com monitoramento.
- Antes disso, índices parciais, BRIN em colunas correlacionadas ao tempo e retenção por `DELETE` em lotes resolvem a maior parte dos casos.

## Anti-padrões a recusar

- EAV (entidade-atributo-valor) e "tabelão" com `jsonb` para tudo.
- Array/CSV de ids em coluna no lugar de tabela de vínculo.
- `float` para dose, área ou dinheiro.
- Hash de senha feito no SQL (`pgcrypto`): o hash é feito na aplicação com argon2id/bcrypt.
- Chave primária natural mutável (e-mail, nome de fazenda).
- Trigger com regra de negócio ou chamada de rede.
- Colunas `status` livres sem `CHECK` ou tabela de domínio.
- Datas como `text`.
