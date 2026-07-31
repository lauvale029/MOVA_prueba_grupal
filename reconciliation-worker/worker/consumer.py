"""Consume el tick de Kafka y dispara un ciclo de conciliacion.

El reloj vive en reconciliation-scheduler, un proceso aparte: este solo
reacciona (ver docs/adr/0006-scheduler-como-servicio-aparte.md). Asi el
worker escala sin duplicar disparos y el reloj no tiene con que hacer dano.
"""

from __future__ import annotations

import asyncio
import json
import signal
from datetime import UTC, datetime

import structlog
from aiokafka import AIOKafkaConsumer
from prometheus_client import start_http_server

from worker.application.reconcile import ReconcileExpired
from worker.config import settings
from worker.infrastructure import metrics
from worker.infrastructure.logging import configure
from worker.infrastructure.startup import connect_with_retry
from worker.run_once import build_use_case
from worker.ticks import MalformedTick, Tick, is_stale, parse_tick

log: structlog.stdlib.BoundLogger = structlog.get_logger("reconciliation-worker")


class TickConsumer:
    def __init__(self, brokers: list[str], topic: str, group_id: str, reconcile: ReconcileExpired):
        self._brokers = brokers
        self._topic = topic
        self._group_id = group_id
        self._reconcile = reconcile
        self._consumer: AIOKafkaConsumer | None = None
        self._last_tick_id: str | None = None

    async def start(self) -> None:
        self._consumer = AIOKafkaConsumer(
            self._topic,
            bootstrap_servers=self._brokers,
            group_id=self._group_id,
            enable_auto_commit=False,
            auto_offset_reset="latest",
            # Un ciclo con miles de pagos tarda minutos. Con el valor por
            # defecto, Kafka expulsaria al consumidor a mitad del trabajo y
            # repartiria la particion a otra replica, que empezaria de cero:
            # un rebalanceo continuo sin que nada parezca estar fallando.
            max_poll_interval_ms=settings.max_poll_interval_ms,
        )
        await connect_with_retry(self._consumer.start, what="kafka-consumer")
        metrics.consumer_up.set(1)
        log.info("kafka_conectado", topic=self._topic, group=self._group_id)

    async def stop(self) -> None:
        metrics.consumer_up.set(0)
        if self._consumer is not None:
            await self._consumer.stop()

    async def run(self) -> None:
        assert self._consumer is not None, "start() antes de run()"
        async for message in self._consumer:
            await self._handle(message.value)
            await self._consumer.commit()

    async def _handle(self, raw: bytes) -> None:
        try:
            tick = parse_tick(raw)
        except (MalformedTick, json.JSONDecodeError, ValueError) as exc:
            metrics.ticks_discarded_total.labels(reason="malformed").inc()
            log.warning("tick_invalido", error=str(exc))
            return

        now = datetime.now(UTC)
        if is_stale(tick, now):
            metrics.ticks_discarded_total.labels(reason="stale").inc()
            log.info("tick_viejo_descartado", tick_id=tick.tick_id, fired_at=tick.fired_at)
            return

        if tick.tick_id == self._last_tick_id:
            # Dos schedulers durante un despliegue producen el mismo id.
            metrics.ticks_discarded_total.labels(reason="duplicate").inc()
            log.info("tick_duplicado_descartado", tick_id=tick.tick_id)
            return

        self._last_tick_id = tick.tick_id
        await self._run_cycle(tick)

    async def _run_cycle(self, tick: Tick) -> None:
        """El ciclo es sincrono (httpx.Client) y puede tardar minutos.

        Se lanza en un hilo para no bloquear el bucle de eventos, que es
        quien mantiene viva la sesion con Kafka. Bloquearlo provocaria
        justo el rebalanceo que max_poll_interval intenta evitar.
        """
        structlog.contextvars.bind_contextvars(correlation_id=tick.correlation_id)
        try:
            report = await asyncio.to_thread(self._reconcile, datetime.now(UTC))
            log.info(
                "ciclo_terminado",
                tick_id=tick.tick_id,
                found=report.found,
                expired=report.expired,
                already=report.already,
                rejected=report.rejected,
                failed=report.failed,
            )
        finally:
            structlog.contextvars.unbind_contextvars("correlation_id")


async def run() -> None:
    consumer = TickConsumer(
        brokers=settings.brokers,
        topic=settings.tick_topic,
        group_id=settings.kafka_group_id,
        reconcile=build_use_case(),
    )
    await consumer.start()

    stop = asyncio.Event()
    loop = asyncio.get_running_loop()
    for sig in (signal.SIGTERM, signal.SIGINT):
        loop.add_signal_handler(sig, stop.set)

    task = asyncio.create_task(consumer.run())
    await stop.wait()
    task.cancel()
    await consumer.stop()


def main() -> int:
    configure(settings.log_level)
    start_http_server(settings.metrics_port, registry=metrics.REGISTRY)
    log.info("worker_arrancado", metrics_port=settings.metrics_port)
    asyncio.run(run())
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
