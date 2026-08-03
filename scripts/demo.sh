#!/usr/bin/env bash
# Guion de demo end-to-end. Cada paso AFIRMA algo y sale distinto de cero si
# falla, para que sirva en CI y no solo en una pantalla.
#
#   ./scripts/demo.sh
#
# Requiere el sistema levantado: docker compose up -d
set -uo pipefail

CORE="${CORE_URL:-http://localhost:8095}"
RISK="${RISK_URL:-http://localhost:8081}"
USER="${AUTH_USERNAME:-svc}"
PASS="${AUTH_PASSWORD:-svc}"

ok=0; fallos=0
verde() { printf "  \033[32m✓\033[0m %s\n" "$1"; ok=$((ok+1)); }
rojo()  { printf "  \033[31m✗\033[0m %s\n" "$1"; fallos=$((fallos+1)); }
paso()  { printf "\n\033[1m%s\033[0m\n" "$1"; }

# En Windows, "python3" suele ser el alias roto de la Store (falla en
# ejecución aunque exista en el PATH) — se prueba de verdad, no solo con
# command -v, y se cae a "python" si hace falta.
PY=python3
python3 -c "" >/dev/null 2>&1 || PY=python
jq_()    { "$PY" -c "import sys,json;d=json.load(sys.stdin);print(d$1)"; }
# uuidgen no existe en Git Bash de Windows — se genera con Python en ese caso.
uuid_()  { command -v uuidgen >/dev/null 2>&1 && uuidgen || "$PY" -c "import uuid;print(uuid.uuid4())"; }

paso "0 · Salud"
[ "$(curl -s -o /dev/null -w '%{http_code}' "$CORE/readiness")" = 200 ] \
  && verde "core-api listo" || rojo "core-api no responde"
[ "$(curl -s -o /dev/null -w '%{http_code}' "$RISK/readiness")" = 200 ] \
  && verde "risk-service conectado a Kafka" || rojo "risk-service no listo"

paso "1 · Reglas de riesgo (directo, sin Kafka)"
evaluar() {
  curl -s "$RISK/api/v1/risk-evaluations" -H 'content-type: application/json' \
    -d "{\"payment_intent_id\":\"$1\",\"merchant_id\":\"m-$1\",\"external_reference\":\"$2\",\"amount_minor\":$3,\"currency\":\"COP\",\"channel\":\"QR\"}"
}
for caso in "bajo|ORDER-1|150000|APPROVE" "alto|ORDER-2|50000000|REVIEW" "sospechoso|FRAUD-9|150000|REJECT"; do
  IFS='|' read -r id ref monto esperado <<< "$caso"
  real=$(evaluar "$id" "$ref" "$monto" | jq_ "['decision']")
  [ "$real" = "$esperado" ] && verde "$ref ($monto) → $real" || rojo "$ref esperaba $esperado y dio $real"
done

paso "2 · Autenticación"
TOKEN=$(curl -s "$CORE/api/v1/auth/login" -H 'content-type: application/json' \
  -d "{\"username\":\"$USER\",\"password\":\"$PASS\"}" | jq_ "['token']")
[ -n "$TOKEN" ] && verde "token emitido" || rojo "login falló"
AUTH=(-H "Authorization: Bearer $TOKEN")
[ "$(curl -s -o /dev/null -w '%{http_code}' "$CORE/api/v1/payment-intents")" = 401 ] \
  && verde "sin token → 401" || rojo "los endpoints no exigen JWT"

paso "2.5 · Crear un comercio"
# payment_intents.merchant_id tiene FK contra payments.merchants desde la
# migración 0006 — hace falta un comercio real antes de poder crear intents.
DOC="900-demo-$(date +%s)"
MERCHANT=$(curl -s -X POST "$CORE/api/v1/merchants" "${AUTH[@]}" \
  -H 'content-type: application/json' \
  -d "{\"name\":\"Comercio Demo\",\"document_number\":\"$DOC\",\"email\":\"demo@comercio.test\"}" | jq_ "['id']")
[ -n "$MERCHANT" ] && verde "comercio creado $MERCHANT" || rojo "no se pudo crear el comercio"

paso "3 · Crear un Payment Intent"
IDEM=$(uuid_); REF="ORDER-DEMO-$(date +%s)"
CUERPO="{\"merchant_id\":\"$MERCHANT\",\"external_reference\":\"$REF\",\"amount_minor\":150000,\"currency\":\"COP\",\"channel\":\"QR\"}"
CREADO=$(curl -s -X POST "$CORE/api/v1/payment-intents" "${AUTH[@]}" \
  -H 'content-type: application/json' -H "Idempotency-Key: $IDEM" -d "$CUERPO")
ID=$(echo "$CREADO" | jq_ "['id']")
EST=$(echo "$CREADO" | jq_ "['status']")
[ -n "$ID" ] && verde "creado $ID en $EST" || rojo "no se creó"
[ "$(echo "$CREADO" | jq_ "['amount_minor']")" = "150000" ] \
  && verde "el monto es entero, sin decimales" || rojo "el monto no cuadra"

paso "4 · El riesgo responde por Kafka"
for _ in $(seq 1 25); do
  ACTUAL=$(curl -s "$CORE/api/v1/payment-intents/$ID" "${AUTH[@]}")
  EST=$(echo "$ACTUAL" | jq_ "['status']")
  [ "$EST" != "UNDER_REVIEW" ] && [ "$EST" != "PENDING" ] && break
  sleep 1
done
DEC=$(echo "$ACTUAL" | jq_ "['risk_decision']")
SCORE=$(echo "$ACTUAL" | jq_ "['risk_score']")
if [ "$EST" = "APPROVED" ] && [ "$DEC" = "APPROVE" ]; then
  verde "resuelto $EST · riesgo=$DEC score=$SCORE"
else
  rojo "esperaba APPROVED/APPROVE y quedó $EST/$DEC"
fi

paso "5 · Idempotencia"
ID2=$(curl -s -X POST "$CORE/api/v1/payment-intents" "${AUTH[@]}" \
  -H 'content-type: application/json' -H "Idempotency-Key: $IDEM" -d "$CUERPO" | jq_ "['id']")
[ "$ID" = "$ID2" ] && verde "la misma llave devuelve el MISMO intent" \
                   || rojo "creó un intent nuevo: $ID2"

paso "6 · Referencia externa duplicada"
COD=$(curl -s -o /dev/null -w '%{http_code}' -X POST "$CORE/api/v1/payment-intents" "${AUTH[@]}" \
  -H 'content-type: application/json' -H "Idempotency-Key: $(uuid_)" -d "$CUERPO")
[ "$COD" = "409" ] && verde "llave nueva + misma referencia → 409" || rojo "esperaba 409 y dio $COD"

paso "7 · Valores inválidos"
COD=$(curl -s -o /dev/null -w '%{http_code}' -X POST "$CORE/api/v1/payment-intents" "${AUTH[@]}" \
  -H 'content-type: application/json' -H "Idempotency-Key: $(uuid_)" \
  -d "{\"merchant_id\":\"$MERCHANT\",\"external_reference\":\"X-$(date +%s)\",\"amount_minor\":0,\"currency\":\"COP\",\"channel\":\"QR\"}")
[ "$COD" = "422" ] && verde "monto cero → 422" || rojo "esperaba 422 y dio $COD"

paso "8 · Historial con actor, motivo, correlación y fecha"
# Ruta relativa al directorio actual, no /tmp: bash (MSYS) y un Python
# nativo de Windows resuelven /tmp de forma distinta, y esto tiene que
# poder leerlo cualquiera de los dos.
HIST_FILE="demo-hist.json"
curl -s "$CORE/api/v1/payment-intents/$ID/history" "${AUTH[@]}" > "$HIST_FILE"
"$PY" - "$HIST_FILE" <<'PY'
import json, sys
h = json.load(open(sys.argv[1]))
for e in h:
    print(f"     {e['previous_status'] or 'null':<13} -> {e['new_status']:<13} by {e['changed_by']:<18} | {e['reason']}")
completo = all(e["changed_by"] and e["correlation_id"] and e["created_at"] for e in h)
print(f"__RESULT__{'ok' if h and completo else 'fail'}|{len(h)}")
PY
RES=$("$PY" -c "
import json,sys;h=json.load(open(sys.argv[1]))
print('ok' if h and all(e['changed_by'] and e['correlation_id'] and e['created_at'] for e in h) else 'fail')" "$HIST_FILE")
[ "$RES" = "ok" ] && verde "toda entrada está atribuida" || rojo "hay entradas sin atribución"
rm -f "$HIST_FILE"

paso "9 · Garantías del esquema (contra PostgreSQL)"
# Se siembra una fila (con su propio comercio, por la FK de la migración
# 0006) para que los triggers tengan sobre que actuar: sin datos, un UPDATE
# afecta 0 filas y no dispara nada.
SEED=$(uuid_)
SEED_MERCHANT=$(uuid_)
docker compose exec -T postgres psql -U mova -d mova_orchestrator -q >/dev/null 2>&1 <<SQL
BEGIN;
INSERT INTO payments.merchants (id, name, document_number, email)
VALUES ('$SEED_MERCHANT', 'Comercio Semilla', 'SEED-DOC-$SEED', 'seed@demo.test');
INSERT INTO payments.payment_intents (id, merchant_id, external_reference, idempotency_key,
  amount_minor, currency, channel, correlation_id, expires_at)
VALUES ('$SEED', '$SEED_MERCHANT', 'SEED-$SEED', '$SEED', 1000, 'COP', 'QR',
        gen_random_uuid(), now() + interval '30 min');
INSERT INTO payments.payment_intent_status_history
  (payment_intent_id, previous_status, new_status, reason, changed_by, correlation_id)
VALUES ('$SEED', NULL, 'PENDING', 'semilla de la demo', 'script:demo', gen_random_uuid());
COMMIT;
SQL

probar_sql() {
  salida=$(docker compose exec -T postgres psql -U mova -d mova_orchestrator -c "$2" 2>&1)
  echo "$salida" | grep -qE "$3" && verde "$1" || rojo "$1 — salida: $(echo "$salida" | head -1)"
}
probar_sql "transición ilegal PENDING→APPROVED rechazada" \
  "UPDATE payments.payment_intents SET status='APPROVED' WHERE id='$SEED';" "transicion invalida"
probar_sql "historial inmutable ante UPDATE" \
  "UPDATE payments.payment_intent_status_history SET reason='falseado' WHERE payment_intent_id='$SEED';" "append-only"
probar_sql "historial inmutable ante DELETE" \
  "DELETE FROM payments.payment_intent_status_history WHERE payment_intent_id='$SEED';" "append-only"
probar_sql "no se borra un intent con historial" \
  "DELETE FROM payments.payment_intents WHERE id='$SEED';" "foreign key|viola"
probar_sql "llave de idempotencia duplicada rechazada" \
  "INSERT INTO payments.payment_intents (merchant_id, external_reference, idempotency_key, amount_minor, currency, channel, correlation_id, expires_at) VALUES ('$SEED_MERCHANT', 'OTRA-$SEED', '$SEED', 1, 'COP', 'QR', gen_random_uuid(), now() + interval '30 min');" "duplicate key|uq_intent_idempotency"
probar_sql "resolver sin decisión de riesgo rechazado" \
  "INSERT INTO payments.payment_intents (merchant_id, external_reference, idempotency_key, amount_minor, currency, channel, status, correlation_id, expires_at) VALUES ('$SEED_MERCHANT', 'X-$SEED', gen_random_uuid()::text, 1, 'COP', 'QR', 'APPROVED', gen_random_uuid(), now() + interval '30 min');" "ck_intent_no_silent_resolution"
probar_sql "la aplicación no puede borrar" \
  "SET ROLE mova_app; DELETE FROM payments.payment_intents;" "permission denied"

paso "10 · PATCH de estado (worker de conciliación)"
# $SEED quedó en PENDING (paso 9, nunca se publicó a Kafka): sirve para
# probar la transición real sin carreras contra risk-service.
COD=$(curl -s -o /dev/null -w '%{http_code}' -X PATCH "$CORE/api/v1/payment-intents/$SEED/status" "${AUTH[@]}" \
  -H 'content-type: application/json' -d '{"status":"EXPIRED","reason":"vencido sin resolverse dentro de la ventana"}')
[ "$COD" = "200" ] && verde "PENDING → EXPIRED (worker) → 200" || rojo "esperaba 200 y dio $COD"

COD=$(curl -s -o /dev/null -w '%{http_code}' -X PATCH "$CORE/api/v1/payment-intents/$SEED/status" "${AUTH[@]}" \
  -H 'content-type: application/json' -d '{"status":"EXPIRED","reason":"reintento del worker"}')
[ "$COD" = "409" ] && verde "ya estaba en ese estado → 409 (el worker lo cuenta como éxito)" || rojo "esperaba 409 y dio $COD"

COD=$(curl -s -o /dev/null -w '%{http_code}' -X PATCH "$CORE/api/v1/payment-intents/$ID/status" "${AUTH[@]}" \
  -H 'content-type: application/json' -d '{"status":"EXPIRED","reason":"no debería aplicar"}')
[ "$COD" = "422" ] && verde "transición inválida ($EST → EXPIRED) → 422" || rojo "esperaba 422 y dio $COD"

paso "11 · Observabilidad"
curl -s "$RISK/metrics" | grep -q "risk_evaluations_total" \
  && verde "risk-service expone métricas" || rojo "faltan métricas de riesgo"
curl -s localhost:8082/metrics | grep -q "reconciliation_" \
  && verde "worker expone métricas" || rojo "faltan métricas del worker"
curl -s localhost:8083/metrics | grep -q "reconciliation_scheduler_ticks_total" \
  && verde "scheduler expone métricas" || rojo "faltan métricas del scheduler"
[ "$(curl -s -o /dev/null -w '%{http_code}' localhost:9090/-/healthy)" = 200 ] \
  && verde "Prometheus arriba" || rojo "Prometheus caído"
[ "$(curl -s -o /dev/null -w '%{http_code}' localhost:3000/api/health)" = 200 ] \
  && verde "Grafana arriba" || rojo "Grafana caído"

printf "\n\033[1m%d comprobaciones OK, %d fallos\033[0m\n" "$ok" "$fallos"
exit $((fallos > 0))
