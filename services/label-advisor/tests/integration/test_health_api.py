from collections.abc import Iterator

import pytest
from fastapi import FastAPI
from fastapi.testclient import TestClient

from label_advisor.adapters.inbound.api.app import create_app
from label_advisor.application.lifecycle import Lifecycle
from label_advisor.application.ports.dependency_probe import DependencyProbe
from label_advisor.application.use_cases import CheckLiveness, CheckReadiness, CheckStartup
from tests.fakes.dependency_probes import FailingProbe, HealthyProbe

PROBE_TIMEOUT_SECONDS = 0.05


def build_app(lifecycle: Lifecycle, *probes: DependencyProbe) -> FastAPI:
    return create_app(
        lifecycle=lifecycle,
        liveness=CheckLiveness(),
        startup=CheckStartup(lifecycle),
        readiness=CheckReadiness(lifecycle, probes, probe_timeout_seconds=PROBE_TIMEOUT_SECONDS),
    )


@pytest.fixture
def lifecycle() -> Lifecycle:
    return Lifecycle()


@pytest.fixture
def running_client(lifecycle: Lifecycle) -> Iterator[TestClient]:
    # Entering the context runs the lifespan, which marks the app as started.
    with TestClient(build_app(lifecycle)) as client:
        yield client


@pytest.mark.parametrize("path", ["/health/live", "/health/startup", "/health/ready"])
def test_probes_are_up_once_started(running_client: TestClient, path: str) -> None:
    response = running_client.get(path)

    assert response.status_code == 200
    assert response.json() == {"status": "up"}
    assert response.headers["cache-control"] == "no-store"


def test_startup_and_readiness_are_down_before_lifespan(lifecycle: Lifecycle) -> None:
    # Without the context manager the lifespan never runs: the app is "starting".
    client = TestClient(build_app(lifecycle))

    assert client.get("/health/live").status_code == 200
    for path in ("/health/startup", "/health/ready"):
        response = client.get(path)
        assert response.status_code == 503
        assert response.json() == {"status": "down"}
        assert response.headers["cache-control"] == "no-store"


def test_readiness_is_down_during_shutdown(lifecycle: Lifecycle) -> None:
    with TestClient(build_app(lifecycle)) as client:
        lifecycle.begin_shutdown()

        response = client.get("/health/ready")

    assert response.status_code == 503
    assert response.json() == {"status": "down"}


def test_lifespan_exit_begins_shutdown(lifecycle: Lifecycle) -> None:
    with TestClient(build_app(lifecycle)):
        assert lifecycle.started
        assert not lifecycle.shutting_down

    assert lifecycle.shutting_down


def test_readiness_lists_checks_without_leaking_causes(lifecycle: Lifecycle) -> None:
    app = build_app(lifecycle, HealthyProbe("database"), FailingProbe("llm"))
    with TestClient(app) as client:
        response = client.get("/health/ready")

    assert response.status_code == 503
    assert response.json() == {
        "status": "down",
        "checks": [
            {"name": "database", "status": "up"},
            {"name": "llm", "status": "down"},
        ],
    }
    assert "secret-host" not in response.text


def test_readiness_is_up_with_healthy_checks(lifecycle: Lifecycle) -> None:
    with TestClient(build_app(lifecycle, HealthyProbe("database"))) as client:
        response = client.get("/health/ready")

    assert response.status_code == 200
    assert response.json() == {"status": "up", "checks": [{"name": "database", "status": "up"}]}


def test_swagger_is_served(running_client: TestClient) -> None:
    assert running_client.get("/docs").status_code == 200
    assert running_client.get("/openapi.json").json()["info"]["title"] == "label-advisor"
