-- 0007 down: refuses to run while ad-hoc bills exist, so no bill disappears
-- silently. Without ad-hoc rows, scheme_id becomes NOT NULL again.
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM student_bills WHERE source_module IS NOT NULL OR scheme_id IS NULL) THEN
        RAISE EXCEPTION 'cannot roll back 0007: ad-hoc bills exist (student_bills.source_module IS NOT NULL)';
    END IF;
END $$;

ALTER TABLE bill_items DROP CONSTRAINT IF EXISTS bill_items_amount_idr_check;
ALTER TABLE bill_items DROP CONSTRAINT IF EXISTS bill_items_net_idr_check;
ALTER TABLE bill_items ADD CONSTRAINT bill_items_amount_idr_check CHECK (amount_idr >= 0);
ALTER TABLE bill_items ADD CONSTRAINT bill_items_net_idr_check CHECK (net_idr >= 0);
ALTER TABLE bill_items DROP COLUMN IF EXISTS adjustment_id;

DROP TABLE IF EXISTS bill_adjustments;
DROP INDEX IF EXISTS student_bills_source_uq;

ALTER TABLE student_bills
    DROP CONSTRAINT IF EXISTS student_bills_source_ref_pair,
    DROP CONSTRAINT IF EXISTS student_bills_scheme_xor_source,
    DROP COLUMN IF EXISTS source_ref,
    DROP COLUMN IF EXISTS source_module,
    ALTER COLUMN scheme_id SET NOT NULL;
