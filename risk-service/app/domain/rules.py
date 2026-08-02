"""Las reglas de riesgo.

Una funcion pura: mismas entradas, misma salida, sin IO ni reloj. Por eso
se prueba con una tabla de casos y no hace falta levantar nada.

Las reglas se evaluan en orden de severidad y la primera que dispara manda:
un comercio con referencia sospechosa se rechaza aunque el monto sea bajo.
"""

from __future__ import annotations

from app.domain.models import (
    Decision,
    ReasonCode,
    RiskEvaluation,
    RiskInput,
    RiskThresholds,
)
from app.domain.scoring import score_for

# Version de las reglas. Viaja en cada respuesta y core-api la persiste
# junto al intent: si manana cambian los umbrales, se puede saber con que
# version se decidio cada pago historico. Un cambio de umbral sube la
# version; un cambio incompatible del contrato seria un topic v2.
MODEL_VERSION = "rules-v1"


def _is_suspicious(external_reference: str, prefixes: tuple[str, ...]) -> bool:
    reference = external_reference.strip().upper()
    if not reference:
        return True
    return any(reference.startswith(prefix.upper()) for prefix in prefixes)


def evaluate(data: RiskInput, thresholds: RiskThresholds) -> RiskEvaluation:
    """Aplica las reglas y devuelve decision, score y motivos."""
    reason_codes: list[ReasonCode] = []
    decision = Decision.APPROVE

    # 1. Referencia sospechosa o vacia: bloqueante.
    if _is_suspicious(data.external_reference, thresholds.suspicious_reference_prefixes):
        reason_codes.append(ReasonCode.SUSPICIOUS_REFERENCE)
        decision = Decision.REJECT

    # 2. Velocidad anormal del comercio: bloqueante.
    elif data.recent_intents > thresholds.velocity_max_recent:
        reason_codes.append(ReasonCode.ABNORMAL_VELOCITY)
        decision = Decision.REJECT

    # 3. Monto alto: no se rechaza, se manda a revision humana.
    elif data.amount_minor > thresholds.review_amount_minor:
        reason_codes.append(ReasonCode.AMOUNT_ABOVE_REVIEW_THRESHOLD)
        decision = Decision.REVIEW

    # 4. Nada disparo.
    else:
        reason_codes.append(ReasonCode.LOW_RISK)

    return RiskEvaluation(
        payment_intent_id=data.payment_intent_id,
        decision=decision,
        score=score_for(reason_codes, data.amount_minor, thresholds.review_amount_minor),
        reason_codes=[code.value for code in reason_codes],
        model_version=MODEL_VERSION,
    )
