ALTER TABLE payments.payment_intents DROP CONSTRAINT fk_intent_merchant;
DROP TABLE payments.merchants;
DROP TYPE payments.merchant_status;
