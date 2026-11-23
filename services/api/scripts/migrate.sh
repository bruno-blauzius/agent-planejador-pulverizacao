#!/usr/bin/env bash
# Aplica as migrações da API (golang-migrate) no banco `api` do Postgres do docker compose,
# com o papel api_migrator (dono; só DDL). A ferramenta roda em container, na rede do compose:
# nenhuma dependência é adicionada ao go.mod e a porta do banco não precisa ser exposta.
#
# Uso: scripts/migrate.sh up | down 1 | down -all | version | force N | goto N
#
# A senha vem do .env da raiz (última ocorrência da chave, como o docker compose resolve) e é
# repassada ao container por variável de ambiente, nunca por argumento nem impressa.
set -euo pipefail

MIGRATE_IMAGE="${MIGRATE_IMAGE:-migrate/migrate:v4.18.3}"
COMPOSE_NETWORK="${COMPOSE_NETWORK:-agentes-agro}"
POSTGRES_HOST="${POSTGRES_HOST:-agro-postgres}"

here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
root="$(cd "$here/../../.." && pwd)"
migrations_dir="$(cd "$here/../migrations" && pwd)"
# Git Bash no Windows precisa do caminho no formato Windows para o bind mount.
if command -v cygpath >/dev/null 2>&1; then migrations_dir="$(cygpath -w "$migrations_dir")"; fi

if [ "$#" -eq 0 ]; then
  echo "uso: $0 up | down N | down -all | version | force N | goto N" >&2
  exit 2
fi
if [ "$1" = "down" ] && [ "${2:-}" != "-all" ] && ! [[ "${2:-}" =~ ^[0-9]+$ ]]; then
  echo "recusado: informe quantas migrações desfazer (down 1) ou -all explicitamente" >&2
  exit 2
fi

env_file="$root/.env"
[ -f "$env_file" ] || { echo "falta $env_file (copie de .env.example)" >&2; exit 1; }
password="$(grep -E '^API_DB_MIGRATOR_PASSWORD=' "$env_file" | tail -n 1 | cut -d= -f2-)"
[ -n "$password" ] || { echo "API_DB_MIGRATOR_PASSWORD vazio no .env" >&2; exit 1; }

docker network inspect "$COMPOSE_NETWORK" >/dev/null 2>&1 \
  || { echo "rede $COMPOSE_NETWORK não existe: suba o Postgres com 'make up'" >&2; exit 1; }

# -all/N entram como argumentos do migrate ("down -all" / "down 1").
MSYS_NO_PATHCONV=1 API_DB_MIGRATOR_PASSWORD="$password" docker run --rm \
  --network "$COMPOSE_NETWORK" \
  -e API_DB_MIGRATOR_PASSWORD \
  -e POSTGRES_HOST="$POSTGRES_HOST" \
  -v "$migrations_dir:/migrations:ro" \
  --entrypoint sh "$MIGRATE_IMAGE" \
  -c 'exec migrate -path /migrations -database "postgres://api_migrator:${API_DB_MIGRATOR_PASSWORD}@${POSTGRES_HOST}:5432/api?sslmode=disable" "$@"' \
  migrate "$@"
