-- Consultas de usuários. Nenhum valor é concatenado: tudo entra por parâmetro.
-- company_id nulo no filtro de escopo significa "todas as empresas" (só super admin; a
-- decisão é do Scope, derivado do Principal, nunca de entrada do cliente).

-- name: GetUser :one
SELECT * FROM users
WHERE id = $1
  AND deleted_at IS NULL
  AND (sqlc.narg(company_id)::uuid IS NULL OR company_id = sqlc.narg(company_id)::uuid);

-- name: GetUserByIdentity :one
SELECT u.* FROM users u
JOIN user_identities i ON i.user_id = u.id
WHERE i.issuer = $1 AND i.subject = $2 AND u.deleted_at IS NULL;

-- name: GetUserByEmail :one
SELECT * FROM users WHERE email = $1 AND deleted_at IS NULL;

-- name: ListFarmIDsByUsers :many
SELECT user_id::text AS user_id, farm_id::text AS farm_id
FROM user_farm_access
WHERE user_id = ANY(sqlc.arg(user_ids)::text[]::uuid[])
ORDER BY user_id, farm_id;

-- name: InsertUser :exec
INSERT INTO users (id, company_id, name, email, role, status, farm_scope_mode, primary_farm_id,
                   last_access_at, deleted_at, created_at, version)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12);

-- Trava a linha do usuário até o fim da transação: serializa escritores do mesmo usuário e
-- devolve o estado gravado para a checagem de versão e do último super admin.
-- name: LockUser :one
SELECT company_id, role, status, version, deleted_at FROM users WHERE id = $1 FOR UPDATE;

-- name: UpdateUser :execrows
UPDATE users
SET company_id = $2, name = $3, email = $4, role = $5, status = $6, farm_scope_mode = $7,
    primary_farm_id = $8, last_access_at = $9, deleted_at = $10,
    version = version + 1, updated_at = now()
WHERE id = $1 AND version = $11 AND deleted_at IS NULL;

-- name: CountOtherActiveUsersByRole :one
SELECT count(*)::int FROM users
WHERE role = $1 AND status = 'active' AND deleted_at IS NULL AND id <> $2;

-- name: CountActiveUsersByRole :one
SELECT count(*)::int FROM users WHERE role = $1 AND status = 'active' AND deleted_at IS NULL;

-- Serializa quem pode remover o último super admin (lock de transação, liberado no commit).
-- name: LockSuperAdminGuard :exec
SELECT pg_advisory_xact_lock(sqlc.arg(lock_key)::bigint);

-- Remove o que saiu do escopo e tudo o que ficou em outra empresa (a FK composta não tem
-- ON UPDATE CASCADE, então isso precisa vir antes de mudar users.company_id).
-- name: DeleteStaleUserFarmAccess :exec
DELETE FROM user_farm_access
WHERE user_id = $1
  AND (company_id <> $2 OR NOT (farm_id = ANY(sqlc.arg(farm_ids)::text[]::uuid[])));

-- name: InsertUserFarmAccess :exec
INSERT INTO user_farm_access (user_id, farm_id, company_id)
SELECT $1, f, $2 FROM unnest(sqlc.arg(farm_ids)::text[]::uuid[]) AS f
ON CONFLICT (user_id, farm_id) DO NOTHING;

-- Listagem da tela Usuários: usuário, nome da empresa e convite aberto (no máximo um, índice
-- único parcial) em uma ida ao banco.
-- name: ListUsers :many
SELECT sqlc.embed(u), c.name AS company_name,
       i.id AS invitation_id, i.company_id AS invitation_company_id, i.user_id AS invitation_user_id,
       i.invited_by_user_id AS invitation_invited_by, i.account_created AS invitation_account_created,
       i.expires_at AS invitation_expires_at, i.last_sent_at AS invitation_last_sent_at,
       i.send_count AS invitation_send_count, i.accepted_at AS invitation_accepted_at,
       i.cancelled_at AS invitation_cancelled_at, i.created_at AS invitation_created_at
FROM users u
JOIN companies c ON c.id = u.company_id
LEFT JOIN invitations i
       ON i.user_id = u.id AND u.status = 'invited' AND i.accepted_at IS NULL AND i.cancelled_at IS NULL
WHERE u.deleted_at IS NULL
  AND (sqlc.narg(scope_company_id)::uuid IS NULL OR u.company_id = sqlc.narg(scope_company_id)::uuid)
  AND (sqlc.narg(company_id)::uuid IS NULL OR u.company_id = sqlc.narg(company_id)::uuid)
  AND (sqlc.narg(role)::text IS NULL OR u.role = sqlc.narg(role)::text)
  AND (sqlc.narg(status)::text IS NULL OR u.status = sqlc.narg(status)::text)
  AND (sqlc.narg(search)::text IS NULL
       OR u.name ILIKE sqlc.narg(search)::text ESCAPE '\'
       OR u.email ILIKE sqlc.narg(search)::text ESCAPE '\'
       OR c.name ILIKE sqlc.narg(search)::text ESCAPE '\')
ORDER BY u.name, u.id
LIMIT sqlc.arg(page_limit)::int OFFSET sqlc.arg(page_offset)::int;

-- name: CountListedUsers :one
SELECT count(*)::int
FROM users u
JOIN companies c ON c.id = u.company_id
WHERE u.deleted_at IS NULL
  AND (sqlc.narg(scope_company_id)::uuid IS NULL OR u.company_id = sqlc.narg(scope_company_id)::uuid)
  AND (sqlc.narg(company_id)::uuid IS NULL OR u.company_id = sqlc.narg(company_id)::uuid)
  AND (sqlc.narg(role)::text IS NULL OR u.role = sqlc.narg(role)::text)
  AND (sqlc.narg(status)::text IS NULL OR u.status = sqlc.narg(status)::text)
  AND (sqlc.narg(search)::text IS NULL
       OR u.name ILIKE sqlc.narg(search)::text ESCAPE '\'
       OR u.email ILIKE sqlc.narg(search)::text ESCAPE '\'
       OR c.name ILIKE sqlc.narg(search)::text ESCAPE '\');

-- name: CountUsersByRole :many
SELECT role, count(*)::int AS total
FROM users
WHERE deleted_at IS NULL
  AND (sqlc.narg(company_id)::uuid IS NULL OR company_id = sqlc.narg(company_id)::uuid)
GROUP BY role;

-- name: CountUserCompanies :one
SELECT count(DISTINCT company_id)::int
FROM users
WHERE deleted_at IS NULL
  AND (sqlc.narg(company_id)::uuid IS NULL OR company_id = sqlc.narg(company_id)::uuid);

-- name: CountPendingInvitations :one
SELECT count(*)::int
FROM invitations
WHERE accepted_at IS NULL AND cancelled_at IS NULL
  AND (sqlc.narg(company_id)::uuid IS NULL OR company_id = sqlc.narg(company_id)::uuid);

-- name: ListCompanyIdentities :many
SELECT i.user_id::text AS user_id, i.issuer, i.subject
FROM user_identities i
JOIN users u ON u.id = i.user_id
WHERE u.company_id = $1 AND u.deleted_at IS NULL
ORDER BY i.user_id;

-- name: CountUsersByStatus :many
SELECT status, count(*)::int AS total
FROM users
WHERE company_id = $1 AND deleted_at IS NULL
GROUP BY status;
