-- Histórico diário do estado do solo (ADR 0022, adendo da fase 4). O executor não tem ator: percorre
-- as empresas ativas e, dentro de cada uma, toda consulta de negócio filtra por company_id.

-- Empresas que o executor percorre: ativas (ou em avaliação) e não removidas.
-- name: ListSoilHistoryCompanies :many
SELECT c.id
FROM companies c
WHERE c.deleted_at IS NULL AND c.status IN ('active', 'trial')
ORDER BY c.id;

-- Talhões não removidos (de fazendas não removidas) da empresa, com a marca de já gravado no dia.
-- O LEFT JOIN usa o índice único (company_id, field_id, local_day).
-- name: ListSoilHistoryFields :many
SELECT x.id::text AS field_id, x.farm_id::text AS farm_id, (h.id IS NOT NULL)::bool AS recorded
FROM fields x
JOIN farms f ON f.company_id = x.company_id AND f.id = x.farm_id AND f.deleted_at IS NULL
LEFT JOIN soil_state_history h
       ON h.company_id = x.company_id AND h.field_id = x.id AND h.local_day = sqlc.arg(local_day)::date
WHERE x.company_id = sqlc.arg(company_id) AND x.deleted_at IS NULL
ORDER BY x.farm_id, x.id;

-- Um registro por talhão e dia: reexecução e outras instâncias não duplicam nem sobrescrevem.
-- name: InsertSoilStateHistory :execrows
INSERT INTO soil_state_history (company_id, farm_id, field_id, local_day, state, basis, rain_source,
                                last_48h_mm, last_72h_mm, model_moisture_pct, soil_texture,
                                thresholds_source, recorded_at)
VALUES (sqlc.arg(company_id), sqlc.arg(farm_id), sqlc.arg(field_id), sqlc.arg(local_day)::date,
        sqlc.arg(state), sqlc.arg(basis), sqlc.narg(rain_source),
        sqlc.narg(last_48h_mm)::float8, sqlc.narg(last_72h_mm)::float8, sqlc.narg(model_moisture_pct)::float8,
        sqlc.narg(soil_texture), sqlc.arg(thresholds_source), sqlc.arg(recorded_at))
ON CONFLICT (company_id, field_id, local_day) DO NOTHING;
