-- Revert 0004: student bill visibility and student exit tracking

DROP INDEX IF EXISTS bill_refunds_school_idempotency_idx;
ALTER TABLE bill_refunds DROP COLUMN IF EXISTS idempotency_key;

DELETE FROM bill_refunds WHERE student_bill_id IS NULL;
ALTER TABLE bill_refunds ALTER COLUMN student_bill_id SET NOT NULL;

DROP TABLE IF EXISTS student_billing_exits;
DROP TABLE IF EXISTS student_bill_visibility;
