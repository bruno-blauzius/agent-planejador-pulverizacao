-- Esquema da fazenda (sem endpoints nesta entrega): necessário para as FKs compostas de
-- user_farm_access e users.primary_farm_id. PostGIS é criado em infra/docker/postgres/init
-- (exige superusuário); sem ele, falhamos aqui com uma mensagem clara.
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_extension WHERE extname = 'postgis') THEN
        RAISE EXCEPTION 'extensao postgis ausente no banco %: crie-a como superusuario (ver infra/docker/postgres/init)', current_database();
    END IF;
END
$$;

CREATE TABLE farms (
    id                uuid                      NOT NULL DEFAULT gen_random_uuid(),
    company_id        uuid                      NOT NULL,
    name              text                      NOT NULL,
    code              text,
    cooperative       text,
    production_type   text                      NOT NULL,
    city              text                      NOT NULL,
    state             text                      NOT NULL,
    -- 'pending' do contrato é calculado (proprietário com documento pendente), nunca gravado.
    status            text                      NOT NULL DEFAULT 'draft',
    postal_code       text,
    car_number        text,
    forecast_point    geometry(Point, 4326),
    perimeter         geometry(MultiPolygon, 4326),
    -- Coluna gerada: a área não diverge do polígono (geography usa o esferoide, não graus).
    perimeter_area_ha numeric(14, 4) GENERATED ALWAYS AS (
        round((ST_Area(perimeter::geography) / 10000.0)::numeric, 4)
    ) STORED,
    deleted_at        timestamptz,
    created_at        timestamptz               NOT NULL DEFAULT now(),
    updated_at        timestamptz               NOT NULL DEFAULT now(),
    version           integer                   NOT NULL DEFAULT 1,

    CONSTRAINT pk_farms PRIMARY KEY (id),
    -- Alvo das FKs compostas de tenant (ADR 0013).
    CONSTRAINT uq_farms_company_id_id UNIQUE (company_id, id),
    CONSTRAINT fk_farms_company FOREIGN KEY (company_id) REFERENCES companies (id),
    CONSTRAINT ck_farms_name CHECK (btrim(name) <> ''),
    CONSTRAINT ck_farms_production_type CHECK (production_type IN ('coffee', 'grains', 'livestock', 'mixed')),
    CONSTRAINT ck_farms_state CHECK (state ~ '^[A-Z]{2}$'),
    CONSTRAINT ck_farms_status CHECK (status IN ('draft', 'active')),
    CONSTRAINT ck_farms_postal_code CHECK (postal_code IS NULL OR postal_code ~ '^[0-9]{8}$'),
    CONSTRAINT ck_farms_perimeter_valid CHECK (perimeter IS NULL OR ST_IsValid(perimeter)),
    CONSTRAINT ck_farms_version CHECK (version >= 1)
);

CREATE INDEX ix_farms_company_status ON farms (company_id, status) WHERE deleted_at IS NULL;

-- Exclusão no produto é soft delete (deleted_at); nada de DELETE em runtime.
REVOKE DELETE ON farms FROM api_app;
