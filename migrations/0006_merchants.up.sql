-- Comercios. Faltaba: payment_intents.merchant_id era un UUID suelto,
-- sin nada que referenciar (ver docs/esquema-de-datos.md §11).

CREATE TYPE payments.merchant_status AS ENUM ('ACTIVE', 'INACTIVE');

CREATE TABLE payments.merchants (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name            TEXT NOT NULL,
    document_number TEXT NOT NULL,
    email           TEXT NOT NULL,
    status          payments.merchant_status NOT NULL DEFAULT 'ACTIVE',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT uq_merchant_document UNIQUE (document_number)
);

-- ON DELETE RESTRICT: mismo criterio que el historial — un comercio con
-- pagos no se borra, se marca INACTIVE.
ALTER TABLE payments.payment_intents
    ADD CONSTRAINT fk_intent_merchant FOREIGN KEY (merchant_id)
        REFERENCES payments.merchants (id) ON DELETE RESTRICT;

GRANT SELECT, INSERT, UPDATE ON payments.merchants TO mova_app;
REVOKE DELETE, TRUNCATE ON payments.merchants FROM mova_app;
GRANT SELECT ON payments.merchants TO mova_auditor;
