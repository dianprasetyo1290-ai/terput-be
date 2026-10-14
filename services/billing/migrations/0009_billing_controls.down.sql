DROP INDEX IF EXISTS bill_refunds_school_transfer_ref;
ALTER TABLE bill_refunds DROP CONSTRAINT IF EXISTS bill_refunds_transfer_ref_required;
ALTER TABLE bill_refunds DROP COLUMN IF EXISTS approval_id, DROP COLUMN IF EXISTS transfer_ref,
    DROP COLUMN IF EXISTS transfer_date, DROP COLUMN IF EXISTS process_note;
DROP TABLE IF EXISTS billing_scheme_changes;
ALTER TABLE billing_scheme_components DROP CONSTRAINT IF EXISTS billing_scheme_components_free_amount;
ALTER TABLE billing_scheme_components DROP COLUMN IF EXISTS is_free;
ALTER TABLE billing_schemes DROP CONSTRAINT IF EXISTS billing_schemes_effective_window;
ALTER TABLE billing_schemes DROP COLUMN IF EXISTS effective_from, DROP COLUMN IF EXISTS effective_to;
DROP TABLE IF EXISTS bill_followups;
DROP TABLE IF EXISTS bill_reminder_log;
DROP TABLE IF EXISTS billing_reminder_settings;
