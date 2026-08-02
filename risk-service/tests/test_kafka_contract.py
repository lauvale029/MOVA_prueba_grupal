"""El parseo del evento es el contrato con core-api. Si esto se rompe, se
rompe la integracion con Go, asi que se prueba contra el payload literal
del ADR-0001."""

from __future__ import annotations

import json

import pytest

from app.infrastructure.kafka_consumer import MalformedEvent, parse_requested

# Copiado tal cual de docs/adr/0001-contrato-go-python.md
EVENTO_DEL_ADR = {
    "payment_intent_id": "11111111-1111-1111-1111-111111111111",
    "merchant_id": "22222222-2222-2222-2222-222222222222",
    "external_reference": "ORDER-1001",
    "amount_minor": 15000000,
    "currency": "COP",
    "channel": "QR",
    "correlation_id": "33333333-3333-3333-3333-333333333333",
}


def test_parsea_el_payload_del_contrato() -> None:
    data = parse_requested(json.dumps(EVENTO_DEL_ADR).encode())

    assert data.payment_intent_id == EVENTO_DEL_ADR["payment_intent_id"]
    assert data.merchant_id == EVENTO_DEL_ADR["merchant_id"]
    assert data.amount_minor == 15_000_000
    assert data.channel == "QR"


def test_un_campo_extra_no_rompe_nada() -> None:
    """core-api puede anadir campos opcionales sin coordinarse con nosotros."""
    payload = {**EVENTO_DEL_ADR, "merchant_status": "ACTIVE"}
    assert parse_requested(json.dumps(payload).encode()).merchant_id


@pytest.mark.parametrize("faltante", ["payment_intent_id", "merchant_id", "amount_minor"])
def test_falla_si_falta_un_campo_obligatorio(faltante: str) -> None:
    payload = {k: v for k, v in EVENTO_DEL_ADR.items() if k != faltante}
    with pytest.raises(MalformedEvent):
        parse_requested(json.dumps(payload).encode())


def test_un_json_invalido_no_revienta_como_otra_cosa() -> None:
    with pytest.raises(json.JSONDecodeError):
        parse_requested(b"{no soy json")


def test_el_resultado_se_puede_serializar_al_contrato() -> None:
    """RiskEvaluation es un dataclass con slots=True, asi que NO tiene
    __dict__. Volcarlo con .__dict__ revienta en tiempo de ejecucion y las
    pruebas de reglas no lo notan, porque solo miran el objeto."""
    from dataclasses import asdict

    from app.domain.models import Decision, RiskEvaluation

    evaluacion = RiskEvaluation(
        payment_intent_id="11111111-1111-1111-1111-111111111111",
        decision=Decision.APPROVE,
        score=5,
        reason_codes=["LOW_RISK"],
        model_version="rules-v1",
    )

    with pytest.raises(AttributeError):
        evaluacion.__dict__  # noqa: B018

    datos = asdict(evaluacion)
    mensaje = {
        "payment_intent_id": datos["payment_intent_id"],
        "decision": str(datos["decision"]),
        "score": datos["score"],
        "reason_codes": datos["reason_codes"],
        "model_version": datos["model_version"],
    }

    # Las cinco claves que core-api espera, ni una mas ni una menos.
    assert set(mensaje) == {
        "payment_intent_id",
        "decision",
        "score",
        "reason_codes",
        "model_version",
    }
    assert json.dumps(mensaje)
    assert mensaje["decision"] == "APPROVE"
