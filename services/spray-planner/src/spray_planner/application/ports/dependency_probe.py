"""Porta para verificar a saúde de uma dependência externa."""

from typing import Protocol


class DependencyProbe(Protocol):
    """Verifica se uma dependência (API, banco, LLM...) está acessível.

    `check` não devolve nada quando a dependência está saudável e lança uma
    exceção quando ela falha; a causa é registrada em log pela readiness.
    """

    name: str

    async def check(self) -> None: ...
