"""Tipos del dominio de riesgo. Sin dependencias de framework, red ni reloj."""

from __future__ import annotations

from dataclasses import dataclass, field
from enum import StrEnum


class Decision(StrEnum):
    """Veredicto. Los valores son los que espera core-api (ver ADR-0001)."""

    APPROVE = "APPROVE"
    REVIEW = "REVIEW"
    REJECT = "REJECT"


class ReasonCode(StrEnum):
    """Por que se decidio lo que se decidio.

    El codigo es estable y forma parte del contrato: core-api lo persiste
    en el intent y la observabilidad lo usa como etiqueta. El texto que
    lo acompana en logs puede cambiar; el codigo no.
    """

    LOW_RISK = "LOW_RISK"
    AMOUNT_ABOVE_REVIEW_THRESHOLD = "AMOUNT_ABOVE_REVIEW_THRESHOLD"
    ABNORMAL_VELOCITY = "ABNORMAL_VELOCITY"
    SUSPICIOUS_REFERENCE = "SUSPICIOUS_REFERENCE"
    MERCHANT_BLOCKED = "MERCHANT_BLOCKED"


class MerchantStatus(StrEnum):
    """Estado del comercio, tal como lo publica core-api.

    Es un enum para nombrar el valor que importa, no para validar: un
    estado desconocido o ausente NO bloquea nada (ver rules.evaluate).
    """

    ACTIVE = "ACTIVE"
    INACTIVE = "INACTIVE"


@dataclass(frozen=True, slots=True)
class RiskInput:
    """Todo lo que hace falta para decidir.

    `recent_intents` es el numero que usan las reglas, y lo resuelve quien
    las invoca (ver application/evaluate.py). El dominio no sabe de donde
    sale, y por eso se puede probar con una tabla de casos.

    `merchant_recent_intents` es distinto: es lo que REPORTO core-api en el
    evento, o None si no vino. Existe para que la capa de aplicacion pueda
    preferirlo sobre su propia cuenta en memoria; las reglas no lo miran.

    `merchant_status` puede llegar vacio en eventos de una version anterior
    del contrato. Vacio significa "no se", no "bloqueado".
    """

    payment_intent_id: str
    merchant_id: str
    external_reference: str
    amount_minor: int
    currency: str
    channel: str
    recent_intents: int = 0
    merchant_status: str = ""
    merchant_recent_intents: int | None = None


@dataclass(frozen=True, slots=True)
class RiskThresholds:
    """Umbrales de las reglas. Se cargan de configuracion y se pasan al
    dominio como dato, para que las reglas sigan siendo una funcion pura.
    """

    review_amount_minor: int
    velocity_max_recent: int
    suspicious_reference_prefixes: tuple[str, ...]


@dataclass(frozen=True, slots=True)
class RiskEvaluation:
    """Resultado. Se serializa tal cual al topic risk.evaluation.completed."""

    payment_intent_id: str
    decision: Decision
    score: int
    reason_codes: list[str] = field(default_factory=list)
    model_version: str = ""
