"""Un solo ciclo y termina.

Es el comando que pide el anexo de Python: identifica los pagos vencidos,
los cierra y muestra cuantos proceso. Codigo de salida distinto de cero si
el ciclo no pudo completarse, para que sirva en un cron o en CI.
"""

from __future__ import annotations

import sys

from worker.application.reconcile import ReconcileExpired
from worker.config import settings
from worker.infrastructure.core_client import CoreClient, utcnow
from worker.infrastructure.logging import configure
from worker.infrastructure.resilience import CircuitBreaker


def build_use_case() -> ReconcileExpired:
    client = CoreClient(
        base_url=settings.core_api_url,
        username=settings.core_username,
        password=settings.core_password,
        timeout_seconds=settings.request_timeout_seconds,
        max_retries=settings.max_retries,
        breaker=CircuitBreaker(
            failure_threshold=settings.breaker_failure_threshold,
            reset_timeout_seconds=settings.breaker_reset_seconds,
        ),
    )
    return ReconcileExpired(
        client=client,
        page_size=settings.page_size,
        override_minutes=settings.expiry_override_minutes,
    )


def main() -> int:
    configure(settings.log_level)
    report = build_use_case()(utcnow())
    print(f"Conciliacion terminada: {report}")

    if report.endpoint_missing:
        print(
            "core-api no expone PATCH /api/v1/payment-intents/{id}/status — "
            "integracion pendiente, ver reconciliation-worker/README.md",
            file=sys.stderr,
        )
        return 2
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
