"""Caso de uso: un ciclo de conciliacion.

Lista los intents que pueden vencer, filtra los que ya vencieron y pide
su transicion. No decide nada sobre el estado: eso lo valida el core.
"""

from __future__ import annotations

from dataclasses import dataclass
from datetime import datetime

import structlog

from worker.domain.expiry import EXPIRABLE_STATUSES, PaymentIntentView, filter_expired
from worker.infrastructure import metrics
from worker.infrastructure.core_client import (
    CoreClient,
    CoreUnavailable,
    StatusEndpointMissing,
    TransitionRejected,
)

log: structlog.stdlib.BoundLogger = structlog.get_logger("reconciliation-worker")

EXPIRY_REASON = "vencido sin resolverse dentro de la ventana"


@dataclass(frozen=True, slots=True)
class CycleReport:
    """Lo que el ciclo hizo. Se imprime al terminar, que es lo que pide el
    anexo de Python: mostrar cuantos pagos se procesaron."""

    found: int = 0
    expired: int = 0
    already: int = 0
    rejected: int = 0
    failed: int = 0
    endpoint_missing: bool = False

    def __str__(self) -> str:
        return (
            f"encontrados={self.found} expirados={self.expired} "
            f"ya_estaban={self.already} rechazados={self.rejected} fallidos={self.failed}"
        )


class ReconcileExpired:
    def __init__(
        self,
        client: CoreClient,
        page_size: int = 100,
        override_minutes: int | None = None,
    ) -> None:
        self._client = client
        self._page_size = page_size
        self._override_minutes = override_minutes

    def __call__(self, now: datetime) -> CycleReport:
        with metrics.cycle_duration.time():
            return self._run(now)

    def _run(self, now: datetime) -> CycleReport:
        try:
            candidates = self._collect(now)
        except CoreUnavailable as exc:
            # El ciclo entero se pierde. No pasa nada: los vencidos no
            # empeoran por esperar y el siguiente disparo los recupera.
            metrics.cycle_failures_total.labels(reason="core_unavailable").inc()
            log.warning("ciclo_abortado", error=str(exc))
            return CycleReport()

        metrics.intents_found_total.inc(len(candidates))

        expired = already = rejected = failed = 0
        endpoint_missing = False

        for intent in candidates:
            try:
                outcome = self._client.expire(intent.id, EXPIRY_REASON, intent.correlation_id)
            except StatusEndpointMissing as exc:
                # Integracion pendiente, no una caida: se corta el ciclo
                # para no repetir el mismo error N veces.
                endpoint_missing = True
                metrics.cycle_failures_total.labels(reason="status_endpoint_missing").inc()
                log.error("integracion_pendiente", error=str(exc))
                break
            except TransitionRejected as exc:
                # Alguien lo movio entre el listado y el PATCH. Se registra
                # y se sigue: no es un error nuestro.
                rejected += 1
                metrics.intents_resolved_total.labels(outcome="rejected").inc()
                log.info("transicion_rechazada", payment_intent_id=intent.id, error=str(exc))
            except CoreUnavailable as exc:
                failed += 1
                metrics.intents_resolved_total.labels(outcome="failed").inc()
                log.warning("core_no_disponible", payment_intent_id=intent.id, error=str(exc))
            else:
                if outcome == "already_expired":
                    already += 1
                    metrics.intents_resolved_total.labels(outcome="already").inc()
                else:
                    expired += 1
                    metrics.intents_resolved_total.labels(outcome="expired").inc()
                log.info(
                    "intent_cerrado",
                    payment_intent_id=intent.id,
                    outcome=outcome,
                    correlation_id=intent.correlation_id,
                )

        metrics.last_cycle_timestamp.set_to_current_time()
        return CycleReport(
            found=len(candidates),
            expired=expired,
            already=already,
            rejected=rejected,
            failed=failed,
            endpoint_missing=endpoint_missing,
        )

    def _collect(self, now: datetime) -> list[PaymentIntentView]:
        """Pide al core solo los estados de los que se puede salir a
        EXPIRED. Consultar la API y no una proyeccion propia garantiza
        leer la fuente de verdad, no una copia unos milisegundos atrasada.
        """
        candidates: list[PaymentIntentView] = []
        for status in sorted(EXPIRABLE_STATUSES):
            intents = self._client.list_by_status(status, limit=self._page_size)
            candidates.extend(filter_expired(intents, now, self._override_minutes))
        return candidates
