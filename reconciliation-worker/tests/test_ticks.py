from __future__ import annotations

import json
from datetime import UTC, datetime, timedelta

import pytest

from worker.ticks import MalformedTick, is_stale, parse_tick

NOW = datetime(2026, 7, 31, 16, 10, 0, tzinfo=UTC)

# Copiado del contrato que publica reconciliation-scheduler.
TICK = {
    "tick_id": "reconcile:1785000000",
    "fired_at": "2026-07-31T16:05:00Z",
    "interval_seconds": 300,
    "correlation_id": "44444444-4444-4444-4444-444444444444",
}


def test_parsea_el_tick_del_scheduler() -> None:
    tick = parse_tick(json.dumps(TICK).encode())

    assert tick.tick_id == TICK["tick_id"]
    assert tick.interval_seconds == 300
    assert tick.correlation_id == TICK["correlation_id"]


@pytest.mark.parametrize("faltante", ["tick_id", "fired_at", "interval_seconds"])
def test_falla_si_falta_un_campo_obligatorio(faltante: str) -> None:
    payload = {k: v for k, v in TICK.items() if k != faltante}
    with pytest.raises(MalformedTick):
        parse_tick(json.dumps(payload).encode())


def test_un_tick_reciente_no_esta_viejo() -> None:
    tick = parse_tick(json.dumps(TICK).encode())
    assert is_stale(tick, NOW) is False


def test_un_tick_de_hace_media_hora_se_descarta() -> None:
    """Un worker que vuelve tras estar caido encuentra los ticks
    acumulados: procesarlos todos repetiria el mismo ciclo N veces."""
    tick = parse_tick(json.dumps(TICK).encode())
    assert is_stale(tick, NOW + timedelta(minutes=30)) is True


def test_el_umbral_es_dos_intervalos() -> None:
    tick = parse_tick(json.dumps(TICK).encode())
    justo_dentro = tick.fired_at + timedelta(seconds=600)
    justo_fuera = tick.fired_at + timedelta(seconds=601)

    assert is_stale(tick, justo_dentro) is False
    assert is_stale(tick, justo_fuera) is True


def test_una_variable_vacia_no_impide_arrancar() -> None:
    """EXPIRY_OVERRIDE_MINUTES declarada y vacia en .env llega como cadena
    vacia. Sin el validador, el proceso no levanta."""
    import os

    from worker.config import Settings

    os.environ["EXPIRY_OVERRIDE_MINUTES"] = ""
    try:
        assert Settings().expiry_override_minutes is None
    finally:
        del os.environ["EXPIRY_OVERRIDE_MINUTES"]
