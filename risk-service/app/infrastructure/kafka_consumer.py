"""Consumidor de risk.evaluation.requested y productor de risk.evaluation.completed.

Los nombres y las formas de los mensajes son el contrato con core-api
(ADR-0001). Un cambio aqui rompe a Go, asi que va acompanado de un bump
de version del topic, nunca de una edicion en sitio.
"""

from __future__ import annotations

import asyncio
import json
from dataclasses import asdict
from typing import Any

import structlog
from aiokafka import AIOKafkaConsumer, AIOKafkaProducer

from app.application.evaluate import EvaluateRisk
from app.domain.models import RiskInput
from app.infrastructure import metrics
from app.infrastructure.logging import logger
from app.infrastructure.startup import connect_with_retry

log: structlog.stdlib.BoundLogger = logger()


class MalformedEvent(ValueError):
    """El mensaje no cumple el contrato. No se reintenta: se descarta."""


def _optional_int(value: Any) -> int | None:
    """Entero del evento, o None si no vino o no es un entero.

    Un valor basura no invalida el evento entero: se ignora ese campo y se
    decide con el respaldo. Rechazar el pago por un tipo mal puesto seria
    peor que la ausencia del dato.
    """
    if value is None:
        return None
    try:
        return int(value)
    except (TypeError, ValueError):
        return None


def parse_requested(raw: bytes) -> RiskInput:
    payload: dict[str, Any] = json.loads(raw)

    missing = [
        field
        for field in ("payment_intent_id", "merchant_id", "amount_minor")
        if not payload.get(field)
    ]
    if missing:
        raise MalformedEvent(f"faltan campos obligatorios: {', '.join(missing)}")

    # merchant_status y merchant_recent_intents son opcionales: los anadio
    # core-api despues del contrato original (ADR-0001). Si faltan, este
    # servicio decide como antes.
    return RiskInput(
        payment_intent_id=str(payload["payment_intent_id"]),
        merchant_id=str(payload["merchant_id"]),
        external_reference=str(payload.get("external_reference", "")),
        amount_minor=int(payload["amount_minor"]),
        currency=str(payload.get("currency", "COP")),
        channel=str(payload.get("channel", "")),
        merchant_status=str(payload.get("merchant_status", "")),
        merchant_recent_intents=_optional_int(payload.get("merchant_recent_intents")),
    )


class RiskEventWorker:
    """Consume, evalua y publica. Confirma el offset despues de publicar.

    Ese orden hace la entrega at-least-once: si el proceso muere entre
    publicar y confirmar, el evento se reprocesa y se vuelve a publicar el
    mismo resultado. Es inofensivo porque las reglas son puras y porque
    core-api aplica la decision de forma idempotente (ADR-0003).
    """

    def __init__(
        self,
        brokers: list[str],
        requested_topic: str,
        completed_topic: str,
        group_id: str,
        evaluate: EvaluateRisk,
    ) -> None:
        self._brokers = brokers
        self._requested_topic = requested_topic
        self._completed_topic = completed_topic
        self._group_id = group_id
        self._evaluate = evaluate
        self._consumer: AIOKafkaConsumer | None = None
        self._producer: AIOKafkaProducer | None = None

    async def start(self) -> None:
        self._consumer = AIOKafkaConsumer(
            self._requested_topic,
            bootstrap_servers=self._brokers,
            group_id=self._group_id,
            enable_auto_commit=False,
            auto_offset_reset="earliest",
        )
        self._producer = AIOKafkaProducer(bootstrap_servers=self._brokers, acks="all")
        # Kafka puede tardar en aceptar conexiones aunque el contenedor ya este
        # arriba. Se reintenta en vez de morir.
        await connect_with_retry(self._consumer.start, what="kafka-consumer")
        await connect_with_retry(self._producer.start, what="kafka-producer")
        metrics.consumer_up.set(1)
        log.info(
            "kafka_conectado",
            requested=self._requested_topic,
            completed=self._completed_topic,
        )

    async def stop(self) -> None:
        metrics.consumer_up.set(0)
        if self._consumer is not None:
            await self._consumer.stop()
        if self._producer is not None:
            await self._producer.stop()

    async def run(self) -> None:
        assert self._consumer is not None, "start() antes de run()"
        try:
            async for message in self._consumer:
                await self._handle(message.value)
                await self._consumer.commit()
        except asyncio.CancelledError:
            log.info("consumidor_detenido")
            raise

    async def _handle(self, raw: bytes) -> None:
        try:
            data = parse_requested(raw)
        except (MalformedEvent, json.JSONDecodeError, TypeError, ValueError) as exc:
            # Se descarta y se sigue: un mensaje invalido no puede bloquear
            # la particion para todos los demas. Queda contado y logueado.
            metrics.events_consumed_total.labels(outcome="malformed").inc()
            log.warning("evento_invalido", error=str(exc))
            return

        structlog.contextvars.bind_contextvars(payment_intent_id=data.payment_intent_id)
        try:
            with metrics.evaluation_duration.time():
                evaluation = self._evaluate(data)

            await self._publish(asdict(evaluation))
            metrics.events_consumed_total.labels(outcome="processed").inc()
            metrics.evaluations_total.labels(
                decision=evaluation.decision.value,
                reason_code=evaluation.reason_codes[0] if evaluation.reason_codes else "NONE",
            ).inc()
            log.info(
                "riesgo_evaluado",
                decision=evaluation.decision.value,
                score=evaluation.score,
                reason_codes=evaluation.reason_codes,
            )
        finally:
            structlog.contextvars.unbind_contextvars("payment_intent_id")

    async def _publish(self, evaluation: dict[str, Any]) -> None:
        """Serializa al contrato de risk.evaluation.completed (ADR-0001).

        Se construye campo a campo y no volcando el objeto: si el dominio
        gana un atributo interno, no se filtra al contrato sin querer.
        """
        assert self._producer is not None
        message = {
            "payment_intent_id": evaluation["payment_intent_id"],
            "decision": str(evaluation["decision"]),
            "score": evaluation["score"],
            "reason_codes": evaluation["reason_codes"],
            "model_version": evaluation["model_version"],
        }
        try:
            await self._producer.send_and_wait(
                self._completed_topic,
                json.dumps(message).encode(),
                # Misma clave que usa core-api: los mensajes de un intent
                # van a la misma particion y conservan el orden.
                key=message["payment_intent_id"].encode(),
            )
            metrics.events_published_total.labels(outcome="ok").inc()
        except Exception:
            metrics.events_published_total.labels(outcome="error").inc()
            raise
