-- Relevo do talhão (ADR 0021): os blocos do modelo de elevação ficam em colunas `raster`, que
-- exigem a extensão postgis_raster e o driver GDAL GTiff habilitado. As duas coisas são do init
-- (superusuário): infra/docker/postgres/init cria a extensão e roda
--   ALTER DATABASE api SET postgis.gdal_enabled_drivers = 'GTiff';
-- Num volume anterior ao ADR, faça isso à mão (ver CLAUDE.md). Sem elas, falhamos aqui, com
-- mensagem clara, em vez de deixar a ingestão quebrar em runtime.
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_extension WHERE extname = 'postgis_raster') THEN
        RAISE EXCEPTION 'extensao postgis_raster ausente no banco %: crie-a como superusuario (ver infra/docker/postgres/init e ADR 0021)', current_database();
    END IF;
    -- Prova funcional do driver: o PostGIS desabilita todos os drivers GDAL por padrão e
    -- ST_AsGDALRaster/ST_FromGDALRaster falham sem o GTiff.
    BEGIN
        PERFORM ST_FromGDALRaster(ST_AsGDALRaster(ST_AddBand(ST_MakeEmptyRaster(1, 1, 0, 0, 1), '8BUI'::text), 'GTiff'));
    EXCEPTION WHEN OTHERS THEN
        RAISE EXCEPTION 'driver GDAL GTiff indisponivel no banco % (%): execute ALTER DATABASE % SET postgis.gdal_enabled_drivers = ''GTiff'' como superusuario (ver ADR 0021)',
            current_database(), SQLERRM, current_database();
    END;
END
$$;
