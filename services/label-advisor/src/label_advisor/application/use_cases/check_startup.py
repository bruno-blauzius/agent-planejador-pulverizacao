from label_advisor.application.dto.health import HealthReport, HealthStatus
from label_advisor.application.lifecycle import Lifecycle


class CheckStartup:
    """Answers "has initialization finished?"."""

    def __init__(self, lifecycle: Lifecycle) -> None:
        self._lifecycle = lifecycle

    def execute(self) -> HealthReport:
        """Report up once the lifecycle was marked as started."""
        if self._lifecycle.started:
            return HealthReport(status=HealthStatus.UP)
        return HealthReport(status=HealthStatus.DOWN)
