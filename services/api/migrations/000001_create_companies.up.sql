-- Empresa é o tenant (ADR 0013). Empresa nunca é apagada: desativar é status = 'inactive'.
-- Contagens (farm_count, user_count) são calculadas na leitura, não guardadas.
CREATE TABLE companies (
    id                   uuid        NOT NULL DEFAULT gen_random_uuid(),
    name                 text        NOT NULL,
    document             text        NOT NULL,
    city                 text        NOT NULL,
    state                text        NOT NULL,
    plan                 text        NOT NULL,
    status               text        NOT NULL,
    trial_ends_at        timestamptz,
    is_platform_operator boolean     NOT NULL DEFAULT false,
    created_at           timestamptz NOT NULL DEFAULT now(),
    updated_at           timestamptz NOT NULL DEFAULT now(),
    version              integer     NOT NULL DEFAULT 1,

    CONSTRAINT pk_companies PRIMARY KEY (id),
    CONSTRAINT uq_companies_document UNIQUE (document),
    CONSTRAINT ck_companies_name CHECK (btrim(name) <> ''),
    CONSTRAINT ck_companies_city CHECK (btrim(city) <> ''),
    CONSTRAINT ck_companies_document_cnpj CHECK (document ~ '^[0-9]{14}$'),
    CONSTRAINT ck_companies_state CHECK (state ~ '^[A-Z]{2}$'),
    CONSTRAINT ck_companies_plan CHECK (plan IN ('basic', 'pro', 'trial')),
    CONSTRAINT ck_companies_status CHECK (status IN ('active', 'trial', 'inactive')),
    CONSTRAINT ck_companies_trial_end CHECK (status <> 'trial' OR trial_ends_at IS NOT NULL),
    -- Desativar a operadora trancaria para fora todos os super admins.
    CONSTRAINT ck_companies_operator_active CHECK (NOT is_platform_operator OR status = 'active'),
    CONSTRAINT ck_companies_version CHECK (version >= 1)
);

-- No máximo uma empresa operadora; o super admin pertence a ela.
CREATE UNIQUE INDEX uq_companies_platform_operator
    ON companies (is_platform_operator)
    WHERE is_platform_operator;

-- DML mínimo do api_app: sem DELETE (empresa não é apagada). Os default privileges do init
-- concedem SELECT, INSERT, UPDATE e DELETE; aqui retiramos o que esta tabela não usa.
REVOKE DELETE ON companies FROM api_app;
