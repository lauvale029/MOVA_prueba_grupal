"""Configuracion desde entorno, validada al arrancar."""

from __future__ import annotations

from pydantic import Field, field_validator
from pydantic_settings import BaseSettings, SettingsConfigDict


class Settings(BaseSettings):
    model_config = SettingsConfigDict(env_file=".env", extra="ignore")

    core_api_url: str = Field(default="http://core-api:8080")
    core_username: str = Field(default="")
    core_password: str = Field(default="")

    # El reloj vive en reconciliation-scheduler: este proceso solo
    # reacciona al tick (ver ADR-0006).
    kafka_brokers: str = Field(default="kafka:29092")
    tick_topic: str = Field(default="reconciliation.tick")
    kafka_group_id: str = Field(default="reconciliation-worker")
    # Debe superar la duracion del ciclo mas largo, o Kafka expulsa al
    # consumidor a mitad del trabajo.
    max_poll_interval_ms: int = Field(default=600_000)

    # Solo para la demo: si se define, el vencimiento se calcula desde
    # created_at en vez de respetar el expires_at que puso el core. Sirve
    # para no esperar 30 minutos en una sustentacion.
    expiry_override_minutes: int | None = Field(default=None)

    @field_validator("expiry_override_minutes", mode="before")
    @classmethod
    def _vacio_es_none(cls, value: object) -> object:
        """Una variable declarada y vacia en .env llega como cadena vacia, no
        como ausente. Sin esto el proceso no arranca — y arrancar es
        justamente lo que se espera cuando la palanca de demo esta apagada."""
        if isinstance(value, str) and not value.strip():
            return None
        return value

    page_size: int = Field(default=100)
    request_timeout_seconds: float = Field(default=2.0)
    max_retries: int = Field(default=3)
    breaker_failure_threshold: int = Field(default=10)
    breaker_reset_seconds: float = Field(default=60.0)

    metrics_port: int = Field(default=8082)
    log_level: str = Field(default="INFO")

    @property
    def brokers(self) -> list[str]:
        return [b.strip() for b in self.kafka_brokers.split(",") if b.strip()]


settings = Settings()
