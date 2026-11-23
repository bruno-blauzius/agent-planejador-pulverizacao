-- Catálogo global de culturas: igual para todas as empresas, a única tabela sem company_id.
-- É dado de referência carregado por migração (000003); o api_app só lê.
CREATE TABLE crops (
    code       text        NOT NULL,
    name       text        NOT NULL,
    is_active  boolean     NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT pk_crops PRIMARY KEY (code),
    CONSTRAINT ck_crops_code CHECK (code ~ '^[a-z0-9_]+$'),
    CONSTRAINT ck_crops_name CHECK (btrim(name) <> '')
);

REVOKE INSERT, UPDATE, DELETE ON crops FROM api_app;
