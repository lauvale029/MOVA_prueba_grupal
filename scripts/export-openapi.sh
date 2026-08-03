#!/usr/bin/env bash
# Regenera docs/openapi/risk-service-v1.yaml desde el codigo.
# El de core-api se escribe a mano: es el contrato que consumimos.
set -euo pipefail
cd "$(dirname "$0")/../risk-service"
KAFKA_ENABLED=false .venv/bin/python - <<'PY' > ../docs/openapi/risk-service-v1.yaml
import yaml
from app.main import app
spec = app.openapi()
spec["info"]["description"] = (
    "Evaluacion de riesgo deterministica. El camino de produccion es Kafka "
    "(ver ADR-0001); este endpoint HTTP existe para probar y demostrar las "
    "reglas sin levantar el broker."
)
spec["servers"] = [{"url": "http://localhost:8081", "description": "local"}]
print(yaml.safe_dump(spec, sort_keys=False, allow_unicode=True))
PY
echo "docs/openapi/risk-service-v1.yaml regenerado"
