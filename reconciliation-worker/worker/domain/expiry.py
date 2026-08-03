"""Que se considera vencido.

Funcion pura sobre fechas: se prueba con un reloj fijo, sin red ni API.
Esa es la unica regla de negocio del worker; todo lo demas es transporte.
"""

from __future__ import annotations

from dataclasses import dataclass
from datetime import UTC, datetime, timedelta

# Estados de los que un intent puede salir hacia EXPIRED, segun la tabla
# de transiciones del core (core-api/internal/domain/payment_intent.go).
# Pedir cualquier otra transicion daria 422 y seria ruido en las metricas.
EXPIRABLE_STATUSES = frozenset({"PENDING", "UNDER_REVIEW"})


@dataclass(frozen=True, slots=True)
class PaymentIntentView:
    """Lo que el worker necesita de un intent. Es un subconjunto de lo que
    devuelve la API: el worker no depende de campos que no usa."""

    id: str
    status: str
    created_at: datetime
    expires_at: datetime
    correlation_id: str = ""


def parse_timestamp(value: str) -> datetime:
    """La API emite RFC3339. `fromisoformat` acepta el sufijo Z desde 3.11."""
    parsed = datetime.fromisoformat(value)
    return parsed if parsed.tzinfo else parsed.replace(tzinfo=UTC)


def is_expired(
    intent: PaymentIntentView,
    now: datetime,
    override_minutes: int | None = None,
) -> bool:
    """Vencio si paso su `expires_at`.

    La ventana la define el core al crear el intent, no el worker: si los
    dos tuvieran su propia idea de cuando vence un pago, tarde o temprano
    discreparian. `override_minutes` existe solo para la demo, para no
    esperar media hora en una sustentacion.
    """
    if intent.status not in EXPIRABLE_STATUSES:
        return False

    if override_minutes is not None:
        return now - intent.created_at >= timedelta(minutes=override_minutes)

    return now >= intent.expires_at


def filter_expired(
    intents: list[PaymentIntentView],
    now: datetime,
    override_minutes: int | None = None,
) -> list[PaymentIntentView]:
    return [intent for intent in intents if is_expired(intent, now, override_minutes)]
