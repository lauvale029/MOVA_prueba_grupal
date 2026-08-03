# Colección de Postman

33 peticiones en 6 carpetas con **55 aserciones**. No es un catálogo de
endpoints: cada petición afirma algo que se puede romper sin darse cuenta.

| Archivo | Qué es |
|---|---|
| [`MOVA.postman_collection.json`](MOVA.postman_collection.json) | La colección |
| [`MOVA.postman_environment.json`](MOVA.postman_environment.json) | Urls y credenciales locales |

## Cómo se corre

Con el sistema levantado (`cp .env.example .env && docker compose up -d`):

```bash
newman run docs/postman/MOVA.postman_collection.json -e docs/postman/MOVA.postman_environment.json
```

Sale distinto de cero si alguna aserción falla, así que sirve en CI y no solo
en una pantalla. Desde la interfaz de Postman: importar los dos archivos y
correr la colección entera con el runner.

## El orden importa

Las carpetas están numeradas porque **cada una siembra lo que necesita la
siguiente**:

```
1 · Autenticacion   →  deja el JWT en {{token}}
3 · Comercios       →  deja el comercio en {{merchant_id}}
4 · Payment Intents →  deja el intent en {{intent_id}}
5 · Conciliacion    →  crea el suyo en {{expirable_id}}
```

Correr una petición suelta sin haber pasado por su carpeta previa falla, y no
es un defecto: **desde la migración `0006` un pago necesita un comercio real**,
porque hay una clave foránea. Un `merchant_id` inventado devuelve `404`.

## Qué demuestra cada carpeta

| Carpeta | Peticiones | Demuestra |
|---|---|---|
| **0 · Salud** | 4 | `/health` y `/readiness` no son lo mismo: el primero dice que el proceso vive, el segundo que además puede trabajar |
| **1 · Autenticación** | 2 | El JWT se emite y los endpoints protegidos devuelven `401` sin él |
| **2 · Reglas de riesgo** | 6 | Las cuatro decisiones contra `risk-service` directo, sin Kafka: `APPROVE`, `REVIEW`, `REJECT` por referencia y `REJECT` por comercio bloqueado |
| **3 · Comercios** | 4 | Crear, consultar, `409` por documento duplicado y `404` por inexistente |
| **4 · Payment Intents** | 11 | Creación, reintento idempotente, historial atribuido, `409` por referencia duplicada, los tres canales admitidos y los `422` de validación |
| **5 · Conciliación** | 6 | Lo que hace el worker en cada tick: listar abiertos, expirar, y los tres códigos del contrato — `200`, `409` y `422` |

## Detalles que no son obvios

**El entorno solo lleva configuración.** Urls y credenciales, nunca estado de
ejecución. Si el entorno definiera un `token` vacío, pisaría al que escribe el
login —las variables de entorno tienen prioridad sobre las de colección— y todo
respondería `401`.

**`Sin token → 401` lleva `auth: noauth`.** La colección tiene autenticación
bearer heredada; sin desactivarla en esa petición concreta, viajaría *con* token
y la prueba no probaría nada.

**Las URL llevan `host` y `path` además del `raw`.** Sin el desglose, `newman`
considera la url vacía y no dispara la petición — la interfaz de Postman lo
tolera, la línea de comandos no.

**El intent de conciliación se crea aparte**, con monto alto para que el riesgo
lo deje en `UNDER_REVIEW`. Los demás ya están resueltos cuando llega esa carpeta,
y desde `APPROVED` no se puede expirar: la tabla de transiciones no lo permite.

## Qué no cubre, y con qué se cubre

| | Dónde |
|---|---|
| Concurrencia y caída del Risk Service | [`scripts/casos-de-prueba.sh`](../../scripts/casos-de-prueba.sh) — Postman es secuencial y no puede parar contenedores |
| Garantías del esquema con `psql` | [`scripts/demo.sh`](../../scripts/demo.sh) |
| Reglas de riesgo unitarias | `risk-service/tests/` — 50 pruebas sin red |

## Si se añade una petición

- Lleva al menos un `pm.test`. Una petición sin aserción es documentación, no
  prueba, y la colección ya sirve de documentación por otro lado.
- Va en la carpeta cuyo estado necesita, o siembra el suyo.
- La url, desglosada. Si se escribe a mano en la interfaz, exportar de nuevo.
- Correr `newman` antes de mergear: en la interfaz pasan cosas que en línea de
  comandos no.
