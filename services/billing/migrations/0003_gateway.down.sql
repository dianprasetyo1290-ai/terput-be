-- 0003 down: drops the gateway tables. Payments recorded through the gateway,
-- the cashier or deposits cannot exist without their source tables, so their
-- ledger rows are removed before the proof columns become NOT NULL again.

DELETE FROM bill_payment_reversals
 WHERE payment_id IN (SELECT id FROM bill_payments WHERE source <> 'upload');
DELETE FROM bill_payment_allocations
 WHERE payment_id IN (SELECT id FROM bill_payments WHERE source <> 'upload');
DELETE FROM bill_payments WHERE source <> 'upload';

DROP INDEX IF EXISTS bill_payments_source_idx;
ALTER TABLE bill_payments
    DROP CONSTRAINT IF EXISTS bill_payments_source_id_required,
    DROP CONSTRAINT IF EXISTS bill_payments_proof_required,
    ALTER COLUMN object_key SET NOT NULL,
    ALTER COLUMN mime SET NOT NULL,
    ALTER COLUMN size_bytes SET NOT NULL,
    ALTER COLUMN sha256 SET NOT NULL,
    DROP COLUMN IF EXISTS source_id,
    DROP COLUMN IF EXISTS source;

DROP TABLE IF EXISTS deposit_movements;
DROP TABLE IF EXISTS student_deposits;
DROP TABLE IF EXISTS gateway_job_runs;
DROP TABLE IF EXISTS gateway_journal_postings;
DROP TABLE IF EXISTS gateway_accounts;
DROP TABLE IF EXISTS gateway_settings;
DROP TABLE IF EXISTS bank_statement_lines;
DROP TABLE IF EXISTS reconciliation_exceptions;
DROP TABLE IF EXISTS settlement_items;
DROP TABLE IF EXISTS settlement_batches;
DROP TABLE IF EXISTS payment_corrections;
DROP TABLE IF EXISTS cashier_payments;
DROP TABLE IF EXISTS receipt_sequences;
DROP TABLE IF EXISTS receipts;
DROP TABLE IF EXISTS gateway_notifications;
DROP TABLE IF EXISTS gateway_transaction_bills;
DROP TABLE IF EXISTS gateway_transactions;
DROP TABLE IF EXISTS gateway_tx_sequences;
DROP TABLE IF EXISTS payment_methods;
