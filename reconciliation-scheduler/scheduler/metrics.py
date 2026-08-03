"""Metricas del reloj."""

from __future__ import annotations

from prometheus_client import CollectorRegistry, Counter, Gauge

REGISTRY = CollectorRegistry()

ticks_total = Counter(
    "reconciliation_scheduler_ticks_total",
    "Disparos publicados",
    registry=REGISTRY,
)

publish_failures_total = Counter(
    "reconciliation_scheduler_publish_failures_total",
    "Disparos que no se pudieron publicar",
    ["reason"],
    registry=REGISTRY,
)

last_tick_timestamp = Gauge(
    "reconciliation_scheduler_last_tick_timestamp",
    "Instante del ultimo disparo. Si deja de avanzar, nadie esta disparando "
    "la conciliacion y ninguna otra metrica lo diria",
    registry=REGISTRY,
)
