-- Las transiciones legales son FILAS, no codigo.
--
-- El dominio Go las valida antes de escribir; el trigger de 0004 las
-- valida al escribir, contra esta misma tabla. Como son datos, una prueba
-- de integracion puede leerlas y compararlas con el mapa de
-- core-api/internal/domain/payment_intent.go: si alguien anade una
-- transicion en un solo lado, falla.

CREATE TABLE payments.payment_status_transitions (
    from_status payments.payment_status NOT NULL,
    to_status   payments.payment_status NOT NULL,
    description TEXT NOT NULL,
    PRIMARY KEY (from_status, to_status)
);

-- Copiadas de allowedTransitions en el dominio. Todo pago pasa por
-- UNDER_REVIEW al enviarse a riesgo: PENDING -> APPROVED directo no
-- existe, y por eso ningun pago puede aprobarse sin evaluacion (ADR-0001).
INSERT INTO payments.payment_status_transitions VALUES
  ('PENDING',      'UNDER_REVIEW', 'enviado a evaluacion de riesgo'),
  ('PENDING',      'CANCELLED',    'cancelado antes de evaluarse'),
  ('PENDING',      'EXPIRED',      'vencido sin llegar a evaluarse'),
  ('UNDER_REVIEW', 'APPROVED',     'riesgo APPROVE'),
  ('UNDER_REVIEW', 'REJECTED',     'riesgo REJECT'),
  ('UNDER_REVIEW', 'EXPIRED',      'el riesgo nunca respondio (ADR-0003)');

-- APPROVED, REJECTED, CANCELLED y EXPIRED no aparecen nunca como
-- from_status: son terminales porque no tienen salida en esta tabla, no
-- porque alguien lo recuerde.
