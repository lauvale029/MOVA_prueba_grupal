-- Lo que la aplicacion no puede saltarse.
--
-- La validacion en el dominio solo protege lo que pasa por el dominio. Un
-- UPDATE desde psql, una migracion mal escrita o un script de soporte se
-- la saltan entera. En un sistema financiero eso importa: si el historial
-- se puede editar, no es evidencia de nada.

-- 1. Transiciones ilegales, rechazadas vengan de donde vengan ------------

CREATE FUNCTION payments.enforce_status_transition() RETURNS trigger AS $$
BEGIN
    IF NEW.status = OLD.status THEN
        RETURN NEW;
    END IF;

    IF NOT EXISTS (
        SELECT 1 FROM payments.payment_status_transitions
         WHERE from_status = OLD.status AND to_status = NEW.status
    ) THEN
        RAISE EXCEPTION 'transicion invalida: % -> % (intent %)',
            OLD.status, NEW.status, OLD.id
            USING ERRCODE = 'check_violation';
    END IF;

    NEW.updated_at := now();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_intent_transition
    BEFORE UPDATE OF status ON payments.payment_intents
    FOR EACH ROW EXECUTE FUNCTION payments.enforce_status_transition();


-- 2. El historial es append-only ----------------------------------------
--
-- Redundante con el REVOKE de 0005 a proposito: el privilegio no cuesta
-- nada en tiempo de ejecucion y cubre el caso normal; el trigger cubre el
-- dia en que alguien conecte con otro rol.

CREATE FUNCTION payments.reject_history_mutation() RETURNS trigger AS $$
BEGIN
    RAISE EXCEPTION 'payment_intent_status_history es append-only (intento de %)', TG_OP
        USING ERRCODE = 'restrict_violation';
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_history_immutable
    BEFORE UPDATE OR DELETE ON payments.payment_intent_status_history
    FOR EACH ROW EXECUTE FUNCTION payments.reject_history_mutation();


-- 3. Ningun cambio de estado sin su fila de historial --------------------
--
-- CONSTRAINT TRIGGER DEFERRABLE: valida al COMMIT y no en medio de la
-- transaccion, porque el UPDATE del intent ocurre ANTES del INSERT del
-- historial. Si alguien anade un caso de uso que cambia el estado y olvida
-- el historial, la transaccion falla entera.

CREATE FUNCTION payments.require_history_row() RETURNS trigger AS $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM payments.payment_intent_status_history
         WHERE payment_intent_id = NEW.id
           AND new_status = NEW.status
    ) THEN
        RAISE EXCEPTION 'cambio de estado sin historial (intent %, estado %)', NEW.id, NEW.status
            USING ERRCODE = 'restrict_violation';
    END IF;
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

CREATE CONSTRAINT TRIGGER trg_intent_requires_history
    AFTER INSERT OR UPDATE OF status ON payments.payment_intents
    DEFERRABLE INITIALLY DEFERRED
    FOR EACH ROW EXECUTE FUNCTION payments.require_history_row();
