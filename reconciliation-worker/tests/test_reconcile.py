from __future__ import annotations

from datetime import UTC, datetime, timedelta

from worker.application.reconcile import ReconcileExpired
from worker.domain.expiry import PaymentIntentView
from worker.infrastructure.core_client import (
    CoreUnavailable,
    StatusEndpointMissing,
    TransitionRejected,
)

NOW = datetime(2026, 7, 31, 12, 0, 0, tzinfo=UTC)


def view(intent_id: str, status: str = "PENDING", expires_delta: int = -10) -> PaymentIntentView:
    return PaymentIntentView(
        id=intent_id,
        status=status,
        created_at=NOW - timedelta(minutes=40),
        expires_at=NOW + timedelta(minutes=expires_delta),
    )


class FakeCore:
    def __init__(self, by_status: dict[str, list[PaymentIntentView]], expire_side_effect=None):
        self._by_status = by_status
        self._expire_side_effect = expire_side_effect
        self.expired: list[str] = []

    def list_by_status(self, status: str, page: int = 1, limit: int = 100):
        return self._by_status.get(status, [])

    def expire(self, payment_intent_id: str, reason: str, correlation_id: str = "") -> str:
        if self._expire_side_effect is not None:
            raise self._expire_side_effect
        self.expired.append(payment_intent_id)
        return "expired"


def test_cierra_los_vencidos_de_los_dos_estados() -> None:
    core = FakeCore({"PENDING": [view("a")], "UNDER_REVIEW": [view("b", "UNDER_REVIEW")]})

    report = ReconcileExpired(core)(NOW)  # type: ignore[arg-type]

    assert report.found == 2
    assert report.expired == 2
    assert sorted(core.expired) == ["a", "b"]


def test_ignora_los_que_no_vencieron() -> None:
    core = FakeCore({"PENDING": [view("a", expires_delta=10)]})

    report = ReconcileExpired(core)(NOW)  # type: ignore[arg-type]

    assert report.found == 0
    assert core.expired == []


def test_si_el_core_no_responde_el_ciclo_se_pierde_entero() -> None:
    """No es grave: un vencido no empeora por esperar y el siguiente ciclo
    lo recupera."""

    class Caido(FakeCore):
        def list_by_status(self, status: str, page: int = 1, limit: int = 100):
            raise CoreUnavailable("503")

    report = ReconcileExpired(Caido({}))(NOW)  # type: ignore[arg-type]

    assert report.found == 0
    assert report.failed == 0


def test_una_transicion_rechazada_no_detiene_el_ciclo() -> None:
    core = FakeCore(
        {"PENDING": [view("a"), view("b")]},
        expire_side_effect=TransitionRejected("INVALID_TRANSITION"),
    )

    report = ReconcileExpired(core)(NOW)  # type: ignore[arg-type]

    assert report.rejected == 2


def test_el_endpoint_ausente_corta_el_ciclo_y_se_reporta() -> None:
    """Repetir el mismo 404 por cada intent solo llenaria los logs."""
    core = FakeCore(
        {"PENDING": [view("a"), view("b"), view("c")]},
        expire_side_effect=StatusEndpointMissing("falta PATCH /status"),
    )

    report = ReconcileExpired(core)(NOW)  # type: ignore[arg-type]

    assert report.endpoint_missing is True
    assert report.expired == 0


def test_el_override_permite_demostrarlo_sin_esperar_media_hora() -> None:
    core = FakeCore({"PENDING": [view("a", expires_delta=25)]})

    sin_override = ReconcileExpired(core)(NOW)  # type: ignore[arg-type]
    con_override = ReconcileExpired(core, override_minutes=1)(NOW)  # type: ignore[arg-type]

    assert sin_override.found == 0
    assert con_override.found == 1
