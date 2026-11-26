-- Malha municipal do IBGE (ADR 0019): dado de referência global, como crops (sem company_id).
-- Carregada pelo comando cmd/load-municipalities (make api-load-municipalities) com o papel
-- api_migrator; o api_app só lê, para descobrir o município de um perímetro importado.
-- A tabela nasce vazia: sem carga, a importação devolve município e UF nulos.
CREATE TABLE municipalities (
    ibge_code text                         NOT NULL,
    name      text                         NOT NULL,
    state     text                         NOT NULL,
    boundary  geometry(MultiPolygon, 4326) NOT NULL,
    loaded_at timestamptz                  NOT NULL DEFAULT now(),

    CONSTRAINT pk_municipalities PRIMARY KEY (ibge_code),
    CONSTRAINT ck_municipalities_code CHECK (ibge_code ~ '^[0-9]{7}$'),
    CONSTRAINT ck_municipalities_name CHECK (btrim(name) <> ''),
    CONSTRAINT ck_municipalities_state CHECK (state ~ '^[A-Z]{2}$')
);

-- Ponto em polígono (qual município contém o perímetro).
CREATE INDEX ix_municipalities_boundary ON municipalities USING gist (boundary);

REVOKE INSERT, UPDATE, DELETE, TRUNCATE ON municipalities FROM api_app;
