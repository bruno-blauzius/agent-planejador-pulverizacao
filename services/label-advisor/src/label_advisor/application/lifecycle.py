class Lifecycle:
    """Tracks the process phases that Kubernetes probes observe.

    Created once in the composition root and injected where needed. Plain
    attributes are enough: the service runs on a single event loop.
    """

    def __init__(self) -> None:
        self._started = False
        self._shutting_down = False

    @property
    def started(self) -> bool:
        """Whether initialization has finished."""
        return self._started

    @property
    def shutting_down(self) -> bool:
        """Whether the process is draining."""
        return self._shutting_down

    def mark_started(self) -> None:
        """Signal that initialization finished and the app can serve requests."""
        self._started = True

    def begin_shutdown(self) -> None:
        """Signal draining: readiness must fail so traffic stops arriving."""
        self._shutting_down = True
