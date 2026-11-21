# syntax=docker/dockerfile:1

# --- build: resolve the locked environment with uv ---------------------------
FROM python:3.12-slim AS build

COPY --from=ghcr.io/astral-sh/uv:0.13.9 /uv /bin/uv

ENV UV_COMPILE_BYTECODE=1 \
    UV_LINK_MODE=copy \
    UV_PYTHON_DOWNLOADS=never

WORKDIR /app

# Dependencies first, in their own layer: source edits do not invalidate it.
RUN --mount=type=cache,target=/root/.cache/uv \
    --mount=type=bind,source=uv.lock,target=uv.lock \
    --mount=type=bind,source=pyproject.toml,target=pyproject.toml \
    uv sync --frozen --no-dev --no-install-project

COPY pyproject.toml uv.lock ./
COPY src ./src
RUN --mount=type=cache,target=/root/.cache/uv \
    uv sync --frozen --no-dev

# --- runtime: no uv, no build tooling, non-root -------------------------------
FROM python:3.12-slim AS runtime

RUN groupadd --system --gid 10001 app \
    && useradd --system --uid 10001 --gid app --no-create-home app

WORKDIR /app

COPY --from=build --chown=app:app /app/.venv ./.venv
COPY --from=build --chown=app:app /app/src ./src

ENV PATH="/app/.venv/bin:$PATH" \
    PYTHONUNBUFFERED=1

USER 10001

EXPOSE 8000

CMD ["uvicorn", "label_advisor.main:app", "--host", "0.0.0.0", "--port", "8000"]
