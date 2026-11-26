-- Talhões (DER, "fields"). Polígono entra e sai como GeoJSON (WGS84). Toda consulta filtra
-- pela empresa e pela fazenda; talhão removido (deleted_at) nunca aparece.

-- name: ListFields :many
SELECT x.id, x.company_id, x.farm_id, x.crop_code, c.name AS crop_name, x.name, x.block, x.variety,
       x.season, x.planting_date, x.row_spacing_m, x.plant_spacing_m,
       ST_AsGeoJSON(x.polygon, 7)::text AS polygon, x.area_ha::float8 AS area_ha,
       x.created_at, x.updated_at, x.version
FROM fields x
JOIN crops c ON c.code = x.crop_code
WHERE x.company_id = sqlc.arg(company_id) AND x.farm_id = sqlc.arg(farm_id) AND x.deleted_at IS NULL
ORDER BY x.block NULLS FIRST, x.name, x.id;

-- name: GetField :one
SELECT x.id, x.company_id, x.farm_id, x.crop_code, c.name AS crop_name, x.name, x.block, x.variety,
       x.season, x.planting_date, x.row_spacing_m, x.plant_spacing_m,
       ST_AsGeoJSON(x.polygon, 7)::text AS polygon, x.area_ha::float8 AS area_ha,
       x.created_at, x.updated_at, x.version
FROM fields x
JOIN crops c ON c.code = x.crop_code
WHERE x.company_id = sqlc.arg(company_id) AND x.farm_id = sqlc.arg(farm_id) AND x.id = sqlc.arg(id)
  AND x.deleted_at IS NULL;

-- name: LockField :one
SELECT x.id, x.company_id, x.farm_id, x.crop_code, c.name AS crop_name, x.name, x.block, x.variety,
       x.season, x.planting_date, x.row_spacing_m, x.plant_spacing_m,
       ST_AsGeoJSON(x.polygon, 7)::text AS polygon, x.area_ha::float8 AS area_ha,
       x.created_at, x.updated_at, x.version
FROM fields x
JOIN crops c ON c.code = x.crop_code
WHERE x.company_id = sqlc.arg(company_id) AND x.farm_id = sqlc.arg(farm_id) AND x.id = sqlc.arg(id)
  AND x.deleted_at IS NULL
FOR UPDATE OF x;

-- name: InsertField :exec
INSERT INTO fields (id, company_id, farm_id, crop_code, name, block, variety, season, planting_date,
                    row_spacing_m, plant_spacing_m, polygon, created_at, updated_at, version)
VALUES (sqlc.arg(id), sqlc.arg(company_id), sqlc.arg(farm_id), sqlc.arg(crop_code), sqlc.arg(name),
        sqlc.narg(block), sqlc.narg(variety), sqlc.narg(season), sqlc.narg(planting_date),
        sqlc.narg(row_spacing_m)::float8, sqlc.narg(plant_spacing_m)::float8,
        ST_SetSRID(ST_GeomFromGeoJSON(sqlc.arg(polygon)::text), 4326),
        sqlc.arg(created_at), sqlc.arg(updated_at), 1);

-- name: UpdateField :execrows
UPDATE fields
SET crop_code = sqlc.arg(crop_code), name = sqlc.arg(name), block = sqlc.narg(block),
    variety = sqlc.narg(variety), season = sqlc.narg(season), planting_date = sqlc.narg(planting_date),
    row_spacing_m = sqlc.narg(row_spacing_m)::float8, plant_spacing_m = sqlc.narg(plant_spacing_m)::float8,
    polygon = ST_SetSRID(ST_GeomFromGeoJSON(sqlc.arg(polygon)::text), 4326),
    updated_at = sqlc.arg(updated_at), version = version + 1
WHERE company_id = sqlc.arg(company_id) AND farm_id = sqlc.arg(farm_id) AND id = sqlc.arg(id)
  AND deleted_at IS NULL AND version = sqlc.arg(version);

-- name: FieldExists :one
SELECT EXISTS (SELECT 1 FROM fields WHERE company_id = $1 AND farm_id = $2 AND id = $3 AND deleted_at IS NULL);

-- Mede um polígono de talhão dentro da fazenda. Só mede o que é válido: ST_Difference
-- falha com geometria inválida. outside_m2 é 0 sem perímetro (a aplicação recusa antes).
-- name: MeasureField :one
WITH g AS (
    SELECT ST_SetSRID(ST_GeomFromGeoJSON(sqlc.arg(polygon)::text), 4326) AS geom
), v AS (
    SELECT geom, ST_IsValid(geom) AS valid FROM g
)
SELECT v.valid::bool AS valid,
       CASE WHEN v.valid THEN round((ST_Area(v.geom::geography) / 10000.0)::numeric, 4) ELSE 0 END::float8 AS area_ha,
       CASE WHEN v.valid AND f.perimeter IS NOT NULL
            THEN ST_Area(ST_Difference(v.geom, f.perimeter)::geography)
            ELSE 0 END::float8 AS outside_m2
FROM v
JOIN farms f ON f.company_id = sqlc.arg(company_id) AND f.id = sqlc.arg(farm_id) AND f.deleted_at IS NULL;

-- Outros talhões da fazenda que o polígono toca, com a área em comum (o índice GiST filtra
-- pelas caixas; a interseção mede a geometria). Chamar só com polígono válido.
-- name: ListFieldOverlaps :many
WITH g AS (
    SELECT ST_SetSRID(ST_GeomFromGeoJSON(sqlc.arg(polygon)::text), 4326) AS geom
)
SELECT x.id::text AS id, x.name, ST_Area(ST_Intersection(x.polygon, g.geom)::geography)::float8 AS overlap_m2
FROM fields x, g
WHERE x.company_id = sqlc.arg(company_id) AND x.farm_id = sqlc.arg(farm_id) AND x.deleted_at IS NULL
  AND x.id <> sqlc.arg(exclude_id)::uuid
  AND ST_Intersects(x.polygon, g.geom)
ORDER BY x.name, x.id;

-- Talhões da fazenda diante de um perímetro (novo ou atual): quantidade, soma das áreas e
-- área que fica de fora. perimeter nulo: só quantidade e soma.
-- name: MeasureFarmCoverage :one
WITH p AS (
    SELECT ST_Multi(ST_SetSRID(ST_GeomFromGeoJSON(sqlc.narg(perimeter)::text), 4326)) AS geom
)
SELECT count(x.id)::int AS field_count,
       coalesce(sum(x.area_ha), 0)::float8 AS fields_area_ha,
       coalesce(sum(CASE WHEN p.geom IS NULL THEN 0
                         ELSE ST_Area(ST_Difference(x.polygon, p.geom)::geography) END), 0)::float8 AS outside_m2
FROM p
LEFT JOIN fields x ON x.company_id = sqlc.arg(company_id) AND x.farm_id = sqlc.arg(farm_id) AND x.deleted_at IS NULL;

-- Resumo dos talhões por fazenda, para a lista e o detalhe de fazendas.
-- name: SummarizeFieldsByFarms :many
SELECT x.farm_id::text AS farm_id, count(*)::int AS field_count, sum(x.area_ha)::float8 AS area_ha
FROM fields x
WHERE x.company_id = sqlc.arg(company_id) AND x.deleted_at IS NULL
  AND x.farm_id = ANY(sqlc.arg(farm_ids)::text[]::uuid[])
GROUP BY x.farm_id;

-- name: ListCropsByFarms :many
SELECT DISTINCT x.farm_id::text AS farm_id, c.code, c.name
FROM fields x
JOIN crops c ON c.code = x.crop_code
WHERE x.company_id = sqlc.arg(company_id) AND x.deleted_at IS NULL
  AND x.farm_id = ANY(sqlc.arg(farm_ids)::text[]::uuid[])
ORDER BY 1, c.name, c.code;
