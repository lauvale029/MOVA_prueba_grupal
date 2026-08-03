"""Cliente HTTP del core.

El worker NUNCA toca la base de datos: pide las transiciones por la API
publica, de modo que el cambio pasa por la maquina de estados, el
historial con actor y las mismas validaciones que cualquier otro cambio.
Es el limite no negociable del reto.
"""

from __future__ import annotations

import time
from datetime import datetime

import httpx
import structlog

from worker.domain.expiry import PaymentIntentView, parse_timestamp
from worker.infrastructure import metrics
from worker.infrastructure.resilience import (
    BreakerState,
    CircuitBreaker,
    CircuitOpen,
    backoff_with_jitter,
)

log: structlog.stdlib.BoundLogger = structlog.get_logger("reconciliation-worker")

# Errores que SI se reintentan: la peticion pudo no haber llegado.
RETRYABLE_STATUS = frozenset({429, 500, 502, 503, 504})

_BREAKER_GAUGE = {BreakerState.CLOSED: 0, BreakerState.HALF_OPEN: 1, BreakerState.OPEN: 2}


class CoreUnavailable(RuntimeError):
    """No se pudo hablar con el core tras agotar los reintentos."""


class TransitionRejected(RuntimeError):
    """El core rechazo la transicion por una razon de dominio. NO se
    reintenta: un 422 devuelve 422 las tres veces."""


class StatusEndpointMissing(RuntimeError):
    """El core todavia no expone el endpoint de cambio de estado.

    Se distingue de un fallo de red a proposito: no es algo que se
    resuelva reintentando, es una integracion pendiente (ver README).
    """


class CoreClient:
    def __init__(
        self,
        base_url: str,
        username: str,
        password: str,
        timeout_seconds: float = 2.0,
        max_retries: int = 3,
        backoff_base: float = 0.2,
        backoff_cap: float = 5.0,
        breaker: CircuitBreaker | None = None,
        client: httpx.Client | None = None,
    ) -> None:
        self._base_url = base_url.rstrip("/")
        self._username = username
        self._password = password
        self._max_retries = max_retries
        self._backoff_base = backoff_base
        self._backoff_cap = backoff_cap
        self._breaker = breaker or CircuitBreaker()
        self._client = client or httpx.Client(timeout=timeout_seconds)
        self._token: str | None = None

    # -- autenticacion --------------------------------------------------

    def _authenticate(self) -> str:
        response = self._client.post(
            f"{self._base_url}/api/v1/auth/login",
            json={"username": self._username, "password": self._password},
        )
        response.raise_for_status()
        token = str(response.json()["token"])
        self._token = token
        return token

    def _auth_headers(self) -> dict[str, str]:
        if self._token is None:
            self._authenticate()
        return {"Authorization": f"Bearer {self._token}"}

    # -- transporte con reintentos y breaker -----------------------------

    def _request(
        self,
        method: str,
        path: str,
        *,
        params: dict[str, str | int] | None = None,
        json: dict[str, str] | None = None,
        headers: dict[str, str] | None = None,
    ) -> httpx.Response:
        """Reintenta solo infraestructura, nunca dominio.

        El worker es el origen del trabajo, asi que es la capa que
        reintenta. Nadie mas en la cadena lo hace: si el core reintentara
        tambien, un fallo produciria nueve llamadas en vez de tres.
        """
        last_error: Exception | None = None
        extra_headers = headers or {}

        for attempt in range(self._max_retries + 1):
            try:
                self._breaker.before_call()
            except CircuitOpen as exc:
                metrics.circuit_breaker_state.set(_BREAKER_GAUGE[self._breaker.state])
                raise CoreUnavailable("circuito abierto") from exc

            try:
                # Los de autenticacion se resuelven en cada intento: si el
                # token caduco a mitad de los reintentos, el siguiente ya
                # lleva uno nuevo.
                response = self._client.request(
                    method,
                    f"{self._base_url}{path}",
                    params=params,
                    json=json,
                    headers={**self._auth_headers(), **extra_headers},
                )
            except httpx.HTTPError as exc:
                last_error = exc
                self._breaker.on_failure()
                metrics.retry_attempts_total.labels(outcome="network_error").inc()
            else:
                if response.status_code == 401:
                    # El token caduco: se renueva una vez y se reintenta.
                    self._token = None
                    metrics.retry_attempts_total.labels(outcome="reauth").inc()
                    last_error = CoreUnavailable("token rechazado")
                elif response.status_code in RETRYABLE_STATUS:
                    last_error = CoreUnavailable(f"HTTP {response.status_code}")
                    self._breaker.on_failure()
                    metrics.retry_attempts_total.labels(outcome="retryable_status").inc()
                else:
                    self._breaker.on_success()
                    metrics.circuit_breaker_state.set(_BREAKER_GAUGE[self._breaker.state])
                    return response

            metrics.circuit_breaker_state.set(_BREAKER_GAUGE[self._breaker.state])
            if attempt < self._max_retries:
                time.sleep(backoff_with_jitter(attempt, self._backoff_base, self._backoff_cap))

        metrics.retry_attempts_total.labels(outcome="exhausted").inc()
        raise CoreUnavailable(str(last_error))

    # -- operaciones ----------------------------------------------------

    def list_by_status(
        self, status: str, page: int = 1, limit: int = 100
    ) -> list[PaymentIntentView]:
        response = self._request(
            "GET",
            "/api/v1/payment-intents",
            params={"status": status, "page": page, "limit": limit},
        )
        response.raise_for_status()
        body = response.json()

        return [
            PaymentIntentView(
                id=item["id"],
                status=item["status"],
                created_at=parse_timestamp(item["created_at"]),
                expires_at=parse_timestamp(item["expires_at"]),
                correlation_id=item.get("correlation_id", ""),
            )
            for item in body.get("data", [])
        ]

    def expire(self, payment_intent_id: str, reason: str, correlation_id: str = "") -> str:
        """Solicita la transicion a EXPIRED.

        Depende de `PATCH /api/v1/payment-intents/{id}/status`, que core-api
        todavia no expone (ver README de este servicio). Cuando responde
        404/405 se levanta StatusEndpointMissing para que el ciclo lo
        reporte como integracion pendiente y no como una caida.
        """
        headers = {"X-Correlation-Id": correlation_id} if correlation_id else {}
        response = self._request(
            "PATCH",
            f"/api/v1/payment-intents/{payment_intent_id}/status",
            json={"status": "EXPIRED", "reason": reason},
            headers=headers,
        )

        if response.status_code in (404, 405):
            raise StatusEndpointMissing("core-api no expone PATCH /payment-intents/{id}/status")
        if response.status_code == 409:
            # Otro proceso lo cerro entre el listado y esta llamada.
            # Es exito: el intent ya esta donde queriamos.
            return "already_expired"
        if response.status_code == 422:
            raise TransitionRejected(response.text)

        response.raise_for_status()
        return "expired"


def utcnow() -> datetime:
    from datetime import UTC

    return datetime.now(UTC)
