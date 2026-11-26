-- Modelo de elevação (ADR 0021): dado de referência global, como crops e municipalities (sem
-- company_id). A API baixa o bloco de 1° x 1° (Copernicus GLO-30), ingere em blocos raster de
-- 256x256 e recorta por ST_Clip; o cálculo de declividade é do domínio Go, não do banco.
-- O api_app só insere e lê: bloco publicado não é reescrito nem apagado. A ingestão grava a
-- linha de elevation_sources e seus blocos na MESMA transação, então não sobra ingestão pela metade.
CREATE TABLE elevation_sources (
    tile_key    text        NOT NULL,
    source      text        NOT NULL,
    version     text        NOT NULL,
    fetched_at  timestamptz NOT NULL DEFAULT now(),
    block_count integer     NOT NULL,

    -- Ex.: 'S22W047' (canto sudoeste do bloco de 1 grau).
    CONSTRAINT pk_elevation_sources PRIMARY KEY (tile_key),
    CONSTRAINT ck_elevation_sources_tile_key CHECK (tile_key ~ '^[NS][0-9]{2}[EW][0-9]{3}$'),
    CONSTRAINT ck_elevation_sources_source CHECK (btrim(source) <> ''),
    CONSTRAINT ck_elevation_sources_version CHECK (btrim(version) <> ''),
    CONSTRAINT ck_elevation_sources_block_count CHECK (block_count > 0)
);

CREATE TABLE elevation_blocks (
    id       bigint GENERATED ALWAYS AS IDENTITY,
    tile_key text   NOT NULL,
    rast     raster NOT NULL,

    CONSTRAINT pk_elevation_blocks PRIMARY KEY (id),
    CONSTRAINT fk_elevation_blocks_source FOREIGN KEY (tile_key) REFERENCES elevation_sources (tile_key) ON DELETE CASCADE
);

-- Recorte por polígono: seleciona só os blocos que cruzam o talhão.
CREATE INDEX ix_elevation_blocks_hull ON elevation_blocks USING gist (st_convexhull(rast));
-- Lado filho da FK e "quais blocos tem este bloco de 1 grau".
CREATE INDEX ix_elevation_blocks_tile_key ON elevation_blocks (tile_key);

REVOKE UPDATE, DELETE, TRUNCATE ON elevation_sources FROM api_app;
REVOKE UPDATE, DELETE, TRUNCATE ON elevation_blocks FROM api_app;
