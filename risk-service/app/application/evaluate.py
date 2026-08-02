"""Caso de uso: evaluar un intent.

Orquesta puerto + dominio. No sabe de Kafka ni de HTTP: los dos
transportes lo invocan igual.
"""

from __future__ import annotations

from app.application.ports import VelocityCounter
from app.domain.models import RiskEvaluation, RiskInput, RiskThresholds
from app.domain.rules import evaluate as evaluate_rules


class EvaluateRisk:
    def __init__(self, velocity: VelocityCounter, thresholds: RiskThresholds) -> None:
        self._velocity = velocity
        self._thresholds = thresholds

    def __call__(self, data: RiskInput) -> RiskEvaluation:
        # Se consulta ANTES de registrar: el intent que se esta evaluando
        # no se cuenta a si mismo. Si no, el primer pago de un comercio ya
        # empezaria con velocidad 1.
        recent = self._velocity.count(data.merchant_id)
        self._velocity.record(data.merchant_id, data.payment_intent_id)

        enriched = RiskInput(
            payment_intent_id=data.payment_intent_id,
            merchant_id=data.merchant_id,
            external_reference=data.external_reference,
            amount_minor=data.amount_minor,
            currency=data.currency,
            channel=data.channel,
            recent_intents=recent,
        )
        return evaluate_rules(enriched, self._thresholds)
