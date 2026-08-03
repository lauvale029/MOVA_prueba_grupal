"""Caso de uso: evaluar un intent.

Orquesta puerto + dominio. No sabe de Kafka ni de HTTP: los dos
transportes lo invocan igual.
"""

from __future__ import annotations

from app.application.ports import VelocityCounter
from app.domain.models import RiskEvaluation, RiskInput, RiskThresholds
from app.domain.rules import evaluate as evaluate_rules
from app.infrastructure import metrics


class EvaluateRisk:
    def __init__(self, velocity: VelocityCounter, thresholds: RiskThresholds) -> None:
        self._velocity = velocity
        self._thresholds = thresholds

    def __call__(self, data: RiskInput) -> RiskEvaluation:
        # Se consulta ANTES de registrar: el intent que se esta evaluando
        # no se cuenta a si mismo. Si no, el primer pago de un comercio ya
        # empezaria con velocidad 1.
        propia = self._velocity.count(data.merchant_id)
        self._velocity.record(data.merchant_id, data.payment_intent_id)

        recent, fuente = self._resolver_velocidad(data, propia)
        metrics.velocity_source_total.labels(source=fuente).inc()

        enriched = RiskInput(
            payment_intent_id=data.payment_intent_id,
            merchant_id=data.merchant_id,
            external_reference=data.external_reference,
            amount_minor=data.amount_minor,
            currency=data.currency,
            channel=data.channel,
            recent_intents=recent,
            merchant_status=data.merchant_status,
            merchant_recent_intents=data.merchant_recent_intents,
        )
        return evaluate_rules(enriched, self._thresholds)

    def _resolver_velocidad(self, data: RiskInput, propia: int) -> tuple[int, str]:
        """Manda la cuenta de core-api cuando viene; si no, la propia.

        La del core es exacta: la calcula sobre su tabla, con la misma
        semantica que la nuestra —no cuenta el intent actual— y sobrevive
        a reinicios y a varias replicas de este servicio. La ventana en
        memoria era un rodeo mientras ese dato no viajaba en el evento
        (ADR-0004), y se conserva como respaldo porque el campo es
        opcional: un core mas viejo no lo manda.

        Se sigue alimentando la ventana propia pase lo que pase, para que
        el respaldo este caliente si el campo deja de llegar.
        """
        if data.merchant_recent_intents is None:
            return propia, "local"
        return data.merchant_recent_intents, "core"
