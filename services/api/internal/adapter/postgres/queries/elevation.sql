-- Modelo de elevação (ADR 0021): dado de referência global (sem company_id). Só o recorte
-- por talhão (GetFieldBounds, GetElevationWindow) lê a geometria do talhão, e filtra pela empresa.

-- name: HasElevationSource :one
SELECT EXISTS (SELECT 1 FROM elevation_sources WHERE tile_key = sqlc.arg(tile_key));

-- A ingestão de ~50 MB passa do statement_timeout padrão do pool, então a transação de
-- ingestão o sobe só para si (set_config local, vale até o fim da transação).
-- name: SetLocalStatementTimeout :exec
SELECT set_config('statement_timeout', sqlc.arg(value)::text, true);

-- Confere o GeoTIFF antes do tiling: uma banda, WGS84. Decodifica o arquivo inteiro.
-- name: InspectGeoTIFF :one
SELECT ST_NumBands(r)::int AS bands, ST_SRID(r)::int AS srid, ST_Width(r)::int AS width, ST_Height(r)::int AS height
FROM (SELECT ST_FromGDALRaster(sqlc.arg(geotiff)::bytea) AS r) d;

-- Ingere o bloco numa única instrução: a linha de fonte e os blocos entram juntos ou nada
-- entra. O ON CONFLICT DO NOTHING no tile_key resolve a corrida: o segundo ingest espera o
-- primeiro confirmar e então não insere nada (nem fonte, nem blocos). ST_Tile sem padding: o
-- bloco da borda fica menor, em vez de invadir (com nodata) o bloco vizinho de 1 grau. Se a
-- banda não declara nodata, declara -32768 (o Copernicus não declara): sem isso o recorte
-- preencheria a área de fora com o menor valor do tipo, que seria lido como altitude.
-- Devolve quantos blocos entraram (0 quando a chave já existia).
-- name: IngestElevationTile :one
WITH decoded AS MATERIALIZED (
    SELECT CASE WHEN ST_BandNoDataValue(r, 1) IS NULL THEN ST_SetBandNoDataValue(r, 1, -32768) ELSE r END AS rast
    FROM (SELECT ST_FromGDALRaster(sqlc.arg(geotiff)::bytea) AS r) d
), tiles AS MATERIALIZED (
    SELECT ST_Tile(rast, 256, 256) AS rast FROM decoded
), source AS (
    INSERT INTO elevation_sources (tile_key, source, version, block_count)
    SELECT sqlc.arg(tile_key)::text, sqlc.arg(source)::text, sqlc.arg(version)::text, count(*)::int FROM tiles
    ON CONFLICT (tile_key) DO NOTHING
    RETURNING tile_key, block_count
), blocks AS (
    INSERT INTO elevation_blocks (tile_key, rast)
    SELECT s.tile_key, t.rast FROM source s CROSS JOIN tiles t
    RETURNING id
)
SELECT (SELECT count(*) FROM blocks)::int AS stored_blocks,
       coalesce((SELECT sum(block_count) FROM source), 0)::int AS declared_blocks;

-- Caixa do talhão crescida por radius_m: buffer geográfico (metros reais) e depois a caixa em graus.
-- name: GetFieldBounds :one
SELECT ST_XMin(g)::float8 AS min_lon, ST_YMin(g)::float8 AS min_lat,
       ST_XMax(g)::float8 AS max_lon, ST_YMax(g)::float8 AS max_lat
FROM (
    SELECT ST_Buffer(x.polygon::geography, sqlc.arg(radius_m)::float8)::geometry AS g
    FROM fields x
    WHERE x.company_id = sqlc.arg(company_id) AND x.id = sqlc.arg(id) AND x.deleted_at IS NULL
) b;

-- Janela de elevação do talhão + raio. Só os blocos 256x256 que cruzam o buffer entram (o
-- índice GiST em st_convexhull(rast) filtra pela caixa; ST_Intersects tira os falsos positivos),
-- cada um é recortado ao buffer ANTES de unir (nunca materializa o bloco inteiro nem o de 1
-- grau) e os recortes viram um raster só. A máscara Inside é uma banda de 1s com a MESMA
-- grade da janela (ST_MakeEmptyRaster(raster) copia dimensões e georreferência), recortada
-- pelo polígono sem crop: 1 onde o centro da célula cai dentro do talhão, 0 fora.
-- covered: o buffer inteiro está dentro da união dos blocos (cada casco cresce 1e-9 grau, cerca
-- de 0,1 mm, para fechar a fresta de ponto flutuante entre blocos vizinhos). Sem ela, a janela
-- seria silenciosamente menor que o pedido.
-- Valores e máscara saem em arrays de uma dimensão, linha a linha (unnest de matriz é row-major);
-- nodata vira NaN no valor e 0 na máscara.
-- name: GetElevationWindow :one
WITH f AS (
    SELECT x.polygon AS poly,
           ST_Buffer(x.polygon::geography, sqlc.arg(radius_m)::float8)::geometry AS buf
    FROM fields x
    WHERE x.company_id = sqlc.arg(company_id) AND x.id = sqlc.arg(id) AND x.deleted_at IS NULL
), hits AS (
    SELECT b.rast, f.buf
    FROM f
    JOIN elevation_blocks b ON st_convexhull(b.rast) && f.buf AND ST_Intersects(st_convexhull(b.rast), f.buf)
), cover AS (
    SELECT coalesce(ST_Covers(ST_Union(ST_Expand(st_convexhull(rast), 1e-9)), (SELECT buf FROM f)), false) AS covered
    FROM hits
), win AS (
    SELECT ST_Union(ST_Clip(rast, 1, buf, true)) AS rast FROM hits
), mask AS (
    SELECT ST_Clip(ST_AddBand(ST_MakeEmptyRaster(w.rast), '8BUI'::text, 1, 0), 1, f.poly, 0::float8, false) AS rast
    FROM win w, f
    WHERE w.rast IS NOT NULL
)
SELECT (SELECT covered FROM cover)::bool AS covered,
       coalesce(ST_Width(w.rast), 0)::int AS cols,
       coalesce(ST_Height(w.rast), 0)::int AS rows,
       coalesce(ST_UpperLeftX(w.rast), 0)::float8 AS origin_lon,
       coalesce(ST_UpperLeftY(w.rast), 0)::float8 AS origin_lat,
       coalesce(ST_ScaleX(w.rast), 0)::float8 AS scale_x,
       coalesce(ST_ScaleY(w.rast), 0)::float8 AS scale_y,
       coalesce(ARRAY(SELECT coalesce(v, 'NaN'::float8) FROM unnest(ST_DumpValues(w.rast, 1)) WITH ORDINALITY AS u(v, i) ORDER BY i), '{}')::float8[] AS elevations,
       coalesce(ARRAY(SELECT coalesce(v, 0::float8) FROM unnest(ST_DumpValues(m.rast, 1)) WITH ORDINALITY AS u(v, i) ORDER BY i), '{}')::float8[] AS inside
FROM win w
LEFT JOIN mask m ON true;
