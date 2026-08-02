"""Configuracion desde entorno, validada al arrancar.

Si falta algo obligatorio el proceso no levanta, en vez de fallar en la
primera peticion.
"""

from __future__ import annotations

from pydantic import Field
from pydantic_settings import BaseSettings, SettingsConfigDict

from app.domain.models import RiskThresholds


class Settings(BaseSettings):
    model_config = SettingsConfigDict(env_file=".env", extra="ignore")

    # Kafka — los nombres de topic son el contrato con core-api (ADR-0001).
    kafka_brokers: str = Field(default="kafka:29092")
    kafka_requested_topic: str = Field(default="risk.evaluation.requested")
    kafka_completed_topic: str = Field(default="risk.evaluation.completed")
    kafka_group_id: str = Field(default="risk-service")
    kafka_enabled: bool = Field(default=True)

    # Umbrales de las reglas.
    review_amount_minor: int = Field(default=10_000_000)  # 100.000 COP
    velocity_max_recent: int = Field(default=10)
    velocity_window_seconds: int = Field(default=60)
    suspicious_reference_prefixes: str = Field(default="TEST-,FRAUD-")

    # Servidor.
    port: int = Field(default=8081)
    log_level: str = Field(default="INFO")

    @property
    def brokers(self) -> list[str]:
        return [b.strip() for b in self.kafka_brokers.split(",") if b.strip()]

    @property
    def thresholds(self) -> RiskThresholds:
        prefixes = tuple(
            p.strip() for p in self.suspicious_reference_prefixes.split(",") if p.strip()
        )
        return RiskThresholds(
            review_amount_minor=self.review_amount_minor,
            velocity_max_recent=self.velocity_max_recent,
            suspicious_reference_prefixes=prefixes,
        )


settings = Settings()
