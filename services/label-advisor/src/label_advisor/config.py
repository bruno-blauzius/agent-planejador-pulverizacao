from pydantic import Field, SecretStr
from pydantic_settings import BaseSettings, SettingsConfigDict


class Settings(BaseSettings):
    """Service configuration read from LABEL_ADVISOR_* environment variables."""

    model_config = SettingsConfigDict(env_prefix="LABEL_ADVISOR_", frozen=True)

    log_level: str = "INFO"
    # Used from Phase 1 (pgvector). SecretStr keeps it out of reprs and logs.
    database_url: SecretStr | None = None
    port: int = Field(default=8000, ge=1, le=65535)
    probe_timeout_seconds: float = Field(default=2.0, gt=0)
