"""Las reglas de riesgo.

Una funcion pura: mismas entradas, misma salida, sin IO ni reloj. Por eso
se prueba con una tabla de casos y no hace falta levantar nada.

Las reglas se evaluan en orden de severidad y la primera que dispara manda:
un comercio bloqueado se rechaza aunque todo lo demas este limpio, y una
referencia sospechosa se rechaza aunque el monto sea bajo.
"""

from __future__ import annotations

from app.domain.models import (
    Decision,
    MerchantStatus,
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
#
# v2 anade la regla de comercio bloqueado. Sube porque el mismo pago puede
# decidirse distinto que con v1: sin esto, un REJECT viejo y uno nuevo
# serian indistinguibles al auditar.
MODEL_VERSION = "rules-v2"


def _is_suspicious(external_reference: str, prefixes: tuple[str, ...]) -> bool:
    reference = external_reference.strip().upper()
    if not reference:
        return True
    return any(reference.startswith(prefix.upper()) for prefix in prefixes)


def _is_blocked(merchant_status: str) -> bool:
    """Solo INACTIVE bloquea.

    Un estado vacio —eventos de una version anterior del contrato— o uno
    que este servicio no conozca se tratan como "no se", y no se rechaza
    por no saber: rechazar por defecto convertiria cualquier despliegue
    desalineado en una caida de las aprobaciones.
    """
    return merchant_status.strip().upper() == MerchantStatus.INACTIVE


def evaluate(data: RiskInput, thresholds: RiskThresholds) -> RiskEvaluation:
    """Aplica las reglas y devuelve decision, score y motivos."""
    reason_codes: list[ReasonCode] = []
    decision = Decision.APPROVE

    # 1. Comercio bloqueado: lo mas categorico que hay. Si el comercio no
    #    puede operar, el resto de senales sobra.
    if _is_blocked(data.merchant_status):
        reason_codes.append(ReasonCode.MERCHANT_BLOCKED)
        decision = Decision.REJECT

    # 2. Referencia sospechosa o vacia: bloqueante.
    elif _is_suspicious(data.external_reference, thresholds.suspicious_reference_prefixes):
        reason_codes.append(ReasonCode.SUSPICIOUS_REFERENCE)
        decision = Decision.REJECT

    # 3. Velocidad anormal del comercio: bloqueante.
    elif data.recent_intents > thresholds.velocity_max_recent:
        reason_codes.append(ReasonCode.ABNORMAL_VELOCITY)
        decision = Decision.REJECT

    # 4. Monto alto: no se rechaza, se manda a revision humana.
    elif data.amount_minor > thresholds.review_amount_minor:
        reason_codes.append(ReasonCode.AMOUNT_ABOVE_REVIEW_THRESHOLD)
        decision = Decision.REVIEW

    # 5. Nada disparo.
    else:
        reason_codes.append(ReasonCode.LOW_RISK)

    return RiskEvaluation(
        payment_intent_id=data.payment_intent_id,
        decision=decision,
        score=score_for(reason_codes, data.amount_minor, thresholds.review_amount_minor),
        reason_codes=[code.value for code in reason_codes],
        model_version=MODEL_VERSION,
    )
