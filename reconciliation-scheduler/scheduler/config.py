"""Configuracion.

El entorno de este proceso es deliberadamente minimo: no tiene URL de
ninguna base, ni credencial de la API, ni nada con lo que llamar a nadie.
Aunque alguien anadiera codigo por error, no tendria con que.
"""

from __future__ import annotations

from pydantic import Field
from pydantic_settings import BaseSettings, SettingsConfigDict


class Settings(BaseSettings):
    model_config = SettingsConfigDict(env_file=".env", extra="ignore")

    kafka_brokers: str = Field(default="kafka:29092")
    tick_topic: str = Field(default="reconciliation.tick")
    interval_seconds: int = Field(default=300, gt=0)

    metrics_port: int = Field(default=8083)
    log_level: str = Field(default="INFO")

    @property
    def brokers(self) -> list[str]:
        return [b.strip() for b in self.kafka_brokers.split(",") if b.strip()]


settings = Settings()
