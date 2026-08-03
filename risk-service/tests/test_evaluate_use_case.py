from __future__ import annotations

from app.application.evaluate import EvaluateRisk
from app.domain.models import Decision, RiskInput, RiskThresholds
from app.infrastructure.velocity import SlidingWindowVelocity

THRESHOLDS = RiskThresholds(
    review_amount_minor=10_000_000,
    velocity_max_recent=2,
    suspicious_reference_prefixes=("TEST-",),
)


def make_input(
    intent_id: str,
    merchant: str = "merchant-a",
    merchant_recent_intents: int | None = None,
) -> RiskInput:
    return RiskInput(
        payment_intent_id=intent_id,
        merchant_id=merchant,
        external_reference=f"ORDER-{intent_id}",
        amount_minor=150_000,
        currency="COP",
        channel="QR",
        merchant_recent_intents=merchant_recent_intents,
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


def test_manda_la_cuenta_de_core_api_sobre_la_propia() -> None:
    """core-api cuenta sobre su tabla: es exacta con varias replicas de
    este servicio y sobrevive a reinicios. Cuando viene, gana."""
    evaluate = EvaluateRisk(SlidingWindowVelocity(60), THRESHOLDS)

    # La ventana propia esta vacia y aun asi se rechaza por velocidad.
    assert evaluate(make_input("1", merchant_recent_intents=99)).decision is Decision.REJECT


def test_sin_el_dato_del_core_se_usa_la_ventana_propia() -> None:
    """El campo es opcional: un core que no lo mande no puede dejar la
    regla de velocidad sin efecto."""
    evaluate = EvaluateRisk(SlidingWindowVelocity(60), THRESHOLDS)
    for i in range(4):
        evaluate(make_input(str(i)))

    assert evaluate(make_input("ultimo")).decision is Decision.REJECT


def test_la_ventana_propia_se_sigue_alimentando_aunque_mande_el_core() -> None:
    """Si el campo dejara de llegar, el respaldo tiene que estar caliente,
    no arrancar de cero."""
    velocidad = SlidingWindowVelocity(60)
    evaluate = EvaluateRisk(velocidad, THRESHOLDS)

    for i in range(4):
        evaluate(make_input(str(i), merchant_recent_intents=0))

    assert velocidad.count("merchant-a") == 4
