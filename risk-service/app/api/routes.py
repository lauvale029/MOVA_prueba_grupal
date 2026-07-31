"""Rutas HTTP: salud, metricas y evaluacion sincrona para pruebas."""

from __future__ import annotations

from fastapi import APIRouter, Response
from prometheus_client import CONTENT_TYPE_LATEST, generate_latest

from app.api.schemas import EvaluateRequest, EvaluateResponse
from app.application.evaluate import EvaluateRisk
from app.domain.models import RiskInput
from app.infrastructure import metrics

router = APIRouter()


def build_router(evaluate: EvaluateRisk, kafka_enabled: bool) -> APIRouter:
    @router.get("/health")
    def health() -> dict[str, str]:
        """Vive el proceso. No toca dependencias a proposito."""
        return {"status": "ok"}

    @router.get("/readiness")
    def readiness() -> Response:
        """Listo para trabajar. Con Kafka activo exige estar conectado: un
        consumidor arriba pero sin broker no sirve para nada."""
        ready = (not kafka_enabled) or metrics.consumer_up._value.get() == 1
        return Response(
            content='{"status":"ready"}' if ready else '{"status":"not_ready"}',
            status_code=200 if ready else 503,
            media_type="application/json",
        )

    @router.get("/metrics")
    def prometheus_metrics() -> Response:
        return Response(generate_latest(metrics.REGISTRY), media_type=CONTENT_TYPE_LATEST)

    @router.post("/api/v1/risk-evaluations", response_model=EvaluateResponse)
    def evaluate_now(request: EvaluateRequest) -> EvaluateResponse:
        result = evaluate(
            RiskInput(
                payment_intent_id=request.payment_intent_id,
                merchant_id=request.merchant_id,
                external_reference=request.external_reference,
                amount_minor=request.amount_minor,
                currency=request.currency,
                channel=request.channel,
            )
        )
        return EvaluateResponse(
            payment_intent_id=result.payment_intent_id,
            decision=result.decision.value,
            score=result.score,
            reason_codes=result.reason_codes,
            model_version=result.model_version,
        )

    return router
