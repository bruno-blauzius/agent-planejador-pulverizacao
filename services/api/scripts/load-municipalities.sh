#!/usr/bin/env bash
# Carrega a malha municipal do IBGE no banco `api` do Postgres local (ADR 0019), com o papel
# api_migrator (a tabela é dado de referência; o api_app só lê). Baixa nomes e limites da API
# pública do IBGE (servicodados.ibge.gov.br) e substitui a tabela municipalities numa transação.
#
# Uso: scripts/load-municipalities.sh            (qualidade intermediaria)
#      API_IBGE_MESH_QUALITY=minima scripts/load-municipalities.sh
#
# A senha vem do .env da raiz e só trafega por variável de ambiente deste processo.
set -euo pipefail

here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
root="$(cd "$here/../../.." && pwd)"
env_file="$root/.env"
[ -f "$env_file" ] || { echo "falta $env_file (copie de .env.example)" >&2; exit 1; }
env_value() { grep -E "^$1=" "$env_file" | tail -n 1 | cut -d= -f2-; }

password="$(env_value API_DB_MIGRATOR_PASSWORD)"
[ -n "$password" ] || { echo "API_DB_MIGRATOR_PASSWORD vazio no .env" >&2; exit 1; }
port="$(env_value POSTGRES_HOST_PORT)"
port="${port:-5433}"

cd "$here/.."
API_MIGRATOR_DATABASE_URL="postgres://api_migrator:${password}@127.0.0.1:${port}/api?sslmode=disable" \
  go run ./cmd/load-municipalities
