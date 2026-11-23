-- auth_sessions guarda credenciais: só GetSessionByTokenHash devolve refresh_token_enc, e é
-- usada apenas pelo adaptador de sessão.

-- name: InsertSession :exec
INSERT INTO auth_sessions (id, user_id, token_hash, issuer, idp_session_id, refresh_token_enc,
                           enc_key_id, granted_roles, access_expires_at, expires_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10);

-- name: TouchIdentityLogin :exec
UPDATE user_identities SET last_login_at = $3 WHERE user_id = $1 AND issuer = $2;

-- Revogada ou expirada some para quem consulta: sessão inválida é "não encontrada".
-- name: GetSessionByTokenHash :one
SELECT * FROM auth_sessions
WHERE token_hash = $1 AND revoked_at IS NULL AND expires_at > now();

-- Sem refresh novo (nulo), mantém o cifrado atual e a chave com que foi cifrado.
-- name: RenewSession :execrows
UPDATE auth_sessions
SET granted_roles = $2,
    access_expires_at = $3,
    refresh_token_enc = COALESCE(sqlc.narg(refresh_token_enc)::bytea, refresh_token_enc),
    enc_key_id = CASE WHEN sqlc.narg(refresh_token_enc)::bytea IS NULL THEN enc_key_id ELSE sqlc.arg(enc_key_id)::smallint END
WHERE id = $1 AND revoked_at IS NULL AND expires_at > now();

-- name: TouchSession :exec
UPDATE auth_sessions
SET last_seen_at = sqlc.arg(seen_at)::timestamptz
WHERE id = $1 AND revoked_at IS NULL
  AND (last_seen_at IS NULL OR last_seen_at <= sqlc.arg(seen_at)::timestamptz - sqlc.arg(min_interval)::interval);

-- name: RevokeSession :exec
UPDATE auth_sessions SET revoked_at = $2 WHERE id = $1 AND revoked_at IS NULL;

-- name: RevokeSessionsByIdPSession :exec
UPDATE auth_sessions SET revoked_at = $3
WHERE issuer = $1 AND idp_session_id = $2 AND revoked_at IS NULL;

-- name: RevokeUserSessions :exec
UPDATE auth_sessions SET revoked_at = $2 WHERE user_id = $1 AND revoked_at IS NULL;

-- Todos os usuários da empresa, inclusive removidos: a sessão de quem saiu também termina.
-- name: RevokeCompanySessions :exec
UPDATE auth_sessions SET revoked_at = $2
WHERE revoked_at IS NULL AND user_id IN (SELECT id FROM users WHERE company_id = $1);

-- name: ExpireUserAccess :exec
UPDATE auth_sessions SET access_expires_at = LEAST(access_expires_at, $2)
WHERE user_id = $1 AND revoked_at IS NULL;
