-- Proprietários da empresa (ADR 0013). A busca casa nome (ILIKE sem acento, ADR 0017) ou dígitos do CPF/CNPJ.
-- visible_farm_ids (escopo selecionado do operador) restringe aos proprietários vinculados
-- a essas fazendas.

-- name: CountOwners :one
SELECT count(*)::int
FROM owners o
WHERE o.company_id = sqlc.arg(company_id) AND o.deleted_at IS NULL
  AND (sqlc.narg(person_type)::text IS NULL OR o.person_type = sqlc.narg(person_type)::text)
  AND (sqlc.narg(search)::text IS NULL
       OR unaccent(o.name) ILIKE unaccent(sqlc.narg(search)::text) ESCAPE '\'
       OR o.document LIKE sqlc.narg(document_search)::text)
  AND (sqlc.narg(visible_farm_ids)::text[] IS NULL OR EXISTS (
       SELECT 1 FROM farm_owners l
       WHERE l.company_id = o.company_id AND l.owner_id = o.id
         AND l.farm_id = ANY(sqlc.narg(visible_farm_ids)::text[]::uuid[])));

-- name: ListOwners :many
SELECT o.*
FROM owners o
WHERE o.company_id = sqlc.arg(company_id) AND o.deleted_at IS NULL
  AND (sqlc.narg(person_type)::text IS NULL OR o.person_type = sqlc.narg(person_type)::text)
  AND (sqlc.narg(search)::text IS NULL
       OR unaccent(o.name) ILIKE unaccent(sqlc.narg(search)::text) ESCAPE '\'
       OR o.document LIKE sqlc.narg(document_search)::text)
  AND (sqlc.narg(visible_farm_ids)::text[] IS NULL OR EXISTS (
       SELECT 1 FROM farm_owners l
       WHERE l.company_id = o.company_id AND l.owner_id = o.id
         AND l.farm_id = ANY(sqlc.narg(visible_farm_ids)::text[]::uuid[])))
ORDER BY o.name, o.id
LIMIT sqlc.arg(page_limit)::int OFFSET sqlc.arg(page_offset)::int;

-- name: GetOwner :one
SELECT * FROM owners WHERE company_id = $1 AND id = $2 AND deleted_at IS NULL;

-- name: LockOwner :one
SELECT * FROM owners WHERE company_id = $1 AND id = $2 AND deleted_at IS NULL FOR UPDATE;

-- name: ListExistingOwnerIDs :many
SELECT o.id::text AS id
FROM owners o
WHERE o.company_id = sqlc.arg(company_id) AND o.deleted_at IS NULL
  AND o.id = ANY(sqlc.arg(owner_ids)::text[]::uuid[]);

-- name: InsertOwner :exec
INSERT INTO owners (id, company_id, person_type, name, document, birth_date, phone, email,
                    window_alerts_enabled, created_at, updated_at, version)
VALUES (sqlc.arg(id), sqlc.arg(company_id), sqlc.arg(person_type), sqlc.arg(name), sqlc.arg(document),
        sqlc.narg(birth_date), sqlc.narg(phone), sqlc.narg(email), sqlc.arg(window_alerts_enabled),
        sqlc.arg(created_at), sqlc.arg(updated_at), 1);

-- Documento e tipo de pessoa identificam o proprietário e nunca são regravados.
-- name: UpdateOwner :execrows
UPDATE owners
SET name = sqlc.arg(name), birth_date = sqlc.narg(birth_date), phone = sqlc.narg(phone),
    email = sqlc.narg(email), window_alerts_enabled = sqlc.arg(window_alerts_enabled),
    updated_at = sqlc.arg(updated_at), version = version + 1
WHERE company_id = sqlc.arg(company_id) AND id = sqlc.arg(id) AND deleted_at IS NULL
  AND version = sqlc.arg(version);

-- name: OwnerExists :one
SELECT EXISTS (SELECT 1 FROM owners WHERE company_id = $1 AND id = $2 AND deleted_at IS NULL);
