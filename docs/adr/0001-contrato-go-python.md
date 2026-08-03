# ADR-0001: Contrato entre `core-api` (Go) y el Risk Service (Python), vía Kafka

## Estado
Aceptado

## Contexto
`core-api` necesita una decisión de riesgo (`APPROVE`/`REVIEW`/`REJECT`)
antes de resolver un Payment Intent, pero esa evaluación la hace un
servicio Python aparte. Llamarlo de forma síncrona por HTTP bloquearía
el request del cliente mientras el Risk Service responde, y si el Risk
Service está lento o caído, el request quedaría colgado o fallaría por
completo.

## Decisión
La comunicación se hace de forma **asíncrona, vía Kafka**, con dos
tópicos:

- `risk.evaluation.requested` — `core-api` publica acá cada vez que un
  Payment Intent se envía a evaluar. Payload:
  ```json
  {
    "payment_intent_id": "uuid",
    "merchant_id": "uuid",
    "external_reference": "string",
    "amount_minor": 15000000,
    "currency": "COP",
    "channel": "QR",
    "correlation_id": "uuid",
    "merchant_status": "ACTIVE",
    "merchant_recent_intents": 3
  }
  ```
  Los dos últimos campos se añadieron después del contrato original y son
  **opcionales**: `core-api` es dueño de ambos datos y el Risk Service los
  usa si vienen. Si faltan, decide como antes — un estado ausente no se
  interpreta como bloqueado, y la cuenta se deriva del propio flujo de
  eventos ([ADR-0004](0004-velocidad-sin-acceso-a-la-base.md)).
- `risk.evaluation.completed` — el Risk Service publica acá el
  resultado. Payload:
  ```json
  {
    "payment_intent_id": "uuid",
    "decision": "APPROVE",
    "score": 12,
    "reason_codes": [],
    "model_version": "rules-v2"
  }
  ```
  `model_version` sube cuando cambian las reglas o sus umbrales, para que
  al auditar un pago histórico se sepa con qué versión se decidió. `v2`
  añadió la regla de comercio bloqueado. Un cambio **incompatible** de la
  forma del mensaje no subiría esto: sería un topic `v2`.

`core-api` mueve el Payment Intent de `PENDING` a `UNDER_REVIEW` de
forma **atómica junto con su entrada de historial, antes de publicar el
evento** — no al revés. Si el proceso se cayera entre publicar y guardar
el estado, quedaría un evento en Kafka sin que el intent refleje que ya
se envió, lo cual sería inconsistente con lo que el resto del sistema
puede observar.

Ambos tópicos se crean explícitamente al arrancar `core-api`
(`kafka.EnsureTopics`), en vez de depender de la auto-creación al primer
mensaje — evita una carrera real donde un consumer arranca contra un
tópico que todavía no existe (ver `internal/infrastructure/kafka/topics.go`).

## Alternativas consideradas
- **HTTP síncrono con timeout:** más simple de entender, pero bloquea el
  request del cliente mientras el Risk Service responde, y un timeout no
  resuelve el caso de "el Risk Service sí procesó pero la respuesta se
  perdió" — solo lo empeora, porque no hay dónde reintentar sin volver a
  cobrar/re-evaluar desde cero.
- **gRPC síncrono:** mismo problema de fondo que HTTP síncrono, cambia el
  protocolo pero no la naturaleza bloqueante de la llamada.

## Consecuencias
- El cliente recibe el Payment Intent en `UNDER_REVIEW` de inmediato — la
  resolución final (`APPROVED`/`REJECTED`) llega después, de forma
  asíncrona. Cualquier consumidor de la API debe consultar
  `GET /payment-intents/{id}` (o su historial) para ver el resultado
  final, no asumir que la respuesta del `POST` ya es definitiva.
- Si el Risk Service está caído, los eventos simplemente se acumulan en
  `risk.evaluation.requested` — Kafka los retiene, y se procesan cuando
  el servicio vuelve (ver ADR-0003 para el detalle de qué pasa mientras
  tanto).
- Falta un circuit breaker (Kafka→HTTP directo si Kafka mismo falla,
  no el Risk Service) — quedó fuera de esta iteración, documentado como
  pendiente en el README.
