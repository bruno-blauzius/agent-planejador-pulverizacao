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

-- owners: documento por tipo de pessoa, data de nascimento só PF, unicidade por empresa
INSERT INTO owners (id, company_id, person_type, name, document) VALUES
    ('0a0a0000-0000-0000-0000-00000000000a', 'a0000000-0000-0000-0000-00000000000a', 'individual', 'Dono A', '52998224725'),
    ('0b0b0000-0000-0000-0000-00000000000b', 'b0000000-0000-0000-0000-00000000000b', 'company', 'Dona B', '11222333000181');
SELECT t.expect_error($q$INSERT INTO owners (company_id, person_type, name, document) VALUES ('a0000000-0000-0000-0000-00000000000a', 'individual', 'X', '11222333000181')$q$, '23514');
SELECT t.expect_error($q$INSERT INTO owners (company_id, person_type, name, document) VALUES ('a0000000-0000-0000-0000-00000000000a', 'company', 'X', '52998224725')$q$, '23514');
SELECT t.expect_error($q$INSERT INTO owners (company_id, person_type, name, document, birth_date) VALUES ('a0000000-0000-0000-0000-00000000000a', 'company', 'X', '11444777000161', '1980-01-01')$q$, '23514');
SELECT t.expect_error($q$INSERT INTO owners (company_id, person_type, name, document, phone) VALUES ('a0000000-0000-0000-0000-00000000000a', 'individual', 'X', '11144477735', '(35) 9999')$q$, '23514');
SELECT t.expect_error($q$INSERT INTO owners (company_id, person_type, name, document) VALUES ('a0000000-0000-0000-0000-00000000000a', 'individual', 'X', '52998224725')$q$, '23505');
-- o mesmo CPF em outra empresa é permitido
INSERT INTO owners (company_id, person_type, name, document) VALUES ('b0000000-0000-0000-0000-00000000000b', 'individual', 'Dono A em B', '52998224725');

-- farm_owners: tenant cruzado recusado; participação só para o papel owner
SELECT t.expect_error($q$INSERT INTO farm_owners (company_id, farm_id, owner_id, role, share_percent)
    VALUES ('a0000000-0000-0000-0000-00000000000a', 'fa000000-0000-0000-0000-0000000000fa', '0b0b0000-0000-0000-0000-00000000000b', 'owner', 50)$q$, '23503'); -- proprietário de B em fazenda de A
SELECT t.expect_error($q$INSERT INTO farm_owners (company_id, farm_id, owner_id, role, share_percent)
    VALUES ('b0000000-0000-0000-0000-00000000000b', 'fa000000-0000-0000-0000-0000000000fa', '0b0b0000-0000-0000-0000-00000000000b', 'owner', 50)$q$, '23503'); -- fazenda de A com company_id B
SELECT t.expect_error($q$INSERT INTO farm_owners (company_id, farm_id, owner_id, role, share_percent)
    VALUES ('a0000000-0000-0000-0000-00000000000a', 'fa000000-0000-0000-0000-0000000000fa', '0a0a0000-0000-0000-0000-00000000000a', 'tenant', 10)$q$, '23514');
SELECT t.expect_error($q$INSERT INTO farm_owners (company_id, farm_id, owner_id, role, share_percent)
    VALUES ('a0000000-0000-0000-0000-00000000000a', 'fa000000-0000-0000-0000-0000000000fa', '0a0a0000-0000-0000-0000-00000000000a', 'owner', NULL)$q$, '23514');
SELECT t.expect_error($q$INSERT INTO farm_owners (company_id, farm_id, owner_id, role, share_percent)
    VALUES ('a0000000-0000-0000-0000-00000000000a', 'fa000000-0000-0000-0000-0000000000fa', '0a0a0000-0000-0000-0000-00000000000a', 'owner', 100.01)$q$, '23514');

-- fields: tenant cruzado recusado, polígono válido com área, nome único por fazenda
INSERT INTO fields (id, company_id, farm_id, crop_code, name, polygon) VALUES
    ('f1e10000-0000-0000-0000-0000000000f1', 'a0000000-0000-0000-0000-00000000000a', 'fd000000-0000-0000-0000-0000000000fd',
     'coffee_arabica', 'Talhao 1', ST_GeomFromText('POLYGON((-47.0 -21.0, -47.0 -20.998, -46.998 -20.998, -46.998 -21.0, -47.0 -21.0))', 4326));
SELECT t.expect((SELECT area_ha BETWEEN 4 AND 5 FROM fields WHERE id = 'f1e10000-0000-0000-0000-0000000000f1'), 'area gerada do talhao (~4,6 ha)');
SELECT t.expect_error($q$INSERT INTO fields (company_id, farm_id, crop_code, name, polygon)
    VALUES ('b0000000-0000-0000-0000-00000000000b', 'fd000000-0000-0000-0000-0000000000fd', 'corn', 'X',
            ST_GeomFromText('POLYGON((-47.0 -21.0, -47.0 -20.999, -46.999 -20.999, -46.999 -21.0, -47.0 -21.0))', 4326))$q$, '23503'); -- fazenda de A com company_id B
SELECT t.expect_error($q$INSERT INTO fields (company_id, farm_id, crop_code, name, polygon)
    VALUES ('a0000000-0000-0000-0000-00000000000a', 'fd000000-0000-0000-0000-0000000000fd', 'banana', 'X',
            ST_GeomFromText('POLYGON((-47.0 -21.0, -47.0 -20.999, -46.999 -20.999, -46.999 -21.0, -47.0 -21.0))', 4326))$q$, '23503'); -- cultura inexistente
SELECT t.expect_error($q$INSERT INTO fields (company_id, farm_id, crop_code, name, polygon)
    VALUES ('a0000000-0000-0000-0000-00000000000a', 'fd000000-0000-0000-0000-0000000000fd', 'corn', 'Talhao 1',
            ST_GeomFromText('POLYGON((-46.99 -21.0, -46.99 -20.999, -46.989 -20.999, -46.989 -21.0, -46.99 -21.0))', 4326))$q$, '23505'); -- nome repetido
SELECT t.expect_error($q$INSERT INTO fields (company_id, farm_id, crop_code, name, polygon)
    VALUES ('a0000000-0000-0000-0000-00000000000a', 'fd000000-0000-0000-0000-0000000000fd', 'corn', 'Gravata',
            ST_GeomFromText('POLYGON((0 0, 1 1, 1 0, 0 1, 0 0))', 4326))$q$, '23514'); -- autointerseção
SELECT t.expect_error($q$INSERT INTO fields (company_id, farm_id, crop_code, name, polygon)
    VALUES ('a0000000-0000-0000-0000-00000000000a', 'fd000000-0000-0000-0000-0000000000fd', 'corn', 'Linha',
            ST_GeomFromText('POLYGON((0 0, 0 0, 0 0, 0 0))', 4326))$q$, '23514'); -- sem área
SELECT t.expect_error($q$UPDATE fields SET season = '2026-27'$q$, '23514');
SELECT t.expect_error($q$UPDATE fields SET row_spacing_m = 0$q$, '23514');
SELECT t.expect_error($q$UPDATE fields SET area_ha = 1$q$, '428C9'); -- coluna gerada não é gravável

-- owner_documents: tenant cruzado recusado, chave sempre no prefixo do proprietário
SELECT t.expect((SELECT documents_pending FROM owners WHERE id = '0a0a0000-0000-0000-0000-00000000000a'), 'proprietario nasce com documento pendente');
INSERT INTO owner_documents (id, company_id, owner_id, kind, file_name, content_type, size_bytes, storage_key, uploaded_by_user_id)
VALUES ('d0c00000-0000-0000-0000-0000000000d1', 'a0000000-0000-0000-0000-00000000000a', '0a0a0000-0000-0000-0000-00000000000a',
        'id_card', 'rg.pdf', 'application/pdf', 1024,
        'companies/a0000000-0000-0000-0000-00000000000a/owners/0a0a0000-0000-0000-0000-00000000000a/documents/d0c00000-0000-0000-0000-0000000000d1',
        '0a000000-0000-0000-0000-00000000000a');
SELECT t.expect_error($q$INSERT INTO owner_documents (id, company_id, owner_id, kind, file_name, content_type, size_bytes, storage_key, uploaded_by_user_id)
    VALUES ('d0c00000-0000-0000-0000-0000000000d2', 'b0000000-0000-0000-0000-00000000000b', '0a0a0000-0000-0000-0000-00000000000a', 'id_card', 'x.pdf', 'application/pdf', 1,
            'companies/b0000000-0000-0000-0000-00000000000b/owners/0a0a0000-0000-0000-0000-00000000000a/documents/d0c00000-0000-0000-0000-0000000000d2',
            '0a000000-0000-0000-0000-00000000000a')$q$, '23503'); -- proprietário de A com company_id B
SELECT t.expect_error($q$INSERT INTO owner_documents (company_id, owner_id, kind, file_name, content_type, size_bytes, storage_key, uploaded_by_user_id)
    VALUES ('a0000000-0000-0000-0000-00000000000a', '0a0a0000-0000-0000-0000-00000000000a', 'id_card', 'x.pdf', 'application/pdf', 1,
            'outro/prefixo', '0a000000-0000-0000-0000-00000000000a')$q$, '23514'); -- chave fora do prefixo
SELECT t.expect_error($q$UPDATE owner_documents SET kind = 'passport'$q$, '23514');
SELECT t.expect_error($q$UPDATE owner_documents SET content_type = 'application/msword'$q$, '23514');
SELECT t.expect_error($q$UPDATE owner_documents SET size_bytes = 0$q$, '23514');

-- ---------------------------------------------------------------------------
-- Relevo do talhão (migrações 000019 a 000021)
-- ---------------------------------------------------------------------------
-- Fumaça do raster: prova que postgis_raster e o driver GTiff estão ativos (ST_FromGDALRaster
-- falha com os drivers desabilitados), ladrilha em 256x256 e recorta por polígono.
CREATE TABLE t.dem AS
SELECT ST_MapAlgebra(
           ST_AddBand(ST_MakeEmptyRaster(512, 512, -47.1, -20.9, 0.0003, -0.0003, 0, 0, 4326), '32BF'::text, 0, -9999),
           1, '32BF'::text, '[rast.x] + [rast.y]'::text) AS rast;
SELECT t.expect((SELECT ST_Width(rast) = 512 AND ST_Height(rast) = 512 FROM t.dem), 'raster sintetico 512x512');
CREATE TABLE t.dem_tiff AS SELECT ST_AsGDALRaster(rast, 'GTiff') AS bytes FROM t.dem;
SELECT t.expect((SELECT length(bytes) > 1000 FROM t.dem_tiff), 'GTiff serializado');
CREATE TABLE t.dem_read AS SELECT ST_FromGDALRaster(bytes) AS rast FROM t.dem_tiff;
SELECT t.expect((SELECT ST_Width(rast) = 512 AND ST_SRID(rast) = 4326 AND ST_NumBands(rast) = 1 FROM t.dem_read), 'ST_FromGDALRaster releu o GTiff (driver ativo)');
SELECT t.expect((SELECT ST_Value(r.rast, 1, 1, 1) = ST_Value(d.rast, 1, 1, 1) AND ST_Value(r.rast, 1, 300, 400) = ST_Value(d.rast, 1, 300, 400)
                 FROM t.dem_read r, t.dem d), 'valores preservados no ida e volta GTiff');
-- Ladrilhar em 256x256 (4 blocos) e inserir como o ingestor da API faz: fonte e blocos na mesma transação.
INSERT INTO elevation_sources (tile_key, source, version, block_count) VALUES ('S21W048', 'copernicus-glo30', '2021_1', 4);
INSERT INTO elevation_blocks (tile_key, rast) SELECT 'S21W048', ST_Tile(rast, 256, 256) FROM t.dem_read;
SELECT t.expect((SELECT count(*) = 4 FROM elevation_blocks WHERE tile_key = 'S21W048'), 'ST_Tile gerou 4 blocos');
-- Polígono que cruza a fronteira entre blocos (x = -47.1 + 256*0.0003 = -47.0232).
CREATE TABLE t.clip_geom AS SELECT ST_GeomFromText('POLYGON((-47.03 -20.95, -47.03 -20.93, -47.01 -20.93, -47.01 -20.95, -47.03 -20.95))', 4326) AS geom;
SELECT t.expect((SELECT count(*) >= 2 FROM elevation_blocks b, t.clip_geom g WHERE b.tile_key = 'S21W048' AND ST_Intersects(ST_ConvexHull(b.rast), g.geom)), 'recorte seleciona mais de um bloco');
-- Soma de pixels do recorte nos blocos == recorte do raster inteiro (nenhum pixel perdido na fronteira).
SELECT t.expect((SELECT sum(ST_Count(ST_Clip(b.rast, 1, g.geom, -9999, false), 1, true)) FROM elevation_blocks b, t.clip_geom g
                 WHERE b.tile_key = 'S21W048' AND ST_Intersects(ST_ConvexHull(b.rast), g.geom))
                = (SELECT ST_Count(ST_Clip(d.rast, 1, g.geom, -9999, false), 1, true) FROM t.dem d, t.clip_geom g), 'recorte por blocos == recorte do raster inteiro');
SELECT t.expect((SELECT sum(ST_Count(ST_Clip(b.rast, 1, g.geom, -9999, false), 1, true)) > 0 FROM elevation_blocks b, t.clip_geom g
                 WHERE b.tile_key = 'S21W048' AND ST_Intersects(ST_ConvexHull(b.rast), g.geom)), 'recorte tem pixels');
-- ST_DumpValues devolve a janela de altitudes (a base do cálculo de declividade no Go).
SELECT t.expect((SELECT array_length(ST_DumpValues(ST_Clip(b.rast, 1, g.geom, -9999, false), 1), 1) > 0 FROM elevation_blocks b, t.clip_geom g
                 WHERE b.tile_key = 'S21W048' AND ST_Intersects(ST_ConvexHull(b.rast), g.geom) LIMIT 1), 'ST_DumpValues devolve valores');
-- Constraints de elevação.
SELECT t.expect_error($q$INSERT INTO elevation_sources (tile_key, source, version, block_count) VALUES ('s21w048', 'x', '1', 1)$q$, '23514');
SELECT t.expect_error($q$INSERT INTO elevation_sources (tile_key, source, version, block_count) VALUES ('S22W048', 'x', '1', 0)$q$, '23514');
SELECT t.expect_error($q$INSERT INTO elevation_sources (tile_key, source, version, block_count) VALUES ('S21W048', 'x', '1', 1)$q$, '23505');
SELECT t.expect_error($q$INSERT INTO elevation_blocks (tile_key, rast) SELECT 'S99W099', rast FROM t.dem$q$, '23503');

-- Relevo: fixtures. Talhão f1 e fazenda fd são da empresa A; fb é de B.
INSERT INTO terrain_runs (id, company_id, farm_id, scope, requested_by)
VALUES ('7a000000-0000-0000-0000-0000000000a1', 'a0000000-0000-0000-0000-00000000000a', 'fd000000-0000-0000-0000-0000000000fd', 'missing', '0a000000-0000-0000-0000-00000000000a');
SELECT t.expect_error($q$INSERT INTO terrain_runs (company_id, farm_id, scope, requested_by)
    VALUES ('b0000000-0000-0000-0000-00000000000b', 'fd000000-0000-0000-0000-0000000000fd', 'missing', '0b000000-0000-0000-0000-00000000000b')$q$, '23503'); -- fazenda de A com company_id B
SELECT t.expect_error($q$INSERT INTO terrain_runs (company_id, farm_id, scope, requested_by)
    VALUES ('a0000000-0000-0000-0000-00000000000a', 'fd000000-0000-0000-0000-0000000000fd', 'some', '0a000000-0000-0000-0000-00000000000a')$q$, '23514');

INSERT INTO field_terrain (id, company_id, farm_id, field_id, status, run_id)
VALUES ('7e000000-0000-0000-0000-0000000000e1', 'a0000000-0000-0000-0000-00000000000a', 'fd000000-0000-0000-0000-0000000000fd',
        'f1e10000-0000-0000-0000-0000000000f1', 'pending', '7a000000-0000-0000-0000-0000000000a1');
SELECT t.expect_error($q$INSERT INTO field_terrain (company_id, farm_id, field_id, status)
    VALUES ('a0000000-0000-0000-0000-00000000000a', 'fd000000-0000-0000-0000-0000000000fd', 'f1e10000-0000-0000-0000-0000000000f1', 'pending')$q$, '23505'); -- um relevo por talhão
SELECT t.expect_error($q$INSERT INTO field_terrain (company_id, farm_id, field_id, status)
    VALUES ('b0000000-0000-0000-0000-00000000000b', 'fd000000-0000-0000-0000-0000000000fd', 'f1e10000-0000-0000-0000-0000000000f1', 'pending')$q$, '23503'); -- talhão de outra empresa
SELECT t.expect_error($q$INSERT INTO field_terrain (company_id, farm_id, field_id, status, run_id)
    VALUES ('b0000000-0000-0000-0000-00000000000b', 'fb000000-0000-0000-0000-0000000000fb', 'f1e10000-0000-0000-0000-0000000000f1', 'pending', '7a000000-0000-0000-0000-0000000000a1')$q$, '23503'); -- talhão e execução de A com company_id B
SELECT t.expect_error($q$UPDATE field_terrain SET status = 'done'$q$, '23514');
SELECT t.expect_error($q$UPDATE field_terrain SET status = 'ready'$q$, '23514'); -- ready sem medidas
SELECT t.expect_error($q$UPDATE field_terrain SET status = 'failed'$q$, '23514'); -- failed sem motivo
SELECT t.expect_error($q$UPDATE field_terrain SET status = 'failed', failure_reason = '  '$q$, '23514');
SELECT t.expect_error($q$UPDATE field_terrain SET position = 'peak'$q$, '23514');
SELECT t.expect_error($q$UPDATE field_terrain SET aspect = 'up'$q$, '23514');
SELECT t.expect_error($q$UPDATE field_terrain SET slope_mean_pct = -1$q$, '23514');
SELECT t.expect_error($q$UPDATE field_terrain SET slope_mean_pct = 10, slope_max_pct = 5$q$, '23514'); -- máxima < média
SELECT t.expect_error($q$UPDATE field_terrain SET elevation_min_m = 900, elevation_max_m = 800$q$, '23514');
SELECT t.expect_error($q$UPDATE field_terrain SET slope_histogram_ha = array_fill(0::numeric, ARRAY[100])$q$, '23514'); -- tamanho errado
SELECT t.expect_error($q$UPDATE field_terrain SET slope_histogram_ha = array_fill(0::numeric, ARRAY[102])$q$, '23514');
SELECT t.expect_error($q$UPDATE field_terrain SET slope_histogram_ha = array_fill(-1::numeric, ARRAY[101])$q$, '23514'); -- negativo
SELECT t.expect_error($q$UPDATE field_terrain SET slope_histogram_ha = array_fill(NULL::numeric, ARRAY[101])$q$, '23514'); -- elementos nulos
SELECT t.expect_error($q$UPDATE field_terrain SET profile = '{}'::jsonb$q$, '23514');
-- ready completo passa; failed com motivo também.
UPDATE field_terrain SET status = 'ready', calculated_at = now(),
    slope_mean_pct = 8.2, slope_p90_pct = 14.5, slope_max_pct = 22.1,
    elevation_min_m = 801.5, elevation_max_m = 842.0, position = 'slope', aspect = 'southeast',
    slope_histogram_ha = array_fill(0.04::numeric, ARRAY[101]), profile = '[{"d":0,"z":801.5},{"d":120,"z":842.0}]'::jsonb,
    elevation_source = 'copernicus-glo30', elevation_resolution_m = 30, elevation_version = '2021_1'
WHERE id = '7e000000-0000-0000-0000-0000000000e1';
UPDATE field_terrain SET status = 'failed', failure_reason = 'bloco de elevacao indisponivel' WHERE id = '7e000000-0000-0000-0000-0000000000e1';

-- Ajuste manual.
SELECT t.expect_error($q$INSERT INTO field_terrain_adjustments (company_id, field_id, origin, slope_mean_pct, slope_max_pct, position, justification, adjusted_by)
    VALUES ('b0000000-0000-0000-0000-00000000000b', 'f1e10000-0000-0000-0000-0000000000f1', 'topographic_survey', 5, 9, 'slope', 'x', '0b000000-0000-0000-0000-00000000000b')$q$, '23503'); -- talhão de A com company_id B
SELECT t.expect_error($q$INSERT INTO field_terrain_adjustments (company_id, field_id, origin, slope_mean_pct, slope_max_pct, position, justification, adjusted_by)
    VALUES ('a0000000-0000-0000-0000-00000000000a', 'f1e10000-0000-0000-0000-0000000000f1', 'topographic_survey', 12, 9, 'slope', 'x', '0a000000-0000-0000-0000-00000000000a')$q$, '23514'); -- máxima < média
SELECT t.expect_error($q$INSERT INTO field_terrain_adjustments (company_id, field_id, origin, slope_mean_pct, slope_max_pct, position, justification, adjusted_by)
    VALUES ('a0000000-0000-0000-0000-00000000000a', 'f1e10000-0000-0000-0000-0000000000f1', 'topographic_survey', 5, 101, 'slope', 'x', '0a000000-0000-0000-0000-00000000000a')$q$, '23514');
SELECT t.expect_error($q$INSERT INTO field_terrain_adjustments (company_id, field_id, origin, slope_mean_pct, slope_max_pct, position, justification, adjusted_by)
    VALUES ('a0000000-0000-0000-0000-00000000000a', 'f1e10000-0000-0000-0000-0000000000f1', 'topographic_survey', 5, 9, 'slope', '   ', '0a000000-0000-0000-0000-00000000000a')$q$, '23514'); -- justificativa vazia
SELECT t.expect_error($q$INSERT INTO field_terrain_adjustments (company_id, field_id, origin, slope_mean_pct, slope_max_pct, position, justification, adjusted_by)
    VALUES ('a0000000-0000-0000-0000-00000000000a', 'f1e10000-0000-0000-0000-0000000000f1', 'topographic_survey', 5, 9, 'slope', repeat('x', 1001), '0a000000-0000-0000-0000-00000000000a')$q$, '23514');
SELECT t.expect_error($q$INSERT INTO field_terrain_adjustments (company_id, field_id, origin, slope_mean_pct, slope_max_pct, position, justification, adjusted_by)
    VALUES ('a0000000-0000-0000-0000-00000000000a', 'f1e10000-0000-0000-0000-0000000000f1', 'guess', 5, 9, 'slope', 'x', '0a000000-0000-0000-0000-00000000000a')$q$, '23514');
INSERT INTO field_terrain_adjustments (id, company_id, field_id, origin, slope_mean_pct, slope_max_pct, position, justification, adjusted_by)
VALUES ('ad000000-0000-0000-0000-0000000000a1', 'a0000000-0000-0000-0000-00000000000a', 'f1e10000-0000-0000-0000-0000000000f1', 'field_assessment', 5, 9, 'slope', 'Terracos nao vistos no MDE', '0a000000-0000-0000-0000-00000000000a');
SELECT t.expect_error($q$INSERT INTO field_terrain_adjustments (company_id, field_id, origin, slope_mean_pct, slope_max_pct, position, justification, adjusted_by)
    VALUES ('a0000000-0000-0000-0000-00000000000a', 'f1e10000-0000-0000-0000-0000000000f1', 'field_assessment', 5, 9, 'slope', 'outro', '0a000000-0000-0000-0000-00000000000a')$q$, '23505'); -- um ajuste por talhão

-- ---------------------------------------------------------------------------
-- Chuva, solo e trafegabilidade (migrações 000022 e 000023)
-- ---------------------------------------------------------------------------
-- Textura do solo: enum ou nula.
UPDATE fields SET soil_texture = 'clay' WHERE id = 'f1e10000-0000-0000-0000-0000000000f1';
SELECT t.expect_error($q$UPDATE fields SET soil_texture = 'loam'$q$, '23514');
-- Talhões de outra fazenda da mesma empresa (fa) e de outra empresa (fb).
INSERT INTO fields (id, company_id, farm_id, crop_code, name, polygon) VALUES
    ('f1e20000-0000-0000-0000-0000000000f2', 'a0000000-0000-0000-0000-00000000000a', 'fa000000-0000-0000-0000-0000000000fa',
     'corn', 'Talhao FA', ST_GeomFromText('POLYGON((-46.0 -21.0, -46.0 -20.998, -45.998 -20.998, -45.998 -21.0, -46.0 -21.0))', 4326)),
    ('f1e30000-0000-0000-0000-0000000000f3', 'b0000000-0000-0000-0000-00000000000b', 'fb000000-0000-0000-0000-0000000000fb',
     'corn', 'Talhao FB', ST_GeomFromText('POLYGON((-48.0 -21.0, -48.0 -20.998, -47.998 -20.998, -47.998 -21.0, -48.0 -21.0))', 4326));
-- Pluviômetros: um na fazenda fd e um na fa (empresa A), um na fb (empresa B).
INSERT INTO rain_gauges (id, company_id, farm_id, name, location) VALUES
    ('9a000000-0000-0000-0000-0000000000a1', 'a0000000-0000-0000-0000-00000000000a', 'fd000000-0000-0000-0000-0000000000fd', 'Sede', ST_SetSRID(ST_MakePoint(-47.0, -21.0), 4326)),
    ('9a000000-0000-0000-0000-0000000000a2', 'a0000000-0000-0000-0000-00000000000a', 'fa000000-0000-0000-0000-0000000000fa', 'Sede', NULL),
    ('9b000000-0000-0000-0000-0000000000b1', 'b0000000-0000-0000-0000-00000000000b', 'fb000000-0000-0000-0000-0000000000fb', 'Sede', NULL);
SELECT t.expect_error($q$INSERT INTO rain_gauges (company_id, farm_id, name)
    VALUES ('a0000000-0000-0000-0000-00000000000a', 'fd000000-0000-0000-0000-0000000000fd', 'SEDE')$q$, '23505'); -- nome repetido sem caixa
SELECT t.expect_error($q$INSERT INTO rain_gauges (company_id, farm_id, name)
    VALUES ('b0000000-0000-0000-0000-00000000000b', 'fd000000-0000-0000-0000-0000000000fd', 'X')$q$, '23503'); -- fazenda de A com company_id B
SELECT t.expect_error($q$INSERT INTO rain_gauges (company_id, farm_id, name)
    VALUES ('a0000000-0000-0000-0000-00000000000a', 'fd000000-0000-0000-0000-0000000000fd', '   ')$q$, '23514');
SELECT t.expect_error($q$INSERT INTO rain_gauges (company_id, farm_id, name)
    VALUES ('a0000000-0000-0000-0000-00000000000a', 'fd000000-0000-0000-0000-0000000000fd', repeat('x', 101))$q$, '23514');
SELECT t.expect_error($q$INSERT INTO rain_gauges (company_id, farm_id, name, location)
    VALUES ('a0000000-0000-0000-0000-00000000000a', 'fd000000-0000-0000-0000-0000000000fd', 'Y', ST_SetSRID(ST_MakePoint(-47.0, -21.0), 3857))$q$, '22023'); -- SRID errado
-- Soft delete libera o nome.
INSERT INTO rain_gauges (id, company_id, farm_id, name, deleted_at)
VALUES ('9a000000-0000-0000-0000-0000000000a9', 'a0000000-0000-0000-0000-00000000000a', 'fd000000-0000-0000-0000-0000000000fd', 'sede', now());
-- Vínculo talhão -> pluviômetro: mesma empresa E mesma fazenda, um por talhão.
INSERT INTO rain_gauge_fields (company_id, farm_id, field_id, rain_gauge_id)
VALUES ('a0000000-0000-0000-0000-00000000000a', 'fd000000-0000-0000-0000-0000000000fd', 'f1e10000-0000-0000-0000-0000000000f1', '9a000000-0000-0000-0000-0000000000a1');
SELECT t.expect_error($q$INSERT INTO rain_gauge_fields (company_id, farm_id, field_id, rain_gauge_id)
    VALUES ('a0000000-0000-0000-0000-00000000000a', 'fa000000-0000-0000-0000-0000000000fa', 'f1e20000-0000-0000-0000-0000000000f2', '9a000000-0000-0000-0000-0000000000a1')$q$, '23503'); -- pluviômetro de outra fazenda
SELECT t.expect_error($q$INSERT INTO rain_gauge_fields (company_id, farm_id, field_id, rain_gauge_id)
    VALUES ('a0000000-0000-0000-0000-00000000000a', 'fd000000-0000-0000-0000-0000000000fd', 'f1e20000-0000-0000-0000-0000000000f2', '9a000000-0000-0000-0000-0000000000a1')$q$, '23503'); -- talhão de outra fazenda
SELECT t.expect_error($q$INSERT INTO rain_gauge_fields (company_id, farm_id, field_id, rain_gauge_id)
    VALUES ('b0000000-0000-0000-0000-00000000000b', 'fb000000-0000-0000-0000-0000000000fb', 'f1e30000-0000-0000-0000-0000000000f3', '9a000000-0000-0000-0000-0000000000a1')$q$, '23503'); -- pluviômetro de outra empresa
SELECT t.expect_error($q$INSERT INTO rain_gauge_fields (company_id, farm_id, field_id, rain_gauge_id)
    VALUES ('a0000000-0000-0000-0000-00000000000a', 'fd000000-0000-0000-0000-0000000000fd', 'f1e10000-0000-0000-0000-0000000000f1', '9a000000-0000-0000-0000-0000000000a9')$q$, '23505'); -- segundo vínculo do talhão
-- Leituras: uma por pluviômetro e horário, faixa física, tenant.
INSERT INTO rain_readings (company_id, rain_gauge_id, rain_mm, observed_at, recorded_by)
VALUES ('a0000000-0000-0000-0000-00000000000a', '9a000000-0000-0000-0000-0000000000a1', 18.5, '2026-11-26T10:00:00Z', '0a000000-0000-0000-0000-00000000000a');
SELECT t.expect_error($q$INSERT INTO rain_readings (company_id, rain_gauge_id, rain_mm, observed_at, recorded_by)
    VALUES ('a0000000-0000-0000-0000-00000000000a', '9a000000-0000-0000-0000-0000000000a1', 2, '2026-11-26T07:00:00-03:00', '0a000000-0000-0000-0000-00000000000a')$q$, '23505'); -- mesmo instante
SELECT t.expect_error($q$INSERT INTO rain_readings (company_id, rain_gauge_id, rain_mm, observed_at, recorded_by)
    VALUES ('b0000000-0000-0000-0000-00000000000b', '9a000000-0000-0000-0000-0000000000a1', 2, now(), '0a000000-0000-0000-0000-00000000000a')$q$, '23503'); -- pluviômetro de A com company_id B
SELECT t.expect_error($q$INSERT INTO rain_readings (company_id, rain_gauge_id, rain_mm, observed_at, recorded_by)
    VALUES ('a0000000-0000-0000-0000-00000000000a', '9a000000-0000-0000-0000-0000000000a1', -0.1, now(), '0a000000-0000-0000-0000-00000000000a')$q$, '23514');
SELECT t.expect_error($q$INSERT INTO rain_readings (company_id, rain_gauge_id, rain_mm, observed_at, recorded_by)
    VALUES ('a0000000-0000-0000-0000-00000000000a', '9a000000-0000-0000-0000-0000000000a1', 500.01, now(), '0a000000-0000-0000-0000-00000000000a')$q$, '23514');
-- Vistorias: estado conhecido, talhão da mesma empresa e fazenda, nota curta.
INSERT INTO soil_inspections (company_id, farm_id, field_id, state, inspected_at, inspected_by, note)
VALUES ('a0000000-0000-0000-0000-00000000000a', 'fd000000-0000-0000-0000-0000000000fd', 'f1e10000-0000-0000-0000-0000000000f1', 'moist', now(), '0a000000-0000-0000-0000-00000000000a', 'barro na baixada');
SELECT t.expect_error($q$INSERT INTO soil_inspections (company_id, farm_id, field_id, state, inspected_at, inspected_by)
    VALUES ('a0000000-0000-0000-0000-00000000000a', 'fd000000-0000-0000-0000-0000000000fd', 'f1e10000-0000-0000-0000-0000000000f1', 'unknown', now(), '0a000000-0000-0000-0000-00000000000a')$q$, '23514');
SELECT t.expect_error($q$INSERT INTO soil_inspections (company_id, farm_id, field_id, state, inspected_at, inspected_by)
    VALUES ('b0000000-0000-0000-0000-00000000000b', 'fd000000-0000-0000-0000-0000000000fd', 'f1e10000-0000-0000-0000-0000000000f1', 'dry', now(), '0a000000-0000-0000-0000-00000000000a')$q$, '23503'); -- talhão de A com company_id B
SELECT t.expect_error($q$INSERT INTO soil_inspections (company_id, farm_id, field_id, state, inspected_at, inspected_by)
    VALUES ('a0000000-0000-0000-0000-00000000000a', 'fa000000-0000-0000-0000-0000000000fa', 'f1e10000-0000-0000-0000-0000000000f1', 'dry', now(), '0a000000-0000-0000-0000-00000000000a')$q$, '23503'); -- talhão em outra fazenda
SELECT t.expect_error($q$INSERT INTO soil_inspections (company_id, farm_id, field_id, state, inspected_at, inspected_by, note)
    VALUES ('a0000000-0000-0000-0000-00000000000a', 'fd000000-0000-0000-0000-0000000000fd', 'f1e10000-0000-0000-0000-0000000000f1', 'dry', now(), '0a000000-0000-0000-0000-00000000000a', '  ')$q$, '23514');
-- Limites: semeados como heurística; faixas crescentes; uma linha por textura e vigência.
SELECT t.expect((SELECT count(*) = 3 AND bool_and(source = 'heuristic') FROM trafficability_thresholds), 'seed de limites heuristicos');
SELECT t.expect((SELECT field_capacity_m3m3 = 0.360 FROM trafficability_thresholds WHERE soil_texture = 'clay'), 'capacidade de campo do argiloso');
SELECT t.expect_error($q$INSERT INTO trafficability_thresholds (soil_texture, effective_from, rain_48h_dry_mm, rain_48h_ideal_mm, rain_48h_moist_mm,
    moisture_dry_pct, moisture_ideal_pct, moisture_moist_pct, rain_72h_factor, drying_rate_mm_per_day, field_capacity_m3m3, source)
    VALUES ('clay', '2027-01-01', 2, 15, 8, 60, 90, 100, 1.5, 6, 0.36, 'calibrated')$q$, '23514'); -- faixa decrescente
SELECT t.expect_error($q$INSERT INTO trafficability_thresholds (soil_texture, effective_from, rain_48h_dry_mm, rain_48h_ideal_mm, rain_48h_moist_mm,
    moisture_dry_pct, moisture_ideal_pct, moisture_moist_pct, rain_72h_factor, drying_rate_mm_per_day, field_capacity_m3m3, source)
    VALUES ('clay', '2026-11-26', 2, 8, 15, 60, 90, 100, 1.5, 6, 0.36, 'calibrated')$q$, '23505'); -- mesma vigência
SELECT t.expect_error($q$INSERT INTO trafficability_thresholds (soil_texture, effective_from, rain_48h_dry_mm, rain_48h_ideal_mm, rain_48h_moist_mm,
    moisture_dry_pct, moisture_ideal_pct, moisture_moist_pct, rain_72h_factor, drying_rate_mm_per_day, field_capacity_m3m3, source)
    VALUES ('clay', '2027-01-01', 2, 8, 15, 60, 90, 100, 1.5, 6, 1.2, 'calibrated')$q$, '23514'); -- capacidade acima de 1
SELECT t.expect_error($q$INSERT INTO trafficability_thresholds (soil_texture, effective_from, rain_48h_dry_mm, rain_48h_ideal_mm, rain_48h_moist_mm,
    moisture_dry_pct, moisture_ideal_pct, moisture_moist_pct, rain_72h_factor, drying_rate_mm_per_day, field_capacity_m3m3, source)
    VALUES ('clay', '2027-01-01', 2, 8, 15, 60, 90, 100, 1.5, 6, 0.36, 'guess')$q$, '23514'); -- origem desconhecida

-- ---------------------------------------------------------------------------
-- Histórico do estado do solo (migração 000024)
-- ---------------------------------------------------------------------------
INSERT INTO soil_state_history (company_id, farm_id, field_id, local_day, state, basis, rain_source, last_48h_mm, last_72h_mm, model_moisture_pct, soil_texture, thresholds_source)
VALUES ('a0000000-0000-0000-0000-00000000000a', 'fd000000-0000-0000-0000-0000000000fd', 'f1e10000-0000-0000-0000-0000000000f1', '2026-11-26', 'waterlogged', 'measured_rain', 'gauge', 24, 55, 113.9, 'clay', 'heuristic');
INSERT INTO soil_state_history (company_id, farm_id, field_id, local_day, state, basis, rain_source, last_48h_mm, last_72h_mm, model_moisture_pct, soil_texture, thresholds_source)
VALUES ('a0000000-0000-0000-0000-00000000000a', 'fd000000-0000-0000-0000-0000000000fd', 'f1e10000-0000-0000-0000-0000000000f1', '2026-11-25', 'unknown', 'none', NULL, NULL, NULL, NULL, NULL, 'heuristic');
SELECT t.expect_error($q$INSERT INTO soil_state_history (company_id, farm_id, field_id, local_day, state, basis, rain_source, last_48h_mm, last_72h_mm, model_moisture_pct, soil_texture, thresholds_source)
    VALUES ('a0000000-0000-0000-0000-00000000000a', 'fd000000-0000-0000-0000-0000000000fd', 'f1e10000-0000-0000-0000-0000000000f1', '2026-11-26', 'dry', 'model', 'model', 0, 0, NULL, 'clay', 'heuristic')$q$, '23505'); -- mesmo talhão e dia
SELECT t.expect_error($q$INSERT INTO soil_state_history (company_id, farm_id, field_id, local_day, state, basis, rain_source, last_48h_mm, last_72h_mm, model_moisture_pct, soil_texture, thresholds_source)
    VALUES ('b0000000-0000-0000-0000-00000000000b', 'fd000000-0000-0000-0000-0000000000fd', 'f1e10000-0000-0000-0000-0000000000f1', '2026-11-27', 'dry', 'model', 'model', 0, 0, NULL, 'clay', 'heuristic')$q$, '23503'); -- talhão de A com company_id B
SELECT t.expect_error($q$INSERT INTO soil_state_history (company_id, farm_id, field_id, local_day, state, basis, rain_source, last_48h_mm, last_72h_mm, model_moisture_pct, soil_texture, thresholds_source)
    VALUES ('a0000000-0000-0000-0000-00000000000a', 'fa000000-0000-0000-0000-0000000000fa', 'f1e10000-0000-0000-0000-0000000000f1', '2026-11-27', 'dry', 'model', 'model', 0, 0, NULL, 'clay', 'heuristic')$q$, '23503'); -- talhão em outra fazenda
SELECT t.expect_error($q$INSERT INTO soil_state_history (company_id, farm_id, field_id, local_day, state, basis, rain_source, last_48h_mm, last_72h_mm, model_moisture_pct, soil_texture, thresholds_source)
    VALUES ('a0000000-0000-0000-0000-00000000000a', 'fd000000-0000-0000-0000-0000000000fd', 'f1e10000-0000-0000-0000-0000000000f1', '2026-11-27', 'muddy', 'model', 'model', 0, 0, NULL, 'clay', 'heuristic')$q$, '23514'); -- estado desconhecido
SELECT t.expect_error($q$INSERT INTO soil_state_history (company_id, farm_id, field_id, local_day, state, basis, rain_source, last_48h_mm, last_72h_mm, model_moisture_pct, soil_texture, thresholds_source)
    VALUES ('a0000000-0000-0000-0000-00000000000a', 'fd000000-0000-0000-0000-0000000000fd', 'f1e10000-0000-0000-0000-0000000000f1', '2026-11-27', 'dry', 'guess', 'model', 0, 0, NULL, 'clay', 'heuristic')$q$, '23514'); -- base desconhecida
SELECT t.expect_error($q$INSERT INTO soil_state_history (company_id, farm_id, field_id, local_day, state, basis, rain_source, last_48h_mm, last_72h_mm, model_moisture_pct, soil_texture, thresholds_source)
    VALUES ('a0000000-0000-0000-0000-00000000000a', 'fd000000-0000-0000-0000-0000000000fd', 'f1e10000-0000-0000-0000-0000000000f1', '2026-11-27', 'dry', 'model', 'radar', 0, 0, NULL, 'clay', 'heuristic')$q$, '23514'); -- fonte desconhecida
SELECT t.expect_error($q$INSERT INTO soil_state_history (company_id, farm_id, field_id, local_day, state, basis, rain_source, last_48h_mm, last_72h_mm, model_moisture_pct, soil_texture, thresholds_source)
    VALUES ('a0000000-0000-0000-0000-00000000000a', 'fd000000-0000-0000-0000-0000000000fd', 'f1e10000-0000-0000-0000-0000000000f1', '2026-11-27', 'dry', 'model', NULL, 3, 3, NULL, 'clay', 'heuristic')$q$, '23514'); -- chuva sem fonte
SELECT t.expect_error($q$INSERT INTO soil_state_history (company_id, farm_id, field_id, local_day, state, basis, rain_source, last_48h_mm, last_72h_mm, model_moisture_pct, soil_texture, thresholds_source)
    VALUES ('a0000000-0000-0000-0000-00000000000a', 'fd000000-0000-0000-0000-0000000000fd', 'f1e10000-0000-0000-0000-0000000000f1', '2026-11-27', 'dry', 'model', 'model', NULL, NULL, NULL, 'clay', 'heuristic')$q$, '23514'); -- fonte sem chuva
SELECT t.expect_error($q$INSERT INTO soil_state_history (company_id, farm_id, field_id, local_day, state, basis, rain_source, last_48h_mm, last_72h_mm, model_moisture_pct, soil_texture, thresholds_source)
    VALUES ('a0000000-0000-0000-0000-00000000000a', 'fd000000-0000-0000-0000-0000000000fd', 'f1e10000-0000-0000-0000-0000000000f1', '2026-11-27', 'dry', 'model', 'model', 10, 5, NULL, 'clay', 'heuristic')$q$, '23514'); -- 72 h menor que 48 h
SELECT t.expect_error($q$INSERT INTO soil_state_history (company_id, farm_id, field_id, local_day, state, basis, rain_source, last_48h_mm, last_72h_mm, model_moisture_pct, soil_texture, thresholds_source)
    VALUES ('a0000000-0000-0000-0000-00000000000a', 'fd000000-0000-0000-0000-0000000000fd', 'f1e10000-0000-0000-0000-0000000000f1', '2026-11-27', 'dry', 'model', 'model', 0, 0, -1, 'clay', 'heuristic')$q$, '23514'); -- umidade negativa
SELECT t.expect_error($q$INSERT INTO soil_state_history (company_id, farm_id, field_id, local_day, state, basis, rain_source, last_48h_mm, last_72h_mm, model_moisture_pct, soil_texture, thresholds_source)
    VALUES ('a0000000-0000-0000-0000-00000000000a', 'fd000000-0000-0000-0000-0000000000fd', 'f1e10000-0000-0000-0000-0000000000f1', '2026-11-27', 'dry', 'model', 'model', 0, 0, NULL, 'loam', 'heuristic')$q$, '23514'); -- textura desconhecida
SELECT t.expect_error($q$INSERT INTO soil_state_history (company_id, farm_id, field_id, local_day, state, basis, rain_source, last_48h_mm, last_72h_mm, model_moisture_pct, soil_texture, thresholds_source)
    VALUES ('a0000000-0000-0000-0000-00000000000a', 'fd000000-0000-0000-0000-0000000000fd', 'f1e10000-0000-0000-0000-0000000000f1', '2026-11-27', 'dry', 'model', 'model', 0, 0, NULL, 'clay', 'guess')$q$, '23514'); -- origem dos limites desconhecida

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
SELECT t.expect_error('DELETE FROM public.owners', '42501');
SELECT t.expect_error('DELETE FROM public.fields', '42501');
SELECT t.expect_error('DELETE FROM public.owner_documents', '42501');
SELECT t.expect_error('TRUNCATE public.users', '42501');
-- Catálogo de referência: só leitura
SELECT t.expect((SELECT count(*) FROM crops) = 6, 'api_app le crops');
SELECT t.expect_error('INSERT INTO public.crops (code, name) VALUES (''x'', ''X'')', '42501');
SELECT t.expect_error('UPDATE public.crops SET name = ''X''', '42501');
SELECT t.expect_error('DELETE FROM public.crops', '42501');
SELECT t.expect((SELECT count(*) FROM municipalities) = 0, 'api_app le municipalities (nasce vazia)');
SELECT t.expect_error($q$INSERT INTO public.municipalities (ibge_code, name, state, boundary)
    VALUES ('3128709', 'X', 'MG', ST_GeomFromText('MULTIPOLYGON(((0 0, 0 1, 1 1, 0 0)))', 4326))$q$, '42501');
SELECT t.expect_error('DELETE FROM public.municipalities', '42501');
-- Relevo: elevação é referência global (insere e lê; nunca reescreve nem apaga); relevo sem DELETE
SELECT t.expect((SELECT count(*) = 4 FROM elevation_blocks), 'api_app le elevation_blocks');
SELECT t.expect_error('UPDATE public.elevation_blocks SET tile_key = tile_key', '42501');
SELECT t.expect_error('DELETE FROM public.elevation_blocks', '42501');
SELECT t.expect_error('DELETE FROM public.elevation_sources', '42501');
SELECT t.expect_error('UPDATE public.elevation_sources SET version = ''x''', '42501');
SELECT t.expect_error('DELETE FROM public.field_terrain', '42501');
SELECT t.expect_error('DELETE FROM public.terrain_runs', '42501');
SELECT t.expect_error('TRUNCATE public.field_terrain', '42501');
INSERT INTO elevation_sources (tile_key, source, version, block_count) VALUES ('S22W048', 'copernicus-glo30', '2021_1', 1);
INSERT INTO elevation_blocks (tile_key, rast) SELECT 'S22W048', ST_Tile(ST_AddBand(ST_MakeEmptyRaster(256, 256, -48, -21, 0.0003, -0.0003, 0, 0, 4326), '32BF'::text, 0, -9999), 256, 256);
-- Chuva e solo: leituras e vistorias não se apagam nem se reescrevem; pluviômetro só soft delete;
-- limites são referência (só leitura); o vínculo do talhão é substituído.
SELECT t.expect((SELECT count(*) = 3 FROM trafficability_thresholds), 'api_app le trafficability_thresholds');
SELECT t.expect_error('UPDATE public.trafficability_thresholds SET source = ''calibrated''', '42501');
SELECT t.expect_error('DELETE FROM public.trafficability_thresholds', '42501');
SELECT t.expect_error($q$INSERT INTO public.trafficability_thresholds (soil_texture, effective_from, rain_48h_dry_mm, rain_48h_ideal_mm, rain_48h_moist_mm,
    moisture_dry_pct, moisture_ideal_pct, moisture_moist_pct, rain_72h_factor, drying_rate_mm_per_day, field_capacity_m3m3, source)
    VALUES ('clay', '2027-01-01', 2, 8, 15, 60, 90, 100, 1.5, 6, 0.36, 'calibrated')$q$, '42501');
SELECT t.expect_error('UPDATE public.rain_readings SET rain_mm = 0', '42501');
SELECT t.expect_error('DELETE FROM public.rain_readings', '42501');
SELECT t.expect_error('UPDATE public.soil_inspections SET state = ''dry''', '42501');
SELECT t.expect_error('DELETE FROM public.soil_inspections', '42501');
SELECT t.expect_error('DELETE FROM public.rain_gauges', '42501');
SELECT t.expect_error('TRUNCATE public.rain_gauge_fields', '42501');
INSERT INTO rain_readings (company_id, rain_gauge_id, rain_mm, observed_at, recorded_by)
VALUES ('a0000000-0000-0000-0000-00000000000a', '9a000000-0000-0000-0000-0000000000a1', 0, '2026-11-26T11:00:00Z', '0a000000-0000-0000-0000-00000000000a');
INSERT INTO soil_inspections (company_id, farm_id, field_id, state, inspected_at, inspected_by)
VALUES ('a0000000-0000-0000-0000-00000000000a', 'fd000000-0000-0000-0000-0000000000fd', 'f1e10000-0000-0000-0000-0000000000f1', 'ideal', now(), '0a000000-0000-0000-0000-00000000000a');
UPDATE rain_gauges SET name = 'Sede nova', updated_at = now(), version = version + 1 WHERE id = '9a000000-0000-0000-0000-0000000000a1';
UPDATE fields SET soil_texture = 'medium' WHERE id = 'f1e10000-0000-0000-0000-0000000000f1';
DELETE FROM rain_gauge_fields WHERE field_id = 'f1e10000-0000-0000-0000-0000000000f1';
SELECT t.expect((SELECT count(*) = 0 FROM rain_gauge_fields), 'api_app removeu o vinculo');
-- Histórico do solo: só INSERT e SELECT.
SELECT t.expect((SELECT count(*) = 2 FROM soil_state_history), 'api_app le soil_state_history');
INSERT INTO soil_state_history (company_id, farm_id, field_id, local_day, state, basis, rain_source, last_48h_mm, last_72h_mm, model_moisture_pct, soil_texture, thresholds_source)
VALUES ('a0000000-0000-0000-0000-00000000000a', 'fd000000-0000-0000-0000-0000000000fd', 'f1e10000-0000-0000-0000-0000000000f1', '2026-11-27', 'ideal', 'model', 'model', 4, 4, 70, 'medium', 'heuristic');
INSERT INTO soil_state_history (company_id, farm_id, field_id, local_day, state, basis, rain_source, last_48h_mm, last_72h_mm, model_moisture_pct, soil_texture, thresholds_source)
VALUES ('a0000000-0000-0000-0000-00000000000a', 'fd000000-0000-0000-0000-0000000000fd', 'f1e10000-0000-0000-0000-0000000000f1', '2026-11-27', 'dry', 'model', 'model', 0, 0, NULL, 'medium', 'heuristic')
ON CONFLICT (company_id, field_id, local_day) DO NOTHING;
SELECT t.expect((SELECT state = 'ideal' FROM soil_state_history WHERE local_day = '2026-11-27'), 'ON CONFLICT DO NOTHING preserva o primeiro registro do dia');
SELECT t.expect_error('UPDATE public.soil_state_history SET state = ''dry''', '42501');
SELECT t.expect_error('DELETE FROM public.soil_state_history', '42501');
SELECT t.expect_error('TRUNCATE public.soil_state_history', '42501');
-- O ajuste manual PODE ser removido de verdade (volta ao valor calculado).
DELETE FROM field_terrain_adjustments WHERE id = 'ad000000-0000-0000-0000-0000000000a1';
SELECT t.expect((SELECT count(*) = 0 FROM field_terrain_adjustments), 'api_app removeu o ajuste');
-- DML do dia a dia funciona
UPDATE users SET last_access_at = now(), updated_at = now(), version = version + 1 WHERE id = '0a000000-0000-0000-0000-00000000000a';
DELETE FROM auth_sessions WHERE user_id = '0a000000-0000-0000-0000-00000000000a';
INSERT INTO user_farm_access (user_id, farm_id, company_id)
VALUES ('0b000000-0000-0000-0000-00000000000b', 'fb000000-0000-0000-0000-0000000000fb', 'b0000000-0000-0000-0000-00000000000b');
DELETE FROM user_farm_access;
INSERT INTO farm_owners (company_id, farm_id, owner_id, role, share_percent)
VALUES ('a0000000-0000-0000-0000-00000000000a', 'fa000000-0000-0000-0000-0000000000fa', '0a0a0000-0000-0000-0000-00000000000a', 'owner', 100);
DELETE FROM farm_owners; -- substituir vínculos apaga e regrava
-- Sequência da identity de audit_events utilizável (já provado pelo INSERT acima).

RESET ROLE;
DROP SCHEMA t CASCADE;
\o
\echo 'assertions.sql: OK'
