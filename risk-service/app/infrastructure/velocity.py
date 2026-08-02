"""Contador de velocidad sobre ventana deslizante, en memoria.

Por que no se consulta la base del core: el limite del reto es que Python
no toque las tablas de pagos, y ademas hoy core-api persiste en memoria
(ver README, seccion Pendientes) — no existe una base que leer. Este
servicio ya ve TODOS los eventos de creacion, asi que puede contar por su
cuenta lo que necesita. Detalle y limitaciones en ADR-0004.
"""

from __future__ import annotations

import threading
import time
from collections.abc import Callable


class SlidingWindowVelocity:
    """Cuenta intents por comercio dentro de una ventana temporal.

    `record` es idempotente por payment_intent_id: una reentrega de Kafka
    no infla el contador, que seria un rechazo por velocidad inventado.
    """

    def __init__(self, window_seconds: int, clock: Callable[[], float] = time.monotonic) -> None:
        self._window = window_seconds
        self._clock = clock
        self._lock = threading.Lock()
        # merchant_id -> {payment_intent_id: instante en que se vio}
        self._seen: dict[str, dict[str, float]] = {}

    def count(self, merchant_id: str) -> int:
        with self._lock:
            self._prune(merchant_id)
            return len(self._seen.get(merchant_id, {}))

    def record(self, merchant_id: str, payment_intent_id: str) -> None:
        with self._lock:
            self._prune(merchant_id)
            self._seen.setdefault(merchant_id, {})[payment_intent_id] = self._clock()

    def tracked_merchants(self) -> int:
        with self._lock:
            return len(self._seen)

    def _prune(self, merchant_id: str) -> None:
        """Descarta lo que salio de la ventana. Se llama en cada acceso, asi
        que la memoria queda acotada por el trafico de la ventana, no por
        el historico."""
        entries = self._seen.get(merchant_id)
        if entries is None:
            return

        cutoff = self._clock() - self._window
        alive = {intent: seen_at for intent, seen_at in entries.items() if seen_at > cutoff}
        if alive:
            self._seen[merchant_id] = alive
        else:
            # Sin comercio vacio no hay fuga: un comercio que dejo de
            # operar deja de ocupar espacio.
            del self._seen[merchant_id]
