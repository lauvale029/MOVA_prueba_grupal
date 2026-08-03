DROP TRIGGER IF EXISTS trg_intent_requires_history ON payments.payment_intents;
DROP TRIGGER IF EXISTS trg_history_immutable ON payments.payment_intent_status_history;
DROP TRIGGER IF EXISTS trg_intent_transition ON payments.payment_intents;
DROP FUNCTION IF EXISTS payments.require_history_row();
DROP FUNCTION IF EXISTS payments.reject_history_mutation();
DROP FUNCTION IF EXISTS payments.enforce_status_transition();
