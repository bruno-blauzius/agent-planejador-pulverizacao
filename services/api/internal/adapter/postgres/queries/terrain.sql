-- Relevo do talhão (ADR 0021, DER: terrain_runs, field_terrain, field_terrain_adjustments).
-- Leituras e escritas do produto sempre filtram por company_id. As únicas consultas entre
-- empresas são as do executor em segundo plano (ClaimNextPendingTerrain, RequeueStaleCalculatingTerrain,
-- FinishIdleTerrainRuns), que varrem as execuções abertas de todos os tenants e devolvem a
-- empresa de cada linha. Talhão removido (deleted_at) nunca entra em lista, execução nem contagem.

-- Talhões da fazenda com o relevo e o ajuste (LEFT JOIN: sem linha = "não calculado"). Mesma
-- ordem de ListFields. O autor do ajuste vem de users sem filtro de empresa: a FK é simples
-- porque ele pode ser o super admin da operadora, de outra empresa.
-- name: ListFarmTerrain :many
SELECT x.id, x.company_id, x.farm_id, x.crop_code, c.name AS crop_name, x.name, x.block, x.variety,
       x.season, x.planting_date, x.row_spacing_m, x.plant_spacing_m,
       ST_AsGeoJSON(x.polygon, 7)::text AS polygon, x.area_ha::float8 AS area_ha,
       x.created_at, x.updated_at, x.version,
       t.id AS terrain_id, t.status AS terrain_status, t.failure_reason, t.requested_at AS terrain_requested_at,
       t.calculated_at, t.slope_mean_pct AS slope_mean_pct, t.slope_p90_pct AS slope_p90_pct,
       t.slope_max_pct AS slope_max_pct, t.elevation_min_m AS elevation_min_m,
       t.elevation_max_m AS elevation_max_m, t.position AS terrain_position, t.aspect,
       t.slope_histogram_ha::float8[] AS slope_histogram_ha, t.profile, t.elevation_source,
       t.elevation_resolution_m AS elevation_resolution_m, t.elevation_version,
       a.id AS adjustment_id, a.origin AS adjustment_origin, a.slope_mean_pct AS adjustment_slope_mean_pct,
       a.slope_max_pct AS adjustment_slope_max_pct, a.position AS adjustment_position,
       a.justification AS adjustment_justification, a.adjusted_by AS adjustment_adjusted_by,
       a.adjusted_at AS adjustment_adjusted_at, u.name AS adjustment_adjusted_by_name
FROM fields x
JOIN crops c ON c.code = x.crop_code
LEFT JOIN field_terrain t ON t.company_id = x.company_id AND t.field_id = x.id
LEFT JOIN field_terrain_adjustments a ON a.company_id = x.company_id AND a.field_id = x.id
LEFT JOIN users u ON u.id = a.adjusted_by
WHERE x.company_id = sqlc.arg(company_id) AND x.farm_id = sqlc.arg(farm_id) AND x.deleted_at IS NULL
ORDER BY x.block NULLS FIRST, x.name, x.id;

-- name: GetFieldTerrain :one
SELECT x.id, x.company_id, x.farm_id, x.crop_code, c.name AS crop_name, x.name, x.block, x.variety,
       x.season, x.planting_date, x.row_spacing_m, x.plant_spacing_m,
       ST_AsGeoJSON(x.polygon, 7)::text AS polygon, x.area_ha::float8 AS area_ha,
       x.created_at, x.updated_at, x.version,
       t.id AS terrain_id, t.status AS terrain_status, t.failure_reason, t.requested_at AS terrain_requested_at,
       t.calculated_at, t.slope_mean_pct AS slope_mean_pct, t.slope_p90_pct AS slope_p90_pct,
       t.slope_max_pct AS slope_max_pct, t.elevation_min_m AS elevation_min_m,
       t.elevation_max_m AS elevation_max_m, t.position AS terrain_position, t.aspect,
       t.slope_histogram_ha::float8[] AS slope_histogram_ha, t.profile, t.elevation_source,
       t.elevation_resolution_m AS elevation_resolution_m, t.elevation_version,
       a.id AS adjustment_id, a.origin AS adjustment_origin, a.slope_mean_pct AS adjustment_slope_mean_pct,
       a.slope_max_pct AS adjustment_slope_max_pct, a.position AS adjustment_position,
       a.justification AS adjustment_justification, a.adjusted_by AS adjustment_adjusted_by,
       a.adjusted_at AS adjustment_adjusted_at, u.name AS adjustment_adjusted_by_name
FROM fields x
JOIN crops c ON c.code = x.crop_code
LEFT JOIN field_terrain t ON t.company_id = x.company_id AND t.field_id = x.id
LEFT JOIN field_terrain_adjustments a ON a.company_id = x.company_id AND a.field_id = x.id
LEFT JOIN users u ON u.id = a.adjusted_by
WHERE x.company_id = sqlc.arg(company_id) AND x.farm_id = sqlc.arg(farm_id) AND x.id = sqlc.arg(id)
  AND x.deleted_at IS NULL;

-- Execução mais recente da fazenda, aberta ou não. Total e Done contam as linhas de
-- field_terrain da execução (Done = ready ou failed) dos talhões não removidos.
-- name: GetLatestTerrainRun :one
SELECT r.id, r.company_id, r.farm_id, r.scope, r.requested_by, r.requested_at, r.finished_at,
       (SELECT count(*) FROM field_terrain t JOIN fields x ON x.company_id = t.company_id AND x.id = t.field_id
         WHERE t.company_id = r.company_id AND t.run_id = r.id AND x.deleted_at IS NULL)::int AS total,
       (SELECT count(*) FROM field_terrain t JOIN fields x ON x.company_id = t.company_id AND x.id = t.field_id
         WHERE t.company_id = r.company_id AND t.run_id = r.id AND x.deleted_at IS NULL
           AND t.status IN ('ready', 'failed'))::int AS done
FROM terrain_runs r
WHERE r.company_id = sqlc.arg(company_id) AND r.farm_id = sqlc.arg(farm_id)
ORDER BY r.requested_at DESC, r.id DESC
LIMIT 1;

-- name: GetOpenTerrainRun :one
SELECT r.id, r.company_id, r.farm_id, r.scope, r.requested_by, r.requested_at, r.finished_at,
       (SELECT count(*) FROM field_terrain t JOIN fields x ON x.company_id = t.company_id AND x.id = t.field_id
         WHERE t.company_id = r.company_id AND t.run_id = r.id AND x.deleted_at IS NULL)::int AS total,
       (SELECT count(*) FROM field_terrain t JOIN fields x ON x.company_id = t.company_id AND x.id = t.field_id
         WHERE t.company_id = r.company_id AND t.run_id = r.id AND x.deleted_at IS NULL
           AND t.status IN ('ready', 'failed'))::int AS done
FROM terrain_runs r
WHERE r.company_id = sqlc.arg(company_id) AND r.farm_id = sqlc.arg(farm_id) AND r.finished_at IS NULL
ORDER BY r.requested_at DESC, r.id DESC
LIMIT 1;

-- Serializa os pedidos de "calcular relevo" da mesma fazenda até o fim da transação.
-- name: LockFarmForTerrain :one
SELECT id FROM farms
WHERE company_id = sqlc.arg(company_id) AND id = sqlc.arg(id) AND deleted_at IS NULL
FOR UPDATE;

-- name: SelectFieldsForRunAll :many
SELECT x.id::text AS id
FROM fields x
WHERE x.company_id = sqlc.arg(company_id) AND x.farm_id = sqlc.arg(farm_id) AND x.deleted_at IS NULL
ORDER BY x.block NULLS FIRST, x.name, x.id;

-- Sem linha, pendentes ou com falha.
-- name: SelectFieldsForRunMissing :many
SELECT x.id::text AS id
FROM fields x
LEFT JOIN field_terrain t ON t.company_id = x.company_id AND t.field_id = x.id
WHERE x.company_id = sqlc.arg(company_id) AND x.farm_id = sqlc.arg(farm_id) AND x.deleted_at IS NULL
  AND (t.id IS NULL OR t.status IN ('pending', 'failed'))
ORDER BY x.block NULLS FIRST, x.name, x.id;

-- name: InsertTerrainRun :exec
INSERT INTO terrain_runs (id, company_id, farm_id, scope, requested_by, requested_at)
VALUES (sqlc.arg(id), sqlc.arg(company_id), sqlc.arg(farm_id), sqlc.arg(scope), sqlc.arg(requested_by),
        sqlc.arg(requested_at));

-- Cria ou reabre as linhas como pending na execução. A busca em fields garante empresa e
-- fazenda (id de fora não vira linha). O último cálculo é preservado: só status, motivo,
-- execução, pedido e versão mudam.
-- name: MarkFieldsPending :exec
INSERT INTO field_terrain (company_id, farm_id, field_id, status, run_id, requested_at, created_at, updated_at)
SELECT x.company_id, x.farm_id, x.id, 'pending', sqlc.arg(run_id)::uuid, sqlc.arg(at)::timestamptz,
       sqlc.arg(at)::timestamptz, sqlc.arg(at)::timestamptz
FROM fields x
WHERE x.company_id = sqlc.arg(company_id) AND x.farm_id = sqlc.arg(farm_id) AND x.deleted_at IS NULL
  AND x.id = ANY(sqlc.arg(field_ids)::text[]::uuid[])
ON CONFLICT (company_id, field_id) DO UPDATE
SET status = 'pending', failure_reason = NULL, run_id = EXCLUDED.run_id, requested_at = EXCLUDED.requested_at,
    updated_at = EXCLUDED.updated_at, version = field_terrain.version + 1;

-- O polígono mudou: as medidas descrevem outra forma. Mantém run_id; limpa tudo o que foi calculado.
-- name: MarkTerrainStale :exec
UPDATE field_terrain
SET status = 'pending', failure_reason = NULL, requested_at = sqlc.arg(at), updated_at = sqlc.arg(at),
    version = version + 1, calculated_at = NULL, slope_mean_pct = NULL, slope_p90_pct = NULL,
    slope_max_pct = NULL, elevation_min_m = NULL, elevation_max_m = NULL, position = NULL, aspect = NULL,
    slope_histogram_ha = NULL, profile = NULL, elevation_source = NULL, elevation_resolution_m = NULL,
    elevation_version = NULL
WHERE company_id = sqlc.arg(company_id) AND field_id = sqlc.arg(field_id);

-- Pega atomicamente um talhão pendente de execução aberta (a mais antiga primeiro). SKIP
-- LOCKED: dois executores nunca recebem o mesmo talhão; o AND status = 'pending' do UPDATE
-- repete o filtro por segurança. Não troca a versão: ela só muda quando o dado muda, e é o
-- que o SaveCalculation compara para detectar edição no meio do caminho.
-- name: ClaimNextPendingTerrain :one
WITH next AS (
    SELECT t.id
    FROM field_terrain t
    JOIN terrain_runs r ON r.company_id = t.company_id AND r.id = t.run_id
    JOIN fields x ON x.company_id = t.company_id AND x.id = t.field_id AND x.deleted_at IS NULL
    WHERE t.status = 'pending' AND r.finished_at IS NULL
    ORDER BY r.requested_at, r.id, t.requested_at, t.id
    LIMIT 1
    FOR UPDATE OF t SKIP LOCKED
)
UPDATE field_terrain t
SET status = 'calculating', updated_at = sqlc.arg(at)
FROM next
WHERE t.id = next.id AND t.status = 'pending'
RETURNING t.company_id, t.farm_id, t.field_id, t.run_id, t.version;

-- Lease por idade: devolve a pending só o que está calculando há mais tempo que o limite
-- (updated_at é gravado pelo claim). Linha mais nova é de um executor vivo, inclusive de outra
-- instância, e não se toca. A versão sobe para que o save atrasado de quem perdeu o lease
-- vire conflito descartado em vez de sobrescrever o recomeço.
-- name: RequeueStaleCalculatingTerrain :exec
UPDATE field_terrain
SET status = 'pending', updated_at = sqlc.arg(at), version = version + 1
WHERE status = 'calculating' AND updated_at < sqlc.arg(stale_before)::timestamptz;

-- Fecha execuções abertas sem linha em andamento (de talhão não removido).
-- name: FinishIdleTerrainRuns :exec
UPDATE terrain_runs r
SET finished_at = GREATEST(sqlc.arg(at)::timestamptz, r.requested_at)
WHERE r.finished_at IS NULL
  AND NOT EXISTS (
      SELECT 1 FROM field_terrain t
      JOIN fields x ON x.company_id = t.company_id AND x.id = t.field_id AND x.deleted_at IS NULL
      WHERE t.company_id = r.company_id AND t.run_id = r.id AND t.status IN ('pending', 'calculating'));

-- name: FinishTerrainRun :execrows
UPDATE terrain_runs r
SET finished_at = GREATEST(sqlc.arg(at)::timestamptz, r.requested_at)
WHERE r.company_id = sqlc.arg(company_id) AND r.id = sqlc.arg(id) AND r.finished_at IS NULL
  AND NOT EXISTS (
      SELECT 1 FROM field_terrain t
      JOIN fields x ON x.company_id = t.company_id AND x.id = t.field_id AND x.deleted_at IS NULL
      WHERE t.company_id = r.company_id AND t.run_id = r.id AND t.status IN ('pending', 'calculating'));

-- Grava o resultado se a linha ainda está na versão do claim; 0 linhas = conflito. O histograma
-- chega como float8[] e é guardado como numeric[]; o perfil, como JSON.
-- name: SaveTerrainCalculation :execrows
UPDATE field_terrain
SET status = 'ready', failure_reason = NULL, calculated_at = sqlc.arg(calculated_at), updated_at = sqlc.arg(at),
    slope_mean_pct = sqlc.arg(slope_mean_pct)::float8, slope_p90_pct = sqlc.arg(slope_p90_pct)::float8,
    slope_max_pct = sqlc.arg(slope_max_pct)::float8, elevation_min_m = sqlc.arg(elevation_min_m)::float8,
    elevation_max_m = sqlc.arg(elevation_max_m)::float8, position = sqlc.arg(position), aspect = sqlc.arg(aspect),
    slope_histogram_ha = sqlc.arg(slope_histogram_ha)::float8[]::numeric[], profile = sqlc.arg(profile)::jsonb,
    elevation_source = sqlc.arg(elevation_source), elevation_resolution_m = sqlc.arg(elevation_resolution_m)::float8,
    elevation_version = sqlc.arg(elevation_version), version = version + 1
WHERE company_id = sqlc.arg(company_id) AND field_id = sqlc.arg(field_id) AND version = sqlc.arg(row_version);

-- name: SaveTerrainFailure :execrows
UPDATE field_terrain
SET status = 'failed', failure_reason = sqlc.arg(reason), updated_at = sqlc.arg(at), version = version + 1
WHERE company_id = sqlc.arg(company_id) AND field_id = sqlc.arg(field_id) AND version = sqlc.arg(row_version);

-- Ajuste manual: um por talhão. A busca em fields garante empresa, fazenda e talhão não removido
-- (0 linhas = não encontrado). O autor e a data são os do ajuste, não de quem regrava.
-- name: UpsertTerrainAdjustment :execrows
INSERT INTO field_terrain_adjustments (company_id, field_id, origin, slope_mean_pct, slope_max_pct, position,
                                       justification, adjusted_by, adjusted_at)
SELECT x.company_id, x.id, sqlc.arg(origin), sqlc.arg(slope_mean_pct)::float8, sqlc.arg(slope_max_pct)::float8,
       sqlc.arg(position), sqlc.arg(justification), sqlc.arg(adjusted_by), sqlc.arg(adjusted_at)::timestamptz
FROM fields x
WHERE x.company_id = sqlc.arg(company_id) AND x.farm_id = sqlc.arg(farm_id) AND x.id = sqlc.arg(field_id)
  AND x.deleted_at IS NULL
ON CONFLICT (company_id, field_id) DO UPDATE
SET origin = EXCLUDED.origin, slope_mean_pct = EXCLUDED.slope_mean_pct, slope_max_pct = EXCLUDED.slope_max_pct,
    position = EXCLUDED.position, justification = EXCLUDED.justification, adjusted_by = EXCLUDED.adjusted_by,
    adjusted_at = EXCLUDED.adjusted_at, version = field_terrain_adjustments.version + 1;

-- name: DeleteTerrainAdjustment :execrows
DELETE FROM field_terrain_adjustments WHERE company_id = sqlc.arg(company_id) AND field_id = sqlc.arg(field_id);
