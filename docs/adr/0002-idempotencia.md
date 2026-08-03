# ADR-0002: Estrategia de idempotencia y concurrencia

## Estado
Aceptado

## Contexto
Dos solicitudes concurrentes con la misma `idempotency_key` deben
devolver el mismo Payment Intent, nunca crear dos filas. La garantía no
puede depender únicamente de un componente que puede no estar disponible
(Redis).

```mermaid
flowchart TB
    q{"¿dónde vive la garantía<br/>de idempotencia?"}
    q -->|"A"| redis["solo Redis<br/>SET NX + TTL"]
    q -->|"B"| pg["solo Postgres<br/>restricción única"]
    q -->|"C"| both["las dos capas"]

    redis --> redisx["si Redis cae o el TTL<br/>expira mal, no queda<br/>ninguna garantía real"]
    pg --> pgx["correcta por sí sola, pero deja<br/>que dos requests concurrentes<br/>golpeen la base a la vez"]
    both --> bothx["Postgres es la garantía final;<br/>Redis solo evita la carrera"]

    bothx --> ok(["ELEGIDA"])

    style redisx stroke-dasharray: 4 4
    style pgx stroke-dasharray: 4 4
```

## Decisión
Estrategia de **dos capas**, reutilizando el mismo patrón validado en el
proyecto individual de Go/Python:

1. **Restricción única en Postgres sobre `idempotency_key`** — la
   garantía final. Aunque todo lo demás falle, la base de datos nunca
   permite dos filas con la misma key.
2. **Lock best-effort en Redis** (`IdempotencyLocker.Acquire`) — una
   optimización para que dos requests concurrentes no lleguen ambos a
   intentar el `INSERT` al mismo tiempo. Si el lock no se consigue, se
   espera un instante corto y se vuelve a chequear si la key ya existe
   antes de seguir.

Si la restricción de Postgres rechaza el `INSERT` por conflicto
(`ErrConflict`), `PaymentIntentService.Create` busca el intent existente
por esa `idempotency_key` y lo devuelve — nunca propaga el conflicto como
un error al cliente cuando en realidad es un reintento legítimo.

```mermaid
sequenceDiagram
    participant A as Request A
    participant B as Request B
    participant Redis as Redis
    participant PG as Postgres

    A->>Redis: SET NX idempotency-lock:key
    Redis-->>A: OK (adquirido)
    B->>Redis: SET NX idempotency-lock:key
    Redis-->>B: false (ya existe)
    Note over B: espera corta,<br/>revisa si la key ya existe
    A->>PG: INSERT ... idempotency_key
    PG-->>A: OK, fila creada
    B->>PG: SELECT ... WHERE idempotency_key
    PG-->>B: la fila que creó A
    Note over A,B: las dos devuelven<br/>el MISMO intent
```

**Si Redis no está disponible:** `IdempotencyLocker.Acquire` devuelve
`acquired=false` (ver `internal/infrastructure/redis/locker.go`) — el
mismo camino que ya toma cuando pierde la carrera contra otro request.
`PaymentIntentService.Create` espera un instante corto, revisa si la key
ya existe y, si no, sigue adelante igual: se pierde la optimización de
evitar que dos requests concurrentes golpeen la base al mismo tiempo,
pero la restricción única de Postgres sigue garantizando que nunca se
cree una fila duplicada. El sistema es correcto con o sin Redis; Redis
solo lo hace más eficiente bajo concurrencia real.

## Alternativas consideradas
- **Solo Redis (`SET NX` + TTL) sin restricción en Postgres:** se
  descartó — si Redis cae o el TTL expira en mal momento, no queda
  ninguna garantía real contra duplicados.
- **Lock pesimista en Postgres (`SELECT ... FOR UPDATE`) antes de
  insertar:** añade una consulta extra y contención en la tabla para un
  caso que la restricción única ya resuelve sin coordinación adicional.

## Consecuencias
- El comportamiento es correcto incluso sin Redis — se degrada en
  rendimiento, no en corrección.
- Probado con un test de concurrencia real (20 goroutines, misma
  `idempotency_key`, se verifica una sola fila) — ver
  `TestCreate_Concurrent_OnlyOneRowCreated`.
