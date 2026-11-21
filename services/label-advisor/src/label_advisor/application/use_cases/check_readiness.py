import asyncio
import logging
from collections.abc import Sequence

from label_advisor.application.dto.health import CheckResult, HealthReport, HealthStatus
from label_advisor.application.lifecycle import Lifecycle
from label_advisor.application.ports.dependency_probe import DependencyProbe

logger = logging.getLogger(__name__)


class CheckReadiness:
    """Answers "can this instance receive traffic right now?"."""

    def __init__(
        self,
        lifecycle: Lifecycle,
        probes: Sequence[DependencyProbe],
        probe_timeout_seconds: float,
    ) -> None:
        self._lifecycle = lifecycle
        self._probes = tuple(probes)
        self._probe_timeout_seconds = probe_timeout_seconds

    async def execute(self) -> HealthReport:
        """Report down while starting, while draining, or when any dependency fails."""
        if not self._lifecycle.started or self._lifecycle.shutting_down:
            return HealthReport(status=HealthStatus.DOWN)

        # Concurrent, so the probe answers within one timeout regardless of probe count.
        checks = tuple(await asyncio.gather(*(self._run(probe) for probe in self._probes)))
        healthy = all(check.status is HealthStatus.UP for check in checks)
        status = HealthStatus.UP if healthy else HealthStatus.DOWN
        return HealthReport(status=status, checks=checks)

    async def _run(self, probe: DependencyProbe) -> CheckResult:
        try:
            # Bounded so a hanging dependency cannot make the probe itself hang.
            async with asyncio.timeout(self._probe_timeout_seconds):
                await probe.check()
        except Exception:
            # Any failure means "not ready". The cause is logged, never returned:
            # probe bodies are reachable over the network.
            logger.warning("dependency check failed: %s", probe.name, exc_info=True)
            return CheckResult(name=probe.name, status=HealthStatus.DOWN)
        return CheckResult(name=probe.name, status=HealthStatus.UP)
