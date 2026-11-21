from fastapi.testclient import TestClient

from spray_planner.adapters.inbound.api.app import create_app
from spray_planner.application.lifecycle import Lifecycle
from spray_planner.application.use_cases.check_liveness import CheckLiveness
from spray_planner.application.use_cases.check_readiness import CheckReadiness
from spray_planner.application.use_cases.check_startup import CheckStartup
from tests.fakes.dependency_probe import FailingProbe, HealthyProbe


def build_client(probes: list[HealthyProbe | FailingProbe] | None = None) -> TestClient:
    lifecycle = Lifecycle()
    app = create_app(
        lifecycle=lifecycle,
        liveness=CheckLiveness(),
        startup=CheckStartup(lifecycle),
        readiness=CheckReadiness(lifecycle, probes or []),
    )
    return TestClient(app)


def test_liveness_is_up_even_before_lifespan_starts() -> None:
    response = build_client().get("/health/live")

    assert response.status_code == 200
    assert response.json() == {"status": "up"}
    assert response.headers["cache-control"] == "no-store"


def test_startup_and_readiness_are_down_before_lifespan_starts() -> None:
    client = build_client()

    for path in ("/health/startup", "/health/ready"):
        response = client.get(path)
        assert response.status_code == 503, path
        assert response.json() == {"status": "down"}, path
        assert response.headers["cache-control"] == "no-store", path


def test_all_probes_are_up_after_lifespan_starts() -> None:
    with build_client([HealthyProbe("api")]) as client:
        live = client.get("/health/live")
        startup = client.get("/health/startup")
        ready = client.get("/health/ready")

    assert (live.status_code, startup.status_code, ready.status_code) == (200, 200, 200)
    assert startup.json() == {"status": "up"}
    assert ready.json() == {"status": "up", "checks": [{"name": "api", "status": "up"}]}
    assert ready.headers["cache-control"] == "no-store"


def test_readiness_is_down_when_a_dependency_fails() -> None:
    with build_client([HealthyProbe("api"), FailingProbe("llm")]) as client:
        response = client.get("/health/ready")

    assert response.status_code == 503
    assert response.json() == {
        "status": "down",
        "checks": [{"name": "api", "status": "up"}, {"name": "llm", "status": "down"}],
    }
    assert response.headers["cache-control"] == "no-store"
    assert "unreachable" not in response.text


def test_readiness_is_down_after_shutdown() -> None:
    lifecycle = Lifecycle()
    app = create_app(
        lifecycle=lifecycle,
        liveness=CheckLiveness(),
        startup=CheckStartup(lifecycle),
        readiness=CheckReadiness(lifecycle, []),
    )
    with TestClient(app):
        pass

    response = TestClient(app).get("/health/ready")

    assert response.status_code == 503
    assert response.json() == {"status": "down"}
