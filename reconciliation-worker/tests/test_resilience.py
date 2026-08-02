from __future__ import annotations

import pytest

from worker.infrastructure.resilience import (
    BreakerState,
    CircuitBreaker,
    CircuitOpen,
    backoff_with_jitter,
)


class FakeClock:
    def __init__(self) -> None:
        self.now = 0.0

    def __call__(self) -> float:
        return self.now

    def advance(self, seconds: float) -> None:
        self.now += seconds


def test_abre_al_llegar_al_umbral() -> None:
    breaker = CircuitBreaker(failure_threshold=3, clock=FakeClock())
    for _ in range(2):
        breaker.on_failure()
    assert breaker.state is BreakerState.CLOSED

    breaker.on_failure()
    assert breaker.state is BreakerState.OPEN
    with pytest.raises(CircuitOpen):
        breaker.before_call()


def test_pasa_a_semiabierto_tras_el_tiempo_de_espera() -> None:
    clock = FakeClock()
    breaker = CircuitBreaker(failure_threshold=1, reset_timeout_seconds=30, clock=clock)
    breaker.on_failure()

    clock.advance(30)

    assert breaker.state is BreakerState.HALF_OPEN
    breaker.before_call()  # deja pasar la sonda


def test_la_sonda_fallida_vuelve_a_abrir_sin_esperar_al_umbral() -> None:
    clock = FakeClock()
    breaker = CircuitBreaker(failure_threshold=10, reset_timeout_seconds=30, clock=clock)
    for _ in range(10):
        breaker.on_failure()
    clock.advance(30)
    assert breaker.state is BreakerState.HALF_OPEN

    breaker.on_failure()

    assert breaker.state is BreakerState.OPEN


def test_un_exito_cierra_y_reinicia_el_contador() -> None:
    breaker = CircuitBreaker(failure_threshold=3, clock=FakeClock())
    breaker.on_failure()
    breaker.on_failure()
    breaker.on_success()
    breaker.on_failure()

    assert breaker.state is BreakerState.CLOSED


def test_el_backoff_respeta_el_tope() -> None:
    for attempt in range(10):
        assert 0 <= backoff_with_jitter(attempt, base_seconds=0.2, cap_seconds=5.0) <= 5.0


def test_el_jitter_no_devuelve_siempre_lo_mismo() -> None:
    """Sin jitter, todos los clientes reintentan a la vez y tumban el
    servicio justo cuando se esta recuperando."""
    esperas = {backoff_with_jitter(3, 0.2, 5.0) for _ in range(50)}
    assert len(esperas) > 1
