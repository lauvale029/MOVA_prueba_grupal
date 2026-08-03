"""Las reglas son una funcion pura, asi que se prueban con una tabla de
casos: sin base, sin Kafka, sin fixtures. Es la ventaja concreta de
haberlas disenado sin estado."""

from __future__ import annotations

import pytest

from app.domain.models import Decision, ReasonCode, RiskInput, RiskThresholds
from app.domain.rules import MODEL_VERSION, evaluate

THRESHOLDS = RiskThresholds(
    review_amount_minor=10_000_000,
    velocity_max_recent=10,
    suspicious_reference_prefixes=("TEST-", "FRAUD-"),
)


def make_input(**overrides: object) -> RiskInput:
    base = {
        "payment_intent_id": "11111111-1111-1111-1111-111111111111",
        "merchant_id": "22222222-2222-2222-2222-222222222222",
        "external_reference": "ORDER-1001",
        "amount_minor": 150_000,
        "currency": "COP",
        "channel": "QR",
        "recent_intents": 0,
    }
    base.update(overrides)
    return RiskInput(**base)  # type: ignore[arg-type]


@pytest.mark.parametrize(
    ("overrides", "decision", "reason"),
    [
        ({}, Decision.APPROVE, ReasonCode.LOW_RISK),
        ({"amount_minor": 10_000_001}, Decision.REVIEW, ReasonCode.AMOUNT_ABOVE_REVIEW_THRESHOLD),
        ({"amount_minor": 10_000_000}, Decision.APPROVE, ReasonCode.LOW_RISK),
        ({"recent_intents": 11}, Decision.REJECT, ReasonCode.ABNORMAL_VELOCITY),
        ({"recent_intents": 10}, Decision.APPROVE, ReasonCode.LOW_RISK),
        ({"external_reference": "TEST-9"}, Decision.REJECT, ReasonCode.SUSPICIOUS_REFERENCE),
        ({"external_reference": "fraud-1"}, Decision.REJECT, ReasonCode.SUSPICIOUS_REFERENCE),
        ({"external_reference": "   "}, Decision.REJECT, ReasonCode.SUSPICIOUS_REFERENCE),
        ({"merchant_status": "INACTIVE"}, Decision.REJECT, ReasonCode.MERCHANT_BLOCKED),
        ({"merchant_status": "inactive"}, Decision.REJECT, ReasonCode.MERCHANT_BLOCKED),
        ({"merchant_status": "ACTIVE"}, Decision.APPROVE, ReasonCode.LOW_RISK),
    ],
)
def test_cada_regla_produce_su_decision(overrides, decision, reason) -> None:
    result = evaluate(make_input(**overrides), THRESHOLDS)
    assert result.decision is decision
    assert result.reason_codes == [reason.value]


def test_la_regla_mas_severa_gana() -> None:
    """Referencia sospechosa Y monto alto: manda el rechazo, no la revision."""
    result = evaluate(make_input(external_reference="FRAUD-1", amount_minor=99_000_000), THRESHOLDS)
    assert result.decision is Decision.REJECT
    assert result.reason_codes == [ReasonCode.SUSPICIOUS_REFERENCE.value]


def test_el_comercio_bloqueado_gana_a_todo_lo_demas() -> None:
    """Es lo mas categorico: si el comercio no puede operar, da igual que
    la referencia ademas sea sospechosa."""
    result = evaluate(
        make_input(merchant_status="INACTIVE", external_reference="FRAUD-1"),
        THRESHOLDS,
    )
    assert result.reason_codes == [ReasonCode.MERCHANT_BLOCKED.value]
    assert result.score == 100


@pytest.mark.parametrize("estado", ["", "   ", "SUSPENDIDO", "unknown"])
def test_no_se_rechaza_por_no_saber_el_estado(estado: str) -> None:
    """Un evento sin el campo, o con un estado que este servicio no
    conoce, no puede convertirse en un rechazo: seria tumbar todas las
    aprobaciones cada vez que los dos lados se desalinean."""
    result = evaluate(make_input(merchant_status=estado), THRESHOLDS)
    assert result.decision is Decision.APPROVE


def test_es_determinista() -> None:
    data = make_input(amount_minor=12_345_678)
    assert evaluate(data, THRESHOLDS) == evaluate(data, THRESHOLDS)


def test_lleva_la_version_del_modelo() -> None:
    """core-api la persiste: sin ella no se sabe con que reglas se decidio
    un pago historico."""
    assert evaluate(make_input(), THRESHOLDS).model_version == MODEL_VERSION


def test_el_score_siempre_esta_acotado() -> None:
    for amount in (1, 150_000, 10_000_001, 10**12):
        for recent in (0, 50):
            result = evaluate(make_input(amount_minor=amount, recent_intents=recent), THRESHOLDS)
            assert 0 <= result.score <= 100
