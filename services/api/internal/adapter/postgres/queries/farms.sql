-- name: CountFarmsInCompany :one
SELECT count(*)::int FROM farms
WHERE company_id = $1 AND deleted_at IS NULL AND id = ANY(sqlc.arg(farm_ids)::text[]::uuid[]);

-- name: GetFarmRef :one
SELECT id::text AS id, name FROM farms
WHERE company_id = $1 AND id = $2 AND deleted_at IS NULL;
