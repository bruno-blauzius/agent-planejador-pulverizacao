"""Fakes de DependencyProbe."""


class HealthyProbe:
    """Dependência sempre acessível."""

    def __init__(self, name: str) -> None:
        self.name = name

    async def check(self) -> None:
        return None


class FailingProbe:
    """Dependência sempre fora do ar."""

    def __init__(self, name: str, error: Exception | None = None) -> None:
        self.name = name
        self._error = error or ConnectionError(f"{name} unreachable")

    async def check(self) -> None:
        raise self._error
