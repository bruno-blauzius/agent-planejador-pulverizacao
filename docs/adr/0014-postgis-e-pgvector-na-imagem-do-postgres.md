# ADR 0014: PostGIS e pgvector na imagem do Postgres

- **Status:** proposto
- **Data:** 2026-11-21

## Contexto

O front desenha e exibe talhões e fazendas em mapa (MapLibre, ADR 0012), e o ADR 0013 faz de `Company` o tenant da `api`. A `api` precisará guardar a geometria dos talhões e, provavelmente, consultar por ela (talhão que contém um ponto, sobreposição, área real em hectares). O `label-advisor` já usa pgvector (ADR 0009).

A imagem atual, `pgvector/pgvector:pg17`, não traz o PostGIS. Extensões exigem superusuário e os binários precisam existir no servidor, então a escolha afeta a imagem, o init e a produção.

## Opções

1. **`jsonb` com GeoJSON**, sem extensão nova. Guarda e devolve o polígono, mas sem índice espacial nem cálculo de área ou interseção no banco.
2. **Imagem própria: `pgvector/pgvector:pg17` + pacote PostGIS** (escolhida). Mantém a base já usada e acrescenta só o PostGIS.
3. **Partir de `postgis/postgis` e instalar pgvector.** Equivalente, mas troca a base já validada pela do ADR 0009.
4. **Um servidor Postgres por extensão.** Isola, mas dobra a operação local sem ganho: as extensões não conflitam.

## Decisão

- Imagem própria `agentes-agro/postgres:dev`, construída por `infra/docker/postgres/Dockerfile` a partir de `pgvector/pgvector:pg17`, instalando `postgresql-17-postgis-3` e `postgresql-17-postgis-3-scripts` do repositório PGDG. O compose passa a usar `build:` em `agro-postgres`.
- **Cada extensão no banco do serviço que a usa**, criada no init (exige superusuário): `vector` em `label_advisor`, `postgis` em `api`. As extensões não coexistem no mesmo banco, mas a imagem tem as duas. Respeita "cada dado tem um único dono".
- O PostGIS fica disponível para a `api`; **usá-lo no schema** (`geometry(MultiPolygon, 4326)`, índice GiST) é decisão de cada migração, com SRID 4326 (WGS84). Para área em hectares use `geography` ou projeção adequada, e valide em teste.
- O papel `<svc>_app` continua só com DML: o teste com `api_app` confirmou que ele usa `ST_*` e lê `spatial_ref_sys` sem privilégio extra.

## Consequências

**Positivas**
- Consulta espacial e índices GiST disponíveis na `api` sem serviço novo.
- Nenhuma mudança para o `label-advisor`.

**Negativas / custos**
- Imagem local maior (~985 MB) e build na primeira subida (`make up` passa a construir).
- **Volume existente não recebe a extensão:** o init só roda com volume vazio. Em um banco já criado, com a imagem nova rodando, execute como superusuário: `docker compose exec agro-postgres psql -U <superusuário> -d api -c 'CREATE EXTENSION IF NOT EXISTS postgis;'`. Não é preciso `down -v`.
- Trocar a imagem de um volume existente exige o mesmo major (17). Mudar de major exige dump/restore ou `pg_upgrade`.
- **Produção:** o banco gerenciado precisa oferecer PostGIS e pgvector na versão do Postgres escolhida; confirmar antes de fixar o provedor.
- Atualizações do PostGIS entram por rebuild da imagem; versão e CVEs devem ser acompanhados como os da imagem base.
- O pacote é instalado sem fixar a versão menor; fixar quando houver pipeline de imagens.
