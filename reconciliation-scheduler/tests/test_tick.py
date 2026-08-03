from __future__ import annotations

from datetime import UTC, datetime

from scheduler.tick import build_tick, tick_id_for


def test_el_id_es_el_mismo_dentro_de_la_ventana() -> None:
    """Dos schedulers vivos a la vez producen el mismo id y el worker
    descarta el duplicado."""
    base = datetime(2026, 7, 31, 16, 5, 0, tzinfo=UTC)
    otro = datetime(2026, 7, 31, 16, 9, 59, tzinfo=UTC)

    assert tick_id_for(base, 300) == tick_id_for(otro, 300)


def test_el_id_cambia_al_pasar_a_la_siguiente_ventana() -> None:
    dentro = datetime(2026, 7, 31, 16, 5, 0, tzinfo=UTC)
    fuera = datetime(2026, 7, 31, 16, 10, 1, tzinfo=UTC)

    assert tick_id_for(dentro, 300) != tick_id_for(fuera, 300)


def test_un_intervalo_cero_no_divide_por_cero() -> None:
    assert tick_id_for(datetime.now(UTC), 0)


def test_el_mensaje_lleva_lo_que_el_worker_necesita() -> None:
    message = build_tick(300).to_message()

    assert set(message) == {"tick_id", "fired_at", "interval_seconds", "correlation_id"}
    assert message["interval_seconds"] == 300
    assert message["fired_at"].endswith("Z")


def test_cada_tick_lleva_su_propia_correlacion() -> None:
    """El correlation_id acaba en el historial de cada pago que se cierre:
    permite pedir todo lo que hizo un ciclo concreto."""
    assert build_tick(300).correlation_id != build_tick(300).correlation_id
