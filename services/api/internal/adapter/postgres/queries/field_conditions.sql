-- Chuva, solo e trafegabilidade do talhão (ADR 0022, adendo). Toda consulta de negócio filtra pela
-- empresa; pluviômetro e talhão removidos (deleted_at) nunca aparecem. O estado do solo é calculado
-- no domínio Go: aqui só se lê e grava dado.

-- Localização opcional: has_location diz se latitude e longitude valem (0 sem ponto).
-- name: ListRainGauges :many
SELECT g.id, g.company_id, g.farm_id, g.name,
       (g.location IS NOT NULL)::bool AS has_location,
       coalesce(ST_Y(g.location), 0)::float8 AS latitude, coalesce(ST_X(g.location), 0)::float8 AS longitude,
       g.created_at, g.updated_at, g.version
FROM rain_gauges g
WHERE g.company_id = sqlc.arg(company_id) AND g.farm_id = sqlc.arg(farm_id) AND g.deleted_at IS NULL
ORDER BY lower(g.name), g.id;

-- name: GetRainGauge :one
SELECT g.id, g.company_id, g.farm_id, g.name,
       (g.location IS NOT NULL)::bool AS has_location,
       coalesce(ST_Y(g.location), 0)::float8 AS latitude, coalesce(ST_X(g.location), 0)::float8 AS longitude,
       g.created_at, g.updated_at, g.version
FROM rain_gauges g
WHERE g.company_id = sqlc.arg(company_id) AND g.farm_id = sqlc.arg(farm_id) AND g.id = sqlc.arg(id)
  AND g.deleted_at IS NULL;

-- name: LockRainGauge :one
SELECT g.id, g.company_id, g.farm_id, g.name,
       (g.location IS NOT NULL)::bool AS has_location,
       coalesce(ST_Y(g.location), 0)::float8 AS latitude, coalesce(ST_X(g.location), 0)::float8 AS longitude,
       g.created_at, g.updated_at, g.version
FROM rain_gauges g
WHERE g.company_id = sqlc.arg(company_id) AND g.farm_id = sqlc.arg(farm_id) AND g.id = sqlc.arg(id)
  AND g.deleted_at IS NULL
FOR UPDATE;

-- name: InsertRainGauge :exec
INSERT INTO rain_gauges (id, company_id, farm_id, name, location, created_at, updated_at, version)
VALUES (sqlc.arg(id), sqlc.arg(company_id), sqlc.arg(farm_id), sqlc.arg(name),
        CASE WHEN sqlc.narg(latitude)::float8 IS NULL THEN NULL
             ELSE ST_SetSRID(ST_MakePoint(sqlc.narg(longitude)::float8, sqlc.narg(latitude)::float8), 4326) END,
        sqlc.arg(created_at), sqlc.arg(updated_at), 1);

-- name: UpdateRainGauge :execrows
UPDATE rain_gauges
SET name = sqlc.arg(name),
    location = CASE WHEN sqlc.narg(latitude)::float8 IS NULL THEN NULL
                    ELSE ST_SetSRID(ST_MakePoint(sqlc.narg(longitude)::float8, sqlc.narg(latitude)::float8), 4326) END,
    updated_at = sqlc.arg(updated_at), version = version + 1
WHERE company_id = sqlc.arg(company_id) AND farm_id = sqlc.arg(farm_id) AND id = sqlc.arg(id)
  AND deleted_at IS NULL AND version = sqlc.arg(version);

-- name: InsertRainReading :exec
INSERT INTO rain_readings (id, company_id, rain_gauge_id, rain_mm, observed_at, recorded_by, recorded_at)
VALUES (sqlc.arg(id), sqlc.arg(company_id), sqlc.arg(rain_gauge_id), sqlc.arg(rain_mm)::float8,
        sqlc.arg(observed_at), sqlc.arg(recorded_by), sqlc.arg(recorded_at));

-- Leituras com observed_at em (from, to], da mais recente para a mais antiga. Usa o índice único
-- (company_id, rain_gauge_id, observed_at).
-- name: ListRainReadings :many
SELECT r.id, r.company_id, r.rain_gauge_id, r.rain_mm::float8 AS rain_mm, r.observed_at,
       r.recorded_by, u.name AS recorded_by_name, r.recorded_at
FROM rain_readings r
JOIN users u ON u.id = r.recorded_by
WHERE r.company_id = sqlc.arg(company_id) AND r.rain_gauge_id = sqlc.arg(rain_gauge_id)
  AND r.observed_at > sqlc.arg(observed_after) AND r.observed_at <= sqlc.arg(observed_until)
ORDER BY r.observed_at DESC;

-- Talhão com textura, centroide (para a célula do modelo) e o pluviômetro vinculado, se houver.
-- name: GetFieldConditions :one
SELECT x.id, x.farm_id, x.soil_texture,
       ST_Y(ST_Centroid(x.polygon))::float8 AS centroid_latitude,
       ST_X(ST_Centroid(x.polygon))::float8 AS centroid_longitude,
       g.id AS gauge_id, g.name AS gauge_name,
       (g.location IS NOT NULL)::bool AS gauge_has_location,
       coalesce(ST_Y(g.location), 0)::float8 AS gauge_latitude, coalesce(ST_X(g.location), 0)::float8 AS gauge_longitude,
       g.created_at AS gauge_created_at, g.updated_at AS gauge_updated_at, g.version AS gauge_version
FROM fields x
LEFT JOIN rain_gauge_fields l ON l.company_id = x.company_id AND l.field_id = x.id
LEFT JOIN rain_gauges g ON g.company_id = l.company_id AND g.id = l.rain_gauge_id AND g.deleted_at IS NULL
WHERE x.company_id = sqlc.arg(company_id) AND x.farm_id = sqlc.arg(farm_id) AND x.id = sqlc.arg(id)
  AND x.deleted_at IS NULL;

-- name: GetLatestSoilInspection :one
SELECT s.id, s.company_id, s.farm_id, s.field_id, s.state, s.inspected_at, s.inspected_by,
       u.name AS inspected_by_name, s.note, s.recorded_at
FROM soil_inspections s
JOIN users u ON u.id = s.inspected_by
WHERE s.company_id = sqlc.arg(company_id) AND s.field_id = sqlc.arg(field_id)
ORDER BY s.inspected_at DESC, s.recorded_at DESC
LIMIT 1;

-- Trava o talhão até o fim da transação (perfil de solo e vistoria do mesmo talhão em série).
-- name: LockFieldForConditions :one
SELECT x.id, x.company_id, x.farm_id
FROM fields x
WHERE x.company_id = sqlc.arg(company_id) AND x.farm_id = sqlc.arg(farm_id) AND x.id = sqlc.arg(id)
  AND x.deleted_at IS NULL
FOR UPDATE;

-- Serializa o cadastro de pluviômetros da mesma fazenda (nome único sem caixa).
-- name: LockFarmForConditions :one
SELECT id FROM farms
WHERE company_id = sqlc.arg(company_id) AND id = sqlc.arg(id) AND deleted_at IS NULL
FOR UPDATE;

-- name: UpdateFieldSoilTexture :execrows
UPDATE fields SET soil_texture = sqlc.narg(soil_texture)
WHERE company_id = sqlc.arg(company_id) AND farm_id = sqlc.arg(farm_id) AND id = sqlc.arg(id)
  AND deleted_at IS NULL;

-- name: UpsertRainGaugeField :exec
INSERT INTO rain_gauge_fields (company_id, farm_id, field_id, rain_gauge_id, linked_at)
VALUES (sqlc.arg(company_id), sqlc.arg(farm_id), sqlc.arg(field_id), sqlc.arg(rain_gauge_id), sqlc.arg(linked_at))
ON CONFLICT (field_id) DO UPDATE
SET rain_gauge_id = EXCLUDED.rain_gauge_id, linked_at = EXCLUDED.linked_at
WHERE rain_gauge_fields.company_id = EXCLUDED.company_id AND rain_gauge_fields.rain_gauge_id <> EXCLUDED.rain_gauge_id;

-- name: DeleteRainGaugeField :exec
DELETE FROM rain_gauge_fields
WHERE company_id = sqlc.arg(company_id) AND farm_id = sqlc.arg(farm_id) AND field_id = sqlc.arg(field_id);

-- name: InsertSoilInspection :exec
INSERT INTO soil_inspections (id, company_id, farm_id, field_id, state, inspected_at, inspected_by, note, recorded_at)
VALUES (sqlc.arg(id), sqlc.arg(company_id), sqlc.arg(farm_id), sqlc.arg(field_id), sqlc.arg(state),
        sqlc.arg(inspected_at), sqlc.arg(inspected_by), sqlc.narg(note), sqlc.arg(recorded_at));

-- Limites vigentes: por textura, a linha mais recente com vigência até a data (referência global).
-- name: ListCurrentTrafficabilityThresholds :many
SELECT DISTINCT ON (t.soil_texture)
       t.soil_texture, t.effective_from,
       t.rain_48h_dry_mm::float8 AS rain_48h_dry_mm, t.rain_48h_ideal_mm::float8 AS rain_48h_ideal_mm,
       t.rain_48h_moist_mm::float8 AS rain_48h_moist_mm,
       t.moisture_dry_pct::float8 AS moisture_dry_pct, t.moisture_ideal_pct::float8 AS moisture_ideal_pct,
       t.moisture_moist_pct::float8 AS moisture_moist_pct,
       t.rain_72h_factor::float8 AS rain_72h_factor, t.drying_rate_mm_per_day::float8 AS drying_rate_mm_per_day,
       t.field_capacity_m3m3::float8 AS field_capacity_m3m3, t.source
FROM trafficability_thresholds t
WHERE t.effective_from <= sqlc.arg(on_date)::date
ORDER BY t.soil_texture, t.effective_from DESC;
