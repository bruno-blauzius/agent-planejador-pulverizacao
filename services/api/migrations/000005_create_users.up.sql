-- Usuário do produto. Conta, senha e MFA são do Keycloak (ADR 0015); aqui ficam empresa,
-- escopo de fazendas, status no produto e a projeção do papel. Sem coluna de senha ou token.
-- Cada usuário pertence a uma única empresa (ADR 0013).
CREATE TABLE users (
    id              uuid        NOT NULL DEFAULT gen_random_uuid(),
    company_id      uuid        NOT NULL,
    name            text        NOT NULL,
    -- Normalizado pela aplicação (minúsculas, sem espaços nas pontas); o CHECK recusa o contrário,
    -- o que permite índice único simples em vez de índice sobre lower(email).
    email           text        NOT NULL,
    -- Projeção do client role do Keycloak; a fonte do papel é o token (ADR 0015, D3).
    role            text        NOT NULL,
    -- Vale só para o produto: desativar aqui nunca desabilita a conta do realm (D5).
    status          text        NOT NULL DEFAULT 'invited',
    farm_scope_mode text        NOT NULL DEFAULT 'all',
    primary_farm_id uuid,
    last_access_at  timestamptz,
    -- Soft delete (convite cancelado ou remoção): libera o e-mail para novo convite.
    deleted_at      timestamptz,
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now(),
    version         integer     NOT NULL DEFAULT 1,

    CONSTRAINT pk_users PRIMARY KEY (id),
    CONSTRAINT uq_users_company_id_id UNIQUE (company_id, id),
    CONSTRAINT fk_users_company FOREIGN KEY (company_id) REFERENCES companies (id),
    -- Fazenda removida não derruba o usuário: só zera a fazenda principal.
    CONSTRAINT fk_users_primary_farm FOREIGN KEY (company_id, primary_farm_id)
        REFERENCES farms (company_id, id) ON DELETE SET NULL (primary_farm_id),
    CONSTRAINT ck_users_name CHECK (btrim(name) <> ''),
    CONSTRAINT ck_users_email CHECK (
        email = lower(btrim(email))
        AND length(email) <= 254
        AND email ~ '^[^@[:space:]]+@[^@[:space:]]+$'
    ),
    CONSTRAINT ck_users_role CHECK (role IN ('super_admin', 'admin', 'planning_operator')),
    CONSTRAINT ck_users_status CHECK (status IN ('invited', 'active', 'inactive')),
    CONSTRAINT ck_users_farm_scope_mode CHECK (farm_scope_mode IN ('all', 'selected')),
    -- Só o operador de planejamento pode ter escopo restrito.
    CONSTRAINT ck_users_scope_role CHECK (farm_scope_mode = 'all' OR role = 'planning_operator'),
    CONSTRAINT ck_users_deleted_inactive CHECK (deleted_at IS NULL OR status = 'inactive'),
    CONSTRAINT ck_users_version CHECK (version >= 1)
);

-- E-mail único por produto entre usuários não removidos.
CREATE UNIQUE INDEX uq_users_email_active ON users (email) WHERE deleted_at IS NULL;

-- Tela Usuários e contagem de convites pendentes.
CREATE INDEX ix_users_company_status ON users (company_id, status) WHERE deleted_at IS NULL;

-- Apoia o ON DELETE SET NULL da FK composta da fazenda principal.
CREATE INDEX ix_users_primary_farm ON users (company_id, primary_farm_id) WHERE primary_farm_id IS NOT NULL;

REVOKE DELETE ON users FROM api_app;
