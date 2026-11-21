"""Caso de uso da sonda de readiness."""

import asyncio
import logging
from collections.abc import Sequence

from spray_planner.application.dto.health import CheckResult, HealthReport
from spray_planner.application.lifecycle import Lifecycle
from spray_planner.application.ports.dependency_probe import DependencyProbe

logger = logging.getLogger(__name__)


class CheckReadiness:
    """Indica se o serviço pode receber tráfego.

    Fica `down` ao iniciar, durante o shutdown (para drenar o tráfego) e quando
    alguma dependência falha. A causa da falha vai só para o log: a resposta da
    sonda é pública e não deve vazar detalhes internos.
    """

    def __init__(self, lifecycle: Lifecycle, probes: Sequence[DependencyProbe]) -> None:
        self._lifecycle = lifecycle
        self._probes = tuple(probes)

    async def execute(self) -> HealthReport:
        """Verifica o ciclo de vida e, se estiver pronto, todas as dependências."""
        if not self._lifecycle.started or self._lifecycle.shutting_down:
            return HealthReport(status="down")

        # As sondas rodam em paralelo para que uma dependência lenta não some sua
        # latência à das demais.
        outcomes = await asyncio.gather(
            *(probe.check() for probe in self._probes), return_exceptions=True
        )
        checks = tuple(
            self._to_check_result(probe, outcome)
            for probe, outcome in zip(self._probes, outcomes, strict=True)
        )
        all_up = all(check.status == "up" for check in checks)
        return HealthReport(status="up" if all_up else "down", checks=checks)

    @staticmethod
    def _to_check_result(probe: DependencyProbe, outcome: BaseException | None) -> CheckResult:
        if outcome is None:
            return CheckResult(name=probe.name, status="up")
        # Cancelamento e interrupção não são falhas da dependência; propagam.
        if not isinstance(outcome, Exception):
            raise outcome
        logger.warning(
            "dependency probe failed",
            extra={"probe": probe.name, "error": repr(outcome)},
        )
        return CheckResult(name=probe.name, status="down")
