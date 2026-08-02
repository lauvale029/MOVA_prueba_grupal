"""El reloj.

Publica un tick cada `interval_seconds` y nada mas. No consulta la API, no
lee ninguna base y no toma ninguna decision — por eso es el unico proceso
que puede correr como replica unica sin que eso sea un riesgo: se puede
reiniciar, detener o desplegar en cualquier momento, y un ciclo perdido se
recupera en el siguiente.

Ver docs/adr/0006-scheduler-como-servicio-aparte.md
"""

from __future__ import annotations

import asyncio
import contextlib
import signal

import structlog
from prometheus_client import start_http_server

from scheduler import metrics
from scheduler.config import settings
from scheduler.logging import configure
from scheduler.publisher import TickPublisher
from scheduler.tick import build_tick

log: structlog.stdlib.BoundLogger = structlog.get_logger("reconciliation-scheduler")


async def run() -> None:
    publisher = TickPublisher(settings.brokers, settings.tick_topic)
    await publisher.start()

    stop = asyncio.Event()
    loop = asyncio.get_running_loop()
    for sig in (signal.SIGTERM, signal.SIGINT):
        loop.add_signal_handler(sig, stop.set)

    log.info("reloj_arrancado", interval=settings.interval_seconds, topic=settings.tick_topic)

    try:
        while not stop.is_set():
            await publisher.publish(build_tick(settings.interval_seconds))
            # Espera cancelable: SIGTERM no tiene que aguantar el
            # intervalo completo antes de que el contenedor pare.
            with contextlib.suppress(TimeoutError):
                await asyncio.wait_for(stop.wait(), timeout=settings.interval_seconds)
    finally:
        await publisher.stop()
        log.info("reloj_detenido")


def main() -> int:
    configure(settings.log_level)
    start_http_server(settings.metrics_port, registry=metrics.REGISTRY)
    asyncio.run(run())
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
