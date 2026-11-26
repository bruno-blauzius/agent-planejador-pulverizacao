-- Catálogo global de culturas (sem company_id): só as ativas são oferecidas e aceitas.

-- name: ListActiveCrops :many
SELECT code, name FROM crops WHERE is_active ORDER BY name, code;

-- name: GetActiveCrop :one
SELECT code, name FROM crops WHERE code = $1 AND is_active;
