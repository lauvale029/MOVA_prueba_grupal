-- Privilegios, ya con las tablas creadas.
--
-- Cuestan cero en tiempo de ejecucion —un REVOKE no se ejecuta en cada
-- peticion— y no se pueden evadir desde el codigo.

-- La aplicacion (core-api) lee y escribe intents, e inserta historial.
GRANT SELECT, INSERT, UPDATE ON payments.payment_intents               TO mova_app;
GRANT SELECT, INSERT         ON payments.payment_intent_status_history TO mova_app;
GRANT SELECT                 ON payments.payment_status_transitions    TO mova_app;

-- Solo lectura, para verificar el esquema y responder consultas de
-- negocio sin poder tocar nada.
GRANT SELECT ON ALL TABLES IN SCHEMA payments TO mova_auditor;

-- Lo que la aplicacion NO puede hacer, dicho explicito:
--   - borrar o truncar nada
--   - alterar o borrar el historial
--   - crear objetos en el esquema
REVOKE DELETE, TRUNCATE ON ALL TABLES IN SCHEMA payments        FROM mova_app;
REVOKE UPDATE, DELETE   ON payments.payment_intent_status_history FROM mova_app;
REVOKE CREATE           ON SCHEMA payments                       FROM mova_app, mova_auditor;

-- Que lo anterior siga valiendo para lo que se cree despues.
ALTER DEFAULT PRIVILEGES FOR ROLE mova_owner IN SCHEMA payments
    GRANT SELECT ON TABLES TO mova_auditor;
