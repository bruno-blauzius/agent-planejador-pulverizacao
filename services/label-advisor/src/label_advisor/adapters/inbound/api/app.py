from collections.abc import AsyncIterator
from contextlib import asynccontextmanager

from fastapi import FastAPI

from label_advisor.adapters.inbound.api.health import (
    LivenessChecker,
    ReadinessChecker,
    StartupChecker,
    create_health_router,
)
from label_advisor.application.lifecycle import Lifecycle


def create_app(
    lifecycle: Lifecycle,
    liveness: LivenessChecker,
    startup: StartupChecker,
    readiness: ReadinessChecker,
) -> FastAPI:
    """Build the HTTP application over use cases assembled by the composition root."""

    @asynccontextmanager
    async def lifespan(_: FastAPI) -> AsyncIterator[None]:
        lifecycle.mark_started()
        try:
            yield
        finally:
            lifecycle.begin_shutdown()

    app = FastAPI(title="label-advisor", docs_url="/docs", lifespan=lifespan)
    app.include_router(create_health_router(liveness, startup, readiness))
    return app
