#!/usr/bin/env bash
# Testes de INTEGRAÇÃO do adaptador Postgres da API em banco DESCARTÁVEL.
#
# Sobe o Postgres temporário (scripts/lib/disposable-db.sh), aplica as migrações com o papel
# api_migrator e roda `go test -tags integration` no host, conectando com o papel de runtime
# `api_app` (o mesmo privilégio de produção: só DML). Remove tudo ao final (trap), mesmo em falha.
# Nunca toca no agro-postgres nem no db-keycloak.
#
# Uso: scripts/integration-test.sh [argumentos extras do go test, ex.: -run TestUsers -v]
set -euo pipefail

here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib/disposable-db.sh
source "$here/lib/disposable-db.sh"
trap disposable_db_cleanup EXIT

disposable_db_start publish
step "migrações"
migrate up

step "go test -tags integration (papel api_app, 127.0.0.1:$db_port)"
cd "$api_dir"
# A senha só trafega por variável de ambiente deste processo; o teste nunca a imprime.
API_TEST_DATABASE_URL="postgres://api_app:${app_password}@127.0.0.1:${db_port}/api?sslmode=disable" \
  go test -count=1 -tags integration "$@" ./internal/adapter/postgres/...
step "OK: integração do adaptador Postgres"
