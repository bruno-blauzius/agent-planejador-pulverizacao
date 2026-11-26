-- Vínculos fazenda ↔ proprietário. share_percent (numeric(5,2)) trafega em centésimos de
-- ponto percentual (inteiro), como no domínio; 0 significa "sem participação".

-- name: ListLinksByFarms :many
SELECT l.farm_id::text AS farm_id, l.owner_id::text AS owner_id, l.role,
       coalesce((l.share_percent * 100)::int, 0)::int AS share_hundredths,
       o.name AS owner_name
FROM farm_owners l
JOIN owners o ON o.company_id = l.company_id AND o.id = l.owner_id
WHERE l.company_id = sqlc.arg(company_id) AND l.farm_id = ANY(sqlc.arg(farm_ids)::text[]::uuid[])
ORDER BY l.farm_id, o.name, l.owner_id;

-- name: ListLinksByOwners :many
SELECT l.owner_id::text AS owner_id, l.farm_id::text AS farm_id, l.role,
       coalesce((l.share_percent * 100)::int, 0)::int AS share_hundredths,
       f.name AS farm_name
FROM farm_owners l
JOIN farms f ON f.company_id = l.company_id AND f.id = l.farm_id AND f.deleted_at IS NULL
WHERE l.company_id = sqlc.arg(company_id) AND l.owner_id = ANY(sqlc.arg(owner_ids)::text[]::uuid[])
  AND (sqlc.narg(visible_farm_ids)::text[] IS NULL OR l.farm_id = ANY(sqlc.narg(visible_farm_ids)::text[]::uuid[]))
ORDER BY l.owner_id, f.name, l.farm_id;

-- name: DeleteFarmLinks :exec
DELETE FROM farm_owners WHERE company_id = $1 AND farm_id = $2;

-- name: DeleteOwnerLinks :exec
DELETE FROM farm_owners WHERE company_id = $1 AND owner_id = $2;

-- Uma linha por vínculo: uma fazenda tem poucos proprietários.
-- name: InsertLink :exec
INSERT INTO farm_owners (company_id, farm_id, owner_id, role, share_percent)
VALUES (sqlc.arg(company_id), sqlc.arg(farm_id), sqlc.arg(owner_id), sqlc.arg(role),
        NULLIF(sqlc.arg(share_hundredths)::int, 0) / 100.0);
