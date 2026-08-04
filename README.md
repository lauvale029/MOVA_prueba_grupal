# MOVA Payment Orchestrator

Vertical de procesamiento de pagos para comercios: Payment Intents con
evaluación de riesgo asíncrona, trazabilidad completa, e idempotencia
respaldada por base de datos. Trabajo en equipo — ver
[Matriz de contribuciones](#matriz-de-contribuciones) para quién hizo qué.

## Arquitectura

```mermaid
flowchart TB
    cliente["Cliente / Comercio<br/>Postman o integracion"]

    subgraph app["APLICACION"]
        direction TB
        core["<b>core-api</b> · Go<br/>intents · estados · idempotencia<br/>UNICO que escribe en la base"]
        risk["<b>risk-service</b> · Python<br/>reglas deterministicas<br/>SIN estado"]
        beat["<b>reconciliation-scheduler</b> · Python<br/>el reloj · replica UNICA"]
        worker["<b>reconciliation-worker</b> · Python<br/>expira vencidos via API"]
    end

    subgraph bus["KAFKA"]
        direction TB
        t1[["risk.evaluation.requested"]]
        t2[["risk.evaluation.completed"]]
        t3[["reconciliation.tick"]]
    end

    subgraph datos["DATOS"]
        direction TB
        mig["<b>db-migrator</b><br/>SQL plano · corre y termina"]
        pg[("PostgreSQL<br/>mova_orchestrator<br/>esquema payments")]
        redis[("Redis<br/>lock best-effort")]
    end

    subgraph obs["OBSERVABILIDAD"]
        direction TB
        prom["Prometheus"]
        graf["Grafana"]
        kexp["kafka-exporter<br/>consumer lag"]
    end

    cliente -->|"HTTPS · JWT · Idempotency-Key"| core

    core -->|"publica"| t1
    t1 --> risk
    risk -->|"publica"| t2
    t2 --> core

    beat -->|"cada 5 min"| t3
    t3 --> worker
    worker -->|"GET intents abiertos"| core
    worker -->|"PATCH /status"| core

    mig -->|"DDL, una vez"| pg
    core -->|"SQL"| pg
    core -.->|"lock best-effort"| redis

    core -.->|"/metrics"| prom
    risk -.-> prom
    beat -.-> prom
    worker -.-> prom
    kexp --> prom
    prom --> graf

    style core stroke-width:3px
    style pg stroke-width:3px
```

| | Significa |
|---|---|
| Línea sólida | Camino en funcionamiento |
| Línea punteada | Opcional, degradable o **pendiente** |
| Borde grueso | La fuente de verdad y su único escritor |

**Lo que el diagrama deja ver de un vistazo:** el riesgo nunca toca la base y ni siquiera conoce al
core —solo intercambia eventos—; el reloj está separado del ejecutor; y el worker entra por la misma
puerta que cualquier cliente, con el mismo `PATCH /status` que ya usa en producción. La única flecha
punteada del camino de negocio es el lock de Redis: es una optimización best-effort, no una garantía
(ver [ADR-0002](docs/adr/0002-idempotencia.md)) — el sistema sigue siendo correcto si Redis cae.

| Componente | Tecnología | Responsabilidad | Dueño | Documentación |
|---|---|---|---|---|
| `core-api` | Go | Payment Intents, estados, idempotencia, API HTTP | Valentina | — |
| `risk-service` | Python | Evaluación de riesgo, reglas determinísticas | Sergio | [README](risk-service/README.md) |
| `reconciliation-scheduler` | Python | El reloj: publica el tick de conciliación | Sergio | [README](reconciliation-scheduler/README.md) |
| `reconciliation-worker` | Python | Expira Payment Intents vencidos | Sergio | [README](reconciliation-worker/README.md) |
| `db-migrator` | SQL + `migrate` | Aplica migraciones y termina | Sergio | [Esquema](docs/esquema-de-datos.md) |
| PostgreSQL | Infra | Fuente de verdad — `core-api` conecta como `mova_app` | Sergio (esquema) · Valentina (repos) | [Esquema](docs/esquema-de-datos.md) |
| Redis | Infra | Lock best-effort de idempotencia (`SET NX` + TTL) | Valentina | [ADR-0002](docs/adr/0002-idempotencia.md) |
| Kafka | Infra | Mensajería entre servicios | Valentina · Sergio | [ADR-0001](docs/adr/0001-contrato-go-python.md) |
| Prometheus + Grafana | Infra | Métricas y dashboard aprovisionado | Sergio | [Observabilidad](#observabilidad) |

**Límite no negociable:** ningún servicio Python escribe directo en las
tablas del core — todo pasa por eventos de Kafka o por la API pública de
`core-api`. No es una convención de equipo: **ninguno de los tres servicios
Python tiene cadena de conexión a la base en su entorno**. La única del
sistema es la de `core-api`.

| Proceso | Escribe en | Cómo obtiene los datos |
|---|---|---|
| `core-api` | **`mova_orchestrator`** | SQL propio · Kafka |
| `db-migrator` | El esquema, **solo DDL**, y termina | — |
| `risk-service` | Nada | Solo Kafka |
| `reconciliation-scheduler` | Nada | Solo Kafka, y solo produce |
| `reconciliation-worker` | Nada | Kafka el disparo · **API** los datos |

El worker consulta la API y no una proyección propia a propósito: una copia
local sería una segunda interpretación de los datos del core, siempre unos
milisegundos por detrás, y acabaría intentando cerrar pagos que ya cambiaron.

### Flujo

```mermaid
sequenceDiagram
    participant Cliente
    participant CoreAPI as core-api (Go)
    participant Kafka
    participant Risk as Risk Service (Python)

    Cliente->>CoreAPI: POST /payment-intents (Idempotency-Key)
    CoreAPI->>CoreAPI: Guarda PENDING
    CoreAPI->>CoreAPI: Marca UNDER_REVIEW (atómico con su historial)
    CoreAPI->>Kafka: publish risk.evaluation.requested
    CoreAPI-->>Cliente: 201 { status: "UNDER_REVIEW" }

    Note over Kafka,Risk: Asíncrono — Kafka retiene el mensaje si Risk está caído/lento

    Kafka->>Risk: consume risk.evaluation.requested
    Risk->>Risk: evalúa reglas determinísticas
    Risk->>Kafka: publish risk.evaluation.completed
    Kafka->>CoreAPI: consume risk.evaluation.completed
    CoreAPI->>CoreAPI: Marca APPROVED/REJECTED (o sigue en UNDER_REVIEW si REVIEW)
```

Ver [ADR-0001](docs/adr/0001-contrato-go-python.md) para el contrato
exacto de los eventos, y [ADR-0003](docs/adr/0003-politica-risk-service-caido.md)
para qué pasa si el Risk Service se demora, cae, o responde dos veces.

### Flujo de conciliación

Lo que cierra los pagos que se quedaron abiertos — un `UNDER_REVIEW` que nunca
se resolvió porque el Risk Service no volvió.

```mermaid
sequenceDiagram
    participant Beat as reconciliation-scheduler
    participant Kafka
    participant Worker as reconciliation-worker
    participant CoreAPI as core-api (Go)

    loop cada INTERVAL_SECONDS
        Beat->>Kafka: publish reconciliation.tick
    end

    Kafka->>Worker: consume tick
    Note over Worker: descarta ticks viejos o repetidos
    Worker->>CoreAPI: GET /payment-intents?status=PENDING
    Worker->>CoreAPI: GET /payment-intents?status=UNDER_REVIEW
    Note over Worker: filtra por expires_at
    Worker->>CoreAPI: PATCH /payment-intents/{id}/status EXPIRED
    CoreAPI->>CoreAPI: valida la transición y escribe el historial
```

**El reloj va aparte del ejecutor** ([ADR-0006](docs/adr/0006-scheduler-como-servicio-aparte.md)):
el scheduler debe correr como réplica única y el worker querría escalar. Con el
bucle dentro del worker, dos réplicas eran dos relojes y trabajo duplicado.

## Servicios Python

Los tres son de Sergio. Cada uno tiene su README con el detalle completo; aquí
va lo que hay que saber para entender el sistema sin abrirlos.

### `risk-service` — la decisión de riesgo

Consume `risk.evaluation.requested`, aplica reglas determinísticas y publica
`risk.evaluation.completed`. **No tiene base de datos, ni caché, ni estado
persistente**, y no conoce a `core-api`: solo intercambia eventos.

```mermaid
flowchart LR
    req[["risk.evaluation.requested"]] --> r0{"¿comercio<br/>bloqueado?"}
    r0 -->|"si"| rej0["REJECT<br/>MERCHANT_BLOCKED"]
    r0 -->|"no"| r1{"¿referencia<br/>sospechosa?"}
    r1 -->|"si"| rej["REJECT<br/>SUSPICIOUS_REFERENCE"]
    r1 -->|"no"| r2{"¿velocidad<br/>anormal?"}
    r2 -->|"si"| rej2["REJECT<br/>ABNORMAL_VELOCITY"]
    r2 -->|"no"| r3{"¿monto sobre<br/>el umbral?"}
    r3 -->|"si"| rev["REVIEW<br/>AMOUNT_ABOVE_REVIEW_THRESHOLD"]
    r3 -->|"no"| app["APPROVE<br/>LOW_RISK"]

    rej0 --> out[["risk.evaluation.completed"]]
    rej --> out
    rej2 --> out
    rev --> out
    app --> out
```

Se evalúan **en orden de severidad y la primera que dispara manda**: un comercio
bloqueado se rechaza aunque todo lo demás esté limpio, y una referencia
sospechosa se rechaza aunque el monto sea bajo. Un `merchant_status` ausente o
desconocido **no bloquea**: rechazar por no saber tumbaría las aprobaciones cada
vez que los dos lados se desalinean.

El `score` no es un modelo: es una suma acotada a `[0,100]` que se puede
explicar en voz alta — 5 de base, hasta 20 graduales por monto, 55 si supera el
umbral, 95 si disparó una regla bloqueante. El componente gradual existe para
que **discrimine dentro de una misma decisión**: dos pagos aprobados de 1.000 y
de 90.000 COP no deberían tener el mismo número.

`model_version` (`rules-v2`) viaja en cada respuesta y `core-api` la persiste: si
mañana cambian los umbrales, se sabe con qué versión se decidió cada pago
histórico. `v2` añadió la regla de comercio bloqueado.

**La velocidad se cuenta sin tocar la base.** Manda el `merchant_recent_intents`
que publica `core-api` —exacto con varias réplicas y a prueba de reinicios—, y
si el campo no viene se usa una ventana deslizante en memoria alimentada por los
eventos que el servicio ya consume. Las dos fuentes usan la misma semántica: el
intent que se evalúa no se cuenta a sí mismo. El registro local es idempotente
por `payment_intent_id`, porque Kafka entrega al menos una vez y una reentrega
inventaría un rechazo
([ADR-0004](docs/adr/0004-velocidad-sin-acceso-a-la-base.md)).

→ [README completo](risk-service/README.md) · [OpenAPI](docs/openapi/risk-service-v1.yaml)

### `reconciliation-scheduler` — el reloj

Publica `reconciliation.tick` cada `INTERVAL_SECONDS` y nada más. Sin base, sin
credencial de API, sin decisiones.

Es el **único proceso que debe correr como réplica única**, y por eso es también
el que no puede hacer daño: se puede reiniciar o desplegar en cualquier momento
porque un ciclo perdido se recupera en el siguiente.

El `tick_id` es determinista por ventana de tiempo, así que dos schedulers vivos
durante un despliegue producen el mismo id y el worker descarta el duplicado.

→ [README completo](reconciliation-scheduler/README.md) · [ADR-0006](docs/adr/0006-scheduler-como-servicio-aparte.md)

### `reconciliation-worker` — cierra lo que quedó abierto

Consume el tick, lista los intents abiertos y solicita su expiración **por la
API pública**. Nunca toca la base: el cambio pasa por la máquina de estados, el
historial con actor y las mismas validaciones que cualquier otro.

| Estado | ¿Se expira? | Por qué |
|---|---|---|
| `PENDING` | Sí | Se creó pero nunca llegó a evaluarse |
| `UNDER_REVIEW` | Sí | El riesgo no respondió nunca ([ADR-0003](docs/adr/0003-politica-risk-service-caido.md)) |
| Terminales | No | Pedirlo daría `422` y sería ruido |

Son exactamente los dos estados desde los que la tabla del dominio permite salir
a `EXPIRED`: el worker no inventa su propia idea de qué es legal.

**La ventana la define el core**, no el worker: se usa el `expires_at` que puso
al crear el intent. Si los dos tuvieran su propia noción de cuándo vence un
pago, tarde o temprano discreparían.

Su resiliencia es la parte con más cuidado: reintenta la infraestructura y nunca
el dominio, con backoff exponencial y jitter completo, y un circuit breaker
propio. **Solo esta capa reintenta en toda la cadena** — si el core reintentara
también, un fallo produciría nueve llamadas en vez de tres, y hay una prueba que
afirma el número exacto ([ADR-0005](docs/adr/0005-reintentos-y-breaker-del-worker.md)).

→ [README completo](reconciliation-worker/README.md)

### Lo que comparten los tres

| | |
|---|---|
| **Sin credenciales de la base** | La única cadena de conexión del sistema es la de `core-api` |
| **Configuración validada al arrancar** | Si falta algo obligatorio el proceso no levanta, en vez de fallar en la primera petición |
| **Logs JSON con `correlation_id`** | Es el único hilo que permite seguir un pago cruzando Go y Python |
| **`/health` ≠ `/readiness`** | Un consumidor vivo pero sin broker está arriba y no sirve para nada |
| **Métricas sin cardinalidad no acotada** | Ninguna etiqueta lleva `merchant_id` ni `payment_intent_id` |
| **74 pruebas sin red ni contenedores** | Es la prueba de que la separación por capas es real |

### Estructura de carpetas

```
MOVA_prueba_grupal/
├── core-api/                  # Go — dominio, aplicación, infraestructura, transporte
│   ├── cmd/api/
│   └── internal/
│       ├── domain/
│       ├── application/
│       ├── infrastructure/{kafka,redis,postgres,auth}/
│       └── transport/http/
├── risk-service/               # Python — reglas de riesgo, consumidor Kafka
│   ├── app/{domain,application,infrastructure,api}/
│   └── tests/                  # 35 tests, ninguno necesita red
├── reconciliation-scheduler/   # Python — el reloj, publica el tick
│   ├── scheduler/
│   └── tests/                  # 5 tests
├── reconciliation-worker/      # Python — expira intents vencidos vía API
│   ├── worker/{domain,application,infrastructure}/
│   └── tests/                  # 34 tests, core simulado con respx
├── migrations/                 # SQL plano, .up y .down, aplicado por db-migrator
├── observability/
│   ├── prometheus/             # scrape config
│   └── grafana/{provisioning,dashboards}/
├── docs/
│   ├── adr/                    # decisiones, una por archivo
│   └── esquema-de-datos.md     # DDL, triggers, roles y las garantías verificadas
└── docker-compose.yml
```

### Cómo están estructurados los servicios Python

Los tres siguen la misma forma, para que quien revise uno reconozca el
siguiente. La regla de dependencia es estricta y va **hacia adentro**:

```mermaid
flowchart RL
    api["api/ · entry points<br/>HTTP · consumidores Kafka · comandos"]
    infra["infrastructure/<br/>adaptadores concretos"]
    app["application/<br/>casos de uso + puertos"]
    dom["domain/<br/>reglas puras"]

    api --> app
    infra -.implementa los puertos.-> app
    app --> dom
```

| Capa | Qué vive ahí | Qué **no** puede importar |
|---|---|---|
| `domain/` | Entidades, reglas, invariantes | Nada. Ni framework, ni red, ni reloj, ni las otras capas |
| `application/` | Casos de uso y los puertos que necesitan | Framework, HTTP, Kafka, SQL |
| `infrastructure/` | Implementaciones concretas de los puertos | — |
| `api/` · entry points | Traducir de fuera hacia el caso de uso | Dominio directamente |

**La prueba de que la separación es real** no es que las carpetas tengan esos
nombres: es que `domain/` se prueba sin levantar nada. Las 74 pruebas de los
tres servicios corren sin red ni contenedores, y eso solo es posible si la
dependencia apunta de verdad hacia adentro.

**Un caso de uso, varias entradas.** El `risk-service` se invoca por Kafka y por
HTTP; el `reconciliation-worker`, por el tick y por un comando manual. En los
dos, las entradas son adaptadores finos sobre el mismo caso de uso, así que la
ruta de prueba no puede divergir de la de producción.

| Servicio | Entradas | Detalle |
|---|---|---|
| `risk-service` | Consumidor Kafka · `POST /api/v1/risk-evaluations` | [Estructura](risk-service/README.md#estructura) |
| `reconciliation-scheduler` | Bucle de reloj | [Estructura](reconciliation-scheduler/README.md#estructura) |
| `reconciliation-worker` | Consumidor del tick · `python -m worker.run_once` | [Estructura](reconciliation-worker/README.md#estructura) |

### Estructura documental

Cada cosa vive en un solo sitio, y el resto enlaza:

| Documento | Responde a | Cuándo se toca |
|---|---|---|
| Este README | *¿Qué es el sistema y cómo lo levanto?* | Cuando cambia la arquitectura o el arranque |
| `<servicio>/README.md` | *¿Qué hace este proceso, cómo decide y qué falla?* | En la misma PR que cambia el servicio |
| [`docs/contratos-entre-servicios.md`](docs/contratos-entre-servicios.md) | *¿Quién le habla a quién y qué pasa si falla?* | Al cambiar cualquier contrato |
| [`docs/adr/`](docs/adr/) | *¿Por qué se hizo así y qué se descartó?* | Cuando se toma una decisión con alternativas |
| [`docs/esquema-de-datos.md`](docs/esquema-de-datos.md) | *¿Cómo son los datos y qué garantizan?* | En la misma PR que una migración |

**Regla:** si una decisión tenía alternativas viables, va en un ADR. Si es solo
*qué hace* algo, va en el README del servicio. Un README que explica un porqué
largo suele ser un ADR que nadie escribió.

Cada README de servicio sigue la misma estructura —**dónde encaja, cómo decide,
qué configura, qué métricas emite y qué puede salir mal**— para que quien revise
uno reconozca el siguiente. Y todos enlazan sus ADRs asociados arriba del todo.

## Requisitos

- Docker + Docker Compose
- Go 1.25+ (solo si querés correr `core-api` fuera de Docker)
- Python 3.12+ (solo si querés correr los servicios Python fuera de Docker)

## Ejecución

```bash
cp .env.example .env
docker compose up -d --build
```

Levanta Postgres, Redis, Kafka, `core-api`, `risk-service` y
`reconciliation-worker` juntos. La API queda en
`http://localhost:${PORT}` (`8095` por defecto en `.env.example` — `8080`
suele estar ocupado por otros servicios locales en Windows).

Verificar que arrancó bien:

```bash
curl http://localhost:8095/readiness
```

## Variables de entorno

Ver [`.env.example`](.env.example). Resumen:

| Variable | Descripción |
|---|---|
| `PORT` | Puerto HTTP de `core-api` |
| `DB_*`, `DATABASE_URL` | Postgres — `core-api` conecta como `mova_app`, `db-migrator` como el superusuario de arranque (ver ADR-0007) |
| `REDIS_PORT`, `REDIS_ADDR` | Redis — lock best-effort de idempotencia (ver ADR-0002) |
| `KAFKA_BROKERS` | Broker(s) de Kafka, separados por coma |
| `RISK_SERVICE_URL` | A dónde llamar directo si Kafka mismo falla (circuit breaker, ADR-0008) |
| `JWT_SECRET`, `JWT_EXPIRATION_MINUTES`, `AUTH_USERNAME`, `AUTH_PASSWORD` | Autenticación (ver PR de auth) |
| `REVIEW_AMOUNT_MINOR`, `VELOCITY_*`, `SUSPICIOUS_REFERENCE_PREFIXES` | Umbrales de las reglas de riesgo |
| `RECONCILIATION_INTERVAL_SECONDS` | Cada cuánto corre un ciclo de conciliación |
| `EXPIRY_OVERRIDE_MINUTES` | Solo demo: cierra por antigüedad en vez de esperar los 30 min |

## Endpoints

| Método | Ruta | Auth | Descripción |
|---|---|---|---|
| `GET` | `/readiness` | No | Confirma que Kafka y Postgres son alcanzables |
| `GET` | `/docs` | No | Swagger UI contra `docs/openapi/core-api-v1.yaml` |
| `POST` | `/api/v1/auth/login` | No | Autentica con la credencial de servicio, devuelve un JWT |
| `POST` | `/api/v1/merchants` | **Sí** | Crea un comercio (siempre `ACTIVE`) |
| `GET` | `/api/v1/merchants/{id}` | **Sí** | Consulta un comercio |
| `POST` | `/api/v1/payment-intents` | **Sí** | Crea un Payment Intent (requiere `Idempotency-Key`, `merchant_id` de un comercio existente) |
| `GET` | `/api/v1/payment-intents` | **Sí** | Lista con filtros `merchant_id`, `status`, `page`, `limit` |
| `GET` | `/api/v1/payment-intents/{id}` | **Sí** | Consulta un Payment Intent |
| `GET` | `/api/v1/payment-intents/{id}/history` | **Sí** | Historial de cambios de estado |
| `PATCH` | `/api/v1/payment-intents/{id}/status` | **Sí** | Transición manual de estado — la usa el `reconciliation-worker` para expirar vencidos |

Los servicios Python exponen lo suyo aparte:

| Servicio | Puerto | Rutas |
|---|---|---|
| `risk-service` | `8081` | `/health`, `/readiness`, `/metrics`, `/docs` (Swagger UI), `POST /api/v1/risk-evaluations` |
| `reconciliation-worker` | `8082` | `/metrics` |

### Contratos y colección de Postman

Los cinco contratos del sistema —quién habla con quién, con qué forma y qué pasa
cuando falla— están en
[`docs/contratos-entre-servicios.md`](docs/contratos-entre-servicios.md).

| Artefacto | Qué es |
|---|---|
| [`docs/openapi/core-api-v1.yaml`](docs/openapi/core-api-v1.yaml) | Contrato de `core-api`, **escrito desde el lado del consumidor**. Servido en vivo en `/docs` |
| [`docs/openapi/risk-service-v1.yaml`](docs/openapi/risk-service-v1.yaml) | Generado del código con [`scripts/export-openapi.sh`](scripts/export-openapi.sh) |
| [`docs/postman/`](docs/postman/README.md) | Colección con `pm.test` en cada petición, su entorno y cómo se corre |

Swagger UI sale gratis en `risk-service` (FastAPI lo genera solo) y está servido
a mano en `core-api`: http://localhost:8081/docs y http://localhost:8095/docs.

**La colección afirma, no solo dispara.** 33 peticiones en 6 carpetas con **55
aserciones**: que el reintento devuelve el mismo intent, que el dinero es
entero, que cada entrada del historial está atribuida, que la referencia
duplicada da `409` y no `201`, que un comercio inexistente da `404` antes de
llegar a crear un pago, y que una transición ilegal da `422` con un código de
error estable.

```bash
newman run docs/postman/MOVA.postman_collection.json \
       -e docs/postman/MOVA.postman_environment.json
```

| Carpeta | Demuestra |
|---|---|
| 0 · Salud | Que todo está arriba, y que `/health` no es lo mismo que `/readiness` |
| 1 · Autenticación | Login y el `401` sin credencial |
| 2 · Reglas de riesgo | Las cuatro decisiones contra `risk-service`, y que el score discrimina |
| 3 · Comercios | Creación, consulta, `409` por documento duplicado y `404` por inexistente |
| 4 · Payment Intents | Creación, **reintento idempotente**, historial atribuido, `409` por referencia duplicada, los tres canales |
| 5 · Conciliación | Lo que hace el worker: listar abiertos, expirar, y los tres códigos del contrato |

**Las carpetas corren en orden y cada una siembra la siguiente.** Desde la
migración `0006` un pago necesita un comercio real, así que `3 · Comercios` deja
el `merchant_id` que usan las de abajo. El detalle está en
[`docs/postman/README.md`](docs/postman/README.md).

### Los ocho casos del enunciado

```bash
docker compose up -d
./scripts/casos-de-prueba.sh
```

Un bloque por cada fila de la tabla *"casos que deben demostrar"*, en el mismo
orden y con los mismos nombres, para poder irlos marcando mientras corre.

| Caso | Qué hace el script |
|---|---|
| **1 · Caso feliz** | Pago por QR, espera la decisión por Kafka y muestra el historial entrada por entrada |
| **2 · Reintento** | Misma `Idempotency-Key` → mismo intent. Llave nueva + misma referencia → `409` |
| **3 · Concurrencia** | 10 solicitudes en paralelo, y comprueba que en PostgreSQL quedó **una sola fila** |
| **4 · Revisión** | 50.000.000 → `UNDER_REVIEW` conservando score y razones |
| **5 · Rechazo** | Referencia sospechosa **y** comercio `INACTIVE`; verifica que el historial nunca pasó por `APPROVED` |
| **6 · Expiración** | Un ciclo real del worker, y comprueba que el intent quedó `EXPIRED` con actor |
| **7 · Fallo de dependencia** | **Detiene `risk-service`**, comprueba el estado seguro, lo levanta y ve cómo se resuelve solo |
| **8 · Transición ilegal** | `422 INVALID_TRANSITION` desde estados terminales, y `409` al repetir la actual |

El caso 7 es el que más enseña: con el riesgo caído el pago se queda en
`UNDER_REVIEW` sin `risk_decision` —ninguna aprobación silenciosa— y al volver el
servicio se resuelve solo, porque Kafka retuvo el evento (y, si en cambio fuera
Kafka mismo el que estuviera caído, se resuelve igual de rápido por el camino
HTTP directo del circuit breaker — ver ADR-0008).

### El recorrido completo, para CI

Dos scripts equivalentes corren contra el sistema real, narrado en la terminal
en vez de en Postman:

```bash
docker compose up -d
./scripts/demo.sh              # recorrido completo del sistema, pensado para CI
./scripts/casos-de-prueba.sh   # los 8 casos del enunciado, uno por fila de la tabla,
                                # pensado para la sustentación
```

`demo.sh` son once pasos que recorren el sistema entero y además comprueban las
garantías del esquema con `psql` y el estado de la observabilidad;
`casos-de-prueba.sh` se enfoca solo en los ocho casos de la tabla, con
manipulación real de contenedores para el de fallo de dependencia. Los dos
salen distinto de cero si algo falla.

### Autenticación

No hay tabla de usuarios — una única credencial de servicio configurada
por variables de entorno (`AUTH_USERNAME`/`AUTH_PASSWORD`), igual que en
el proyecto individual de referencia. `POST /auth/login` devuelve un JWT
(HS256, expira según `JWT_EXPIRATION_MINUTES`); el resto de los
endpoints de pagos exige `Authorization: Bearer <token>`. El `subject`
del token queda como `changed_by` en cada entrada del historial.

```bash
curl -X POST http://localhost:8095/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username":"...","password":"..."}'
```

## Observabilidad

```bash
docker compose up -d prometheus grafana
```

| Servicio | URL | Credenciales |
|---|---|---|
| Grafana | http://localhost:3000 | `admin` / `admin` |
| Prometheus | http://localhost:9090 | — |

El dashboard **MOVA Payment Orchestrator — Riesgo y Conciliación** se aprovisiona
como código desde [`observability/grafana/dashboards/`](observability/grafana/dashboards/).
Nada se crea a mano en la interfaz: un dashboard que solo existe en la base de
Grafana desaparece con el volumen.

Cuatro filas, y cada panel responde a una pregunta concreta:

| Fila | Qué contesta |
|---|---|
| **Riesgo** | Qué se aprueba y qué se rechaza, qué regla dispara, si llegan eventos malformados |
| **Conciliación** | Cuántos vencidos se cierran, estado del circuit breaker, ciclos abortados y por qué |
| **Reloj** | Si el scheduler sigue vivo y si el worker lo está escuchando |
| **Kafka** | Consumer lag por grupo — la señal que dice si alguien dejó de consumir sin haberse caído |

### Las métricas que importan

| Métrica | Por qué |
|---|---|
| `reconciliation_scheduler_last_tick_timestamp` | Si deja de avanzar, **nadie está conciliando y ninguna otra métrica lo diría**: el worker inactivo se ve igual que "no hay trabajo pendiente" |
| `reconciliation_circuit_breaker_state` | En `2`, el core está caído y el worker falla rápido en vez de gastar timeouts |
| `risk_events_consumed_total{outcome="malformed"}` | Debería ser cero. Si no, el contrato del ADR-0001 cambió sin avisar |
| `kafka_consumergroup_lag` | Un consumidor que se queda atrás sin estar caído |

Ninguna métrica lleva `merchant_id` ni `payment_intent_id` como etiqueta: son de
cardinalidad no acotada y convertirían cada comercio en una serie nueva. Es una
regla de revisión, no una recomendación.

## Pruebas

```bash
cd core-api
go test ./...                        # unitarias, sin dependencias externas
go test -tags=integration ./...      # contra Kafka, Postgres y Redis reales (requiere docker compose up)
```

- `internal/domain` (12 tests): reglas de validación, tabla de
  transiciones completa, aplicación de decisiones de riesgo, validación
  de `Merchant`.
- `internal/application` (20 tests): `PaymentIntentService` (creación,
  idempotencia, `UpdateStatus`, `merchant_status`/`merchant_recent_intents`
  en el evento publicado, aplicar de inmediato la decisión cuando el
  publisher cae al HTTP directo) y `MerchantService`, con repositorios
  falsos en memoria — incluye un test de concurrencia real (20
  goroutines, misma `idempotency_key`, una sola fila creada).
- `internal/infrastructure/auth` (4 tests): firma/validación de JWT
  (round-trip, secreto incorrecto, expirado, malformado).
- `internal/middleware` (3 tests): `RequireAuth` — token válido,
  ausente, inválido.
- `internal/infrastructure/kafka` (11 tests, 3 de integración): el
  circuit breaker (cerrado/abierto/semi-abierto), el publisher cayendo
  al HTTP directo cuando Kafka es inalcanzable, y el backoff con jitter
  de la reconexión del consumer — sin necesitar un broker real para
  nada de eso; productor, consumidor y `EnsureTopics` sí contra un
  broker Kafka real (ver ADR-0008).
- `internal/infrastructure/riskhttp` (3 tests): el cliente del camino
  de emergencia, contra un servidor HTTP de prueba — éxito, error de
  dominio, e inalcanzable.
- `internal/infrastructure/postgres` (11 tests, integración): `Merchant`
  y `PaymentIntent`/historial reales contra PostgreSQL — creación,
  conflictos de unicidad, transiciones en dos pasos, listado filtrado,
  conteo de intents recientes para la velocidad.
- `internal/infrastructure/redis` (2 tests, integración): el lock real
  — un segundo intento con la misma key falla mientras el primero lo
  tiene tomado, y se libera correctamente después.
- `internal/transport/http` (30 tests, 1 de integración): login,
  comercios, endpoints de pagos protegidos (incluye el `PATCH` de estado
  y el caso sin token → 401), `/docs` sirviendo el contrato OpenAPI, y
  `/readiness` contra Kafka y Postgres reales.

### Servicios Python

```bash
cd risk-service              && .venv/bin/python -m pytest    # 35
cd reconciliation-scheduler  && .venv/bin/python -m pytest    #  5
cd reconciliation-worker     && .venv/bin/python -m pytest    # 34
```

**74 pruebas y ninguna necesita red ni contenedores.** Las reglas de riesgo son
una función pura, así que se prueban con una tabla de casos; el cliente HTTP se
prueba con `respx` contra las rutas y los cuerpos reales del contrato.

Tres merecen mención porque afirman algo que se puede romper sin darse cuenta:

| Prueba | Qué protege |
|---|---|
| `test_kafka_contract.py` | Valida contra el **payload literal** del ADR-0001. Si Go cambia el evento, falla aquí |
| `test_no_reintenta_indefinidamente` | Afirma el número exacto de llamadas: impide que alguien añada un segundo nivel de reintentos y multiplique la carga |
| `test_una_reentrega_no_infla_el_contador` | Kafka entrega al menos una vez; sin idempotencia, un reproceso inventaría un rechazo por velocidad |

Calidad, además de pruebas:

```bash
.venv/bin/python -m ruff check .    # lint y formato
.venv/bin/python -m mypy            # tipado estricto
```

Los tres servicios pasan `mypy --strict` sin excepciones salvo el módulo
`aiokafka`, que no publica `py.typed`.

### Esquema de datos

Las nueve garantías del esquema están verificadas contra un PostgreSQL real
—transición ilegal, historial inmutable, cambio sin historial, llave duplicada,
resolución sin riesgo, borrado con historial, permisos del rol— y listadas en
[`docs/esquema-de-datos.md` §8](docs/esquema-de-datos.md). Se reproducen con
`psql` en la sustentación.

## Decisiones documentadas (ADR)

- [ADR-0001 — Contrato Go/Python vía Kafka](docs/adr/0001-contrato-go-python.md)
- [ADR-0002 — Idempotencia y concurrencia](docs/adr/0002-idempotencia.md)
- [ADR-0003 — Estado seguro ante fallas del Risk Service](docs/adr/0003-politica-risk-service-caido.md)
- [ADR-0004 — Velocidad sin acceso a la base del core](docs/adr/0004-velocidad-sin-acceso-a-la-base.md)
- [ADR-0005 — Reintentos, timeouts y circuit breaker del worker](docs/adr/0005-reintentos-y-breaker-del-worker.md)
- [ADR-0006 — El reloj de conciliación va en un servicio aparte](docs/adr/0006-scheduler-como-servicio-aparte.md)
- [ADR-0007 — Migraciones como proceso aparte, SQL plano, una sola base](docs/adr/0007-migraciones-como-proceso-aparte.md)
- [ADR-0008 — Circuit breaker Kafka→HTTP cuando el broker mismo falla](docs/adr/0008-circuit-breaker-kafka-http.md)

| Servicio | ADRs que lo gobiernan |
|---|---|
| `core-api` | [0001](docs/adr/0001-contrato-go-python.md) · [0002](docs/adr/0002-idempotencia.md) · [0003](docs/adr/0003-politica-risk-service-caido.md) · [0007](docs/adr/0007-migraciones-como-proceso-aparte.md) · [0008](docs/adr/0008-circuit-breaker-kafka-http.md) |
| `risk-service` | [0001](docs/adr/0001-contrato-go-python.md) · [0003](docs/adr/0003-politica-risk-service-caido.md) · [0004](docs/adr/0004-velocidad-sin-acceso-a-la-base.md) · [0008](docs/adr/0008-circuit-breaker-kafka-http.md) |
| `reconciliation-scheduler` | [0006](docs/adr/0006-scheduler-como-servicio-aparte.md) |
| `reconciliation-worker` | [0003](docs/adr/0003-politica-risk-service-caido.md) · [0005](docs/adr/0005-reintentos-y-breaker-del-worker.md) · [0006](docs/adr/0006-scheduler-como-servicio-aparte.md) |
| `db-migrator` y el esquema | [0002](docs/adr/0002-idempotencia.md) · [0007](docs/adr/0007-migraciones-como-proceso-aparte.md) |

## Pendientes

Nada pendiente por ahora — la última pieza de resiliencia que quedaba
(circuit breaker Kafka→HTTP) se cerró en el ADR-0008.

## Flujo de trabajo con Git

El historial cuenta cómo se construyó el sistema, no es una lista plana de
cambios.

```mermaid
gitGraph
    commit id: "init"
    branch develop
    checkout develop
    commit id: "dominio"
    branch feature/payment-intent-core
    checkout feature/payment-intent-core
    commit id: "core + kafka"
    checkout develop
    merge feature/payment-intent-core
    branch feature/risk-service
    checkout feature/risk-service
    commit id: "reglas"
    commit id: "worker + scheduler"
    checkout develop
    merge feature/risk-service
    checkout main
    merge develop tag: "v0.1.0"
```

| Rama | Propósito |
|---|---|
| `main` | Solo versiones liberadas y etiquetadas |
| `develop` | Integración del trabajo terminado |
| `feature/*` | Una rama por unidad de trabajo, nace y vuelve a `develop` |

**Nada se hace directamente sobre `main`.** Cada persona abre al menos dos Pull
Requests sustanciales y revisa al menos dos de sus compañeros, con
observaciones técnicas reales — una aprobación sin comentario no cuenta como
revisión.

Integraciones con `--no-ff`, para que el historial conserve la agrupación por
funcionalidad: se ve de un vistazo qué commits pertenecen a qué trabajo, y una
funcionalidad completa se revierte con un solo *revert* del *merge*.

### Convención de commits

`<emoji> <tipo>: <qué cambió, en imperativo y en minúscula>`

El emoji no es decoración: hace que `git log --oneline` se lea de un vistazo y
que en una revisión se vea al instante si una PR trae funcionalidad, seguridad o
solo documentación.

| Emoji | Tipo | Se usa para |
|---|---|---|
| 🎉 | `init` | Commit inicial de un servicio |
| ✨ | `feat` | Funcionalidad nueva |
| 🐛 | `fix` | Corrección de un defecto |
| 🗃️ | `db` | Migraciones, esquema, índices |
| 🔒️ | `security` | Autenticación, permisos, roles de base de datos |
| 📨 | `event` | Contratos de evento, productores, consumidores |
| 📈 | `metrics` | Instrumentación, dashboards, alertas |
| ♻️ | `refactor` | Cambio interno sin efecto observable |
| ✅ | `test` | Pruebas que no acompañan a un `feat` |
| 📝 | `docs` | README, ADRs, OpenAPI |
| 🐳 | `docker` | Compose, Dockerfiles, arranque |
| 🔧 | `chore` | Dependencias, CI, configuración |
| 🔀 | `merge` | Integración de una rama |
| 🔖 | `release` | Versión etiquetada |

```
✨ feat: implementar el reloj de conciliacion como servicio aparte
🗃️ db: agregar triggers de transicion e historial append-only
📨 event: consumir risk.evaluation.requested y publicar el resultado
🔒️ security: separar el rol de migraciones del rol de la aplicacion
📝 docs: documentar el esquema y sus nueve garantias verificadas
```

**Un tipo por commit.** Si una PR toca esquema, código y documentación, son tres
commits y no uno con tres emojis — así el `revert` de una parte no se lleva las
otras.

> Los commits anteriores a esta convención (`feat:`, `docs:` sin emoji) se dejan
> como están: reescribir el historial de ramas ya mergeadas cuesta más de lo que
> vale la uniformidad. Aplica de aquí en adelante.

### Definición de "terminado"

Una PR se puede mergear cuando cumple **todo**:

- Pruebas automatizadas que fallan sin el cambio y pasan con él.
- Lint, formato y tipos en verde en el stack que toca (`go test` y
  `golangci-lint`; o `ruff` y `mypy --strict`).
- El README del servicio refleja lo que hace ahora.
- Si la decisión tenía alternativas viables, hay un ADR.
- Al menos un comentario técnico real del revisor.
- Ningún secreto en el repositorio.

Las PRs se mantienen por debajo de ~400 líneas de diff. Una PR de dos mil
líneas no se revisa, se aprueba.

## Matriz de contribuciones

<!-- Se actualiza en cada PR: qué se implementó, quién lo revisó, y qué se aprendió. -->

| PR | Autor | Resumen | Archivos principales |
|---|---|---|---|
| `feature/payment-intent-core` | Valentina | Esqueleto del repo, dominio y aplicación de `PaymentIntent`, mensajería Kafka real (productor + consumidor + `EnsureTopics`), endpoints HTTP + `/readiness`, 3 ADRs. Persistencia real (Postgres/Redis) queda pendiente de Eduard — ver [Pendientes](#pendientes). | `core-api/internal/domain`, `core-api/internal/application`, `core-api/internal/infrastructure/{kafka,memory,redis}`, `core-api/internal/transport/http`, `docker-compose.yml`, `docs/adr/` |
| `feature/auth-middleware` | Valentina | Autenticación JWT (login + middleware) sobre los endpoints de pagos, sin tabla de usuarios — misma credencial de servicio del proyecto individual de referencia. `changed_by` en el historial ahora sale del subject del token, no de un valor fijo. | `core-api/internal/infrastructure/auth`, `core-api/internal/middleware`, `core-api/internal/transport/http/{auth_handler.go,auth_context.go,router.go}` |
| `feature/risk-service-reconciliation-worker` | Sergio | Los tres servicios Python (74 tests, `mypy --strict`): reglas de riesgo determinísticas sobre Kafka, el reloj de conciliación como servicio aparte, y el worker que expira intents vencidos vía API con reintentos, jitter y circuit breaker propios. Esquema de datos completo en SQL plano con triggers y roles, aplicado por un `db-migrator` externo y con las 9 garantías verificadas contra PostgreSQL real. Prometheus + Grafana con dashboard aprovisionado. 4 ADRs. Detectada y documentada una integración pendiente: `core-api` no expone `PATCH /status`, que el worker necesita. | `risk-service/`, `reconciliation-scheduler/`, `reconciliation-worker/`, `migrations/`, `observability/`, `docs/adr/0004`–`0007`, `docs/esquema-de-datos.md`, `docker-compose.yml` |
| `feature/merchant` | Valentina | Vertical completo de comercios (dominio, Postgres real, HTTP) que le tocaba originalmente a Eduard, absorbido tras su salida del equipo. Migración 0006 agrega la FK `payment_intents.merchant_id` → `payments.merchants`. | `core-api/internal/domain/merchant.go`, `core-api/internal/application/merchant_service.go`, `core-api/internal/infrastructure/postgres/merchant_repository.go`, `core-api/internal/transport/http/merchant_handler.go`, `migrations/0006_merchants.*` |
| `feature/payment-intent-postgres` | Valentina | Reemplaza los repositorios en memoria de `PaymentIntent`/historial por Postgres real; borra `internal/infrastructure/memory/` por completo. Cierra la referencia externa duplicada (issue #13), ya inerte al no depender más de memoria. Arregla `scripts/demo.sh` para crear un comercio real antes de usarlo (la FK de la migración 0006 lo exige) y lo deja portable en Windows. | `core-api/internal/infrastructure/postgres/payment_intent_repository.go`, `core-api/internal/infrastructure/postgres/payment_intent_status_history_repository.go`, `scripts/demo.sh` |
| `feature/risk-event-merchant-context` | Valentina | Agrega `merchant_status` y `merchant_recent_intents` al evento `risk.evaluation.requested`, calculados antes de persistir el intent para no contarse a sí mismo (ver ADR-0004). Corrige un bug real encontrado al verificar: el producer de Kafka nunca serializaba estos dos campos, así que se calculaban pero se perdían al publicar. | `core-api/internal/application/payment_intent_service.go`, `core-api/internal/infrastructure/kafka/producer.go` |
| `feature/payment-intent-status-endpoint` | Valentina | Expone `PATCH /payment-intents/{id}/status`, que el `reconciliation-worker` de Sergio ya esperaba y tenía probado. Separa "ya estaba en ese estado" (`409`) de una transición genuinamente inválida (`422`) — antes ambas mapeaban al mismo error. Verificado contra el worker real, no solo con tests propios. | `core-api/internal/application/payment_intent_service.go`, `core-api/internal/transport/http/payment_intent_handler.go`, `core-api/internal/transport/http/errors.go` |
| `feature/redis-idempotency-lock` | Valentina | Lock real de idempotencia en Redis (`SET NX` + TTL + liberación por script Lua), reemplazando el no-op. Best-effort a propósito: Postgres sigue siendo la garantía final (ADR-0002). | `core-api/internal/infrastructure/redis/locker.go` |
| `feature/core-api-healthcheck-and-roles` | Valentina | `core-api` conecta a Postgres como `mova_app` (no el superusuario); healthcheck real de Docker contra `/readiness`, que ahora también revisa Postgres. `db-migrator` se queda con el superusuario a propósito — la migración 0001 crea los roles y eso solo lo puede hacer uno. | `docker-compose.yml`, `core-api/internal/transport/http/readiness_handler.go` |
| `feature/risk-service-merchant-context` | Sergio | `risk-service` empieza a usar `merchant_status` (regla de comercio bloqueado, tratando ausente/desconocido como "no bloqueado") y prefiere `merchant_recent_intents` de `core-api` sobre su propia ventana en memoria, conservada como respaldo. `model_version` sube a `rules-v2`. | `risk-service/app/domain/rules.py`, `risk-service/app/application/evaluate.py`, `risk-service/app/domain/models.py` |
| `feature/casos-de-prueba-y-postman` | Sergio | Los 8 casos del enunciado narrados en `scripts/casos-de-prueba.sh`, corriendo contra el sistema real (para/levanta `risk-service` en vivo para el caso de fallo de dependencia). Colección de Postman ampliada a 33 requests/55 assertions con comercios, los tres canales y conciliación real. | `scripts/casos-de-prueba.sh`, `docs/postman/MOVA.postman_collection.json`, `docs/postman/README.md` |
| `feature/docs-final-openapi-adr` | Valentina | Cierre de documentación: OpenAPI de `core-api` al día (comercios, `PATCH /status` ya no pendiente, `/readiness` revisa Postgres) servido en vivo en `/docs`, diagramas de decisión y mecanismo en ADR-0001/0002/0003 (issue #16), y el README puesto al día con el estado real del sistema. | `docs/openapi/core-api-v1.yaml`, `docs/adr/0001-contrato-go-python.md`, `docs/adr/0002-idempotencia.md`, `docs/adr/0003-politica-risk-service-caido.md`, `core-api/internal/transport/http/docs_handler.go`, `README.md` |
| `feature/kafka-resilience` | Valentina | Circuit breaker Kafka→HTTP (ADR-0008): si Kafka mismo falla, cae al endpoint síncrono de `risk-service` y aplica la decisión de inmediato — verificado en vivo parando el contenedor de Kafka. Verificando esto en vivo apareció un segundo hallazgo real: el consumer de `risk.evaluation.completed` tampoco se reconectaba solo tras perder la conexión — corregido con el mismo patrón de backoff con jitter del worker de Sergio, también verificado en vivo. | `core-api/internal/infrastructure/kafka/breaker.go`, `core-api/internal/infrastructure/kafka/consumer.go`, `core-api/internal/infrastructure/riskhttp/`, `docs/adr/0008-circuit-breaker-kafka-http.md` |
| `feature/core-api-health-and-metrics` | Valentina | `core-api` era el único de los cuatro servicios sin `/health` ni `/metrics`, y por eso no aparecía en el dashboard de Grafana. Se agregan los dos, con contadores reales (intents creados, resueltos por decisión y por camino Kafka/HTTP directo, resultado de cada publish) conectados al código, no decorativos — verificado en vivo creando un intent real y viendo los contadores moverse, y confirmando el target `core-api` como `up` en Prometheus. | `core-api/internal/infrastructure/metrics/metrics.go`, `core-api/internal/transport/http/{health_handler.go,metrics_handler.go,router.go,payment_intent_handler.go}`, `core-api/internal/infrastructure/kafka/producer.go`, `observability/prometheus/prometheus.yml` |
