# pgvector e busca para RAG (`label-advisor`)

Contexto: o `label-advisor` responde sobre bulas e boletins técnicos e **toda resposta cita documento e página**; se os trechos recuperados não sustentam a resposta, o agente recusa (`CLAUDE.md`, regra 8). O banco é `label_advisor`, na imagem `agentes-agro/postgres` (base `pgvector/pgvector:pg17`, que também traz PostGIS para a `api`), extensão `vector` criada no init. O fluxo de chunking, extração de PDF (Strategy) e o LLM ficam em `adapters/`; o banco guarda o resultado.

## Schema de referência (ponto de partida, a ser validado na Fase 1)

```sql
-- documents: um registro por PDF/boletim ingerido
CREATE TABLE documents (
  id            bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  kind          text        NOT NULL CHECK (kind IN ('product_label', 'technical_bulletin')),
  title         text        NOT NULL,
  source_uri    text        NOT NULL,
  content_hash  text        NOT NULL,                 -- idempotência da ingestão
  page_count    integer     NOT NULL CHECK (page_count > 0),
  ingested_at   timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT uq_documents_content_hash UNIQUE (content_hash)
);

-- chunks: unidade recuperável, sempre rastreável a documento e página
CREATE TABLE chunks (
  id               bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  document_id      bigint   NOT NULL REFERENCES documents (id) ON DELETE CASCADE,
  page_number      integer  NOT NULL CHECK (page_number > 0),
  chunk_index      integer  NOT NULL,
  content          text     NOT NULL,
  content_tsv      tsvector GENERATED ALWAYS AS (to_tsvector('portuguese', content)) STORED,
  embedding        vector(<dim>) NOT NULL,           -- dim fixa pelo modelo de embedding
  embedding_model  text     NOT NULL,                -- ex.: nome+versão; permite reindexar
  CONSTRAINT uq_chunks_document_position UNIQUE (document_id, page_number, chunk_index)
);
```

Decisões embutidas (justifique no ADR do modelo de embedding):

- **`page_number NOT NULL`** e `document_id` obrigatórios: sem eles não há citação. Metadado de produto (nome comercial, princípio ativo) vai em colunas/tabelas próprias quando for filtro, não em `jsonb` opaco.
- **`embedding_model`** por chunk: trocar de modelo exige reindexar tudo; a coluna evita misturar vetores incomparáveis e permite migração gradual.
- **Dimensão** é parte do tipo (`vector(1536)` etc.). Trocar de modelo com outra dimensão é migração (coluna nova + reindex + troca), nunca `ALTER` silencioso.
- **`content_hash`** faz a ingestão ser idempotente: reprocessar o mesmo PDF não duplica.
- Se um documento for reprocessado com nova estratégia de chunking, versione por `ingest_run` ou substitua em transação (delete + insert) para nunca deixar corpus pela metade visível.

## Tipos e operadores

| Tipo | Uso |
|---|---|
| `vector(n)` | float32, até 2.000 dimensões indexáveis por HNSW/IVFFlat |
| `halfvec(n)` | float16: metade do espaço; indexa até 4.000 dimensões |
| `sparsevec` | vetores esparsos |
| `bit(n)` | quantização binária |

Operadores/ops classes: `<=>` distância de cosseno (`vector_cosine_ops`), `<->` L2 (`vector_l2_ops`), `<#>` produto interno negativo (`vector_ip_ops`). **Use o operador que combina com o índice e com o que o modelo de embedding recomenda** (a maioria dos modelos de texto: cosseno; se os vetores forem normalizados, produto interno e cosseno ordenam igual).

Similaridade de cosseno = `1 - (embedding <=> query)`. Limiares de similaridade são configuração, calibrados pelos evals, nunca números mágicos na query.

## Índice vetorial

Sem índice, a busca é exata (seq scan): correta e aceitável para poucos milhares de chunks (corpus de estudo). Crie índice aproximado quando a latência pedir.

**HNSW** (padrão recomendado): melhor *recall* × velocidade, não precisa de dados para treinar, mais caro de construir e em memória.

```sql
-- m e ef_construction: tradeoff entre recall, tempo de build e tamanho. Valores
-- iniciais; ajuste medindo recall nos evals.
CREATE INDEX CONCURRENTLY ix_chunks_embedding_hnsw
  ON chunks USING hnsw (embedding vector_cosine_ops)
  WITH (m = 16, ef_construction = 64);
```

- Busca: `SET LOCAL hnsw.ef_search = <n>;` dentro da transação da consulta (padrão 40; precisa ser ≥ `LIMIT`). Mais alto = mais recall e mais lento. Vem de configuração.
- Build: aumente `maintenance_work_mem` na sessão que cria o índice (índice que não cabe em memória constrói muito mais devagar) e use `max_parallel_maintenance_workers`.

**IVFFlat**: build rápido e leve, exige dados já carregados (centróides), recall depende de `lists` e `probes`; ajuste `ivfflat.probes`. Prefira HNSW, salvo restrição de memória/tempo de build.

Observação operacional: índices aproximados podem **retornar menos linhas que o `LIMIT`** quando há filtro (o filtro é aplicado depois da busca no índice). Alternativas: `SET LOCAL hnsw.iterative_scan = relaxed_order` (pgvector ≥ 0.8), aumentar `ef_search`, índice parcial por categoria, ou particionamento por `kind`/produto. Verifique a versão instalada da extensão (`SELECT extversion FROM pg_extension WHERE extname='vector';`) antes de usar recurso novo.

## Consulta de recuperação

```sql
-- $1 = embedding da pergunta; $2 = top_k (configuração); sempre parametrizado.
SELECT c.id, c.document_id, d.title, c.page_number, c.content,
       1 - (c.embedding <=> $1) AS similarity
FROM chunks c
JOIN documents d ON d.id = c.document_id
WHERE c.embedding_model = $3
ORDER BY c.embedding <=> $1
LIMIT $2;
```

- `ORDER BY embedding <=> $1 LIMIT k` é o formato que o índice atende; não envolva a coluna em expressão.
- O retorno inclui `document` e `page_number` para a citação. A aplicação aplica o corte por limiar e decide recusar quando nada passa (regra de domínio da aplicação, não da query).
- Saída compacta para o agente: devolva trechos e metadados, não o corpus.

## Busca híbrida (vetorial + textual)

Termos exatos importam em bula (nomes comerciais, princípios ativos, doses, siglas) e embeddings falham neles. Combine:

```sql
CREATE INDEX CONCURRENTLY ix_chunks_content_tsv ON chunks USING gin (content_tsv);
```

- Recupere N candidatos por cada via (vetorial e `content_tsv @@ websearch_to_tsquery('portuguese', $q)` ordenados por `ts_rank_cd`) e funda por **Reciprocal Rank Fusion** (RRF): `score = Σ 1 / (k + rank)`, com `k` configurável (~60).
- A configuração de texto `portuguese` já é o default do servidor (`default_text_search_config`); mantenha-a explícita no `to_tsvector`/`tsquery` para a coluna gerada ser determinística.
- Considere `unaccent` (extensão → ADR) para casar com/sem acento. Faça a fusão na aplicação ou em CTE única; a regra de fusão é parâmetro de configuração testado por eval.

## Ingestão, reindexação e consistência

- Ingestão em transação por documento; chunks e embeddings entram juntos. Inserção em lote (`COPY` ou `INSERT` multi-linha) com tamanho de lote configurável.
- Com o índice HNSW já criado, inserções em massa são mais lentas: para carga inicial grande, carregue e crie o índice depois.
- Trocar o modelo de embedding: nova coluna (ou tabela) + reindex em lotes + validação por evals + troca atômica do que a consulta lê + remoção da antiga. Registre o ADR.
- `VACUUM`/`ANALYZE` depois de cargas grandes; reindexe HNSW (`REINDEX INDEX CONCURRENTLY`) se houver muito `DELETE`.

## Avaliação e custo

- **Recall do índice aproximado** se mede contra a busca exata (desligue o índice na sessão: `SET LOCAL enable_indexscan = off;`) em amostra de perguntas; reporte recall@k junto com a latência.
- Mudou chunking, modelo, `top_k`, limiar, `ef_search` ou a fusão híbrida? **Rode `make eval` antes e depois e reporte o placar.** Nunca altere o dataset de avaliação para passar.
- Tamanho: `vector(1536)` ≈ 6 KB por linha; 100 mil chunks ≈ 600 MB só de vetores, e o índice HNSW costuma ser de tamanho similar. Dimensione `shared_buffers`/RAM para o índice caber em memória. `halfvec` reduz pela metade com perda de precisão geralmente aceitável (valide nos evals).
- Evite que o prompt dependa do banco: o LLM e o *embedder* são adaptadores de saída; a porta de recuperação (`Retriever`) é um `Protocol` na `application`, a implementação com pgvector fica em `adapters/outbound/`.
