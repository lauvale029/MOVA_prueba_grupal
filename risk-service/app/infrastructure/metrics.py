"""Metricas Prometheus del flujo asincrono.

Ninguna etiqueta lleva merchant_id ni payment_intent_id: son de
cardinalidad no acotada y convertirian cada comercio en una serie nueva.
"""

from __future__ import annotations

from prometheus_client import CollectorRegistry, Counter, Gauge, Histogram

REGISTRY = CollectorRegistry()

evaluations_total = Counter(
    "risk_evaluations_total",
    "Evaluaciones de riesgo emitidas",
    ["decision", "reason_code"],
    registry=REGISTRY,
)

evaluation_duration = Histogram(
    "risk_evaluation_duration_seconds",
    "Tiempo de aplicar las reglas",
    registry=REGISTRY,
)

events_consumed_total = Counter(
    "risk_events_consumed_total",
    "Mensajes leidos de risk.evaluation.requested",
    ["outcome"],
    registry=REGISTRY,
)

events_published_total = Counter(
    "risk_events_published_total",
    "Mensajes escritos en risk.evaluation.completed",
    ["outcome"],
    registry=REGISTRY,
)

velocity_tracked_merchants = Gauge(
    "risk_velocity_tracked_merchants",
    "Comercios con actividad dentro de la ventana de velocidad",
    registry=REGISTRY,
)

# source="core" o "local". Si "local" deja de ser residual, core-api dejo
# de mandar merchant_recent_intents y estamos decidiendo con una cuenta
# por instancia sin que nada mas lo delate.
velocity_source_total = Counter(
    "risk_velocity_source_total",
    "De donde salio la cuenta de intents recientes usada para decidir",
    ["source"],
    registry=REGISTRY,
)

consumer_up = Gauge(
    "risk_consumer_up",
    "1 si el consumidor de Kafka esta conectado",
    registry=REGISTRY,
)
