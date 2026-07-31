"""Circuit breaker y backoff con jitter.

Escrito a mano, unas 80 lineas, en vez de traer una dependencia: las
librerias de breaker en Python estan poco mantenidas y necesitamos que
emita nuestras propias metricas.
"""

from __future__ import annotations

import random
import time
from collections.abc import Callable
from enum import StrEnum


class BreakerState(StrEnum):
    CLOSED = "closed"
    OPEN = "open"
    HALF_OPEN = "half_open"


class CircuitOpen(RuntimeError):
    """El breaker esta abierto: se falla rapido sin gastar el timeout."""


class CircuitBreaker:
    """Uno por dependencia, nunca global.

    Su valor no es dejar de llamar: es fallar rapido hacia el estado
    seguro. Con la API caida y el breaker cerrado, cada intent gasta su
    timeout completo antes de rendirse; con el breaker abierto, el ciclo
    termina de inmediato y el siguiente disparo lo reintenta.
    """

    def __init__(
        self,
        failure_threshold: int = 10,
        reset_timeout_seconds: float = 60.0,
        clock: Callable[[], float] = time.monotonic,
    ) -> None:
        self._failure_threshold = failure_threshold
        self._reset_timeout = reset_timeout_seconds
        self._clock = clock
        self._failures = 0
        self._opened_at = 0.0
        self._state = BreakerState.CLOSED

    @property
    def state(self) -> BreakerState:
        if self._state is BreakerState.OPEN:
            expirado = self._clock() - self._opened_at >= self._reset_timeout
            if expirado:
                # Se deja pasar una sonda para ver si el otro lado volvio.
                self._state = BreakerState.HALF_OPEN
        return self._state

    def before_call(self) -> None:
        if self.state is BreakerState.OPEN:
            raise CircuitOpen("circuito abierto hacia el core")

    def on_success(self) -> None:
        self._failures = 0
        self._state = BreakerState.CLOSED

    def on_failure(self) -> None:
        if self.state is BreakerState.HALF_OPEN:
            # La sonda fallo: se vuelve a abrir sin esperar al umbral.
            self._trip()
            return

        self._failures += 1
        if self._failures >= self._failure_threshold:
            self._trip()

    def _trip(self) -> None:
        self._state = BreakerState.OPEN
        self._opened_at = self._clock()
        self._failures = 0


def backoff_with_jitter(attempt: int, base_seconds: float, cap_seconds: float) -> float:
    """Jitter completo: random(0, min(tope, base * 2^intento)).

    El componente aleatorio evita que todos los clientes reintenten
    sincronizados y tumben el servicio justo cuando se esta recuperando.
    """
    ceiling = min(cap_seconds, base_seconds * (2**attempt))
    return random.uniform(0, ceiling)
