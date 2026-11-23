-- name: GetInvitation :one
SELECT i.* FROM invitations i
JOIN users u ON u.id = i.user_id
WHERE i.id = $1
  AND u.deleted_at IS NULL
  AND (sqlc.narg(company_id)::uuid IS NULL OR i.company_id = sqlc.narg(company_id)::uuid);

-- name: GetOpenInvitationByUser :one
SELECT * FROM invitations
WHERE user_id = $1 AND accepted_at IS NULL AND cancelled_at IS NULL;

-- name: InsertInvitation :exec
INSERT INTO invitations (id, company_id, user_id, invited_by_user_id, account_created, expires_at,
                         last_sent_at, send_count, accepted_at, cancelled_at, created_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11);

-- name: UpdateInvitation :execrows
UPDATE invitations
SET expires_at = $2, last_sent_at = $3, send_count = $4, accepted_at = $5, cancelled_at = $6,
    updated_at = now()
WHERE id = $1;
