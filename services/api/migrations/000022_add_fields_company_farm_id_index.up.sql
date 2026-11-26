-- Alvo das FKs compostas (company_id, farm_id, field_id) de rain_gauge_fields e soil_inspections
-- (ADR 0022, adendo): com ele o banco recusa ligar um talhão a um pluviômetro de OUTRA fazenda,
-- não só de outra empresa. fields já existe, por isso CONCURRENTLY (sem bloquear escrita) e
-- sozinho neste arquivo: CREATE INDEX CONCURRENTLY não roda dentro de transação nem junto
-- de outros comandos. Um índice único (não parcial) basta como alvo de FK.
CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS uq_fields_company_farm_id ON fields (company_id, farm_id, id);
