-- Asserções de integração do schema da API. Roda como superusuário, em banco DESCARTÁVEL,
-- depois de `up`, `down` e `up` (ver services/api/scripts/db-test.sh).
-- Cada expect_error executa o comando dentro de um sub-bloco: a falha esperada não aborta o teste.
\set ON_ERROR_STOP on
\set QUIET on

CREATE SCHEMA t;
GRANT USAGE ON SCHEMA t TO api_app;

CREATE FUNCTION t.expect_error(stmt text, expected text) RETURNS void LANGUAGE plpgsql AS $$
DECLARE got text;
BEGIN
    BEGIN
        EXECUTE stmt;
    EXCEPTION WHEN OTHERS THEN
        got := SQLSTATE;
    END;
    IF got IS NULL THEN
        RAISE EXCEPTION 'esperava erro % mas o comando passou: %', expected, stmt;
    ELSIF got <> expected THEN
        RAISE EXCEPTION 'esperava erro % mas veio %: %', expected, got, stmt;
    END IF;
END
$$;

CREATE FUNCTION t.expect(condition boolean, label text) RETURNS void LANGUAGE plpgsql AS $$
BEGIN
    IF condition IS DISTINCT FROM true THEN
        RAISE EXCEPTION 'falhou: %', label;
    END IF;
END
$$;

GRANT EXECUTE ON ALL FUNCTIONS IN SCHEMA t TO api_app;

-- Os SELECT das asserções só interessam se falharem (o erro aborta com ON_ERROR_STOP).
\o /dev/null

-- ---------------------------------------------------------------------------
-- Fixtures (superusuário, ignora privilégios)
-- ---------------------------------------------------------------------------
INSERT INTO companies (id, name, document, city, state, plan, status, is_platform_operator) VALUES
    ('a0000000-0000-0000-0000-00000000000a', 'Operadora', '11111111000111', 'Sao Paulo', 'SP', 'pro', 'active', true),
    ('b0000000-0000-0000-0000-00000000000b', 'Cliente B', '22222222000122', 'Uberaba', 'MG', 'basic', 'active', false);

INSERT INTO farms (id, company_id, name, production_type, city, state) VALUES
    ('fa000000-0000-0000-0000-0000000000fa', 'a0000000-0000-0000-0000-00000000000a', 'Fazenda A', 'coffee', 'X', 'SP'),
    ('fb000000-0000-0000-0000-0000000000fb', 'b0000000-0000-0000-0000-00000000000b', 'Fazenda B', 'grains', 'Y', 'MG');

INSERT INTO users (id, company_id, name, email, role, status, primary_farm_id) VALUES
    ('0a000000-0000-0000-0000-00000000000a', 'a0000000-0000-0000-0000-00000000000a', 'Super', 'super@x.com', 'super_admin', 'active', 'fa000000-0000-0000-0000-0000000000fa'),
    ('0b000000-0000-0000-0000-00000000000b', 'b0000000-0000-0000-0000-00000000000b', 'Operador B', 'op@b.com', 'planning_operator', 'active', 'fb000000-0000-0000-0000-0000000000fb');

-- ---------------------------------------------------------------------------
-- Isolamento entre empresas: FKs compostas recusam tenant cruzado
-- ---------------------------------------------------------------------------
SELECT t.expect_error($q$INSERT INTO user_farm_access (user_id, farm_id, company_id)
    VALUES ('0b000000-0000-0000-0000-00000000000b', 'fa000000-0000-0000-0000-0000000000fa', 'b0000000-0000-0000-0000-00000000000b')$q$, '23503'); -- fazenda de A para usuário de B, company_id B
SELECT t.expect_error($q$INSERT INTO user_farm_access (user_id, farm_id, company_id)
    VALUES ('0b000000-0000-0000-0000-00000000000b', 'fa000000-0000-0000-0000-0000000000fa', 'a0000000-0000-0000-0000-00000000000a')$q$, '23503'); -- mesma, company_id A
SELECT t.expect_error($q$UPDATE users SET primary_farm_id = 'fa000000-0000-0000-0000-0000000000fa'
    WHERE id = '0b000000-0000-0000-0000-00000000000b'$q$, '23503');
SELECT t.expect_error($q$INSERT INTO invitations (company_id, user_id, invited_by_user_id, expires_at)
    VALUES ('a0000000-0000-0000-0000-00000000000a', '0b000000-0000-0000-0000-00000000000b', '0a000000-0000-0000-0000-00000000000a', now() + interval '7 days')$q$, '23503');
-- Caminho feliz do mesmo tenant.
INSERT INTO user_farm_access (user_id, farm_id, company_id)
VALUES ('0b000000-0000-0000-0000-00000000000b', 'fb000000-0000-0000-0000-0000000000fb', 'b0000000-0000-0000-0000-00000000000b');
-- Convite de operadora (super admin de A) para usuário de B: invited_by é FK simples, deve passar.
INSERT INTO invitations (id, company_id, user_id, invited_by_user_id, expires_at)
VALUES ('c0000000-0000-0000-0000-0000000000c1', 'b0000000-0000-0000-0000-00000000000b', '0b000000-0000-0000-0000-00000000000b', '0a000000-0000-0000-0000-00000000000a', now() + interval '7 days');

SELECT t.expect((SELECT count(*) FROM farms WHERE company_id = 'a0000000-0000-0000-0000-00000000000a') = 1, 'consulta filtrada por A ve so a fazenda de A');
SELECT t.expect((SELECT count(*) FROM users WHERE company_id = 'b0000000-0000-0000-0000-00000000000b') = 1, 'consulta filtrada por B ve so o usuario de B');
SELECT t.expect((SELECT count(*) FROM user_farm_access WHERE company_id = 'a0000000-0000-0000-0000-00000000000a') = 0, 'A nao ve acessos de B');

-- Mudar usuário de empresa: bloqueado enquanto houver acesso a fazendas da empresa antiga...
SELECT t.expect_error($q$UPDATE users SET company_id = 'a0000000-0000-0000-0000-00000000000a', primary_farm_id = NULL
    WHERE id = '0b000000-0000-0000-0000-00000000000b'$q$, '23503');
-- ...e permitido depois de limpar o acesso; o convite acompanha (ON UPDATE CASCADE).
DELETE FROM user_farm_access WHERE user_id = '0b000000-0000-0000-0000-00000000000b';
UPDATE users SET company_id = 'a0000000-0000-0000-0000-00000000000a', primary_farm_id = NULL
WHERE id = '0b000000-0000-0000-0000-00000000000b';
SELECT t.expect((SELECT company_id FROM invitations WHERE id = 'c0000000-0000-0000-0000-0000000000c1') = 'a0000000-0000-0000-0000-00000000000a', 'convite acompanha a mudanca de empresa');
UPDATE users SET company_id = 'b0000000-0000-0000-0000-00000000000b' WHERE id = '0b000000-0000-0000-0000-00000000000b';

-- ON DELETE SET NULL (primary_farm_id) zera só a coluna, sem tocar company_id.
INSERT INTO farms (id, company_id, name, production_type, city, state)
VALUES ('fc000000-0000-0000-0000-0000000000fc', 'b0000000-0000-0000-0000-00000000000b', 'Temporaria', 'mixed', 'Z', 'MG');
UPDATE users SET primary_farm_id = 'fc000000-0000-0000-0000-0000000000fc' WHERE id = '0b000000-0000-0000-0000-00000000000b';
DELETE FROM farms WHERE id = 'fc000000-0000-0000-0000-0000000000fc';
SELECT t.expect((SELECT primary_farm_id IS NULL AND company_id = 'b0000000-0000-0000-0000-00000000000b'
                 FROM users WHERE id = '0b000000-0000-0000-0000-00000000000b'), 'SET NULL zera so primary_farm_id');

-- ---------------------------------------------------------------------------
-- CHECKs e UNIQUEs: companies
-- ---------------------------------------------------------------------------
SELECT t.expect_error($q$INSERT INTO companies (name, document, city, state, plan, status) VALUES ('X', '123', 'c', 'SP', 'basic', 'active')$q$, '23514'); -- CNPJ curto
SELECT t.expect_error($q$INSERT INTO companies (name, document, city, state, plan, status) VALUES ('X', '33333333000133', 'c', 'sp', 'basic', 'active')$q$, '23514'); -- UF minúscula
SELECT t.expect_error($q$INSERT INTO companies (name, document, city, state, plan, status) VALUES ('X', '33333333000133', 'c', 'SP', 'gold', 'active')$q$, '23514');
SELECT t.expect_error($q$INSERT INTO companies (name, document, city, state, plan, status) VALUES ('X', '33333333000133', 'c', 'SP', 'trial', 'trial')$q$, '23514'); -- trial sem fim
SELECT t.expect_error($q$INSERT INTO companies (name, document, city, state, plan, status) VALUES (' ', '33333333000133', 'c', 'SP', 'basic', 'active')$q$, '23514');
SELECT t.expect_error($q$INSERT INTO companies (name, document, city, state, plan, status) VALUES ('X', '22222222000122', 'c', 'SP', 'basic', 'active')$q$, '23505'); -- CNPJ duplicado
SELECT t.expect_error($q$INSERT INTO companies (name, document, city, state, plan, status, is_platform_operator) VALUES ('Op2', '44444444000144', 'c', 'SP', 'pro', 'active', true)$q$, '23505'); -- 2a operadora
SELECT t.expect_error($q$UPDATE companies SET status = 'inactive' WHERE is_platform_operator$q$, '23514'); -- desativar operadora
INSERT INTO companies (name, document, city, state, plan, status, trial_ends_at)
VALUES ('Em avaliacao', '55555555000155', 'c', 'SP', 'trial', 'trial', now() + interval '14 days');

-- Exclusão lógica de empresa: exige inactive e não pode ser a operadora; CNPJ continua reservado.
SELECT t.expect_error($q$UPDATE companies SET deleted_at = now() WHERE id = 'b0000000-0000-0000-0000-00000000000b'$q$, '23514'); -- ainda active
SELECT t.expect_error($q$UPDATE companies SET deleted_at = now() WHERE is_platform_operator$q$, '23514');
UPDATE companies SET status = 'inactive', deleted_at = now(), version = version + 1 WHERE document = '55555555000155';
SELECT t.expect_error($q$INSERT INTO companies (name, document, city, state, plan, status) VALUES ('Recria', '55555555000155', 'c', 'SP', 'basic', 'active')$q$, '23505'); -- CNPJ de excluída segue reservado
SELECT t.expect((SELECT version FROM companies WHERE document = '55555555000155') = 2, 'version de companies');
SELECT t.expect((SELECT NOT account_created FROM invitations LIMIT 1), 'account_created padrao false');

-- ---------------------------------------------------------------------------
-- CHECKs e UNIQUEs: users, identities, sessions, invitations, audit, farms
-- ---------------------------------------------------------------------------
SELECT t.expect_error($q$INSERT INTO users (company_id, name, email, role) VALUES ('a0000000-0000-0000-0000-00000000000a', 'N', 'Upper@X.com', 'admin')$q$, '23514'); -- e-mail não normalizado
SELECT t.expect_error($q$INSERT INTO users (company_id, name, email, role) VALUES ('a0000000-0000-0000-0000-00000000000a', 'N', 'semarroba', 'admin')$q$, '23514');
SELECT t.expect_error($q$INSERT INTO users (company_id, name, email, role) VALUES ('a0000000-0000-0000-0000-00000000000a', 'N', 'n@x.com', 'owner')$q$, '23514');
SELECT t.expect_error($q$INSERT INTO users (company_id, name, email, role, farm_scope_mode) VALUES ('a0000000-0000-0000-0000-00000000000a', 'N', 'n@x.com', 'admin', 'selected')$q$, '23514'); -- selected só para operador
SELECT t.expect_error($q$INSERT INTO users (company_id, name, email, role, status, deleted_at) VALUES ('a0000000-0000-0000-0000-00000000000a', 'N', 'n@x.com', 'admin', 'active', now())$q$, '23514'); -- removido precisa estar inactive
SELECT t.expect_error($q$INSERT INTO users (company_id, name, email, role) VALUES ('a0000000-0000-0000-0000-00000000000a', 'Dup', 'op@b.com', 'admin')$q$, '23505'); -- e-mail único entre ativos, mesmo em outra empresa
SELECT t.expect_error($q$INSERT INTO users (company_id, name, email, role) VALUES ('99999999-0000-0000-0000-000000000000', 'N', 'n@x.com', 'admin')$q$, '23503'); -- empresa inexistente
-- Soft delete libera o e-mail.
UPDATE users SET status = 'inactive', deleted_at = now() WHERE id = '0b000000-0000-0000-0000-00000000000b';
INSERT INTO users (company_id, name, email, role) VALUES ('a0000000-0000-0000-0000-00000000000a', 'Reconvidado', 'op@b.com', 'admin');
SELECT t.expect_error($q$UPDATE users SET deleted_at = NULL WHERE id = '0b000000-0000-0000-0000-00000000000b'$q$, '23505'); -- restaurar colide com o e-mail em uso
UPDATE users SET status = 'active', deleted_at = NULL, email = 'op.restaurado@b.com' WHERE id = '0b000000-0000-0000-0000-00000000000b';

INSERT INTO user_identities (user_id, issuer, subject) VALUES ('0a000000-0000-0000-0000-00000000000a', 'http://kc/realms/inceres', 'sub-1');
SELECT t.expect_error($q$INSERT INTO user_identities (user_id, issuer, subject) VALUES ('0b000000-0000-0000-0000-00000000000b', 'http://kc/realms/inceres', 'sub-1')$q$, '23505'); -- (issuer, subject) único
SELECT t.expect_error($q$INSERT INTO user_identities (user_id, issuer, subject) VALUES ('0a000000-0000-0000-0000-00000000000a', 'http://kc/realms/inceres', 'sub-2')$q$, '23505'); -- um por provedor
SELECT t.expect_error($q$INSERT INTO user_identities (user_id, issuer, subject) VALUES ('0b000000-0000-0000-0000-00000000000b', ' ', 'sub-3')$q$, '23514');

SELECT t.expect_error($q$INSERT INTO auth_sessions (user_id, token_hash, issuer, refresh_token_enc, enc_key_id, granted_roles, access_expires_at, expires_at)
    VALUES ('0a000000-0000-0000-0000-00000000000a', '\x0102'::bytea, 'iss', '\x01'::bytea, 1, '{admin}', now(), now())$q$, '23514'); -- hash precisa de 32 bytes
INSERT INTO auth_sessions (user_id, token_hash, issuer, refresh_token_enc, enc_key_id, granted_roles, access_expires_at, expires_at)
VALUES ('0a000000-0000-0000-0000-00000000000a', decode(repeat('ab', 32), 'hex'), 'iss', '\x01'::bytea, 1, '{super_admin}', now(), now() + interval '1 day');
SELECT t.expect_error($q$INSERT INTO auth_sessions (user_id, token_hash, issuer, refresh_token_enc, enc_key_id, granted_roles, access_expires_at, expires_at)
    VALUES ('0a000000-0000-0000-0000-00000000000a', decode(repeat('ab', 32), 'hex'), 'iss', '\x01'::bytea, 1, '{}', now(), now())$q$, '23505'); -- token_hash único

SELECT t.expect_error($q$INSERT INTO invitations (company_id, user_id, invited_by_user_id, expires_at)
    VALUES ('b0000000-0000-0000-0000-00000000000b', '0b000000-0000-0000-0000-00000000000b', '0a000000-0000-0000-0000-00000000000a', now())$q$, '23505'); -- 2o convite aberto
SELECT t.expect_error($q$UPDATE invitations SET accepted_at = now(), cancelled_at = now()$q$, '23514');
SELECT t.expect_error($q$UPDATE invitations SET send_count = 0$q$, '23514');
UPDATE invitations SET cancelled_at = now() WHERE id = 'c0000000-0000-0000-0000-0000000000c1';
INSERT INTO invitations (company_id, user_id, invited_by_user_id, expires_at)
VALUES ('b0000000-0000-0000-0000-00000000000b', '0b000000-0000-0000-0000-00000000000b', '0a000000-0000-0000-0000-00000000000a', now() + interval '7 days'); -- reconvite após cancelar

SELECT t.expect_error($q$INSERT INTO audit_events (company_id, actor_kind, actor_name, action, entity_type, entity_id)
    VALUES ('a0000000-0000-0000-0000-00000000000a', 'user', 'Super', 'user.invited', 'user', gen_random_uuid())$q$, '23514'); -- user sem actor_user_id
SELECT t.expect_error($q$INSERT INTO audit_events (company_id, actor_kind, actor_user_id, actor_name, action, entity_type, entity_id)
    VALUES ('a0000000-0000-0000-0000-00000000000a', 'system', '0a000000-0000-0000-0000-00000000000a', 'job', 'x', 'user', gen_random_uuid())$q$, '23514'); -- system com usuário
SELECT t.expect_error($q$INSERT INTO audit_events (company_id, actor_kind, actor_name, action, entity_type, entity_id, metadata)
    VALUES ('a0000000-0000-0000-0000-00000000000a', 'system', 'job', 'x', 'user', gen_random_uuid(), '[1]')$q$, '23514'); -- metadata não objeto
INSERT INTO audit_events (company_id, actor_kind, actor_user_id, actor_name, action, entity_type, entity_id)
VALUES ('a0000000-0000-0000-0000-00000000000a', 'user', '0a000000-0000-0000-0000-00000000000a', 'Super', 'user.invited', 'user', gen_random_uuid());

SELECT t.expect_error($q$INSERT INTO farms (company_id, name, production_type, city, state) VALUES ('a0000000-0000-0000-0000-00000000000a', 'F', 'forestry', 'c', 'SP')$q$, '23514');
SELECT t.expect_error($q$INSERT INTO farms (company_id, name, production_type, city, state, postal_code) VALUES ('a0000000-0000-0000-0000-00000000000a', 'F', 'coffee', 'c', 'SP', '123')$q$, '23514');
-- Polígono em "gravata" (autointerseção) é inválido.
SELECT t.expect_error($q$INSERT INTO farms (company_id, name, production_type, city, state, perimeter)
    VALUES ('a0000000-0000-0000-0000-00000000000a', 'F', 'coffee', 'c', 'SP', ST_GeomFromText('MULTIPOLYGON(((0 0, 1 1, 1 0, 0 1, 0 0)))', 4326))$q$, '23514');
-- SRID errado é recusado pelo tipo.
SELECT t.expect_error($q$INSERT INTO farms (company_id, name, production_type, city, state, perimeter)
    VALUES ('a0000000-0000-0000-0000-00000000000a', 'F', 'coffee', 'c', 'SP', ST_GeomFromText('MULTIPOLYGON(((0 0, 0 1, 1 1, 1 0, 0 0)))', 3857))$q$, '22023');
INSERT INTO farms (id, company_id, name, production_type, city, state, perimeter)
VALUES ('fd000000-0000-0000-0000-0000000000fd', 'a0000000-0000-0000-0000-00000000000a', 'Com perimetro', 'coffee', 'c', 'SP',
        ST_GeomFromText('MULTIPOLYGON(((-47.0 -21.0, -47.0 -20.991, -46.991 -20.991, -46.991 -21.0, -47.0 -21.0)))', 4326));
SELECT t.expect((SELECT perimeter_area_ha BETWEEN 80 AND 120 FROM farms WHERE id = 'fd000000-0000-0000-0000-0000000000fd'), 'area gerada do perimetro (~100 ha)');
SELECT t.expect_error($q$UPDATE farms SET perimeter_area_ha = 1$q$, '428C9'); -- coluna gerada não é gravável

-- ---------------------------------------------------------------------------
-- Estrutura (catálogo)
-- ---------------------------------------------------------------------------
SELECT t.expect((SELECT count(*) FROM crops) = 6, 'seed de culturas');
SELECT t.expect(NOT EXISTS (
    SELECT 1 FROM information_schema.tables tb
    WHERE tb.table_schema = 'public' AND tb.table_type = 'BASE TABLE'
      AND tb.table_name <> 'schema_migrations'
      AND NOT EXISTS (SELECT 1 FROM pg_constraint c WHERE c.conrelid = format('public.%I', tb.table_name)::regclass AND c.contype = 'p')
), 'toda tabela tem PK');
SELECT t.expect(NOT EXISTS (
    SELECT 1 FROM information_schema.columns
    WHERE table_schema = 'public' AND data_type = 'timestamp without time zone'
), 'so timestamptz');
-- Tabelas de tenant: company_id NOT NULL e índice que começa por company_id.
SELECT t.expect(NOT EXISTS (
    SELECT 1 FROM information_schema.columns c
    WHERE c.table_schema = 'public' AND c.column_name = 'company_id' AND c.is_nullable = 'YES'
), 'company_id sempre NOT NULL');
SELECT t.expect(NOT EXISTS (
    SELECT 1
    FROM information_schema.columns c
    WHERE c.table_schema = 'public' AND c.column_name = 'company_id'
      AND NOT EXISTS (
          SELECT 1 FROM pg_index i
          WHERE i.indrelid = format('public.%I', c.table_name)::regclass
            AND i.indkey[0] = (SELECT attnum FROM pg_attribute
                               WHERE attrelid = i.indrelid AND attname = 'company_id')
      )
), 'toda tabela com company_id tem indice iniciando por company_id');

-- ---------------------------------------------------------------------------
-- Privilégios do api_app
-- ---------------------------------------------------------------------------
SELECT t.expect((SELECT NOT rolsuper AND NOT rolcreatedb AND NOT rolcreaterole AND NOT rolbypassrls FROM pg_roles WHERE rolname = 'api_app'), 'api_app sem atributos perigosos');
SELECT t.expect(NOT has_schema_privilege('api_app', 'public', 'CREATE'), 'api_app sem CREATE no schema public');
SELECT t.expect((SELECT pg_get_userbyid(datdba) = 'api_migrator' FROM pg_database WHERE datname = current_database()), 'api_migrator e dono do banco');

SET ROLE api_app;

-- Sem DDL
SELECT t.expect_error('CREATE TABLE public.hack (id int)', '42501');
SELECT t.expect_error('ALTER TABLE public.users ADD COLUMN hack int', '42501');
SELECT t.expect_error('DROP TABLE public.users', '42501');
SELECT t.expect_error('CREATE INDEX ix_hack ON public.users (name)', '42501');
SELECT t.expect_error('CREATE EXTENSION pg_trgm', '42501');
-- audit_events: append-only
INSERT INTO audit_events (company_id, actor_kind, actor_name, action, entity_type, entity_id)
VALUES ('a0000000-0000-0000-0000-00000000000a', 'system', 'job', 'cleanup', 'session', gen_random_uuid());
SELECT t.expect((SELECT count(*) FROM audit_events) >= 2, 'api_app le audit_events');
SELECT t.expect_error('UPDATE public.audit_events SET action = ''x''', '42501');
SELECT t.expect_error('DELETE FROM public.audit_events', '42501');
SELECT t.expect_error('TRUNCATE public.audit_events', '42501');
-- Soft delete: sem DELETE onde a linha nunca some
SELECT t.expect_error('DELETE FROM public.users', '42501');
SELECT t.expect_error('DELETE FROM public.companies', '42501');
SELECT t.expect_error('DELETE FROM public.farms', '42501');
SELECT t.expect_error('DELETE FROM public.invitations', '42501');
SELECT t.expect_error('TRUNCATE public.users', '42501');
-- Catálogo de referência: só leitura
SELECT t.expect((SELECT count(*) FROM crops) = 6, 'api_app le crops');
SELECT t.expect_error('INSERT INTO public.crops (code, name) VALUES (''x'', ''X'')', '42501');
SELECT t.expect_error('UPDATE public.crops SET name = ''X''', '42501');
SELECT t.expect_error('DELETE FROM public.crops', '42501');
-- DML do dia a dia funciona
UPDATE users SET last_access_at = now(), updated_at = now(), version = version + 1 WHERE id = '0a000000-0000-0000-0000-00000000000a';
DELETE FROM auth_sessions WHERE user_id = '0a000000-0000-0000-0000-00000000000a';
INSERT INTO user_farm_access (user_id, farm_id, company_id)
VALUES ('0b000000-0000-0000-0000-00000000000b', 'fb000000-0000-0000-0000-0000000000fb', 'b0000000-0000-0000-0000-00000000000b');
DELETE FROM user_farm_access;
-- Sequência da identity de audit_events utilizável (já provado pelo INSERT acima).

RESET ROLE;
DROP SCHEMA t CASCADE;
\o
\echo 'assertions.sql: OK'
