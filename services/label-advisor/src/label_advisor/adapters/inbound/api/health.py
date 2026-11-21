from typing import Literal, Protocol

from fastapi import APIRouter
from fastapi.responses import JSONResponse
from pydantic import BaseModel

from label_advisor.application.dto.health import HealthReport


class LivenessChecker(Protocol):
    """Answers "is the process alive?"."""

    def execute(self) -> HealthReport: ...


class StartupChecker(Protocol):
    """Answers "has initialization finished?"."""

    def execute(self) -> HealthReport: ...


class ReadinessChecker(Protocol):
    """Answers "can this instance receive traffic?"."""

    async def execute(self) -> HealthReport: ...


class CheckResultResponse(BaseModel):
    """One named dependency check."""

    name: str
    status: Literal["up", "down"]


class HealthResponse(BaseModel):
    """Probe body; `checks` is omitted when there are no dependency checks."""

    status: Literal["up", "down"]
    checks: list[CheckResultResponse] | None = None


_PROBE_RESPONSES: dict[int | str, dict[str, object]] = {
    503: {"model": HealthResponse, "description": "Not healthy"},
}


def create_health_router(
    liveness: LivenessChecker,
    startup: StartupChecker,
    readiness: ReadinessChecker,
) -> APIRouter:
    """Build the Kubernetes probe endpoints over the given use cases."""
    router = APIRouter(prefix="/health", tags=["health"])

    @router.get("/live", response_model=HealthResponse, responses=_PROBE_RESPONSES)
    def live() -> JSONResponse:
        return _to_response(liveness.execute())

    @router.get("/startup", response_model=HealthResponse, responses=_PROBE_RESPONSES)
    def started() -> JSONResponse:
        return _to_response(startup.execute())

    @router.get("/ready", response_model=HealthResponse, responses=_PROBE_RESPONSES)
    async def ready() -> JSONResponse:
        return _to_response(await readiness.execute())

    return router


def _to_response(report: HealthReport) -> JSONResponse:
    body = HealthResponse(
        status=report.status.value,
        checks=[
            CheckResultResponse(name=check.name, status=check.status.value)
            for check in report.checks
        ]
        or None,
    )
    return JSONResponse(
        content=body.model_dump(exclude_none=True),
        status_code=200 if report.healthy else 503,
        # Probes must reflect the current state; intermediaries must not cache them.
        headers={"Cache-Control": "no-store"},
    )
