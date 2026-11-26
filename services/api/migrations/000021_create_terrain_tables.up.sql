-- Relevo do talhão (ADR 0021). Classe de relevo e compatibilidade com o método são derivadas
-- no domínio Go (configuração em platform), nunca gravadas como regra aqui; o banco guarda a
-- medida (inclusive a distribuição) e impõe só invariantes físicas e de consistência.

-- Execução de "calcular relevo" pedida pelo usuário para uma fazenda.
CREATE TABLE terrain_runs (
    id           uuid        NOT NULL DEFAULT gen_random_uuid(),
    company_id   uuid        NOT NULL,
    farm_id      uuid        NOT NULL,
    scope        text        NOT NULL,
    requested_by uuid        NOT NULL,
    requested_at timestamptz NOT NULL DEFAULT now(),
    finished_at  timestamptz,

    CONSTRAINT pk_terrain_runs PRIMARY KEY (id),
    -- Alvo da FK composta de field_terrain.
    CONSTRAINT uq_terrain_runs_company_id_id UNIQUE (company_id, id),
    CONSTRAINT fk_terrain_runs_farm FOREIGN KEY (company_id, farm_id) REFERENCES farms (company_id, id),
    -- FK simples, como audit_events e owner_documents: o autor pode ser super admin da
    -- operadora, que é de outra empresa.
    CONSTRAINT fk_terrain_runs_requester FOREIGN KEY (requested_by) REFERENCES users (id),
    CONSTRAINT ck_terrain_runs_scope CHECK (scope IN ('missing', 'all')),
    CONSTRAINT ck_terrain_runs_finished CHECK (finished_at IS NULL OR finished_at >= requested_at)
);

CREATE INDEX ix_terrain_runs_company_farm ON terrain_runs (company_id, farm_id, requested_at DESC);
CREATE INDEX ix_terrain_runs_requester ON terrain_runs (requested_by);

-- Relevo calculado: um registro por talhão.
CREATE TABLE field_terrain (
    id                     uuid        NOT NULL DEFAULT gen_random_uuid(),
    company_id             uuid        NOT NULL,
    farm_id                uuid        NOT NULL,
    field_id               uuid        NOT NULL,
    status                 text        NOT NULL,
    failure_reason         text,
    run_id                 uuid,
    requested_at           timestamptz NOT NULL DEFAULT now(),
    calculated_at          timestamptz,
    slope_mean_pct         numeric(7, 3),
    slope_p90_pct          numeric(7, 3),
    slope_max_pct          numeric(7, 3),
    elevation_min_m        numeric(8, 2),
    elevation_max_m        numeric(8, 2),
    position               text,
    aspect                 text,
    -- Área (ha) por faixa de 1 % de declividade: faixas [0,1) ... [99,100) e a 101a, '>= 100 %'.
    -- Guardar a distribuição permite derivar a classe de relevo e a fração acima de um limite
    -- sem recalcular (ADR 0021, decisão 3).
    slope_histogram_ha     numeric[],
    -- Perfil de altitude ao longo da linha de maior declive (lista de pontos).
    profile                jsonb,
    elevation_source       text,
    elevation_resolution_m numeric(8, 2),
    elevation_version      text,
    created_at             timestamptz NOT NULL DEFAULT now(),
    updated_at             timestamptz NOT NULL DEFAULT now(),
    version                integer     NOT NULL DEFAULT 1,

    CONSTRAINT pk_field_terrain PRIMARY KEY (id),
    -- Um relevo por talhão.
    CONSTRAINT uq_field_terrain_company_field UNIQUE (company_id, field_id),
    -- FKs compostas: o banco recusa talhão, fazenda ou execução de outra empresa.
    CONSTRAINT fk_field_terrain_field FOREIGN KEY (company_id, field_id) REFERENCES fields (company_id, id),
    CONSTRAINT fk_field_terrain_farm FOREIGN KEY (company_id, farm_id) REFERENCES farms (company_id, id),
    CONSTRAINT fk_field_terrain_run FOREIGN KEY (company_id, run_id) REFERENCES terrain_runs (company_id, id),
    CONSTRAINT ck_field_terrain_status CHECK (status IN ('pending', 'calculating', 'ready', 'failed')),
    CONSTRAINT ck_field_terrain_position CHECK (position IS NULL OR position IN ('hilltop', 'slope', 'lowland')),
    CONSTRAINT ck_field_terrain_aspect CHECK (aspect IS NULL OR aspect IN
        ('north', 'northeast', 'east', 'southeast', 'south', 'southwest', 'west', 'northwest', 'flat')),
    CONSTRAINT ck_field_terrain_failure CHECK (status <> 'failed' OR (failure_reason IS NOT NULL AND btrim(failure_reason) <> '')),
    -- Pronto exige todas as medidas: nunca devolvemos relevo "pela metade".
    CONSTRAINT ck_field_terrain_ready CHECK (status <> 'ready' OR (
        calculated_at IS NOT NULL
        AND slope_mean_pct IS NOT NULL AND slope_p90_pct IS NOT NULL AND slope_max_pct IS NOT NULL
        AND elevation_min_m IS NOT NULL AND elevation_max_m IS NOT NULL
        AND position IS NOT NULL AND aspect IS NOT NULL
        AND slope_histogram_ha IS NOT NULL AND profile IS NOT NULL
        AND elevation_source IS NOT NULL AND elevation_resolution_m IS NOT NULL AND elevation_version IS NOT NULL
    )),
    CONSTRAINT ck_field_terrain_slope_nonneg CHECK (
        (slope_mean_pct IS NULL OR slope_mean_pct >= 0)
        AND (slope_p90_pct IS NULL OR slope_p90_pct >= 0)
        AND (slope_max_pct IS NULL OR slope_max_pct >= 0)
    ),
    CONSTRAINT ck_field_terrain_slope_order CHECK (
        (slope_max_pct IS NULL OR slope_mean_pct IS NULL OR slope_max_pct >= slope_mean_pct)
        AND (slope_max_pct IS NULL OR slope_p90_pct IS NULL OR slope_max_pct >= slope_p90_pct)
    ),
    CONSTRAINT ck_field_terrain_elevation_order CHECK (
        elevation_min_m IS NULL OR elevation_max_m IS NULL OR elevation_max_m >= elevation_min_m
    ),
    CONSTRAINT ck_field_terrain_histogram CHECK (
        slope_histogram_ha IS NULL OR (
            array_length(slope_histogram_ha, 1) = 101
            AND array_position(slope_histogram_ha, NULL) IS NULL
            AND 0 <= ALL (slope_histogram_ha)
        )
    ),
    CONSTRAINT ck_field_terrain_profile CHECK (profile IS NULL OR jsonb_typeof(profile) = 'array'),
    CONSTRAINT ck_field_terrain_resolution CHECK (elevation_resolution_m IS NULL OR elevation_resolution_m > 0),
    CONSTRAINT ck_field_terrain_version CHECK (version >= 1)
);

-- Relevo da fazenda (lista, resumo, "calcular os que faltam"); começa por company_id (tenant).
CREATE INDEX ix_field_terrain_company_farm ON field_terrain (company_id, farm_id);
-- Lado filho da FK da execução e andamento de uma execução.
CREATE INDEX ix_field_terrain_company_run ON field_terrain (company_id, run_id) WHERE run_id IS NOT NULL;

-- Ajuste manual (levantamento topográfico ou avaliação de campo): prevalece sobre o calculado,
-- que continua guardado ao lado. Pode ser removido de verdade (volta ao valor calculado).
CREATE TABLE field_terrain_adjustments (
    id             uuid          NOT NULL DEFAULT gen_random_uuid(),
    company_id     uuid          NOT NULL,
    field_id       uuid          NOT NULL,
    origin         text          NOT NULL,
    slope_mean_pct numeric(5, 2) NOT NULL,
    slope_max_pct  numeric(5, 2) NOT NULL,
    position       text          NOT NULL,
    justification  text          NOT NULL,
    adjusted_by    uuid          NOT NULL,
    adjusted_at    timestamptz   NOT NULL DEFAULT now(),
    version        integer       NOT NULL DEFAULT 1,

    CONSTRAINT pk_field_terrain_adjustments PRIMARY KEY (id),
    CONSTRAINT uq_field_terrain_adjustments_company_field UNIQUE (company_id, field_id),
    CONSTRAINT fk_field_terrain_adjustments_field FOREIGN KEY (company_id, field_id) REFERENCES fields (company_id, id),
    -- FK simples pelo mesmo motivo de terrain_runs.requested_by.
    CONSTRAINT fk_field_terrain_adjustments_user FOREIGN KEY (adjusted_by) REFERENCES users (id),
    CONSTRAINT ck_field_terrain_adjustments_origin CHECK (origin IN ('topographic_survey', 'field_assessment')),
    CONSTRAINT ck_field_terrain_adjustments_mean CHECK (slope_mean_pct BETWEEN 0 AND 100),
    CONSTRAINT ck_field_terrain_adjustments_max CHECK (slope_max_pct BETWEEN 0 AND 100 AND slope_max_pct >= slope_mean_pct),
    CONSTRAINT ck_field_terrain_adjustments_position CHECK (position IN ('hilltop', 'slope', 'lowland')),
    CONSTRAINT ck_field_terrain_adjustments_justification CHECK (btrim(justification) <> '' AND length(justification) <= 1000),
    CONSTRAINT ck_field_terrain_adjustments_version CHECK (version >= 1)
);

CREATE INDEX ix_field_terrain_adjustments_user ON field_terrain_adjustments (adjusted_by);

-- Sem DELETE onde a linha não some (o relevo é recalculado, não apagado). O ajuste mantém DELETE.
REVOKE DELETE, TRUNCATE ON field_terrain FROM api_app;
REVOKE DELETE, TRUNCATE ON terrain_runs FROM api_app;
REVOKE TRUNCATE ON field_terrain_adjustments FROM api_app;
