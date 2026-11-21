import asyncio
import logging

import pytest

from spray_planner.application.dto.health import CheckResult, HealthReport
from spray_planner.application.lifecycle import Lifecycle
from spray_planner.application.use_cases.check_liveness import CheckLiveness
from spray_planner.application.use_cases.check_readiness import CheckReadiness
from spray_planner.application.use_cases.check_startup import CheckStartup
from tests.fakes.dependency_probe import FailingProbe, HealthyProbe


def started_lifecycle() -> Lifecycle:
    lifecycle = Lifecycle()
    lifecycle.mark_started()
    return lifecycle


def shutting_down_lifecycle() -> Lifecycle:
    lifecycle = started_lifecycle()
    lifecycle.begin_shutdown()
    return lifecycle


class TestCheckLiveness:
    def test_is_always_up(self) -> None:
        assert asyncio.run(CheckLiveness().execute()) == HealthReport(status="up")


class TestCheckStartup:
    def test_is_down_before_start(self) -> None:
        report = asyncio.run(CheckStartup(Lifecycle()).execute())

        assert report == HealthReport(status="down")

    def test_is_up_after_start(self) -> None:
        report = asyncio.run(CheckStartup(started_lifecycle()).execute())

        assert report == HealthReport(status="up")

    def test_stays_up_during_shutdown(self) -> None:
        report = asyncio.run(CheckStartup(shutting_down_lifecycle()).execute())

        assert report == HealthReport(status="up")


class TestCheckReadiness:
    def test_is_down_before_start_even_with_healthy_probes(self) -> None:
        use_case = CheckReadiness(Lifecycle(), [HealthyProbe("api")])

        assert asyncio.run(use_case.execute()) == HealthReport(status="down")

    def test_is_down_during_shutdown(self) -> None:
        use_case = CheckReadiness(shutting_down_lifecycle(), [HealthyProbe("api")])

        assert asyncio.run(use_case.execute()) == HealthReport(status="down")

    def test_is_up_after_start_without_probes(self) -> None:
        use_case = CheckReadiness(started_lifecycle(), [])

        assert asyncio.run(use_case.execute()) == HealthReport(status="up")

    def test_is_up_when_all_probes_pass(self) -> None:
        use_case = CheckReadiness(started_lifecycle(), [HealthyProbe("api"), HealthyProbe("llm")])

        assert asyncio.run(use_case.execute()) == HealthReport(
            status="up",
            checks=(CheckResult("api", "up"), CheckResult("llm", "up")),
        )

    def test_is_down_when_any_probe_fails(self) -> None:
        use_case = CheckReadiness(started_lifecycle(), [HealthyProbe("api"), FailingProbe("llm")])

        assert asyncio.run(use_case.execute()) == HealthReport(
            status="down",
            checks=(CheckResult("api", "up"), CheckResult("llm", "down")),
        )

    def test_logs_failure_cause_without_exposing_it(self, caplog: pytest.LogCaptureFixture) -> None:
        secret_cause = ConnectionError("password=hunter2 host=10.0.0.5")
        use_case = CheckReadiness(started_lifecycle(), [FailingProbe("api", secret_cause)])

        with caplog.at_level(logging.WARNING):
            report = asyncio.run(use_case.execute())

        assert "hunter2" not in repr(report)
        logged = [record for record in caplog.records if getattr(record, "probe", None) == "api"]
        assert len(logged) == 1
        assert "hunter2" in logged[0].error
