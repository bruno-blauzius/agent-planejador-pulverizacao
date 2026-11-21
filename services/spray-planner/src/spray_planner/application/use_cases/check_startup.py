"""Caso de uso da sonda de startup."""

from spray_planner.application.dto.health import HealthReport
from spray_planner.application.lifecycle import Lifecycle


class CheckStartup:
    """Fica `down` até a inicialização terminar e `up` depois disso."""

    def __init__(self, lifecycle: Lifecycle) -> None:
        self._lifecycle = lifecycle

    async def execute(self) -> HealthReport:
        """Reflete se o ciclo de vida já foi marcado como iniciado."""
        return HealthReport(status="up" if self._lifecycle.started else "down")
