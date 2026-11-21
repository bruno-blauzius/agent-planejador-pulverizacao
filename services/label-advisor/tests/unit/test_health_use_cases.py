import asyncio
import logging

import pytest

from label_advisor.application.dto.health import CheckResult, HealthReport, HealthStatus
from label_advisor.application.lifecycle import Lifecycle
from label_advisor.application.ports.dependency_probe import DependencyProbe
from label_advisor.application.use_cases import CheckLiveness, CheckReadiness, CheckStartup
from tests.fakes.dependency_probes import FailingProbe, HangingProbe, HealthyProbe

PROBE_TIMEOUT_SECONDS = 0.05


def started_lifecycle() -> Lifecycle:
    lifecycle = Lifecycle()
    lifecycle.mark_started()
    return lifecycle


def readiness(lifecycle: Lifecycle, *probes: DependencyProbe) -> HealthReport:
    use_case = CheckReadiness(lifecycle, probes, probe_timeout_seconds=PROBE_TIMEOUT_SECONDS)
    return asyncio.run(use_case.execute())


class TestLifecycle:
    def test_starts_neither_started_nor_shutting_down(self) -> None:
        lifecycle = Lifecycle()

        assert not lifecycle.started
        assert not lifecycle.shutting_down

    def test_transitions(self) -> None:
        lifecycle = Lifecycle()

        lifecycle.mark_started()
        lifecycle.begin_shutdown()

        assert lifecycle.started
        assert lifecycle.shutting_down


class TestCheckLiveness:
    def test_is_always_up_without_checks(self) -> None:
        assert CheckLiveness().execute() == HealthReport(status=HealthStatus.UP)


class TestCheckStartup:
    def test_is_down_before_start(self) -> None:
        report = CheckStartup(Lifecycle()).execute()

        assert report == HealthReport(status=HealthStatus.DOWN)

    def test_is_up_after_start(self) -> None:
        report = CheckStartup(started_lifecycle()).execute()

        assert report == HealthReport(status=HealthStatus.UP)

    def test_stays_up_during_shutdown(self) -> None:
        lifecycle = started_lifecycle()
        lifecycle.begin_shutdown()

        assert CheckStartup(lifecycle).execute().healthy


class TestCheckReadiness:
    def test_is_down_before_start_and_skips_probes(self) -> None:
        report = readiness(Lifecycle(), FailingProbe("database"))

        assert report == HealthReport(status=HealthStatus.DOWN)

    def test_is_down_during_shutdown(self) -> None:
        lifecycle = started_lifecycle()
        lifecycle.begin_shutdown()

        report = readiness(lifecycle, HealthyProbe("database"))

        assert report == HealthReport(status=HealthStatus.DOWN)

    def test_is_up_without_probes(self) -> None:
        assert readiness(started_lifecycle()) == HealthReport(status=HealthStatus.UP)

    def test_is_up_when_all_probes_pass(self) -> None:
        report = readiness(started_lifecycle(), HealthyProbe("database"), HealthyProbe("llm"))

        assert report == HealthReport(
            status=HealthStatus.UP,
            checks=(
                CheckResult("database", HealthStatus.UP),
                CheckResult("llm", HealthStatus.UP),
            ),
        )

    def test_is_down_when_any_probe_fails(self) -> None:
        report = readiness(started_lifecycle(), HealthyProbe("database"), FailingProbe("llm"))

        assert report == HealthReport(
            status=HealthStatus.DOWN,
            checks=(
                CheckResult("database", HealthStatus.UP),
                CheckResult("llm", HealthStatus.DOWN),
            ),
        )

    def test_is_down_when_a_probe_exceeds_the_timeout(self) -> None:
        report = readiness(started_lifecycle(), HangingProbe("database"))

        assert report == HealthReport(
            status=HealthStatus.DOWN,
            checks=(CheckResult("database", HealthStatus.DOWN),),
        )

    def test_logs_the_cause_but_does_not_return_it(self, caplog: pytest.LogCaptureFixture) -> None:
        with caplog.at_level(logging.WARNING):
            report = readiness(started_lifecycle(), FailingProbe("database"))

        assert "secret-host" not in repr(report)
        assert "database" in caplog.text
        assert "secret-host" in caplog.text
