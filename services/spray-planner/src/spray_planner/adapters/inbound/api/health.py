"""Rotas das sondas de saúde, no mesmo contrato da API Go do monorepo."""

from collections.abc import Awaitable
from typing import Literal, Protocol

from fastapi import APIRouter
from fastapi.responses import JSONResponse
from pydantic import BaseModel

from spray_planner.application.dto.health import HealthReport


class HealthCheck(Protocol):
    """Qualquer caso de uso de saúde: CheckLiveness, CheckStartup ou CheckReadiness."""

    def execute(self) -> Awaitable[HealthReport]: ...


class CheckResultSchema(BaseModel):
    """Estado de uma dependência na resposta HTTP."""

    name: str
    status: Literal["up", "down"]


class HealthReportSchema(BaseModel):
    """Corpo da resposta das sondas; `checks` é omitido quando vazio."""

    status: Literal["up", "down"]
    checks: list[CheckResultSchema] | None = None


_PROBE_RESPONSES: dict[int | str, dict[str, object]] = {
    200: {"model": HealthReportSchema, "description": "Saudável."},
    503: {"model": HealthReportSchema, "description": "Não saudável."},
}


def create_health_router(
    liveness: HealthCheck, startup: HealthCheck, readiness: HealthCheck
) -> APIRouter:
    """Cria o router das sondas a partir dos casos de uso injetados."""
    router = APIRouter(prefix="/health", tags=["Health"])

    @router.get("/live", responses=_PROBE_RESPONSES, summary="Sonda de liveness")
    async def get_liveness() -> JSONResponse:
        return _to_response(await liveness.execute())

    @router.get("/startup", responses=_PROBE_RESPONSES, summary="Sonda de startup")
    async def get_startup() -> JSONResponse:
        return _to_response(await startup.execute())

    @router.get("/ready", responses=_PROBE_RESPONSES, summary="Sonda de readiness")
    async def get_readiness() -> JSONResponse:
        return _to_response(await readiness.execute())

    return router


def _to_response(report: HealthReport) -> JSONResponse:
    body = HealthReportSchema(
        status=report.status,
        checks=[CheckResultSchema(name=check.name, status=check.status) for check in report.checks]
        or None,
    )
    return JSONResponse(
        content=body.model_dump(exclude_none=True),
        status_code=200 if report.is_up else 503,
        # Sondas nunca podem ser servidas de cache, senão o orquestrador vê estado velho.
        headers={"Cache-Control": "no-store"},
    )
