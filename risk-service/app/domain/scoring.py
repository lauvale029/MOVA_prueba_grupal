"""Calculo del score.

El score no es un modelo: es una suma acotada de puntos que se puede
explicar en voz alta. Si alguien pregunta "por que 80", la respuesta es
"5 de base, 20 por monto, 55 por superar el umbral de revision".
"""

from __future__ import annotations

from app.domain.models import ReasonCode

SCORE_MIN = 0
SCORE_MAX = 100

# Puntos base que lleva cualquier pago por el hecho de existir.
BASE_POINTS = 5

# Puntos por regla disparada.
RULE_POINTS: dict[ReasonCode, int] = {
    # Un comercio bloqueado no es una sospecha, es un hecho que reporta
    # core-api: satura el score y no deja lugar a interpretacion.
    ReasonCode.MERCHANT_BLOCKED: 100,
    ReasonCode.SUSPICIOUS_REFERENCE: 95,
    ReasonCode.ABNORMAL_VELOCITY: 95,
    ReasonCode.AMOUNT_ABOVE_REVIEW_THRESHOLD: 55,
    ReasonCode.LOW_RISK: 0,
}

# Techo del componente gradual por monto. Existe para que el score
# discrimine DENTRO de una misma decision: dos pagos aprobados de 1.000 y
# de 90.000 COP no deberian tener el mismo numero.
AMOUNT_POINTS_MAX = 20


def amount_points(amount_minor: int, review_threshold_minor: int) -> int:
    """Puntos graduales por monto, proporcionales al umbral de revision."""
    if review_threshold_minor <= 0:
        return 0
    ratio = amount_minor / review_threshold_minor
    return min(AMOUNT_POINTS_MAX, int(AMOUNT_POINTS_MAX * ratio))


def score_for(
    reason_codes: list[ReasonCode],
    amount_minor: int,
    review_threshold_minor: int,
) -> int:
    """Suma acotada a [0, 100]."""
    total = BASE_POINTS
    total += amount_points(amount_minor, review_threshold_minor)
    total += sum(RULE_POINTS.get(code, 0) for code in reason_codes)
    return max(SCORE_MIN, min(SCORE_MAX, total))
