-- Malha municipal do IBGE (ADR 0019): referência global, sem company_id.

-- Município que contém o ponto. Na divisa (ponto na borda de dois municípios) vence o
-- menor código, para a resposta ser estável.
-- name: LocateMunicipality :one
SELECT ibge_code, name, state
FROM municipalities
WHERE ST_Covers(boundary, ST_SetSRID(ST_MakePoint(sqlc.arg(lng)::float8, sqlc.arg(lat)::float8), 4326))
ORDER BY ibge_code
LIMIT 1;

-- Carga (papel api_migrator, nunca o api_app). ST_MakeValid corrige auto-interseções que a
-- generalização da malha às vezes produz; ST_Multi normaliza Polygon para MultiPolygon.
-- name: UpsertMunicipality :exec
INSERT INTO municipalities (ibge_code, name, state, boundary, loaded_at)
VALUES (sqlc.arg(ibge_code), sqlc.arg(name), sqlc.arg(state),
        ST_Multi(ST_CollectionExtract(ST_MakeValid(ST_SetSRID(ST_GeomFromGeoJSON(sqlc.arg(boundary)::text), 4326)), 3)),
        sqlc.arg(loaded_at))
ON CONFLICT (ibge_code) DO UPDATE
SET name = EXCLUDED.name, state = EXCLUDED.state, boundary = EXCLUDED.boundary, loaded_at = EXCLUDED.loaded_at;

-- Remove os municípios que a nova carga não trouxe (fusões e extinções).
-- name: DeleteMunicipalitiesNotIn :exec
DELETE FROM municipalities WHERE NOT (ibge_code = ANY(sqlc.arg(codes)::text[]));
