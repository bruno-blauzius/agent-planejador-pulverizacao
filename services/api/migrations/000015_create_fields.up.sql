-- Talhões de uma fazenda (DER, seção "fields"). A área é coluna gerada a partir do polígono,
-- como na fazenda. Ficar dentro do perímetro, não sobrepor outro talhão e as áreas mínimas
-- são regras da aplicação (domain.AreaRules), com tolerância configurável: EXCLUDE com &&
-- compara caixas, não geometrias, e daria falso positivo entre talhões vizinhos.
CREATE TABLE fields (
    id              uuid                     NOT NULL DEFAULT gen_random_uuid(),
    company_id      uuid                     NOT NULL,
    farm_id         uuid                     NOT NULL,
    crop_code       text                     NOT NULL,
    name            text                     NOT NULL,
    block           text,
    variety         text,
    season          text,
    planting_date   date,
    row_spacing_m   numeric(6, 3),
    plant_spacing_m numeric(6, 3),
    polygon         geometry(Polygon, 4326)  NOT NULL,
    area_ha         numeric(14, 4) GENERATED ALWAYS AS (
        round((ST_Area(polygon::geography) / 10000.0)::numeric, 4)
    ) STORED,
    deleted_at      timestamptz,
    created_at      timestamptz              NOT NULL DEFAULT now(),
    updated_at      timestamptz              NOT NULL DEFAULT now(),
    version         integer                  NOT NULL DEFAULT 1,

    CONSTRAINT pk_fields PRIMARY KEY (id),
    -- Alvo de FKs compostas de tenant (aplicações, na Fase 4).
    CONSTRAINT uq_fields_company_id_id UNIQUE (company_id, id),
    -- FK composta: o banco recusa talhão em fazenda de outra empresa.
    CONSTRAINT fk_fields_farm FOREIGN KEY (company_id, farm_id) REFERENCES farms (company_id, id),
    CONSTRAINT fk_fields_crop FOREIGN KEY (crop_code) REFERENCES crops (code),
    CONSTRAINT ck_fields_name CHECK (btrim(name) <> ''),
    CONSTRAINT ck_fields_season CHECK (season IS NULL OR season ~ '^[0-9]{4}/[0-9]{2}$'),
    CONSTRAINT ck_fields_row_spacing CHECK (row_spacing_m IS NULL OR row_spacing_m > 0),
    CONSTRAINT ck_fields_plant_spacing CHECK (plant_spacing_m IS NULL OR plant_spacing_m > 0),
    CONSTRAINT ck_fields_polygon_valid CHECK (ST_IsValid(polygon)),
    -- Todo talhão tem área própria em hectares (o mínimo configurado é regra da aplicação).
    CONSTRAINT ck_fields_area CHECK (area_ha > 0),
    CONSTRAINT ck_fields_version CHECK (version >= 1)
);

-- Nome único por fazenda entre os não removidos.
CREATE UNIQUE INDEX uq_fields_farm_name ON fields (farm_id, name) WHERE deleted_at IS NULL;
-- Talhões da fazenda (lista, resumo da fazenda); começa por company_id (tenant).
CREATE INDEX ix_fields_company_farm ON fields (company_id, farm_id) WHERE deleted_at IS NULL;
-- Lado filho da FK de cultura e filtro por cultura.
CREATE INDEX ix_fields_crop ON fields (crop_code);
-- Sobreposição e cobertura do perímetro.
CREATE INDEX ix_fields_polygon ON fields USING gist (polygon);

-- Exclusão no produto é soft delete (deleted_at).
REVOKE DELETE ON fields FROM api_app;
