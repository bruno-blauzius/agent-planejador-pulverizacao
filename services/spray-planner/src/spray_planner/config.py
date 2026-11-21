"""Configuração do serviço lida de variáveis de ambiente (prefixo SPRAY_PLANNER_)."""

from pydantic_settings import BaseSettings, SettingsConfigDict


class Settings(BaseSettings):
    """Configuração externa, usada apenas pela raiz de composição."""

    model_config = SettingsConfigDict(env_prefix="SPRAY_PLANNER_", extra="ignore")

    log_level: str = "INFO"
    # URL da API Go (dona de talhões e aplicações); ainda não é consumida.
    api_base_url: str = "http://api:8080"
    port: int = 8000
