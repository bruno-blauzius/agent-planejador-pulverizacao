-- Documentos de proprietário (ADR 0020). O arquivo fica no S3 (storage_key); aqui só os
-- metadados. owners.documents_pending é mantido pela aplicação (domain.RequiredDocumentsMissing)
-- a cada envio ou remoção: a regra "que documento cada tipo de pessoa exige" fica no domínio,
-- e o banco guarda o resultado para filtrar fazendas pendentes sem repetir a regra em SQL.

-- Padrão constante: no PostgreSQL 11+ o ADD COLUMN não reescreve a tabela. Todo proprietário
-- existente nasce pendente, porque nenhum documento foi enviado ainda.
ALTER TABLE owners ADD COLUMN documents_pending boolean NOT NULL DEFAULT true;

CREATE TABLE owner_documents (
    id                  uuid        NOT NULL DEFAULT gen_random_uuid(),
    company_id          uuid        NOT NULL,
    owner_id            uuid        NOT NULL,
    kind                text        NOT NULL,
    file_name           text        NOT NULL,
    content_type        text        NOT NULL,
    size_bytes          bigint      NOT NULL,
    storage_key         text        NOT NULL,
    uploaded_by_user_id uuid        NOT NULL,
    uploaded_at         timestamptz NOT NULL DEFAULT now(),
    deleted_at          timestamptz,

    CONSTRAINT pk_owner_documents PRIMARY KEY (id),
    -- FK composta: o banco recusa documento de proprietário de outra empresa.
    CONSTRAINT fk_owner_documents_owner FOREIGN KEY (company_id, owner_id) REFERENCES owners (company_id, id),
    -- Simples (como audit_events.actor_user_id): o super admin da operadora envia por outra empresa.
    CONSTRAINT fk_owner_documents_uploader FOREIGN KEY (uploaded_by_user_id) REFERENCES users (id),
    CONSTRAINT uq_owner_documents_storage_key UNIQUE (storage_key),
    CONSTRAINT ck_owner_documents_kind CHECK (kind IN ('id_card', 'driver_license', 'articles_of_incorporation', 'other')),
    CONSTRAINT ck_owner_documents_file_name CHECK (btrim(file_name) <> '' AND char_length(file_name) <= 200),
    CONSTRAINT ck_owner_documents_content_type CHECK (content_type IN ('application/pdf', 'image/jpeg', 'image/png')),
    CONSTRAINT ck_owner_documents_size CHECK (size_bytes > 0),
    -- A chave nunca sai do prefixo do proprietário (ver domain.NewOwnerDocument).
    CONSTRAINT ck_owner_documents_storage_key CHECK (
        storage_key = 'companies/' || company_id || '/owners/' || owner_id || '/documents/' || id
    )
);

-- Documentos de um proprietário (detalhe e cálculo da pendência).
CREATE INDEX ix_owner_documents_company_owner ON owner_documents (company_id, owner_id) WHERE deleted_at IS NULL;
-- Lado filho da FK do usuário que enviou.
CREATE INDEX ix_owner_documents_uploader ON owner_documents (uploaded_by_user_id);

-- Remover é soft delete: o histórico do que foi enviado não some.
REVOKE DELETE ON owner_documents FROM api_app;
