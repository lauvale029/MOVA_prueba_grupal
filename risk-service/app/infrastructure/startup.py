"""Conexion inicial con reintentos.

Un servicio que muere porque una dependencia todavia no levanto es fragil:
el orden de arranque de Docker Compose ayuda, pero no cubre un broker que
tarda mas de lo previsto ni un corte transitorio al reiniciar.

Es la misma politica que el resto del sistema: se reintenta la
infraestructura, con backoff y jitter, y se falla despues de un tope.
"""

from __future__ import annotations

import asyncio
import random
from collections.abc import Awaitable, Callable

import structlog

log: structlog.stdlib.BoundLogger = structlog.get_logger()


async def connect_with_retry(
    connect: Callable[[], Awaitable[None]],
    *,
    what: str,
    attempts: int = 10,
    base_seconds: float = 1.0,
    cap_seconds: float = 15.0,
) -> None:
    for attempt in range(attempts):
        try:
            await connect()
            return
        except Exception as exc:
            if attempt == attempts - 1:
                log.error("conexion_agotada", dependencia=what, intentos=attempts)
                raise
            espera = random.uniform(0, min(cap_seconds, base_seconds * (2**attempt)))
            log.warning(
                "conexion_fallida_reintentando",
                dependencia=what,
                intento=attempt + 1,
                espera_s=round(espera, 2),
                error=str(exc),
            )
            await asyncio.sleep(espera)
