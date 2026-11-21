from typing import Protocol


class DependencyProbe(Protocol):
    """Verifies one external dependency (database, LLM provider...).

    Implementations live in adapters and are registered in the composition root.
    """

    name: str

    async def check(self) -> None:
        """Return normally when the dependency is healthy; raise otherwise."""
        ...
