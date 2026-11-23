#!/usr/bin/env bash
# Biblioteca (para `source`): sobe um Postgres DESCARTÁVEL com a imagem do projeto (pgvector +
# PostGIS), o mesmo postgresql.conf e o MESMO init de produção local (papéis, bancos, extensões),
# com senhas aleatórias. Nome e rede únicos; tudo é removido pelo trap de quem fizer `source`,
# mesmo em falha. Nunca toca no volume do agro-postgres nem no db-keycloak e não usa `down -v`.
#
# Define: container, network, super_password, migrator_password, app_password, migrations_dir,
# e as funções step, disposable_db_start [publish], disposable_db_cleanup, migrate.
# Com `publish`, a porta 5432 é publicada em 127.0.0.1 (porta aleatória) e db_port é definida,
# para testes que rodam no host.

POSTGRES_IMAGE="${POSTGRES_IMAGE:-agentes-agro/postgres:dev}"
MIGRATE_IMAGE="${MIGRATE_IMAGE:-migrate/migrate:v4.18.3}"

_lib_here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
api_dir="$(cd "$_lib_here/../.." && pwd)"
repo_root="$(cd "$api_dir/../.." && pwd)"
migrations_dir="$api_dir/migrations"
init_dir="$repo_root/infra/docker/postgres/init"
conf_file="$repo_root/infra/docker/postgres/postgresql.conf"
if command -v cygpath >/dev/null 2>&1; then
  migrations_dir="$(cygpath -w "$migrations_dir")"
  init_dir="$(cygpath -w "$init_dir")"
  conf_file="$(cygpath -w "$conf_file")"
fi

suffix="$$-$(date +%s)"
container="agro-dbtest-$suffix"
network="agro-dbtest-net-$suffix"

random_secret() { head -c 24 /dev/urandom | od -An -tx1 | tr -d ' \n'; }
super_password="$(random_secret)"
migrator_password="$(random_secret)"
app_password="$(random_secret)"
other_password="$(random_secret)"

docker() { MSYS_NO_PATHCONV=1 command docker "$@"; }
step() { printf '\n== %s\n' "$*"; }

disposable_db_cleanup() {
  docker rm -f "$container" >/dev/null 2>&1 || true
  docker network rm "$network" >/dev/null 2>&1 || true
}

disposable_db_start() {
  local publish_args=()
  if [ "${1:-}" = "publish" ]; then publish_args=(-p "127.0.0.1::5432"); fi

  step "subindo Postgres descartável ($container)"
  docker network create "$network" >/dev/null
  docker run -d --name "$container" --network "$network" "${publish_args[@]}" \
    -e POSTGRES_USER=dbtest_admin -e POSTGRES_PASSWORD="$super_password" -e POSTGRES_DB=postgres \
    -e POSTGRES_HOST_AUTH_METHOD=scram-sha-256 -e POSTGRES_INITDB_ARGS="--auth-host=scram-sha-256 --data-checksums" \
    -e API_DB_MIGRATOR_PASSWORD="$migrator_password" -e API_DB_APP_PASSWORD="$app_password" \
    -e LABEL_ADVISOR_DB_MIGRATOR_PASSWORD="$other_password" -e LABEL_ADVISOR_DB_APP_PASSWORD="$other_password" \
    -v "$init_dir:/docker-entrypoint-initdb.d:ro" \
    -v "$conf_file:/etc/postgresql/postgresql.conf:ro" \
    "$POSTGRES_IMAGE" postgres -c config_file=/etc/postgresql/postgresql.conf >/dev/null

  # O entrypoint sobe um servidor temporário só em socket durante o init e o reinicia: esperar
  # o banco `api` aceitar TCP com o papel migrator garante que o init terminou.
  local ready=0
  for _ in $(seq 1 60); do
    if docker exec -e PGPASSWORD="$migrator_password" "$container" \
        psql -h 127.0.0.1 -U api_migrator -d api -Atqc 'SELECT 1' >/dev/null 2>&1; then ready=1; break; fi
    sleep 1
  done
  [ "$ready" = 1 ] || { echo "Postgres descartável não ficou pronto"; docker logs --tail 30 "$container"; return 1; }

  if [ "${1:-}" = "publish" ]; then
    db_port="$(docker port "$container" 5432/tcp | head -n1 | sed 's/.*://')"
    [ -n "$db_port" ] || { echo "não foi possível descobrir a porta publicada"; return 1; }
  fi
}

migrate() {
  docker run --rm --network "$network" -e MIGRATOR_PASSWORD="$migrator_password" \
    -v "$migrations_dir:/migrations:ro" --entrypoint sh "$MIGRATE_IMAGE" \
    -c 'exec migrate -path /migrations -database "postgres://api_migrator:${MIGRATOR_PASSWORD}@'"$container"':5432/api?sslmode=disable" "$@"' migrate "$@"
}
