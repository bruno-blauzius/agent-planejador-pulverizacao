-- Idempotente para o mesmo usuário: o conflito em (issuer, subject) devolve o dono atual, e o
-- adaptador decide entre "já vinculado a ele" e ErrIdentityAlreadyLinked.
-- name: LinkIdentity :one
INSERT INTO user_identities (user_id, issuer, subject)
VALUES ($1, $2, $3)
ON CONFLICT (issuer, subject) DO UPDATE SET issuer = user_identities.issuer
RETURNING user_id::text AS user_id;

-- name: UnlinkIdentities :exec
DELETE FROM user_identities WHERE user_id = $1;

-- name: GetIdentityByUser :one
SELECT user_id::text AS user_id, issuer, subject FROM user_identities
WHERE user_id = $1
ORDER BY created_at
LIMIT 1;
