-- Sessão do navegador (ADR 0015, D6). Sem company_id: a sessão é resolvida por token_hash e
-- junta users na mesma ida ao banco. O refresh token é credencial: só o api_app acessa e a
-- chave de cifra vem do ambiente, nunca do banco.
CREATE TABLE auth_sessions (
    id                uuid        NOT NULL DEFAULT gen_random_uuid(),
    user_id           uuid        NOT NULL,
    -- SHA-256 do valor opaco do cookie; o valor em claro nunca é gravado.
    token_hash        bytea       NOT NULL,
    issuer            text        NOT NULL,
    -- sid do Keycloak, usado no back-channel logout.
    idp_session_id    text,
    refresh_token_enc bytea       NOT NULL,
    enc_key_id        smallint    NOT NULL,
    -- Papéis do último token renovado (resource_access.agentes-agro.roles).
    granted_roles     text[]      NOT NULL,
    -- A API zera (now()) após mudança de papel/escopo para renovar o token na próxima requisição.
    access_expires_at timestamptz NOT NULL,
    expires_at        timestamptz NOT NULL,
    last_seen_at      timestamptz,
    revoked_at        timestamptz,
    created_at        timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT pk_auth_sessions PRIMARY KEY (id),
    CONSTRAINT uq_auth_sessions_token_hash UNIQUE (token_hash),
    CONSTRAINT fk_auth_sessions_user FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE,
    CONSTRAINT ck_auth_sessions_token_hash CHECK (octet_length(token_hash) = 32),
    CONSTRAINT ck_auth_sessions_issuer CHECK (btrim(issuer) <> ''),
    CONSTRAINT ck_auth_sessions_idp_session CHECK (idp_session_id IS NULL OR btrim(idp_session_id) <> ''),
    CONSTRAINT ck_auth_sessions_refresh_token CHECK (octet_length(refresh_token_enc) > 0)
);

-- Sessões ativas do usuário (encerrar todas ao desativar).
CREATE INDEX ix_auth_sessions_user_active ON auth_sessions (user_id) WHERE revoked_at IS NULL;
-- Back-channel logout por sid.
CREATE INDEX ix_auth_sessions_idp_session ON auth_sessions (issuer, idp_session_id) WHERE idp_session_id IS NOT NULL;
-- Rotina de limpeza das expiradas.
CREATE INDEX ix_auth_sessions_expires_at ON auth_sessions (expires_at);
