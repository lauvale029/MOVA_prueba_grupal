from __future__ import annotations

from app.application.evaluate import EvaluateRisk
from app.domain.models import Decision, RiskInput, RiskThresholds
from app.infrastructure.velocity import SlidingWindowVelocity

THRESHOLDS = RiskThresholds(
    review_amount_minor=10_000_000,
    velocity_max_recent=2,
    suspicious_reference_prefixes=("TEST-",),
)


def make_input(intent_id: str, merchant: str = "merchant-a") -> RiskInput:
    return RiskInput(
        payment_intent_id=intent_id,
        merchant_id=merchant,
        external_reference=f"ORDER-{intent_id}",
        amount_minor=150_000,
        currency="COP",
        channel="QR",
    )


def test_el_primer_pago_no_se_cuenta_a_si_mismo() -> None:
    """Si contara, el primer pago de un comercio ya arrancaria con
    velocidad 1 y el umbral se correria en uno."""
    evaluate = EvaluateRisk(SlidingWindowVelocity(60), THRESHOLDS)
    assert evaluate(make_input("1")).decision is Decision.APPROVE


def test_la_velocidad_se_acumula_hasta_disparar() -> None:
    evaluate = EvaluateRisk(SlidingWindowVelocity(60), THRESHOLDS)

    assert evaluate(make_input("1")).decision is Decision.APPROVE  # 0 previos
    assert evaluate(make_input("2")).decision is Decision.APPROVE  # 1 previo
    assert evaluate(make_input("3")).decision is Decision.APPROVE  # 2 previos, umbral
    assert evaluate(make_input("4")).decision is Decision.REJECT  # 3 previos


def test_la_velocidad_es_por_comercio() -> None:
    evaluate = EvaluateRisk(SlidingWindowVelocity(60), THRESHOLDS)
    for i in range(5):
        evaluate(make_input(str(i), merchant="ruidoso"))

    assert evaluate(make_input("x", merchant="tranquilo")).decision is Decision.APPROVE
