DROP INDEX IF EXISTS bill_payment_reversals_refund_idx;
ALTER TABLE bill_payment_reversals DROP COLUMN IF EXISTS refund_id;
DROP TABLE IF EXISTS bill_refunds;
