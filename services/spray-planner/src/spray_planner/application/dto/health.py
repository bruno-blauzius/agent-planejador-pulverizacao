"""DTOs de saída das sondas de saúde."""

from dataclasses import dataclass, field
from typing import Literal

HealthStatus = Literal["up", "down"]


@dataclass(frozen=True)
class CheckResult:
    """Estado de uma dependência verificada pela readiness."""

    name: str
    status: HealthStatus


@dataclass(frozen=True)
class HealthReport:
    """Resultado de uma sonda: estado geral e, quando houver, de cada dependência."""

    status: HealthStatus
    checks: tuple[CheckResult, ...] = field(default=())

    @property
    def is_up(self) -> bool:
        """Indica se a sonda está saudável."""
        return self.status == "up"
