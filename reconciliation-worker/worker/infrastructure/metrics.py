"""Metricas del worker. Sin etiquetas de cardinalidad no acotada."""

from __future__ import annotations

from prometheus_client import CollectorRegistry, Counter, Gauge, Histogram

REGISTRY = CollectorRegistry()

cycle_duration = Histogram(
    "reconciliation_cycle_duration_seconds",
    "Duracion de un ciclo completo",
    registry=REGISTRY,
)

intents_found_total = Counter(
    "reconciliation_intents_found_total",
    "Intents vencidos detectados",
    registry=REGISTRY,
)

intents_resolved_total = Counter(
    "reconciliation_intents_resolved_total",
    "Intents cerrados, por resultado",
    ["outcome"],
    registry=REGISTRY,
)

cycle_failures_total = Counter(
    "reconciliation_cycle_failures_total",
    "Ciclos que terminaron antes de tiempo",
    ["reason"],
    registry=REGISTRY,
)

retry_attempts_total = Counter(
    "reconciliation_retry_attempts_total",
    "Reintentos hacia el core",
    ["outcome"],
    registry=REGISTRY,
)

circuit_breaker_state = Gauge(
    "reconciliation_circuit_breaker_state",
    "0 cerrado, 1 semiabierto, 2 abierto",
    registry=REGISTRY,
)

ticks_discarded_total = Counter(
    "reconciliation_ticks_discarded_total",
    "Ticks descartados sin ejecutar un ciclo",
    ["reason"],
    registry=REGISTRY,
)

consumer_up = Gauge(
    "reconciliation_consumer_up",
    "1 si el consumidor del tick esta conectado a Kafka",
    registry=REGISTRY,
)

last_cycle_timestamp = Gauge(
    "reconciliation_last_cycle_timestamp",
    "Instante del ultimo ciclo completado. Si deja de avanzar, nadie concilia",
    registry=REGISTRY,
)
