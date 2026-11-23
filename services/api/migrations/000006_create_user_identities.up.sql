-- Vínculo com o provedor de identidade (ADR 0015, D4). Sem company_id de propósito: a busca
-- (issuer, subject) acontece no login, antes de a empresa ser conhecida.
-- DELETE continua concedido ao api_app: ao remover o usuário no produto o vínculo é desfeito,
-- para o mesmo sub poder ser vinculado de novo em um convite futuro.
CREATE TABLE user_identities (
    id            uuid        NOT NULL DEFAULT gen_random_uuid(),
    user_id       uuid        NOT NULL,
    issuer        text        NOT NULL,
    subject       text        NOT NULL,
    last_login_at timestamptz,
    created_at    timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT pk_user_identities PRIMARY KEY (id),
    -- Uma identidade externa pertence a um único usuário.
    CONSTRAINT uq_user_identities_issuer_subject UNIQUE (issuer, subject),
    -- Um usuário tem uma identidade por provedor.
    CONSTRAINT uq_user_identities_user_issuer UNIQUE (user_id, issuer),
    -- Usuários não são apagados (soft delete); a FK recusa apagar um usuário que ainda tem vínculo.
    CONSTRAINT fk_user_identities_user FOREIGN KEY (user_id) REFERENCES users (id),
    CONSTRAINT ck_user_identities_issuer CHECK (btrim(issuer) <> ''),
    CONSTRAINT ck_user_identities_subject CHECK (btrim(subject) <> '')
);
