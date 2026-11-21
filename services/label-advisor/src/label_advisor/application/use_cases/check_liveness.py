from label_advisor.application.dto.health import HealthReport, HealthStatus


class CheckLiveness:
    """Answers "is the process alive?".

    Never inspects dependencies: restarting the pod would not fix a failing database.
    """

    def execute(self) -> HealthReport:
        """Always report up while the process can answer."""
        return HealthReport(status=HealthStatus.UP)
