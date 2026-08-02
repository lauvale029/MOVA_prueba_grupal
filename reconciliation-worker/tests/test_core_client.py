"""El cliente se prueba contra un core simulado con respx: sin levantar Go,
pero contra las rutas y los cuerpos reales del contrato."""

from __future__ import annotations

import httpx
import pytest
import respx

from worker.infrastructure.core_client import (
    CoreClient,
    CoreUnavailable,
    StatusEndpointMissing,
    TransitionRejected,
)
from worker.infrastructure.resilience import CircuitBreaker

BASE = "http://core-api:8080"

LOGIN = f"{BASE}/api/v1/auth/login"
LIST = f"{BASE}/api/v1/payment-intents"


def make_client(**kwargs: object) -> CoreClient:
    defaults = {
        "base_url": BASE,
        "username": "svc",
        "password": "secret",
        "max_retries": 2,
        "backoff_base": 0.0,
        "backoff_cap": 0.0,
    }
    defaults.update(kwargs)
    return CoreClient(**defaults)  # type: ignore[arg-type]


def intent_payload(intent_id: str = "abc", status: str = "PENDING") -> dict[str, object]:
    return {
        "id": intent_id,
        "status": status,
        "created_at": "2026-07-31T11:00:00Z",
        "expires_at": "2026-07-31T11:30:00Z",
        "correlation_id": "corr-1",
    }


@respx.mock
def test_se_autentica_y_lista() -> None:
    respx.post(LOGIN).mock(return_value=httpx.Response(200, json={"token": "t0k3n"}))
    route = respx.get(LIST).mock(
        return_value=httpx.Response(200, json={"data": [intent_payload()], "total": 1})
    )

    intents = make_client().list_by_status("PENDING")

    assert len(intents) == 1
    assert intents[0].id == "abc"
    assert route.calls.last.request.headers["Authorization"] == "Bearer t0k3n"


@respx.mock
def test_un_token_caducado_se_renueva_sin_fallar_el_ciclo() -> None:
    respx.post(LOGIN).mock(
        side_effect=[
            httpx.Response(200, json={"token": "viejo"}),
            httpx.Response(200, json={"token": "nuevo"}),
        ]
    )
    respx.get(LIST).mock(
        side_effect=[
            httpx.Response(401),
            httpx.Response(200, json={"data": [], "total": 0}),
        ]
    )

    assert make_client().list_by_status("PENDING") == []


@respx.mock
def test_reintenta_los_errores_de_infraestructura() -> None:
    respx.post(LOGIN).mock(return_value=httpx.Response(200, json={"token": "t"}))
    route = respx.get(LIST).mock(
        side_effect=[
            httpx.Response(503),
            httpx.Response(503),
            httpx.Response(200, json={"data": [], "total": 0}),
        ]
    )

    make_client().list_by_status("PENDING")

    assert route.call_count == 3


@respx.mock
def test_no_reintenta_indefinidamente() -> None:
    """max_retries=2 son 3 llamadas en total, no nueve: solo esta capa
    reintenta en toda la cadena."""
    respx.post(LOGIN).mock(return_value=httpx.Response(200, json={"token": "t"}))
    route = respx.get(LIST).mock(return_value=httpx.Response(503))

    with pytest.raises(CoreUnavailable):
        make_client().list_by_status("PENDING")

    assert route.call_count == 3


@respx.mock
def test_un_422_no_se_reintenta() -> None:
    """Es una respuesta correcta del dominio: daria 422 las tres veces."""
    respx.post(LOGIN).mock(return_value=httpx.Response(200, json={"token": "t"}))
    route = respx.patch(f"{BASE}/api/v1/payment-intents/abc/status").mock(
        return_value=httpx.Response(422, text="INVALID_TRANSITION")
    )

    with pytest.raises(TransitionRejected):
        make_client().expire("abc", "vencido")

    assert route.call_count == 1


@respx.mock
def test_un_409_cuenta_como_exito() -> None:
    """Otro proceso lo cerro entre el listado y el PATCH: el intent ya esta
    donde queriamos."""
    respx.post(LOGIN).mock(return_value=httpx.Response(200, json={"token": "t"}))
    respx.patch(f"{BASE}/api/v1/payment-intents/abc/status").mock(return_value=httpx.Response(409))

    assert make_client().expire("abc", "vencido") == "already_expired"


@respx.mock
def test_distingue_el_endpoint_que_aun_no_existe() -> None:
    """404 no es una caida: es la integracion pendiente con core-api."""
    respx.post(LOGIN).mock(return_value=httpx.Response(200, json={"token": "t"}))
    respx.patch(f"{BASE}/api/v1/payment-intents/abc/status").mock(return_value=httpx.Response(404))

    with pytest.raises(StatusEndpointMissing):
        make_client().expire("abc", "vencido")


@respx.mock
def test_el_breaker_abierto_falla_rapido() -> None:
    respx.post(LOGIN).mock(return_value=httpx.Response(200, json={"token": "t"}))
    route = respx.get(LIST).mock(return_value=httpx.Response(503))
    client = make_client(breaker=CircuitBreaker(failure_threshold=2, reset_timeout_seconds=999))

    with pytest.raises(CoreUnavailable):
        client.list_by_status("PENDING")
    llamadas_antes = route.call_count

    with pytest.raises(CoreUnavailable):
        client.list_by_status("PENDING")

    # Con el circuito abierto no se gasta ni una llamada mas.
    assert route.call_count == llamadas_antes


@respx.mock
def test_propaga_el_correlation_id_del_intent() -> None:
    """Sin esto se pierde el hilo que cruza Go y Python."""
    respx.post(LOGIN).mock(return_value=httpx.Response(200, json={"token": "t"}))
    route = respx.patch(f"{BASE}/api/v1/payment-intents/abc/status").mock(
        return_value=httpx.Response(200, json=intent_payload(status="EXPIRED"))
    )

    make_client().expire("abc", "vencido", correlation_id="corr-1")

    assert route.calls.last.request.headers["X-Correlation-Id"] == "corr-1"
