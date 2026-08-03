"""El evento de disparo.

Es el contrato entre este servicio y el reconciliation-worker. Va aparte
del transporte a proposito: los dos lados lo importan y se prueba sin
Kafka.
"""

from __future__ import annotations

import uuid
from dataclasses import dataclass
from datetime import UTC, datetime
from typing import Any

TOPIC = "reconciliation.tick"


@dataclass(frozen=True, slots=True)
class Tick:
    tick_id: str
    fired_at: datetime
    interval_seconds: int
    correlation_id: str

    def to_message(self) -> dict[str, Any]:
        return {
            "tick_id": self.tick_id,
            "fired_at": self.fired_at.isoformat().replace("+00:00", "Z"),
            "interval_seconds": self.interval_seconds,
            "correlation_id": self.correlation_id,
        }


def tick_id_for(moment: datetime, interval_seconds: int) -> str:
    """Identificador determinista por ventana de tiempo.

    Dos schedulers vivos unos segundos durante un despliegue producen el
    MISMO id, y el worker descarta el duplicado. No sustituye a la regla
    de replica unica, pero evita que un solapamiento breve genere trabajo
    doble.
    """
    epoch = int(moment.timestamp())
    window_start = epoch - (epoch % max(interval_seconds, 1))
    return f"reconcile:{window_start}"


def build_tick(interval_seconds: int, now: datetime | None = None) -> Tick:
    moment = now or datetime.now(UTC)
    return Tick(
        tick_id=tick_id_for(moment, interval_seconds),
        fired_at=moment,
        # El worker sabe cuanto vale un tick sin leer su propia config:
        # asi decide que ticks acumulados ya no aportan nada.
        interval_seconds=interval_seconds,
        # Recorre el ciclo entero y acaba en el historial de cada pago
        # que se cierre. Permite pedir "todo lo que hizo el ciclo de las
        # 16:05" y obtener la lista completa.
        correlation_id=str(uuid.uuid4()),
    )
