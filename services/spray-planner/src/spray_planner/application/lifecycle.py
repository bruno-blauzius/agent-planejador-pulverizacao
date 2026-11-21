"""Estado do ciclo de vida do processo, consultado pelas sondas de saúde."""


class Lifecycle:
    """Registra se o serviço terminou de iniciar e se entrou em shutdown.

    É mutável de propósito: a borda (lifespan do servidor) avisa as transições
    e os casos de uso de saúde apenas leem o estado.
    """

    def __init__(self) -> None:
        self._started = False
        self._shutting_down = False

    @property
    def started(self) -> bool:
        """Indica se a inicialização terminou."""
        return self._started

    @property
    def shutting_down(self) -> bool:
        """Indica se o serviço está drenando tráfego para encerrar."""
        return self._shutting_down

    def mark_started(self) -> None:
        """Marca a inicialização como concluída."""
        self._started = True

    def begin_shutdown(self) -> None:
        """Marca o início do shutdown, para a readiness tirar o pod do tráfego."""
        self._shutting_down = True
