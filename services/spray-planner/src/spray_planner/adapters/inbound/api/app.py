"""Fábrica da aplicação FastAPI."""

from collections.abc import AsyncIterator
from contextlib import asynccontextmanager

from fastapi import FastAPI

from spray_planner.adapters.inbound.api.health import HealthCheck, create_health_router
from spray_planner.application.lifecycle import Lifecycle


def create_app(
    lifecycle: Lifecycle,
    liveness: HealthCheck,
    startup: HealthCheck,
    readiness: HealthCheck,
) -> FastAPI:
    """Monta a aplicação HTTP com as dependências recebidas da raiz de composição."""

    @asynccontextmanager
    async def lifespan(_: FastAPI) -> AsyncIterator[None]:
        lifecycle.mark_started()
        yield
        lifecycle.begin_shutdown()

    app = FastAPI(title="spray-planner", docs_url="/docs", lifespan=lifespan)
    app.include_router(create_health_router(liveness, startup, readiness))
    return app
