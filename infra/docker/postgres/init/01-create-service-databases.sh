#!/usr/bin/env bash
# Cria um banco por serviço, cada um com dois usuários, como em produção:
#   <service>_migrator  dono do banco e do schema; usado só por migrações (DDL)
#   <service>_app       usado pelo serviço em execução; apenas DML
# Nenhum usuário consegue conectar no banco de outro serviço ("cada dado tem um
# único dono"). Roda apenas na primeira inicialização do volume.
set -euo pipefail

: "${API_DB_MIGRATOR_PASSWORD:?defina API_DB_MIGRATOR_PASSWORD no .env}"
: "${API_DB_APP_PASSWORD:?defina API_DB_APP_PASSWORD no .env}"
: "${LABEL_ADVISOR_DB_MIGRATOR_PASSWORD:?defina LABEL_ADVISOR_DB_MIGRATOR_PASSWORD no .env}"
: "${LABEL_ADVISOR_DB_APP_PASSWORD:?defina LABEL_ADVISOR_DB_APP_PASSWORD no .env}"

run_sql() {
  psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" "$@"
}

# Senhas entram como variáveis do psql (:'name'), que fazem o quoting correto:
# nenhuma senha é concatenada em SQL.
create_service_database() {
  local database="$1" migrator_password="$2" app_password="$3"
  local migrator="${database}_migrator" app="${database}_app"

  run_sql --dbname postgres \
    -v database="$database" -v migrator="$migrator" -v app="$app" \
    -v migrator_password="$migrator_password" -v app_password="$app_password" <<'SQL'
CREATE ROLE :"migrator" LOGIN PASSWORD :'migrator_password';
CREATE ROLE :"app" LOGIN PASSWORD :'app_password' CONNECTION LIMIT 40;
CREATE DATABASE :"database" OWNER :"migrator" ENCODING 'UTF8' TEMPLATE template0;
REVOKE ALL ON DATABASE :"database" FROM PUBLIC;
GRANT CONNECT, TEMPORARY ON DATABASE :"database" TO :"app";
SQL

  run_sql --dbname "$database" -v migrator="$migrator" -v app="$app" <<'SQL'
REVOKE ALL ON SCHEMA public FROM PUBLIC;
ALTER SCHEMA public OWNER TO :"migrator";
GRANT USAGE ON SCHEMA public TO :"app";
ALTER DEFAULT PRIVILEGES FOR ROLE :"migrator" IN SCHEMA public
  GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO :"app";
ALTER DEFAULT PRIVILEGES FOR ROLE :"migrator" IN SCHEMA public
  GRANT USAGE, SELECT ON SEQUENCES TO :"app";
ALTER DEFAULT PRIVILEGES FOR ROLE :"migrator" IN SCHEMA public
  GRANT EXECUTE ON FUNCTIONS TO :"app";
SQL
}

create_service_database api "$API_DB_MIGRATOR_PASSWORD" "$API_DB_APP_PASSWORD"
create_service_database label_advisor "$LABEL_ADVISOR_DB_MIGRATOR_PASSWORD" "$LABEL_ADVISOR_DB_APP_PASSWORD"

# Extensões exigem superusuário, por isso são criadas aqui e não nas migrações.
run_sql --dbname label_advisor <<'SQL'
CREATE EXTENSION IF NOT EXISTS vector;
SQL
for database in postgres api label_advisor; do
  run_sql --dbname "$database" -c 'CREATE EXTENSION IF NOT EXISTS pg_stat_statements;'
done
