#!/usr/bin/env bash
# Medição OPT-IN do relevo com um bloco REAL do Copernicus DEM GLO-30 (ADR 0021): baixa ~42 MB do
# bucket público, ingere num Postgres DESCARTÁVEL (mesma biblioteca do integration-test.sh) e mede
# tempo, memória e disco. Não altera o integration-test.sh e nunca toca no agro-postgres.
#
# Uso: bash scripts/real-tile-test.sh   (exige Docker e a imagem agentes-agro/postgres:dev)
set -euo pipefail

here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib/disposable-db.sh
source "$here/lib/disposable-db.sh"
trap disposable_db_cleanup EXIT

disposable_db_start publish
step "migrações"
migrate up

step "teste do bloco real (tags integration realtile; REAL_TILE=1)"
cd "$api_dir"
REAL_TILE=1 REAL_TILE_DB_CONTAINER="$container" \
API_TEST_DATABASE_URL="postgres://api_app:${app_password}@127.0.0.1:${db_port}/api?sslmode=disable" \
  go test -count=1 -tags 'integration realtile' -run TestRealCopernicusTile -v -timeout 45m ./internal/adapter/postgres/...
step "OK: medição do bloco real"
