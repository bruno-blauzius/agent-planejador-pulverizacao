"""Caso de uso da sonda de liveness."""

from spray_planner.application.dto.health import HealthReport


class CheckLiveness:
    """Indica apenas que o processo responde.

    Nunca consulta dependências: uma falha na API ou no clima não deve fazer o
    orquestrador reiniciar o contêiner, só tirá-lo do tráfego (readiness).
    """

    async def execute(self) -> HealthReport:
        """Devolve sempre `up`."""
        return HealthReport(status="up")
