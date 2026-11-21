from dataclasses import dataclass
from enum import StrEnum


class HealthStatus(StrEnum):
    """Outcome of a health check."""

    UP = "up"
    DOWN = "down"


@dataclass(frozen=True)
class CheckResult:
    """Outcome of one named dependency check inside a report."""

    name: str
    status: HealthStatus


@dataclass(frozen=True)
class HealthReport:
    """Compact outcome of a probe."""

    status: HealthStatus
    checks: tuple[CheckResult, ...] = ()

    @property
    def healthy(self) -> bool:
        """Whether the overall status is up."""
        return self.status is HealthStatus.UP
