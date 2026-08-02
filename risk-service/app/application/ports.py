"""Puertos: lo que la aplicacion necesita del mundo exterior."""

from __future__ import annotations

from typing import Protocol


class VelocityCounter(Protocol):
    """Cuenta intents recientes por comercio.

    Es un puerto y no una consulta directa a la base a proposito: el
    limite del reto es que Python no lea ni escriba las tablas del core
    (ver ADR-0004). La implementacion por defecto cuenta sobre el propio
    flujo de eventos que este servicio ya consume.
    """

    def count(self, merchant_id: str) -> int:
        """Cuantos intents recientes tiene el comercio, sin contar el actual."""
        ...

    def record(self, merchant_id: str, payment_intent_id: str) -> None:
        """Registra un intent. Idempotente por payment_intent_id: una
        reentrega de Kafka no debe inflar el contador."""
        ...
