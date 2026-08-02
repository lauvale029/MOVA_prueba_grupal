from __future__ import annotations

from app.domain.models import ReasonCode
from app.domain.scoring import BASE_POINTS, amount_points, score_for

THRESHOLD = 10_000_000


def test_el_score_discrimina_dentro_de_la_misma_decision() -> None:
    """Dos pagos aprobados no deberian tener el mismo numero si uno es
    diez veces mayor que el otro."""
    bajo = score_for([ReasonCode.LOW_RISK], 100_000, THRESHOLD)
    alto = score_for([ReasonCode.LOW_RISK], 9_000_000, THRESHOLD)
    assert bajo < alto


def test_el_componente_por_monto_tiene_techo() -> None:
    assert amount_points(10**15, THRESHOLD) == 20


def test_un_monto_minimo_deja_solo_la_base() -> None:
    assert score_for([ReasonCode.LOW_RISK], 1, THRESHOLD) == BASE_POINTS


def test_un_rechazo_satura_el_score() -> None:
    assert score_for([ReasonCode.SUSPICIOUS_REFERENCE], 50_000, THRESHOLD) == 100


def test_umbral_cero_no_divide_por_cero() -> None:
    assert amount_points(1_000, 0) == 0
