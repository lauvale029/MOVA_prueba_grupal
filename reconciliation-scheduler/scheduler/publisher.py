"""Publicacion del tick en Kafka."""

from __future__ import annotations

import json

import structlog
from aiokafka import AIOKafkaProducer

from scheduler import metrics
from scheduler.startup import connect_with_retry
from scheduler.tick import Tick

log: structlog.stdlib.BoundLogger = structlog.get_logger("reconciliation-scheduler")


class TickPublisher:
    def __init__(self, brokers: list[str], topic: str) -> None:
        self._brokers = brokers
        self._topic = topic
        self._producer: AIOKafkaProducer | None = None

    async def start(self) -> None:
        self._producer = AIOKafkaProducer(bootstrap_servers=self._brokers, acks="all")
        await connect_with_retry(self._producer.start, what="kafka-producer")
        log.info("kafka_conectado", topic=self._topic)

    async def stop(self) -> None:
        if self._producer is not None:
            await self._producer.stop()

    async def publish(self, tick: Tick) -> bool:
        """Devuelve si se publico.

        Un tick fallido NO se reintenta: el siguiente sale igual y cubre
        exactamente el mismo trabajo, porque el worker pregunta por el
        estado actual y no por el de entonces. Reintentar solo produciria
        dos disparos seguidos para lo mismo.
        """
        assert self._producer is not None, "start() antes de publish()"
        try:
            await self._producer.send_and_wait(
                self._topic,
                json.dumps(tick.to_message()).encode(),
                key=tick.tick_id.encode(),
            )
        except Exception as exc:
            metrics.publish_failures_total.labels(reason=type(exc).__name__).inc()
            log.warning("tick_no_publicado", tick_id=tick.tick_id, error=str(exc))
            return False

        metrics.ticks_total.inc()
        metrics.last_tick_timestamp.set_to_current_time()
        log.info("tick_publicado", tick_id=tick.tick_id, correlation_id=tick.correlation_id)
        return True
