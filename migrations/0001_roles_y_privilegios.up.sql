-- Roles antes que tablas.
--
-- El rol de migraciones es DISTINTO del de la aplicacion, y esa es la
-- decision de la que dependen todas las demas: si core-api conectara como
-- dueno del esquema, podria re-otorgarse permisos y deshabilitar los
-- triggers, y todo lo que viene despues seria decorativo.
--
-- Se ejecuta como superusuario, una sola vez.

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'mova_owner') THEN
        CREATE ROLE mova_owner LOGIN PASSWORD 'mova_owner';
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'mova_app') THEN
        CREATE ROLE mova_app LOGIN PASSWORD 'mova_app';
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'mova_auditor') THEN
        CREATE ROLE mova_auditor LOGIN PASSWORD 'mova_auditor';
    END IF;
END
$$;

CREATE SCHEMA IF NOT EXISTS payments AUTHORIZATION mova_owner;

-- Nadie tiene nada por defecto. Lo que haga falta se concede explicito
-- en 0005_grants.
REVOKE ALL ON SCHEMA payments FROM PUBLIC;
GRANT USAGE ON SCHEMA payments TO mova_app, mova_auditor;

-- Para que la aplicacion no tenga que calificar el esquema en cada query.
ALTER ROLE mova_app SET search_path TO payments, public;
ALTER ROLE mova_auditor SET search_path TO payments, public;
ALTER ROLE mova_owner SET search_path TO payments, public;
