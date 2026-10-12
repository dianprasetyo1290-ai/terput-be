-- 0006 down: only safe while every row still fits the T6 shape (single
-- provider 'nicepay', bills purpose, no retail/autodebit methods). Otherwise
-- it raises instead of deleting payment data.
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM gateway_transactions WHERE provider <> 'nicepay' OR purpose <> 'bills' OR mandate_id IS NOT NULL)
       OR EXISTS (SELECT 1 FROM gateway_notifications WHERE provider <> 'nicepay')
       OR EXISTS (SELECT 1 FROM reconciliation_exceptions WHERE provider <> 'nicepay')
       OR EXISTS (SELECT 1 FROM payment_methods WHERE kind IN ('retail','autodebit'))
       OR EXISTS (SELECT 1 FROM autodebit_mandates) THEN
        RAISE EXCEPTION '0006 down refused: payment data from other providers, purposes, retail or autodebit exists';
    END IF;
END $$;

DROP TABLE IF EXISTS autodebit_attempts;
DROP TABLE IF EXISTS autodebit_mandate_schemes;
DROP TABLE IF EXISTS autodebit_mandates;
DROP TABLE IF EXISTS autodebit_settings;

ALTER TABLE reconciliation_exceptions DROP CONSTRAINT reconciliation_exceptions_school_id_kind_ref_key;
ALTER TABLE reconciliation_exceptions ADD CONSTRAINT reconciliation_exceptions_school_id_kind_ref_key UNIQUE (school_id, kind, ref);
ALTER TABLE reconciliation_exceptions DROP COLUMN provider;

ALTER TABLE gateway_notifications DROP CONSTRAINT gateway_notifications_provider_ref_raw_hash_key;
ALTER TABLE gateway_notifications ADD CONSTRAINT gateway_notifications_provider_ref_raw_hash_key UNIQUE (provider_ref, raw_hash);
ALTER TABLE gateway_notifications DROP COLUMN provider;

DROP INDEX IF EXISTS gateway_transactions_retail_pending_idx;
ALTER TABLE gateway_transactions DROP CONSTRAINT gateway_transactions_provider_ref_key;
ALTER TABLE gateway_transactions ADD CONSTRAINT gateway_transactions_provider_ref_key UNIQUE (provider_ref);
ALTER TABLE gateway_transactions
    DROP CONSTRAINT gateway_transactions_purpose_ref_required,
    DROP COLUMN retail_outlet,
    DROP COLUMN pay_code,
    DROP COLUMN mandate_id,
    DROP COLUMN purpose_ref,
    DROP COLUMN purpose,
    DROP COLUMN failover_from,
    DROP COLUMN provider;

DROP TABLE IF EXISTS payment_routing_rules;
DROP TABLE IF EXISTS payment_method_provider_codes;

ALTER TABLE payment_methods DROP CONSTRAINT payment_methods_kind_check;
ALTER TABLE payment_methods ADD CONSTRAINT payment_methods_kind_check CHECK (kind IN ('va','qris','ewallet'));

DROP TABLE IF EXISTS payment_provider_capabilities;
DROP TABLE IF EXISTS payment_providers;
