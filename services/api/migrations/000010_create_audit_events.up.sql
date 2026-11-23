-- Trilha de auditoria append-only: o api_app só tem INSERT e SELECT. Particionar por mês
-- quando o volume pedir (migração futura).
CREATE TABLE audit_events (
    id            bigint      GENERATED ALWAYS AS IDENTITY,
    company_id    uuid        NOT NULL,
    occurred_at   timestamptz NOT NULL DEFAULT now(),
    actor_kind    text        NOT NULL,
    actor_user_id uuid,
    -- Retrato do nome na hora do evento: sobrevive a renomeações.
    actor_name    text        NOT NULL,
    action        text        NOT NULL,
    entity_type   text        NOT NULL,
    entity_id     uuid        NOT NULL,
    -- Nunca senha, token ou documento.
    metadata      jsonb       NOT NULL DEFAULT '{}'::jsonb,

    CONSTRAINT pk_audit_events PRIMARY KEY (id),
    -- company_id é a empresa afetada. O ator pode ser de outra (super admin da operadora),
    -- por isso as FKs não são compostas.
    CONSTRAINT fk_audit_events_company FOREIGN KEY (company_id) REFERENCES companies (id),
    CONSTRAINT fk_audit_events_actor FOREIGN KEY (actor_user_id) REFERENCES users (id),
    CONSTRAINT ck_audit_events_actor_kind CHECK (actor_kind IN ('user', 'agent', 'system')),
    -- actor_user_id presente se e somente se o ator é um usuário.
    CONSTRAINT ck_audit_events_actor CHECK ((actor_kind = 'user') = (actor_user_id IS NOT NULL)),
    CONSTRAINT ck_audit_events_actor_name CHECK (btrim(actor_name) <> ''),
    CONSTRAINT ck_audit_events_action CHECK (btrim(action) <> ''),
    CONSTRAINT ck_audit_events_entity_type CHECK (btrim(entity_type) <> ''),
    CONSTRAINT ck_audit_events_metadata CHECK (jsonb_typeof(metadata) = 'object')
);

-- "Atividade recente" do painel.
CREATE INDEX ix_audit_events_company_occurred ON audit_events (company_id, occurred_at DESC);
CREATE INDEX ix_audit_events_actor ON audit_events (actor_user_id) WHERE actor_user_id IS NOT NULL;

-- Append-only imposto por privilégio: o serviço não consegue reescrever a história.
REVOKE UPDATE, DELETE, TRUNCATE ON audit_events FROM api_app;
