-- Só INSERT: o api_app não tem UPDATE nem DELETE em audit_events (migração 000010).
-- name: InsertAuditEvent :exec
INSERT INTO audit_events (company_id, occurred_at, actor_kind, actor_user_id, actor_name, action,
                          entity_type, entity_id, metadata)
VALUES ($1, COALESCE(sqlc.narg(occurred_at)::timestamptz, now()), $2, $3, $4, $5, $6, $7, $8);
