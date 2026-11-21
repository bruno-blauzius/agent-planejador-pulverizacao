"""Composition root: the only place where dependencies are assembled.

Run with `uvicorn label_advisor.main:app`.
"""

import logging

from fastapi import FastAPI

from label_advisor.adapters.inbound.api.app import create_app
from label_advisor.application.lifecycle import Lifecycle
from label_advisor.application.use_cases import CheckLiveness, CheckReadiness, CheckStartup
from label_advisor.config import Settings


def build_app(settings: Settings) -> FastAPI:
    """Wire use cases and adapters into the HTTP application."""
    logging.basicConfig(
        level=settings.log_level.upper(),
        format="%(asctime)s %(levelname)s %(name)s %(message)s",
    )
    lifecycle = Lifecycle()
    return create_app(
        lifecycle=lifecycle,
        liveness=CheckLiveness(),
        startup=CheckStartup(lifecycle),
        # Dependency probes (database, LLM) are registered here from Phase 1.
        readiness=CheckReadiness(
            lifecycle,
            probes=(),
            probe_timeout_seconds=settings.probe_timeout_seconds,
        ),
    )


app = build_app(Settings())
