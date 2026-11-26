# ADR 0017: Busca sem acento com a extensão unaccent

- **Status:** aceito
- **Data:** 2026-11-23

## Contexto

A busca de fazendas (nome, código, município) e de proprietários (nome) usava `ILIKE`. Ela ignora maiúsculas, mas não acentos: "guaxupe" não encontrava "Guaxupé", e "conceicao" não encontrava "Conceição". Em nomes brasileiros, isso faz a busca falhar justamente nos casos mais comuns. O DER (pendência 7) já apontava que resolver exigiria uma extensão, e extensão nova exige ADR (ADR 0009).

## Opções

1. **Extensão `unaccent`** (escolhida). Vem no `contrib` da imagem do Postgres; a consulta compara `unaccent(coluna) ILIKE unaccent(busca)`.
2. **Coluna normalizada mantida pela aplicação**: uma `search_name` em cada tabela, preenchida em Go. Funciona sem extensão, mas exige coluna, backfill e código de normalização em duas tabelas.
3. **Manter o `ILIKE`**, aceitando a falha.

## Decisão

- `CREATE EXTENSION unaccent` no banco `api`, no init do Postgres (`infra/docker/postgres/init/01-create-service-databases.sh`), porque a extensão exige superusuário.
- A migração `000016_require_unaccent` só verifica a extensão e falha com uma mensagem clara quando ela falta, como a `000004` faz com o PostGIS.
- As consultas de fazendas e proprietários usam `unaccent(coluna) ILIKE unaccent(busca) ESCAPE '\'`. O padrão de busca continua parametrizado; `unaccent` não mexe em `%`, `_` nem `\`.
- Usuários, convites e empresas mantêm o `ILIKE`; a mudança é aplicada quando alguma dessas telas pedir.
- Não há índice: a busca por "contém" (`%termo%`) não usa índice B-tree com ou sem `unaccent`. Com volume, a evolução é `pg_trgm` com um wrapper `IMMUTABLE`, assunto de outro ADR.

## Consequências

**Positivas**
- A busca encontra nomes com ou sem acento, sem coluna nova e sem código de normalização.

**Negativas / custos**
- **Volume existente não recebe a extensão.** O init só roda com volume vazio. Antes de `make migrate-up`, rode uma vez como superusuário: `docker compose exec agro-postgres psql -U <superusuário> -d api -c 'CREATE EXTENSION IF NOT EXISTS unaccent;'`.
- O banco gerenciado de produção precisa oferecer `unaccent`. Os principais oferecem.
- O fake em memória dos testes (`fakes.Registry`) imita a busca com uma tabela própria de acentos. O teste de integração `TestRegistrySearchIgnoresAccents` é a referência do comportamento real.
