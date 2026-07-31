"""Esquemas del endpoint HTTP de evaluacion.

Este endpoint existe para probar y demostrar las reglas sin levantar
Kafka. El camino de produccion es el topic (ADR-0001).
"""

from __future__ import annotations

from pydantic import BaseModel, Field


class EvaluateRequest(BaseModel):
    payment_intent_id: str
    merchant_id: str
    external_reference: str = ""
    amount_minor: int = Field(gt=0)
    currency: str = "COP"
    channel: str = ""


class EvaluateResponse(BaseModel):
    payment_intent_id: str
    decision: str
    score: int
    reason_codes: list[str]
    model_version: str
