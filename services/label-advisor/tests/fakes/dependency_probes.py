import asyncio


class HealthyProbe:
    """Fake dependency that always answers."""

    def __init__(self, name: str) -> None:
        self.name = name

    async def check(self) -> None:
        return None


class FailingProbe:
    """Fake dependency that always raises."""

    def __init__(self, name: str) -> None:
        self.name = name

    async def check(self) -> None:
        raise ConnectionError(f"{self.name} unreachable: secret-host:5432")


class HangingProbe:
    """Fake dependency that never answers within any reasonable timeout."""

    def __init__(self, name: str) -> None:
        self.name = name

    async def check(self) -> None:
        await asyncio.Event().wait()
