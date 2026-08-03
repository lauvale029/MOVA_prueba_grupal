#!/usr/bin/env bash
# Los ocho casos que el enunciado manda demostrar (seccion 9 de la prueba de
# equipo), en el mismo orden y con los mismos nombres del documento.
#
#   ./scripts/casos-de-prueba.sh
#
# Requiere el sistema levantado:  cp .env.example .env && docker compose up -d
#
# Se diferencia de demo.sh en el proposito: demo.sh recorre el sistema entero
# para CI, y este esta pensado para la sustentacion — un bloque por fila de la
# tabla del enunciado, para poder ir marcandolas mientras corre.
#
# Dos casos manipulan el entorno a proposito (paran risk-service, mueven el
# reloj de expiracion). Lo dejan como estaba al terminar.
set -uo pipefail

CORE="${CORE_URL:-http://localhost:8095}"
RISK="${RISK_URL:-http://localhost:8081}"
USUARIO="${AUTH_USERNAME:-svc}"
CLAVE="${AUTH_PASSWORD:-svc}"

ok=0; fallos=0
verde() { printf "  \033[32m✓\033[0m %s\n" "$1"; ok=$((ok+1)); }
rojo()  { printf "  \033[31m✗\033[0m %s\n" "$1"; fallos=$((fallos+1)); }
nota()  { printf "    \033[90m%s\033[0m\n" "$1"; }
caso()  { printf "\n\033[1m%s\033[0m\n" "$1"; }
jq_()   { python3 -c "import sys,json;d=json.load(sys.stdin);print(d$1)" 2>/dev/null; }

limpiar() { docker compose start risk-service >/dev/null 2>&1 || true; }
trap limpiar EXIT

# --- preparacion --------------------------------------------------------

TOKEN=$(curl -s "$CORE/api/v1/auth/login" -H 'content-type: application/json' \
  -d "{\"username\":\"$USUARIO\",\"password\":\"$CLAVE\"}" | jq_ "['token']")
if [ -z "$TOKEN" ]; then
  echo "No hay token: ¿esta el sistema levantado? (docker compose up -d)" >&2
  exit 1
fi
AUTH=(-H "Authorization: Bearer $TOKEN")

MERCHANT=$(curl -s -X POST "$CORE/api/v1/merchants" "${AUTH[@]}" \
  -H 'content-type: application/json' \
  -d "{\"name\":\"Comercio Sustentacion\",\"document_number\":\"900$(date +%s)\",\"email\":\"s@example.com\"}" \
  | jq_ "['id']")
if [ -z "$MERCHANT" ]; then
  echo "No se pudo crear el comercio de apoyo" >&2
  exit 1
fi
printf "\033[90mComercio de apoyo: %s\033[0m\n" "$MERCHANT"

# crear <referencia> <monto> [idempotency-key] [merchant] -> cuerpo de la respuesta
crear() {
  curl -s -X POST "$CORE/api/v1/payment-intents" "${AUTH[@]}" \
    -H 'content-type: application/json' -H "Idempotency-Key: ${3:-$(uuidgen)}" \
    -d "{\"merchant_id\":\"${4:-$MERCHANT}\",\"external_reference\":\"$1\",\"amount_minor\":$2,\"currency\":\"COP\",\"channel\":\"QR\"}"
}

# esperar_resolucion <id> -> el intent cuando sale de PENDING/UNDER_REVIEW
esperar_resolucion() {
  local actual estado
  for _ in $(seq 1 25); do
    actual=$(curl -s "$CORE/api/v1/payment-intents/$1" "${AUTH[@]}")
    estado=$(echo "$actual" | jq_ "['status']")
    [ "$estado" != "PENDING" ] && [ "$estado" != "UNDER_REVIEW" ] && break
    sleep 1
  done
  echo "$actual"
}

# --- 1 · Caso feliz -----------------------------------------------------
# "Crear un pago por QR, recibir APPROVE, aprobarlo y consultar historial."

caso "1 · Caso feliz"
FELIZ=$(crear "FELIZ-$(date +%s)" 150000)
ID_FELIZ=$(echo "$FELIZ" | jq_ "['id']")
[ "$(echo "$FELIZ" | jq_ "['channel']")" = "QR" ] \
  && verde "creado por QR en $(echo "$FELIZ" | jq_ "['status']")" || rojo "no se creó por QR"

RESUELTO=$(esperar_resolucion "$ID_FELIZ")
EST=$(echo "$RESUELTO" | jq_ "['status']")
DEC=$(echo "$RESUELTO" | jq_ "['risk_decision']")
[ "$EST" = "APPROVED" ] && [ "$DEC" = "APPROVE" ] \
  && verde "riesgo APPROVE → estado APPROVED (score $(echo "$RESUELTO" | jq_ "['risk_score']"))" \
  || rojo "esperaba APPROVED/APPROVE y quedó $EST/$DEC"

curl -s "$CORE/api/v1/payment-intents/$ID_FELIZ/history" "${AUTH[@]}" > /tmp/mova-hist.json
python3 - <<'PY'
import json
for e in json.load(open("/tmp/mova-hist.json")):
    print(f"      {e['previous_status'] or '—':<13} → {e['new_status']:<13} {e['changed_by']:<16} {e['reason']}")
PY
[ "$(python3 -c "
import json;h=json.load(open('/tmp/mova-hist.json'))
print('ok' if h and all(e['changed_by'] and e['reason'] and e['correlation_id'] and e['created_at'] for e in h) else 'fail')")" = "ok" ] \
  && verde "historial completo: actor, motivo, correlación y fecha en cada entrada" \
  || rojo "hay entradas del historial sin atribuir"

# --- 2 · Reintento ------------------------------------------------------
# "Repetir la solicitud con la misma idempotency_key y obtener el mismo recurso."

caso "2 · Reintento"
IDEM=$(uuidgen); REF="REINTENTO-$(date +%s)"
UNO=$(crear "$REF" 150000 "$IDEM" | jq_ "['id']")
DOS=$(crear "$REF" 150000 "$IDEM" | jq_ "['id']")
[ -n "$UNO" ] && [ "$UNO" = "$DOS" ] \
  && verde "la misma llave devuelve el mismo intent ($UNO)" \
  || rojo "la segunda llamada devolvió $DOS"

COD=$(curl -s -o /dev/null -w '%{http_code}' -X POST "$CORE/api/v1/payment-intents" "${AUTH[@]}" \
  -H 'content-type: application/json' -H "Idempotency-Key: $(uuidgen)" \
  -d "{\"merchant_id\":\"$MERCHANT\",\"external_reference\":\"$REF\",\"amount_minor\":150000,\"currency\":\"COP\",\"channel\":\"QR\"}")
[ "$COD" = "409" ] \
  && verde "llave nueva + misma referencia → 409 (merchant_id + external_reference es único)" \
  || rojo "esperaba 409 por referencia duplicada y dio $COD"

# --- 3 · Concurrencia ---------------------------------------------------
# "Lanzar solicitudes paralelas y comprobar que existe un solo Payment Intent."

caso "3 · Concurrencia"
IDEM_C=$(uuidgen); REF_C="CONCURRENTE-$(date +%s)"
TMP=$(mktemp -d)
for i in $(seq 1 10); do
  ( crear "$REF_C" 150000 "$IDEM_C" | jq_ "['id']" > "$TMP/$i" ) &
done
wait
DISTINTOS=$(sort -u "$TMP"/* 2>/dev/null | grep -c .)
nota "10 solicitudes en paralelo con la misma Idempotency-Key"
[ "$DISTINTOS" = "1" ] \
  && verde "resultó 1 solo Payment Intent ($(cat "$TMP"/1))" \
  || rojo "se crearon $DISTINTOS intents distintos"
rm -rf "$TMP"

# La garantia final es la base, no Redis: se comprueba que solo hay una fila.
FILAS=$(docker compose exec -T postgres psql -U "${DB_USER:-mova}" -d "${DB_NAME:-mova_orchestrator}" -tAc \
  "SELECT count(*) FROM payments.payment_intents WHERE idempotency_key = '$IDEM_C';" 2>/dev/null | tr -d '[:space:]')
[ "$FILAS" = "1" ] \
  && verde "y una sola fila en PostgreSQL, que es donde vive la garantía" \
  || rojo "la base tiene $FILAS filas con esa llave"
nota "el lock de Redis es una optimización; quien impide el duplicado es"
nota "uq_intent_idempotency (ADR-0002)"

# --- 4 · Revisión -------------------------------------------------------
# "Monto alto produce UNDER_REVIEW y conserva razones del score."

caso "4 · Revisión"
ID_ALTO=$(crear "ALTO-$(date +%s)" 50000000 | jq_ "['id']")
sleep 6
REV=$(curl -s "$CORE/api/v1/payment-intents/$ID_ALTO" "${AUTH[@]}")
EST=$(echo "$REV" | jq_ "['status']")
[ "$EST" = "UNDER_REVIEW" ] \
  && verde "monto alto (50.000.000) → se queda en UNDER_REVIEW" \
  || rojo "esperaba UNDER_REVIEW y quedó $EST"
RAZONES=$(echo "$REV" | jq_ "['risk_reason_codes']")
[ "$(echo "$REV" | jq_ "['risk_decision']")" = "REVIEW" ] && [ -n "$RAZONES" ] && [ "$RAZONES" != "None" ] \
  && verde "conserva score $(echo "$REV" | jq_ "['risk_score']") y razones $RAZONES" \
  || rojo "no conservó la decisión y las razones del score"

# --- 5 · Rechazo --------------------------------------------------------
# "Señal de riesgo bloqueante produce REJECTED sin registrar aprobación."

caso "5 · Rechazo"
ID_MALO=$(crear "FRAUD-$(date +%s)" 150000 | jq_ "['id']")
MALO=$(esperar_resolucion "$ID_MALO")
EST=$(echo "$MALO" | jq_ "['status']")
[ "$EST" = "REJECTED" ] \
  && verde "referencia sospechosa → REJECTED $(echo "$MALO" | jq_ "['risk_reason_codes']")" \
  || rojo "esperaba REJECTED y quedó $EST"

curl -s "$CORE/api/v1/payment-intents/$ID_MALO/history" "${AUTH[@]}" > /tmp/mova-malo.json
python3 -c "
import json;h=json.load(open('/tmp/mova-malo.json'))
print('limpio' if not any(e['new_status']=='APPROVED' for e in h) else 'sucio')" | grep -q limpio \
  && verde "el historial nunca pasó por APPROVED" \
  || rojo "el historial registra una aprobación intermedia"

# La otra señal bloqueante del enunciado: comercio bloqueado.
BLOQ=$(curl -s -X POST "$CORE/api/v1/merchants" "${AUTH[@]}" -H 'content-type: application/json' \
  -d "{\"name\":\"Bloqueado\",\"document_number\":\"901$(date +%s)\",\"email\":\"b@example.com\"}" | jq_ "['id']")
docker compose exec -T postgres psql -U "${DB_USER:-mova}" -d "${DB_NAME:-mova_orchestrator}" -qc \
  "UPDATE payments.merchants SET status='INACTIVE' WHERE id='$BLOQ';" >/dev/null 2>&1
ID_BLOQ=$(crear "LIMPIA-$(date +%s)" 150000 "$(uuidgen)" "$BLOQ" | jq_ "['id']")
RES_BLOQ=$(esperar_resolucion "$ID_BLOQ")
[ "$(echo "$RES_BLOQ" | jq_ "['status']")" = "REJECTED" ] \
  && verde "comercio INACTIVE → REJECTED $(echo "$RES_BLOQ" | jq_ "['risk_reason_codes']") con referencia y monto limpios" \
  || rojo "un comercio bloqueado no produjo REJECTED"

# --- 6 · Expiración -----------------------------------------------------
# "El worker detecta un PENDING vencido y solicita EXPIRED o REJECTED."

caso "6 · Expiración"
ID_EXP=$(crear "EXPIRA-$(date +%s)" 50000000 | jq_ "['id']")
sleep 6
nota "un ciclo real del worker, con la ventana de vencimiento forzada a 0 min"
SALIDA=$(docker compose run --rm -e EXPIRY_OVERRIDE_MINUTES=0 \
  reconciliation-worker python -m worker.run_once 2>&1)
CODIGO=$?
echo "$SALIDA" | grep -E "Conciliacion terminada" | sed 's/^/      /'
[ "$CODIGO" = "0" ] \
  && verde "el ciclo terminó limpio (código 0)" \
  || rojo "el ciclo falló con código $CODIGO"

EST=$(curl -s "$CORE/api/v1/payment-intents/$ID_EXP" "${AUTH[@]}" | jq_ "['status']")
[ "$EST" = "EXPIRED" ] \
  && verde "el intent abierto quedó en EXPIRED" \
  || rojo "esperaba EXPIRED y quedó $EST"

ULTIMA=$(curl -s "$CORE/api/v1/payment-intents/$ID_EXP/history" "${AUTH[@]}" | jq_ "[-1]['changed_by']")
[ -n "$ULTIMA" ] && verde "y con actor en el historial: $ULTIMA" || rojo "la expiración no quedó atribuida"
nota "el worker no escribió en la base: pidió la transición por la API y el"
nota "core la validó contra su tabla de transiciones"

# --- 7 · Fallo de dependencia -------------------------------------------
# "Simular caída o timeout del Risk Service y mostrar el comportamiento seguro."

caso "7 · Fallo de dependencia"
nota "se detiene risk-service y se crea un pago con el riesgo caído"
docker compose stop risk-service >/dev/null 2>&1
ID_HUERFANO=$(crear "SIN-RIESGO-$(date +%s)" 150000 | jq_ "['id']")
sleep 8
CAIDO=$(curl -s "$CORE/api/v1/payment-intents/$ID_HUERFANO" "${AUTH[@]}")
EST=$(echo "$CAIDO" | jq_ "['status']")
[ "$EST" = "UNDER_REVIEW" ] \
  && verde "con el riesgo caído el pago se queda en UNDER_REVIEW (estado seguro)" \
  || rojo "esperaba UNDER_REVIEW y quedó $EST"
[ "$(echo "$CAIDO" | jq_ "['risk_decision']")" = "None" ] \
  && verde "no hay aprobación silenciosa: risk_decision sigue vacía" \
  || rojo "se resolvió sin que el riesgo respondiera"

nota "se levanta risk-service: el evento seguía esperando en el topic"
docker compose start risk-service >/dev/null 2>&1
RECUPERADO=$(esperar_resolucion "$ID_HUERFANO")
EST=$(echo "$RECUPERADO" | jq_ "['status']")
if [ "$EST" = "APPROVED" ] || [ "$EST" = "REJECTED" ]; then
  verde "al volver el servicio, el pago se resuelve solo → $EST"
else
  rojo "tras recuperar el riesgo el pago sigue en $EST"
fi
nota "Kafka retuvo el evento: no se perdió la solicitud (ADR-0003)"

# --- 8 · Transición ilegal ----------------------------------------------
# "Intentar modificar un estado terminal y recibir error de dominio consistente."

caso "8 · Transición ilegal"
transicion() {
  curl -s -o /tmp/mova-tr.json -w '%{http_code}' -X PATCH \
    "$CORE/api/v1/payment-intents/$1/status" "${AUTH[@]}" \
    -H 'content-type: application/json' -d "{\"status\":\"$2\",\"reason\":\"caso 8\"}"
}

COD=$(transicion "$ID_EXP" "PENDING")
CODIGO_ERROR=$(python3 -c "import json;print(json.load(open('/tmp/mova-tr.json'))['error']['code'])" 2>/dev/null)
[ "$COD" = "422" ] && [ "$CODIGO_ERROR" = "INVALID_TRANSITION" ] \
  && verde "EXPIRED → PENDING rechazada: 422 $CODIGO_ERROR" \
  || rojo "esperaba 422 INVALID_TRANSITION y dio $COD $CODIGO_ERROR"

COD=$(transicion "$ID_FELIZ" "REJECTED")
[ "$COD" = "422" ] \
  && verde "APPROVED → REJECTED rechazada: no se salta la máquina de estados" \
  || rojo "esperaba 422 y dio $COD"

COD=$(transicion "$ID_EXP" "EXPIRED")
[ "$COD" = "409" ] \
  && verde "repetir la transición actual → 409, que para el worker es éxito" \
  || rojo "esperaba 409 al repetir y dio $COD"

nota "la misma garantía existe en la base: el trigger rechaza el UPDATE aunque"
nota "alguien se salte la API. Lo comprueba scripts/demo.sh en su paso 10"

# --- resumen ------------------------------------------------------------

printf "\n\033[1m%d comprobaciones OK · %d fallos\033[0m\n" "$ok" "$fallos"
exit $((fallos > 0))
