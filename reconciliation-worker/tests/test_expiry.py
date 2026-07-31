"""La regla de vencimiento es una funcion sobre fechas: se prueba con un
reloj fijo, sin red."""

from __future__ import annotations

from datetime import UTC, datetime, timedelta

from worker.domain.expiry import PaymentIntentView, filter_expired, is_expired, parse_timestamp

NOW = datetime(2026, 7, 31, 12, 0, 0, tzinfo=UTC)


def intent(status: str = "PENDING", created_delta: int = -40, expires_delta: int = -10):
    return PaymentIntentView(
        id=f"intent-{status}-{expires_delta}",
        status=status,
        created_at=NOW + timedelta(minutes=created_delta),
        expires_at=NOW + timedelta(minutes=expires_delta),
    )


def test_vencio_si_paso_su_expires_at() -> None:
    assert is_expired(intent(expires_delta=-1), NOW) is True
    assert is_expired(intent(expires_delta=1), NOW) is False


def test_solo_vencen_los_estados_de_los_que_se_puede_salir() -> None:
    """Pedir EXPIRED sobre un estado terminal daria 422 y seria ruido."""
    assert is_expired(intent(status="PENDING"), NOW) is True
    assert is_expired(intent(status="UNDER_REVIEW"), NOW) is True
    for terminal in ("APPROVED", "REJECTED", "CANCELLED", "EXPIRED"):
        assert is_expired(intent(status=terminal), NOW) is False


def test_el_override_ignora_el_expires_at_del_core() -> None:
    """Palanca de demo: cierra por antiguedad para no esperar 30 minutos."""
    joven = intent(created_delta=-5, expires_delta=25)
    assert is_expired(joven, NOW) is False
    assert is_expired(joven, NOW, override_minutes=1) is True


def test_filtra_solo_los_vencidos() -> None:
    intents = [
        intent(expires_delta=-5),
        intent(expires_delta=5),
        intent(status="APPROVED", expires_delta=-5),
    ]
    assert len(filter_expired(intents, NOW)) == 1


def test_parsea_el_formato_que_emite_el_core() -> None:
    """core-api usa RFC3339: "2006-01-02T15:04:05Z07:00"."""
    assert parse_timestamp("2026-07-31T12:00:00Z").tzinfo is not None
    assert parse_timestamp("2026-07-31T12:00:00+00:00").hour == 12


def test_una_fecha_sin_zona_se_asume_utc() -> None:
    assert parse_timestamp("2026-07-31T12:00:00").tzinfo is UTC
