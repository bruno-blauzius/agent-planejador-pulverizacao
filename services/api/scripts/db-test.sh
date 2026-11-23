#!/usr/bin/env bash
# Teste de integração das migrações da API em banco DESCARTÁVEL.
#
# Sobe um container Postgres temporário com a imagem do projeto (pgvector + PostGIS), o mesmo
# postgresql.conf e o MESMO init de produção local (papéis, bancos, extensões), com senhas
# aleatórias. Nome e rede únicos; tudo é removido ao final (trap), mesmo em falha.
# Nunca toca no volume do agro-postgres nem no db-keycloak e não usa `down -v`.
#
# Passos: up -> down -all -> up (migrate/migrate em container) -> tests/db/assertions.sql
# (constraints, FK composta de tenant, privilégios do api_app) -> checagens de conexão do api_app.
set -euo pipefail

here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib/disposable-db.sh
source "$here/lib/disposable-db.sh"
assertions="$here/../tests/db/assertions.sql"
if command -v cygpath >/dev/null 2>&1; then
  assertions="$(cygpath -w "$(cd "$(dirname "$assertions")" && pwd)")\assertions.sql"
fi
trap disposable_db_cleanup EXIT

disposable_db_start

step "up"
migrate up
step "down -all"
migrate down -all
remaining="$(docker exec -e PGPASSWORD="$migrator_password" "$container" psql -h 127.0.0.1 -U api_migrator -d api -Atqc \
  "SELECT count(*) FROM information_schema.tables WHERE table_schema='public' AND table_type='BASE TABLE' AND table_name <> 'schema_migrations' AND table_name <> 'spatial_ref_sys'")"
[ "$remaining" = "0" ] || { echo "down -all deixou $remaining tabela(s)"; exit 1; }
echo "down -all: sem tabelas remanescentes"
step "up (de novo)"
migrate up
step "version"
migrate version

step "asserções SQL (constraints, tenant, privilégios)"
docker exec -i -e PGPASSWORD="$super_password" "$container" \
  psql -h 127.0.0.1 -U dbtest_admin -d api -v ON_ERROR_STOP=1 -q < "$assertions"

step "api_app: conexão e fronteiras entre bancos"
if docker exec -e PGPASSWORD="$app_password" "$container" psql -h 127.0.0.1 -U api_app -d label_advisor -Atqc 'SELECT 1' >/dev/null 2>&1; then
  echo "FALHOU: api_app conectou em label_advisor"; exit 1
fi
echo "api_app nao conecta em label_advisor: ok"
docker exec -e PGPASSWORD="$app_password" "$container" psql -h 127.0.0.1 -U api_app -d api -Atqc 'SELECT count(*) FROM crops' >/dev/null
echo "api_app conecta em api: ok"
if docker exec -e PGPASSWORD="$app_password" "$container" psql -h 127.0.0.1 -U api_app -d api -Atqc 'CREATE TABLE public.hack (id int)' >/dev/null 2>&1; then
  echo "FALHOU: api_app criou tabela"; exit 1
fi
echo "api_app nao executa DDL: ok"

step "OK: migrações, constraints e privilégios verificados"
