"""Lectura del tick que publica reconciliation-scheduler.

El contrato vive aqui, del lado del consumidor, y en scheduler/tick.py del
lado del productor. Los dos se prueban por separado y sin Kafka.
"""

from __future__ import annotations

import json
from dataclasses import dataclass
from datetime import UTC, datetime
from typing import Any

TOPIC = "reconciliation.tick"

# Un tick mas viejo que este multiplo del intervalo ya no aporta nada: el
# ultimo cubre exactamente el mismo trabajo, porque la consulta pregunta
# por el estado actual y no por el de entonces.
STALE_MULTIPLIER = 2


class MalformedTick(ValueError):
    """El mensaje no cumple el contrato. Se descarta, no se reintenta."""


@dataclass(frozen=True, slots=True)
class Tick:
    tick_id: str
    fired_at: datetime
    interval_seconds: int
    correlation_id: str = ""


def parse_tick(raw: bytes) -> Tick:
    payload: dict[str, Any] = json.loads(raw)

    missing = [f for f in ("tick_id", "fired_at", "interval_seconds") if not payload.get(f)]
    if missing:
        raise MalformedTick(f"faltan campos obligatorios: {', '.join(missing)}")

    fired_at = datetime.fromisoformat(str(payload["fired_at"]))
    return Tick(
        tick_id=str(payload["tick_id"]),
        fired_at=fired_at if fired_at.tzinfo else fired_at.replace(tzinfo=UTC),
        interval_seconds=int(payload["interval_seconds"]),
        correlation_id=str(payload.get("correlation_id", "")),
    )


def is_stale(tick: Tick, now: datetime) -> bool:
    """Un worker que vuelve tras estar caido encuentra los ticks
    acumulados. Procesarlos todos seria repetir el mismo ciclo N veces
    justo cuando el sistema acaba de recuperarse."""
    edad = (now - tick.fired_at).total_seconds()
    return edad > tick.interval_seconds * STALE_MULTIPLIER
