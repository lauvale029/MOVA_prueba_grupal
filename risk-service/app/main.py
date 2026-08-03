"""Composicion y arranque.

Un solo dominio, dos transportes: el consumidor de Kafka (camino de
produccion) y un endpoint HTTP (pruebas y demo). Los dos invocan el mismo
caso de uso.
"""

from __future__ import annotations

import asyncio
from collections.abc import AsyncIterator
from contextlib import asynccontextmanager

from fastapi import FastAPI

from app.api.routes import build_router
from app.application.evaluate import EvaluateRisk
from app.config import settings
from app.infrastructure import metrics
from app.infrastructure.kafka_consumer import RiskEventWorker
from app.infrastructure.logging import configure, logger
from app.infrastructure.velocity import SlidingWindowVelocity

configure(settings.log_level)
log = logger()

velocity = SlidingWindowVelocity(window_seconds=settings.velocity_window_seconds)
evaluate = EvaluateRisk(velocity=velocity, thresholds=settings.thresholds)


@asynccontextmanager
async def lifespan(app: FastAPI) -> AsyncIterator[None]:
    task: asyncio.Task[None] | None = None
    worker: RiskEventWorker | None = None

    if settings.kafka_enabled:
        worker = RiskEventWorker(
            brokers=settings.brokers,
            requested_topic=settings.kafka_requested_topic,
            completed_topic=settings.kafka_completed_topic,
            group_id=settings.kafka_group_id,
            evaluate=evaluate,
        )
        await worker.start()
        task = asyncio.create_task(worker.run())
    else:
        log.warning("kafka_deshabilitado", motivo="solo el endpoint HTTP esta activo")

    gauge = asyncio.create_task(_refresh_velocity_gauge())

    yield

    gauge.cancel()
    if task is not None:
        task.cancel()
    if worker is not None:
        await worker.stop()


async def _refresh_velocity_gauge() -> None:
    """La metrica de comercios vigilados no vale para alertar: sirve para
    ver si la ventana esta reteniendo mas de lo esperado."""
    while True:
        metrics.velocity_tracked_merchants.set(velocity.tracked_merchants())
        await asyncio.sleep(15)


app = FastAPI(title="MOVA Risk Service", version="1.0.0", lifespan=lifespan)
app.include_router(build_router(evaluate, settings.kafka_enabled))
