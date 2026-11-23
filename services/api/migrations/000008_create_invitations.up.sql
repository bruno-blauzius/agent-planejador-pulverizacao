-- Convite por e-mail (ADR 0015, D7). Papel, empresa e escopo ficam no próprio users.
CREATE TABLE invitations (
    id                 uuid        NOT NULL DEFAULT gen_random_uuid(),
    company_id         uuid        NOT NULL,
    user_id            uuid        NOT NULL,
    invited_by_user_id uuid        NOT NULL,
    expires_at         timestamptz NOT NULL,
    last_sent_at       timestamptz NOT NULL DEFAULT now(),
    send_count         integer     NOT NULL DEFAULT 1,
    accepted_at        timestamptz,
    cancelled_at       timestamptz,
    created_at         timestamptz NOT NULL DEFAULT now(),
    updated_at         timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT pk_invitations PRIMARY KEY (id),
    -- FK composta de tenant. ON UPDATE CASCADE: se o super admin mudar o usuário de empresa,
    -- o histórico de convites o acompanha (senão todo usuário convidado ficaria imóvel).
    CONSTRAINT fk_invitations_user FOREIGN KEY (company_id, user_id)
        REFERENCES users (company_id, id) ON UPDATE CASCADE,
    -- Simples (sem company_id) de propósito: quem convida pode ser super admin da empresa
    -- operadora, que não é a empresa do convidado.
    CONSTRAINT fk_invitations_invited_by FOREIGN KEY (invited_by_user_id) REFERENCES users (id),
    CONSTRAINT ck_invitations_send_count CHECK (send_count >= 1),
    CONSTRAINT ck_invitations_outcome CHECK (accepted_at IS NULL OR cancelled_at IS NULL)
);

-- Um convite aberto por usuário.
CREATE UNIQUE INDEX uq_invitations_open_user
    ON invitations (user_id)
    WHERE accepted_at IS NULL AND cancelled_at IS NULL;

-- Convites pendentes por empresa (resumo das telas Usuários e Empresas).
CREATE INDEX ix_invitations_company_open
    ON invitations (company_id)
    WHERE accepted_at IS NULL AND cancelled_at IS NULL;

-- Lado filho das FKs.
CREATE INDEX ix_invitations_company_user ON invitations (company_id, user_id);
CREATE INDEX ix_invitations_invited_by ON invitations (invited_by_user_id);

REVOKE DELETE ON invitations FROM api_app;
