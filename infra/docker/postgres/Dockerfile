# Postgres local com pgvector (já na imagem base) e PostGIS.
# Decisão e motivos: docs/adr/0014-postgis-e-pgvector-na-imagem-do-postgres.md
FROM pgvector/pgvector:pg17

# O PostGIS vem do repositório PGDG já configurado na imagem base; o major (17)
# precisa acompanhar o da imagem. Os binários ficam disponíveis no servidor, mas
# cada banco só ganha a extensão com CREATE EXTENSION (feito em init/).
RUN apt-get update \
    && apt-get install -y --no-install-recommends \
        postgresql-17-postgis-3 \
        postgresql-17-postgis-3-scripts \
    && rm -rf /var/lib/apt/lists/*
