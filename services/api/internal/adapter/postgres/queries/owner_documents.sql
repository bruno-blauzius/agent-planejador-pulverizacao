-- Documentos de proprietário (ADR 0020): metadados; o arquivo está no S3 em storage_key.

-- name: ListOwnerDocuments :many
SELECT id, company_id, owner_id, kind, file_name, content_type, size_bytes, storage_key,
       uploaded_by_user_id, uploaded_at
FROM owner_documents
WHERE company_id = sqlc.arg(company_id) AND owner_id = sqlc.arg(owner_id) AND deleted_at IS NULL
ORDER BY uploaded_at, id;

-- name: GetOwnerDocument :one
SELECT id, company_id, owner_id, kind, file_name, content_type, size_bytes, storage_key,
       uploaded_by_user_id, uploaded_at
FROM owner_documents
WHERE company_id = sqlc.arg(company_id) AND owner_id = sqlc.arg(owner_id) AND id = sqlc.arg(id)
  AND deleted_at IS NULL;

-- name: InsertOwnerDocument :exec
INSERT INTO owner_documents (id, company_id, owner_id, kind, file_name, content_type, size_bytes,
                             storage_key, uploaded_by_user_id, uploaded_at)
VALUES (sqlc.arg(id), sqlc.arg(company_id), sqlc.arg(owner_id), sqlc.arg(kind), sqlc.arg(file_name),
        sqlc.arg(content_type), sqlc.arg(size_bytes), sqlc.arg(storage_key), sqlc.arg(uploaded_by_user_id),
        sqlc.arg(uploaded_at));

-- name: RemoveOwnerDocument :execrows
UPDATE owner_documents SET deleted_at = sqlc.arg(deleted_at)
WHERE company_id = sqlc.arg(company_id) AND owner_id = sqlc.arg(owner_id) AND id = sqlc.arg(id)
  AND deleted_at IS NULL;

-- Resultado de domain.RequiredDocumentsMissing; não mexe na versão (não é edição do cadastro).
-- name: SetOwnerDocumentsPending :exec
UPDATE owners SET documents_pending = sqlc.arg(documents_pending)
WHERE company_id = sqlc.arg(company_id) AND id = sqlc.arg(id) AND deleted_at IS NULL;
