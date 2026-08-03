from __future__ import annotations

from app.infrastructure.velocity import SlidingWindowVelocity


class FakeClock:
    def __init__(self) -> None:
        self.now = 1000.0

    def __call__(self) -> float:
        return self.now

    def advance(self, seconds: float) -> None:
        self.now += seconds


def test_cuenta_los_intents_de_la_ventana() -> None:
    clock = FakeClock()
    velocity = SlidingWindowVelocity(window_seconds=60, clock=clock)

    for i in range(3):
        velocity.record("merchant-a", f"intent-{i}")

    assert velocity.count("merchant-a") == 3
    assert velocity.count("merchant-b") == 0


def test_olvida_lo_que_sale_de_la_ventana() -> None:
    clock = FakeClock()
    velocity = SlidingWindowVelocity(window_seconds=60, clock=clock)
    velocity.record("merchant-a", "intent-1")

    clock.advance(61)

    assert velocity.count("merchant-a") == 0


def test_una_reentrega_no_infla_el_contador() -> None:
    """Kafka entrega al menos una vez. Sin idempotencia por intent, un
    reproceso inventaria un rechazo por velocidad."""
    velocity = SlidingWindowVelocity(window_seconds=60, clock=FakeClock())

    velocity.record("merchant-a", "intent-1")
    velocity.record("merchant-a", "intent-1")
    velocity.record("merchant-a", "intent-1")

    assert velocity.count("merchant-a") == 1


def test_un_comercio_inactivo_deja_de_ocupar_memoria() -> None:
    clock = FakeClock()
    velocity = SlidingWindowVelocity(window_seconds=60, clock=clock)
    velocity.record("merchant-a", "intent-1")
    assert velocity.tracked_merchants() == 1

    clock.advance(61)
    velocity.count("merchant-a")

    assert velocity.tracked_merchants() == 0
