-- name: CountFarmsInCompany :one
SELECT count(*)::int FROM farms
WHERE company_id = $1 AND deleted_at IS NULL AND id = ANY(sqlc.arg(farm_ids)::text[]::uuid[]);

-- name: GetFarmRef :one
SELECT id::text AS id, name FROM farms
WHERE company_id = $1 AND id = $2 AND deleted_at IS NULL;

-- Status "pending" (domain.DisplayStatus): fazenda ativa com algum vinculado cujo documento
-- obrigatório falta (owners.documents_pending, mantido pela aplicação). O filtro de status
-- separa active de pending do mesmo jeito.
-- Geometria entra e sai como GeoJSON (WGS84). coalesce(..., '') evita NULL em colunas que o
-- sqlc gera como string: '' significa "sem geometria". O contorno (outline) é simplificado
-- (~10 m) para a miniatura e o modo mapa; a geometria completa só vai no detalhe.

-- name: CountFarms :one
SELECT count(*)::int
FROM farms f
CROSS JOIN LATERAL (
    SELECT EXISTS (
        SELECT 1 FROM farm_owners l
        JOIN owners o ON o.company_id = l.company_id AND o.id = l.owner_id AND o.deleted_at IS NULL
        WHERE l.company_id = f.company_id AND l.farm_id = f.id AND o.documents_pending
    ) AS documents_pending
) p
WHERE f.company_id = sqlc.arg(company_id) AND f.deleted_at IS NULL
  AND (sqlc.narg(status)::text IS NULL
       OR (sqlc.narg(status)::text = 'draft' AND f.status = 'draft')
       OR (sqlc.narg(status)::text = 'active' AND f.status = 'active' AND NOT p.documents_pending)
       OR (sqlc.narg(status)::text = 'pending' AND f.status = 'active' AND p.documents_pending))
  AND (sqlc.narg(production_type)::text IS NULL OR f.production_type = sqlc.narg(production_type)::text)
  AND (sqlc.narg(visible_farm_ids)::text[] IS NULL OR f.id = ANY(sqlc.narg(visible_farm_ids)::text[]::uuid[]))
  AND (sqlc.narg(search)::text IS NULL
       OR unaccent(f.name) ILIKE unaccent(sqlc.narg(search)::text) ESCAPE '\'
       OR unaccent(f.code) ILIKE unaccent(sqlc.narg(search)::text) ESCAPE '\'
       OR unaccent(f.city) ILIKE unaccent(sqlc.narg(search)::text) ESCAPE '\');

-- name: ListFarms :many
SELECT f.id, f.company_id, f.name, f.code, f.cooperative, f.production_type, f.city, f.state, f.status,
       f.postal_code, f.car_number,
       coalesce(ST_AsGeoJSON(f.forecast_point, 7), '')::text AS forecast_point,
       coalesce(ST_AsGeoJSON(ST_SimplifyPreserveTopology(f.perimeter, 0.0001), 6), '')::text AS outline,
       coalesce(f.perimeter_area_ha, 0)::float8 AS area_ha,
       p.documents_pending::bool AS documents_pending,
       f.created_at, f.updated_at, f.version
FROM farms f
CROSS JOIN LATERAL (
    SELECT EXISTS (
        SELECT 1 FROM farm_owners l
        JOIN owners o ON o.company_id = l.company_id AND o.id = l.owner_id AND o.deleted_at IS NULL
        WHERE l.company_id = f.company_id AND l.farm_id = f.id AND o.documents_pending
    ) AS documents_pending
) p
WHERE f.company_id = sqlc.arg(company_id) AND f.deleted_at IS NULL
  AND (sqlc.narg(status)::text IS NULL
       OR (sqlc.narg(status)::text = 'draft' AND f.status = 'draft')
       OR (sqlc.narg(status)::text = 'active' AND f.status = 'active' AND NOT p.documents_pending)
       OR (sqlc.narg(status)::text = 'pending' AND f.status = 'active' AND p.documents_pending))
  AND (sqlc.narg(production_type)::text IS NULL OR f.production_type = sqlc.narg(production_type)::text)
  AND (sqlc.narg(visible_farm_ids)::text[] IS NULL OR f.id = ANY(sqlc.narg(visible_farm_ids)::text[]::uuid[]))
  AND (sqlc.narg(search)::text IS NULL
       OR unaccent(f.name) ILIKE unaccent(sqlc.narg(search)::text) ESCAPE '\'
       OR unaccent(f.code) ILIKE unaccent(sqlc.narg(search)::text) ESCAPE '\'
       OR unaccent(f.city) ILIKE unaccent(sqlc.narg(search)::text) ESCAPE '\')
ORDER BY f.name, f.id
LIMIT sqlc.arg(page_limit)::int OFFSET sqlc.arg(page_offset)::int;

-- name: GetFarm :one
SELECT f.id, f.company_id, f.name, f.code, f.cooperative, f.production_type, f.city, f.state, f.status,
       f.postal_code, f.car_number,
       coalesce(ST_AsGeoJSON(f.forecast_point, 7), '')::text AS forecast_point,
       coalesce(ST_AsGeoJSON(f.perimeter, 7), '')::text AS perimeter,
       coalesce(ST_AsGeoJSON(ST_SimplifyPreserveTopology(f.perimeter, 0.0001), 6), '')::text AS outline,
       coalesce(f.perimeter_area_ha, 0)::float8 AS area_ha,
       p.documents_pending::bool AS documents_pending,
       f.created_at, f.updated_at, f.version
FROM farms f
CROSS JOIN LATERAL (
    SELECT EXISTS (
        SELECT 1 FROM farm_owners l
        JOIN owners o ON o.company_id = l.company_id AND o.id = l.owner_id AND o.deleted_at IS NULL
        WHERE l.company_id = f.company_id AND l.farm_id = f.id AND o.documents_pending
    ) AS documents_pending
) p
WHERE f.company_id = sqlc.arg(company_id) AND f.id = sqlc.arg(id) AND f.deleted_at IS NULL;

-- name: LockFarm :one
SELECT f.id, f.company_id, f.name, f.code, f.cooperative, f.production_type, f.city, f.state, f.status,
       f.postal_code, f.car_number,
       coalesce(ST_AsGeoJSON(f.forecast_point, 7), '')::text AS forecast_point,
       coalesce(ST_AsGeoJSON(f.perimeter, 7), '')::text AS perimeter,
       f.created_at, f.updated_at, f.version
FROM farms f
WHERE f.company_id = sqlc.arg(company_id) AND f.id = sqlc.arg(id) AND f.deleted_at IS NULL
FOR UPDATE;

-- Trava em ordem de id (evita deadlock entre transações que tocam as mesmas fazendas).
-- name: LockFarmsByIDs :many
SELECT f.id::text AS id
FROM farms f
WHERE f.company_id = sqlc.arg(company_id) AND f.deleted_at IS NULL
  AND f.id = ANY(sqlc.arg(farm_ids)::text[]::uuid[])
ORDER BY f.id
FOR UPDATE;

-- Sem ponto de previsão informado, usa o centróide do perímetro.
-- name: InsertFarm :exec
INSERT INTO farms (id, company_id, name, code, cooperative, production_type, city, state, status,
                   postal_code, car_number, forecast_point, perimeter, created_at, updated_at, version)
VALUES (sqlc.arg(id), sqlc.arg(company_id), sqlc.arg(name), sqlc.narg(code), sqlc.narg(cooperative),
        sqlc.arg(production_type), sqlc.arg(city), sqlc.arg(state), sqlc.arg(status),
        sqlc.narg(postal_code), sqlc.narg(car_number),
        coalesce(ST_SetSRID(ST_GeomFromGeoJSON(sqlc.narg(forecast_point)::text), 4326),
                 ST_Centroid(ST_SetSRID(ST_GeomFromGeoJSON(sqlc.narg(perimeter)::text), 4326))),
        ST_Multi(ST_SetSRID(ST_GeomFromGeoJSON(sqlc.narg(perimeter)::text), 4326)),
        sqlc.arg(created_at), sqlc.arg(updated_at), 1);

-- name: UpdateFarm :execrows
UPDATE farms
SET name = sqlc.arg(name), code = sqlc.narg(code), cooperative = sqlc.narg(cooperative),
    production_type = sqlc.arg(production_type), city = sqlc.arg(city), state = sqlc.arg(state),
    status = sqlc.arg(status), postal_code = sqlc.narg(postal_code), car_number = sqlc.narg(car_number),
    forecast_point = coalesce(ST_SetSRID(ST_GeomFromGeoJSON(sqlc.narg(forecast_point)::text), 4326),
                              ST_Centroid(ST_SetSRID(ST_GeomFromGeoJSON(sqlc.narg(perimeter)::text), 4326))),
    perimeter = ST_Multi(ST_SetSRID(ST_GeomFromGeoJSON(sqlc.narg(perimeter)::text), 4326)),
    updated_at = sqlc.arg(updated_at), version = version + 1
WHERE company_id = sqlc.arg(company_id) AND id = sqlc.arg(id) AND deleted_at IS NULL
  AND version = sqlc.arg(version);

-- name: FarmExists :one
SELECT EXISTS (SELECT 1 FROM farms WHERE company_id = $1 AND id = $2 AND deleted_at IS NULL);

-- name: MeasurePerimeter :one
SELECT ST_IsValid(g.geom)::bool AS valid,
       round((ST_Area(g.geom::geography) / 10000.0)::numeric, 4)::float8 AS area_ha,
       ST_Y(ST_Centroid(g.geom))::float8 AS centroid_lat,
       ST_X(ST_Centroid(g.geom))::float8 AS centroid_lng,
       ST_Y(ST_PointOnSurface(g.geom))::float8 AS interior_lat,
       ST_X(ST_PointOnSurface(g.geom))::float8 AS interior_lng
FROM (SELECT ST_SetSRID(ST_GeomFromGeoJSON(sqlc.arg(geojson)::text), 4326) AS geom) AS g;
