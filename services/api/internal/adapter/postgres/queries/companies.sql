-- name: GetCompany :one
SELECT * FROM companies WHERE id = $1 AND deleted_at IS NULL;

-- name: LockCompany :one
SELECT * FROM companies WHERE id = $1 AND deleted_at IS NULL FOR UPDATE;

-- name: InsertCompany :exec
INSERT INTO companies (id, name, document, city, state, plan, status, trial_ends_at,
                       is_platform_operator, created_at, version)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11);

-- name: UpdateCompany :execrows
UPDATE companies
SET name = $2, city = $3, state = $4, plan = $5, status = $6, trial_ends_at = $7,
    deleted_at = $8, version = version + 1, updated_at = now()
WHERE id = $1 AND version = $9 AND deleted_at IS NULL;

-- name: CompanyExists :one
SELECT EXISTS (SELECT 1 FROM companies WHERE id = $1 AND deleted_at IS NULL);

-- Contagens por subconsulta correlacionada: só as linhas da página são contadas, usando
-- ix_farms_company_status e ix_users_company_status (índices parciais de não removidos).
-- name: ListCompanies :many
SELECT sqlc.embed(c),
       (SELECT count(*) FROM farms f WHERE f.company_id = c.id AND f.deleted_at IS NULL)::int AS farm_count,
       (SELECT count(*) FROM users u WHERE u.company_id = c.id AND u.deleted_at IS NULL)::int AS user_count
FROM companies c
WHERE c.deleted_at IS NULL AND NOT c.is_platform_operator
  AND (sqlc.narg(status)::text IS NULL OR c.status = sqlc.narg(status)::text)
  AND (sqlc.narg(search)::text IS NULL
       OR c.name ILIKE sqlc.narg(search)::text ESCAPE '\'
       OR c.city ILIKE sqlc.narg(search)::text ESCAPE '\')
ORDER BY c.name, c.id
LIMIT sqlc.arg(page_limit)::int OFFSET sqlc.arg(page_offset)::int;

-- name: CountListedCompanies :one
SELECT count(*)::int
FROM companies c
WHERE c.deleted_at IS NULL AND NOT c.is_platform_operator
  AND (sqlc.narg(status)::text IS NULL OR c.status = sqlc.narg(status)::text)
  AND (sqlc.narg(search)::text IS NULL
       OR c.name ILIKE sqlc.narg(search)::text ESCAPE '\'
       OR c.city ILIKE sqlc.narg(search)::text ESCAPE '\');

-- name: GetCompanyListing :one
SELECT sqlc.embed(c),
       (SELECT count(*) FROM farms f WHERE f.company_id = c.id AND f.deleted_at IS NULL)::int AS farm_count,
       (SELECT count(*) FROM users u WHERE u.company_id = c.id AND u.deleted_at IS NULL)::int AS user_count
FROM companies c
WHERE c.id = $1 AND c.deleted_at IS NULL;

-- Indicadores globais da tela Empresas (empresas clientes, sem a operadora e sem as excluídas).
-- name: SummarizeCompanies :one
SELECT
  (SELECT count(*) FROM companies c
    WHERE c.deleted_at IS NULL AND NOT c.is_platform_operator AND c.status <> 'inactive')::int AS active_companies,
  (SELECT count(*) FROM companies c
    WHERE c.deleted_at IS NULL AND NOT c.is_platform_operator AND c.status = 'trial')::int AS trial_companies,
  (SELECT count(*) FROM farms f JOIN companies c ON c.id = f.company_id
    WHERE f.deleted_at IS NULL AND c.deleted_at IS NULL AND NOT c.is_platform_operator)::int AS farm_count,
  (SELECT COALESCE(sum(f.perimeter_area_ha), 0) FROM farms f JOIN companies c ON c.id = f.company_id
    WHERE f.deleted_at IS NULL AND c.deleted_at IS NULL AND NOT c.is_platform_operator)::float8 AS monitored_area_ha,
  (SELECT count(*) FROM users u JOIN companies c ON c.id = u.company_id
    WHERE u.deleted_at IS NULL AND c.deleted_at IS NULL AND NOT c.is_platform_operator)::int AS user_count,
  (SELECT count(*) FROM invitations i JOIN companies c ON c.id = i.company_id
    WHERE i.accepted_at IS NULL AND i.cancelled_at IS NULL
      AND c.deleted_at IS NULL AND NOT c.is_platform_operator)::int AS pending_invitations;

-- name: GetCompanyName :one
SELECT name FROM companies WHERE id = $1;
