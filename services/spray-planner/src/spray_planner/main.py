"""Raiz de composição: o único lugar onde as dependências são montadas.

Executado com `uvicorn spray_planner.main:app`.
"""

import logging

from fastapi import FastAPI

from spray_planner.adapters.inbound.api.app import create_app
from spray_planner.application.lifecycle import Lifecycle
from spray_planner.application.use_cases.check_liveness import CheckLiveness
from spray_planner.application.use_cases.check_readiness import CheckReadiness
from spray_planner.application.use_cases.check_startup import CheckStartup
from spray_planner.config import Settings


def build_app(settings: Settings) -> FastAPI:
    """Compõe casos de uso e adaptadores a partir da configuração."""
    logging.basicConfig(
        level=settings.log_level.upper(),
        format="%(asctime)s %(levelname)s %(name)s %(message)s",
    )
    lifecycle = Lifecycle()
    return create_app(
        lifecycle=lifecycle,
        liveness=CheckLiveness(),
        startup=CheckStartup(lifecycle),
        # Ainda sem dependências externas; as sondas entram junto com os adaptadores.
        readiness=CheckReadiness(lifecycle, probes=()),
    )


app = build_app(Settings())
