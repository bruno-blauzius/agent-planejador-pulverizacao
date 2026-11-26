-- Histórico diário do estado do solo por talhão (ADR 0022, adendo da fase 4). Gravado uma vez
-- por dia local pelo executor da API com o mesmo veredito da rota de trafegabilidade; serve para
-- comparar com atolamentos reais e recalibrar os limites. É um retrato do que o domínio calculou,
-- não uma regra: nada aqui recalcula estado.
CREATE TABLE soil_state_history (
    id                 uuid          NOT NULL DEFAULT gen_random_uuid(),
    company_id         uuid          NOT NULL,
    farm_id            uuid          NOT NULL,
    field_id           uuid          NOT NULL,
    -- Dia no fuso das fazendas (America/Sao_Paulo); a aplicação decide qual é.
    local_day          date          NOT NULL,
    state              text          NOT NULL,
    basis              text          NOT NULL,
    -- Nulo quando não houve fonte de chuva (nem pluviômetro recente nem modelo).
    rain_source        text,
    last_48h_mm        numeric(7, 2),
    last_72h_mm        numeric(7, 2),
    -- Umidade do modelo em % da capacidade de campo; nula sem modelo ou sem textura.
    model_moisture_pct numeric(7, 2),
    soil_texture       text,
    thresholds_source  text          NOT NULL,
    recorded_at        timestamptz   NOT NULL DEFAULT now(),

    CONSTRAINT pk_soil_state_history PRIMARY KEY (id),
    -- Um registro por talhão e dia: reexecução e várias instâncias não duplicam (ON CONFLICT DO NOTHING).
    -- Também é o índice de leitura (talhão, intervalo de dias), começando por company_id.
    CONSTRAINT uq_soil_state_history_field_day UNIQUE (company_id, field_id, local_day),
    -- FK composta com farm_id: o banco recusa talhão de outra fazenda ou empresa.
    CONSTRAINT fk_soil_state_history_field FOREIGN KEY (company_id, farm_id, field_id)
        REFERENCES fields (company_id, farm_id, id),
    CONSTRAINT ck_soil_state_history_state CHECK (state IN ('dry', 'ideal', 'moist', 'waterlogged', 'unknown')),
    CONSTRAINT ck_soil_state_history_basis CHECK (basis IN ('inspection', 'measured_rain', 'model', 'none')),
    CONSTRAINT ck_soil_state_history_rain_source CHECK (rain_source IS NULL OR rain_source IN ('gauge', 'station', 'model')),
    -- Chuva acumulada existe exatamente quando há fonte de chuva.
    CONSTRAINT ck_soil_state_history_rain CHECK (
        (rain_source IS NULL) = (last_48h_mm IS NULL)
        AND (rain_source IS NULL) = (last_72h_mm IS NULL)
        AND (last_48h_mm IS NULL OR last_48h_mm >= 0)
        AND (last_72h_mm IS NULL OR last_72h_mm >= last_48h_mm)
    ),
    CONSTRAINT ck_soil_state_history_moisture CHECK (model_moisture_pct IS NULL OR model_moisture_pct >= 0),
    CONSTRAINT ck_soil_state_history_texture CHECK (soil_texture IS NULL OR soil_texture IN ('sandy', 'medium', 'clay')),
    CONSTRAINT ck_soil_state_history_thresholds CHECK (thresholds_source IN ('heuristic', 'calibrated'))
);

-- Histórico é registro: o api_app só insere e lê.
REVOKE UPDATE, DELETE, TRUNCATE ON soil_state_history FROM api_app;
