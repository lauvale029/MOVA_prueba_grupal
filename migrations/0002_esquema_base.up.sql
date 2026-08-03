-- Tipos, tablas, restricciones e indices.
--
-- Los enums son de PostgreSQL y no TEXT con CHECK: un valor invalido es
-- imposible de insertar, y anadir un estado obliga a una migracion
-- revisada en vez de a un despliegue silencioso.

CREATE EXTENSION IF NOT EXISTS "pgcrypto";

CREATE TYPE payments.payment_channel AS ENUM (
    'QR', 'PAYMENT_LINK', 'DATAPHONE_SIMULATED'
);

CREATE TYPE payments.payment_status AS ENUM (
    'PENDING',        -- creado, aun no enviado a evaluacion
    'UNDER_REVIEW',   -- enviado a riesgo. Estado seguro (ADR-0003)
    'APPROVED',
    'REJECTED',
    'CANCELLED',
    'EXPIRED'
);

CREATE TYPE payments.risk_decision AS ENUM ('APPROVE', 'REVIEW', 'REJECT');


CREATE TABLE payments.payment_intents (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    merchant_id         UUID NOT NULL,
    external_reference  TEXT NOT NULL,
    idempotency_key     TEXT NOT NULL,

    amount_minor        BIGINT NOT NULL,
    currency            CHAR(3) NOT NULL DEFAULT 'COP',
    channel             payments.payment_channel NOT NULL,
    status              payments.payment_status NOT NULL DEFAULT 'PENDING',

    risk_decision       payments.risk_decision,
    risk_score          SMALLINT,
    risk_reason_codes   TEXT[],
    risk_model_version  TEXT,

    correlation_id      UUID NOT NULL,
    expires_at          TIMESTAMPTZ NOT NULL,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now(),

    -- La corrección de la idempotencia vive aqui, no en Redis (ADR-0002).
    CONSTRAINT uq_intent_idempotency  UNIQUE (idempotency_key),
    -- La pareja comercio + referencia no puede ser dos operaciones.
    CONSTRAINT uq_intent_external_ref UNIQUE (merchant_id, external_reference),

    CONSTRAINT ck_intent_amount   CHECK (amount_minor > 0),
    CONSTRAINT ck_intent_currency CHECK (currency = 'COP'),
    CONSTRAINT ck_intent_expires  CHECK (expires_at > created_at),
    CONSTRAINT ck_intent_score    CHECK (risk_score IS NULL OR risk_score BETWEEN 0 AND 100),

    -- Los tres campos de riesgo van juntos o no van.
    CONSTRAINT ck_intent_risk_complete CHECK (
        (risk_decision IS NULL) = (risk_model_version IS NULL)
    ),

    -- Ningun pago resuelto sin que el riesgo haya respondido. Es la misma
    -- garantia que da la tabla de transiciones, pero esta sobrevive a
    -- cualquier UPDATE manual y no cuesta nada.
    CONSTRAINT ck_intent_no_silent_resolution CHECK (
        status NOT IN ('APPROVED', 'REJECTED') OR risk_decision IS NOT NULL
    )
);

CREATE INDEX ix_intents_merchant        ON payments.payment_intents (merchant_id);
CREATE INDEX ix_intents_correlation     ON payments.payment_intents (correlation_id);
CREATE INDEX ix_intents_merchant_recent ON payments.payment_intents (merchant_id, created_at DESC);

-- El reconciliation-worker solo mira estos dos estados. Un indice parcial
-- cuesta una fraccion de lo que costaria uno sobre toda la tabla, que solo
-- crece.
CREATE INDEX ix_intents_open_by_expiry ON payments.payment_intents (expires_at)
    WHERE status IN ('PENDING', 'UNDER_REVIEW');


CREATE TABLE payments.payment_intent_status_history (
    id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    payment_intent_id  UUID NOT NULL,

    previous_status    payments.payment_status,   -- NULL solo en la creacion
    new_status         payments.payment_status NOT NULL,
    reason             TEXT NOT NULL DEFAULT '',
    -- Quien lo hizo: sale del subject del JWT o del nombre del servicio,
    -- nunca de un campo que el llamador envie.
    changed_by         TEXT NOT NULL,
    correlation_id     UUID NOT NULL,

    -- created_at lo pone la base, no la aplicacion: si alguien falsea una
    -- fecha de negocio, esta lo delata.
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT fk_history_intent FOREIGN KEY (payment_intent_id)
        REFERENCES payments.payment_intents (id) ON DELETE RESTRICT
);

CREATE INDEX ix_history_intent      ON payments.payment_intent_status_history
    (payment_intent_id, created_at);
CREATE INDEX ix_history_correlation ON payments.payment_intent_status_history (correlation_id);
CREATE INDEX ix_history_changed_by  ON payments.payment_intent_status_history (changed_by);
