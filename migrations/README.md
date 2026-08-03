# db-migrator

Proceso de un solo uso que aplica el esquema y termina. **No es un servicio**: arranca, hace su
trabajo, devuelve `0` y desaparece. Ningún servicio de la aplicación aplica migraciones, ni siquiera
al arrancar.

| | |
|---|---|
| Imagen | `migrate/migrate:v4.18.1` |
| Responsable | Sergio |
| Conecta como | superusuario en la `0001`, luego `mova_owner` |
| `restart` | `"no"` — corre una vez y sale |

**ADR asociado:** [ADR-0007 — Migraciones como proceso aparte](../docs/adr/0007-migraciones-como-proceso-aparte.md)
· **Esquema resultante:** [`docs/esquema-de-datos.md`](../docs/esquema-de-datos.md)

## Dónde encaja

```mermaid
flowchart LR
    pg[("postgres<br/>healthy")] --> mig["db-migrator<br/>como mova_owner"]
    mig -->|"exit 0"| core["core-api<br/>como mova_app"]
    mig -->|"exit != 0"| stop(["nada arranca"])

    mig -.->|"DDL · triggers · GRANT"| esquema[("esquema payments")]
    core -.->|"SELECT · INSERT · UPDATE<br/>sin DELETE ni DDL"| esquema

    style stop stroke-dasharray: 4 4
```

En el compose:

```yaml
core-api:
  depends_on:
    db-migrator:
      condition: service_completed_successfully
```

## Por qué es un proceso aparte

**La razón no es el orden de arranque, es de privilegios.**

```mermaid
flowchart TB
    q{"¿quien aplica<br/>las migraciones?"}
    q -->|"core-api"| a["necesita DDL"]
    a --> a2["con DDL puede re-otorgarse privilegios<br/>y deshabilitar triggers"]
    a2 --> a3["las protecciones del esquema<br/>son DECORATIVAS"]

    q -->|"un proceso aparte"| b["core-api conecta como mova_app"]
    b --> b2["sin DELETE, sin TRUNCATE, sin DDL"]
    b2 --> b3["las protecciones se sostienen<br/>aunque el codigo se equivoque"]

    style a3 stroke-dasharray: 4 4
```

Toda la integridad descrita en [`docs/esquema-de-datos.md`](../docs/esquema-de-datos.md) descansa en
que el rol de migraciones y el rol de la aplicación sean distintos. Separar el proceso es lo que
hace posible separar el rol.

Hay un segundo motivo, más mundano: con varias réplicas de `core-api` arrancando a la vez, varias
intentarían migrar al mismo tiempo.

## Cómo decide

```mermaid
flowchart TB
    start(["arranque"]) --> conn{"¿postgres<br/>responde?"}
    conn -->|"no"| wait["espera al healthcheck<br/>del compose"]
    wait --> conn

    conn -->|"si"| ver["lee schema_migrations"]
    ver --> cmp{"¿hay migraciones<br/>sin aplicar?"}
    cmp -->|"no"| ok(["exit 0"])

    cmp -->|"si"| dirty{"¿la version<br/>esta dirty?"}
    dirty -->|"si"| fail(["exit != 0<br/>nada arranca"])
    dirty -->|"no"| apply["aplica la siguiente .up.sql<br/>en una transaccion"]

    apply --> res{"¿exito?"}
    res -->|"no"| rollback["rollback + marca dirty"]
    rollback --> fail
    res -->|"si"| record["registra la version"]
    record --> cmp
```

**Una migración por transacción.** Si una falla, esa migración se revierte entera y el proceso sale
con código distinto de cero, así que **ningún servicio arranca contra un esquema a medias**.

El estado `dirty` es deliberado: si una migración quedó a medias, el proceso se niega a continuar y
obliga a una intervención humana. Reintentar automáticamente sobre un esquema en estado desconocido
es la forma de convertir un problema en dos.

## Las migraciones

| Orden | Contenido | Corre como |
|---|---|---|
| `0001_roles_y_privilegios` | Crea roles, el esquema y revoca `PUBLIC` | Superusuario |
| `0002_esquema_base` | Extensiones, tipos, tablas, restricciones e índices | `mova_owner` |
| `0003_transiciones` | La tabla de transiciones y sus seis filas | `mova_owner` |
| `0004_triggers` | Los tres triggers | `mova_owner` |
| `0005_grants` | `GRANT` a cada rol, ya con las tablas creadas | `mova_owner` |

Cada `.up.sql` tiene su `.down.sql`. No es un lujo: permite probar la migración de ida y vuelta en
CI antes de que llegue a ninguna parte.

**SQL plano, no un DSL.** El esquema depende de cosas que ningún ORM expresa —`CONSTRAINT TRIGGER
DEFERRABLE`, índices parciales, `REVOKE`, funciones plpgsql— y es la única forma que Go y Python
entienden igual. En una revisión de código, un `.sql` se lee y se entiende.

## Ejecución

Va solo con el resto del sistema:

```bash
docker compose up -d
```

A mano, para aplicar o revertir:

```bash
docker compose run --rm db-migrator            # aplica todo lo pendiente
docker compose run --rm db-migrator down 1     # revierte la ultima
docker compose run --rm db-migrator version    # que version esta aplicada
```

Comprobar el resultado:

```bash
docker compose exec postgres psql -U mova -d mova_orchestrator -c "\dt payments.*"
```

## Configuración

La cadena de conexión se arma en el compose desde las variables de Postgres:

```yaml
command:
  - "-path=/migrations"
  - "-database=postgres://${DB_USER}:${DB_PASSWORD}@postgres:5432/${DB_NAME}?sslmode=disable"
  - "up"
```

**Sin `search_path` en la cadena.** El esquema `payments` no existe todavía cuando corre la `0001`,
así que declararlo ahí hace fallar la conexión con `no schema`. Las migraciones califican el esquema
explícitamente (`payments.payment_intents`) y la tabla `schema_migrations` vive en `public`.

## Reglas de escritura

| Regla | Por qué |
|---|---|
| Una migración nunca se edita después de mergearse | Ya se aplicó en la base de alguien. Se corrige con una nueva |
| El nombre dice qué hace, no cuándo | `0004_triggers`, no `0004_cambios_del_martes` |
| Los datos de demo no van en migraciones | Un seed se puede saltar en producción; una migración no |
| `CREATE INDEX CONCURRENTLY` sobre tablas con datos | Un `CREATE INDEX` normal bloquea escrituras |
| Toda `.up.sql` con su `.down.sql` | Sin ella no se puede probar la reversión |

## Qué puede salir mal

| Falla | Qué ocurre |
|---|---|
| PostgreSQL no acepta conexiones | El compose espera a su healthcheck antes de arrancar este contenedor |
| Una migración tiene un error de SQL | `ROLLBACK` de esa migración, versión marcada `dirty`, salida distinta de cero. **Ningún servicio arranca** |
| Dos `db-migrator` a la vez | El lock de aviso de `golang-migrate` deja pasar a uno; el otro espera |
| Alguien aplicó SQL a mano y `schema_migrations` no coincide | Falla al detectar la divergencia. Es el comportamiento deseado |
| Se necesita revertir en caliente | `down 1`, pero revisando antes qué datos se pierden: los `.down.sql` borran tablas |

## Limitación conocida

`core-api` **todavía no usa este esquema**: sigue con los repositorios en memoria
(`internal/infrastructure/memory/`). Las migraciones existen, se aplican y sus garantías están
verificadas contra PostgreSQL real, pero falta cambiar los repositorios respetando la interfaz de
`internal/application/ports.go`. Es el siguiente paso natural y está anotado en el README raíz.
