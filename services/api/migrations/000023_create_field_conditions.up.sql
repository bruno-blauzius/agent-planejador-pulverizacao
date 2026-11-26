-- Chuva, solo e trafegabilidade do talhão (ADR 0022, adendo da fase 2). O estado do solo e o
-- veredito por método são calculados no domínio Go; aqui ficam só os dados (textura, pluviômetros,
-- leituras, vistorias) e os limites de referência, com invariantes de integridade.

-- Textura do solo do talhão: nula até alguém informar (sem textura o estado é unknown).
-- ADD COLUMN sem DEFAULT não reescreve a tabela; NOT VALID + VALIDATE não segura o lock
-- exclusivo durante a varredura (tabela existente).
ALTER TABLE fields ADD COLUMN soil_texture text;
ALTER TABLE fields ADD CONSTRAINT ck_fields_soil_texture
    CHECK (soil_texture IS NULL OR soil_texture IN ('sandy', 'medium', 'clay')) NOT VALID;
ALTER TABLE fields VALIDATE CONSTRAINT ck_fields_soil_texture;

-- Pluviômetro da fazenda. Uma leitura vale para todos os talhões ligados a ele.
CREATE TABLE rain_gauges (
    id         uuid                  NOT NULL DEFAULT gen_random_uuid(),
    company_id uuid                  NOT NULL,
    farm_id    uuid                  NOT NULL,
    name       text                  NOT NULL,
    location   geometry(Point, 4326),
    deleted_at timestamptz,
    created_at timestamptz           NOT NULL DEFAULT now(),
    updated_at timestamptz           NOT NULL DEFAULT now(),
    version    integer               NOT NULL DEFAULT 1,

    CONSTRAINT pk_rain_gauges PRIMARY KEY (id),
    -- Alvo da FK composta de rain_readings.
    CONSTRAINT uq_rain_gauges_company_id_id UNIQUE (company_id, id),
    -- Alvo da FK composta de rain_gauge_fields: vínculo só com pluviômetro da MESMA fazenda.
    -- Também serve à lista de pluviômetros da fazenda (começa por company_id, farm_id).
    CONSTRAINT uq_rain_gauges_company_farm_id UNIQUE (company_id, farm_id, id),
    CONSTRAINT fk_rain_gauges_farm FOREIGN KEY (company_id, farm_id) REFERENCES farms (company_id, id),
    CONSTRAINT ck_rain_gauges_name CHECK (btrim(name) <> '' AND char_length(name) <= 100),
    CONSTRAINT ck_rain_gauges_location CHECK (location IS NULL OR ST_IsValid(location)),
    CONSTRAINT ck_rain_gauges_version CHECK (version >= 1)
);

-- Nome único por fazenda, sem diferenciar maiúsculas, entre os não removidos.
CREATE UNIQUE INDEX uq_rain_gauges_farm_name ON rain_gauges (company_id, farm_id, lower(name)) WHERE deleted_at IS NULL;

-- Vínculo talhão -> pluviômetro: no máximo um por talhão (PK no talhão). As duas FKs compostas
-- usam o mesmo farm_id, então talhão e pluviômetro são da mesma fazenda e da mesma empresa.
CREATE TABLE rain_gauge_fields (
    company_id    uuid        NOT NULL,
    farm_id       uuid        NOT NULL,
    field_id      uuid        NOT NULL,
    rain_gauge_id uuid        NOT NULL,
    linked_at     timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT pk_rain_gauge_fields PRIMARY KEY (field_id),
    CONSTRAINT fk_rain_gauge_fields_field FOREIGN KEY (company_id, farm_id, field_id)
        REFERENCES fields (company_id, farm_id, id),
    CONSTRAINT fk_rain_gauge_fields_gauge FOREIGN KEY (company_id, farm_id, rain_gauge_id)
        REFERENCES rain_gauges (company_id, farm_id, id)
);

-- Lado filho da FK do pluviômetro (talhões de um pluviômetro); começa por company_id (tenant).
CREATE INDEX ix_rain_gauge_fields_company_gauge ON rain_gauge_fields (company_id, farm_id, rain_gauge_id);

-- Leitura do pluviômetro: chuva acumulada desde a leitura anterior, lida em observed_at.
CREATE TABLE rain_readings (
    id            uuid          NOT NULL DEFAULT gen_random_uuid(),
    company_id    uuid          NOT NULL,
    rain_gauge_id uuid          NOT NULL,
    rain_mm       numeric(6, 2) NOT NULL,
    observed_at   timestamptz   NOT NULL,
    recorded_by   uuid          NOT NULL,
    recorded_at   timestamptz   NOT NULL DEFAULT now(),

    CONSTRAINT pk_rain_readings PRIMARY KEY (id),
    -- Uma leitura por pluviômetro e horário; também é o índice das somas por janela
    -- (company_id, pluviômetro, intervalo de observed_at).
    CONSTRAINT uq_rain_readings_gauge_observed UNIQUE (company_id, rain_gauge_id, observed_at),
    CONSTRAINT fk_rain_readings_gauge FOREIGN KEY (company_id, rain_gauge_id) REFERENCES rain_gauges (company_id, id),
    -- FK simples, como terrain_runs.requested_by: o autor pode ser super admin da operadora.
    CONSTRAINT fk_rain_readings_recorded_by FOREIGN KEY (recorded_by) REFERENCES users (id),
    -- Faixa física de uma leitura; acima disso é erro de digitação, não chuva.
    CONSTRAINT ck_rain_readings_rain CHECK (rain_mm >= 0 AND rain_mm <= 500)
);

CREATE INDEX ix_rain_readings_recorded_by ON rain_readings (recorded_by);

-- Vistoria do solo feita por uma pessoa. Só estados conhecidos: "não sei" não é vistoria.
CREATE TABLE soil_inspections (
    id           uuid        NOT NULL DEFAULT gen_random_uuid(),
    company_id   uuid        NOT NULL,
    farm_id      uuid        NOT NULL,
    field_id     uuid        NOT NULL,
    state        text        NOT NULL,
    inspected_at timestamptz NOT NULL,
    inspected_by uuid        NOT NULL,
    note         text,
    recorded_at  timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT pk_soil_inspections PRIMARY KEY (id),
    CONSTRAINT fk_soil_inspections_field FOREIGN KEY (company_id, farm_id, field_id)
        REFERENCES fields (company_id, farm_id, id),
    -- FK simples pelo mesmo motivo de rain_readings.recorded_by.
    CONSTRAINT fk_soil_inspections_inspected_by FOREIGN KEY (inspected_by) REFERENCES users (id),
    CONSTRAINT ck_soil_inspections_state CHECK (state IN ('dry', 'ideal', 'moist', 'waterlogged')),
    CONSTRAINT ck_soil_inspections_note CHECK (note IS NULL OR (btrim(note) <> '' AND char_length(note) <= 500))
);

-- Vistoria mais recente do talhão; começa por company_id (tenant).
CREATE INDEX ix_soil_inspections_company_field ON soil_inspections (company_id, field_id, inspected_at DESC);
-- Lado filho da FK composta do talhão (inclui farm_id) e da FK do autor.
CREATE INDEX ix_soil_inspections_company_farm_field ON soil_inspections (company_id, farm_id, field_id);
CREATE INDEX ix_soil_inspections_inspected_by ON soil_inspections (inspected_by);

-- Limites de trafegabilidade por textura: referência global (sem company_id, como crops),
-- uma linha por textura e data de vigência; vale a mais recente com vigência até hoje.
-- Os valores são política agronômica lida pelo domínio; o banco só garante que formam
-- faixas positivas e crescentes (a classificação continua no Go).
CREATE TABLE trafficability_thresholds (
    id                     uuid          NOT NULL DEFAULT gen_random_uuid(),
    soil_texture           text          NOT NULL,
    effective_from         date          NOT NULL,
    rain_48h_dry_mm        numeric(6, 2) NOT NULL,
    rain_48h_ideal_mm      numeric(6, 2) NOT NULL,
    rain_48h_moist_mm      numeric(6, 2) NOT NULL,
    moisture_dry_pct       numeric(6, 2) NOT NULL,
    moisture_ideal_pct     numeric(6, 2) NOT NULL,
    moisture_moist_pct     numeric(6, 2) NOT NULL,
    rain_72h_factor        numeric(4, 2) NOT NULL,
    drying_rate_mm_per_day numeric(6, 2) NOT NULL,
    -- Capacidade de campo de referência da textura (m³/m³): a Open-Meteo não a fornece.
    field_capacity_m3m3    numeric(4, 3) NOT NULL,
    source                 text          NOT NULL,
    created_at             timestamptz   NOT NULL DEFAULT now(),

    CONSTRAINT pk_trafficability_thresholds PRIMARY KEY (id),
    CONSTRAINT uq_trafficability_thresholds_texture_from UNIQUE (soil_texture, effective_from),
    CONSTRAINT ck_trafficability_thresholds_texture CHECK (soil_texture IN ('sandy', 'medium', 'clay')),
    CONSTRAINT ck_trafficability_thresholds_rain CHECK (
        rain_48h_dry_mm > 0 AND rain_48h_ideal_mm > rain_48h_dry_mm AND rain_48h_moist_mm > rain_48h_ideal_mm
    ),
    CONSTRAINT ck_trafficability_thresholds_moisture CHECK (
        moisture_dry_pct > 0 AND moisture_ideal_pct > moisture_dry_pct AND moisture_moist_pct > moisture_ideal_pct
    ),
    CONSTRAINT ck_trafficability_thresholds_factor CHECK (rain_72h_factor >= 1),
    CONSTRAINT ck_trafficability_thresholds_drying CHECK (drying_rate_mm_per_day > 0),
    CONSTRAINT ck_trafficability_thresholds_capacity CHECK (field_capacity_m3m3 > 0 AND field_capacity_m3m3 <= 1),
    CONSTRAINT ck_trafficability_thresholds_source CHECK (source IN ('heuristic', 'calibrated'))
);

-- Valores iniciais do doc de cruzamento (ADR 0022, decisão 2): heurística até a calibração.
INSERT INTO trafficability_thresholds
    (soil_texture, effective_from, rain_48h_dry_mm, rain_48h_ideal_mm, rain_48h_moist_mm,
     moisture_dry_pct, moisture_ideal_pct, moisture_moist_pct, rain_72h_factor,
     drying_rate_mm_per_day, field_capacity_m3m3, source)
VALUES
    ('sandy',  '2026-11-26', 2, 15, 30, 60, 90, 100, 1.5, 15, 0.150, 'heuristic'),
    ('medium', '2026-11-26', 2, 10, 20, 60, 90, 100, 1.5, 10, 0.250, 'heuristic'),
    ('clay',   '2026-11-26', 2,  8, 15, 60, 90, 100, 1.5,  6, 0.360, 'heuristic');

-- Leitura e vistoria são registros que não se apagam nem se reescrevem; o pluviômetro some por
-- soft delete; o vínculo do talhão é substituído (DELETE permitido). Limites: só leitura.
REVOKE UPDATE, DELETE, TRUNCATE ON rain_readings FROM api_app;
REVOKE UPDATE, DELETE, TRUNCATE ON soil_inspections FROM api_app;
REVOKE DELETE, TRUNCATE ON rain_gauges FROM api_app;
REVOKE TRUNCATE ON rain_gauge_fields FROM api_app;
REVOKE INSERT, UPDATE, DELETE, TRUNCATE ON trafficability_thresholds FROM api_app;
