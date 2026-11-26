-- Proprietários (ADR 0013): pessoa física ou jurídica dona, arrendatária ou responsável
-- legal de fazendas da empresa. O documento é guardado só com dígitos; os dígitos
-- verificadores são validados pela aplicação (domínio), o banco garante o formato.
CREATE TABLE owners (
    id                    uuid        NOT NULL DEFAULT gen_random_uuid(),
    company_id            uuid        NOT NULL,
    person_type           text        NOT NULL,
    name                  text        NOT NULL,
    document              text        NOT NULL,
    birth_date            date,
    phone                 text,
    email                 text,
    window_alerts_enabled boolean     NOT NULL DEFAULT false,
    deleted_at            timestamptz,
    created_at            timestamptz NOT NULL DEFAULT now(),
    updated_at            timestamptz NOT NULL DEFAULT now(),
    version               integer     NOT NULL DEFAULT 1,

    CONSTRAINT pk_owners PRIMARY KEY (id),
    -- Alvo da FK composta de tenant de farm_owners.
    CONSTRAINT uq_owners_company_id_id UNIQUE (company_id, id),
    CONSTRAINT fk_owners_company FOREIGN KEY (company_id) REFERENCES companies (id),
    CONSTRAINT ck_owners_person_type CHECK (person_type IN ('individual', 'company')),
    CONSTRAINT ck_owners_name CHECK (btrim(name) <> ''),
    CONSTRAINT ck_owners_document CHECK (
        (person_type = 'individual' AND document ~ '^[0-9]{11}$')
        OR (person_type = 'company' AND document ~ '^[0-9]{14}$')
    ),
    CONSTRAINT ck_owners_birth_date CHECK (birth_date IS NULL OR person_type = 'individual'),
    CONSTRAINT ck_owners_phone CHECK (phone IS NULL OR phone ~ '^[0-9]{10,11}$'),
    CONSTRAINT ck_owners_email CHECK (email IS NULL OR (email = lower(email) AND position('@' IN email) > 1)),
    CONSTRAINT ck_owners_version CHECK (version >= 1)
);

-- Um CPF/CNPJ por empresa entre os proprietários não excluídos (409 no cadastro).
CREATE UNIQUE INDEX uq_owners_company_document ON owners (company_id, document) WHERE deleted_at IS NULL;
-- Listagem ordenada por nome dentro da empresa.
CREATE INDEX ix_owners_company_name ON owners (company_id, name) WHERE deleted_at IS NULL;

-- Exclusão no produto é soft delete (deleted_at); nada de DELETE em runtime.
REVOKE DELETE ON owners FROM api_app;
