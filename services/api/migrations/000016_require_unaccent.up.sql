-- Busca sem acento (ADR 0017): as consultas de fazendas e proprietários comparam
-- unaccent(coluna) com unaccent(busca). A extensão exige superusuário e é criada em
-- infra/docker/postgres/init; num volume anterior ao ADR, crie-a à mão
-- (CREATE EXTENSION unaccent; como superusuário no banco api). Sem ela, falhamos aqui.
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_extension WHERE extname = 'unaccent') THEN
        RAISE EXCEPTION 'extensao unaccent ausente no banco %: crie-a como superusuario (ver infra/docker/postgres/init e ADR 0017)', current_database();
    END IF;
END
$$;
