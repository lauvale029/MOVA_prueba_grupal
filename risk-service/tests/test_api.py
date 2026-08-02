from __future__ import annotations

import os

os.environ["KAFKA_ENABLED"] = "false"

from fastapi.testclient import TestClient  # noqa: E402

from app.main import app  # noqa: E402

client = TestClient(app)


def test_health_no_toca_dependencias() -> None:
    assert client.get("/health").json() == {"status": "ok"}


def test_readiness_sin_kafka_esta_listo() -> None:
    assert client.get("/readiness").status_code == 200


def test_metrics_expone_formato_prometheus() -> None:
    response = client.get("/metrics")
    assert response.status_code == 200
    assert "risk_evaluations_total" in response.text


def test_evaluacion_sincrona_devuelve_el_contrato_completo() -> None:
    response = client.post(
        "/api/v1/risk-evaluations",
        json={
            "payment_intent_id": "11111111-1111-1111-1111-111111111111",
            "merchant_id": "22222222-2222-2222-2222-222222222222",
            "external_reference": "ORDER-1001",
            "amount_minor": 150000,
            "currency": "COP",
            "channel": "QR",
        },
    )

    assert response.status_code == 200
    body = response.json()
    assert set(body) == {
        "payment_intent_id",
        "decision",
        "score",
        "reason_codes",
        "model_version",
    }
    assert body["decision"] == "APPROVE"


def test_monto_invalido_se_rechaza_en_el_borde() -> None:
    response = client.post(
        "/api/v1/risk-evaluations",
        json={
            "payment_intent_id": "a",
            "merchant_id": "b",
            "amount_minor": 0,
        },
    )
    assert response.status_code == 422
