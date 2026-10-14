DROP TABLE IF EXISTS bill_change_requests;
UPDATE bill_adjustments SET kind = 'adjust' WHERE kind = 'cancel';
ALTER TABLE bill_adjustments DROP CONSTRAINT IF EXISTS bill_adjustments_kind_check;
ALTER TABLE bill_adjustments ADD CONSTRAINT bill_adjustments_kind_check CHECK (kind IN ('prorate','adjust'));
DROP TABLE IF EXISTS bill_installments;
ALTER TABLE billing_schemes DROP COLUMN IF EXISTS program, DROP COLUMN IF EXISTS cohort, DROP COLUMN IF EXISTS admission_track;
ALTER TABLE billing_scheme_components DROP COLUMN IF EXISTS installment_terms, DROP COLUMN IF EXISTS installment_interval_months;
UPDATE billing_scheme_components SET kind = 'one_time' WHERE kind = 'annual';
ALTER TABLE billing_scheme_components DROP CONSTRAINT IF EXISTS billing_scheme_components_kind_check;
ALTER TABLE billing_scheme_components ADD CONSTRAINT billing_scheme_components_kind_check CHECK (kind IN ('monthly','one_time'));
ALTER TABLE gateway_settings DROP COLUMN IF EXISTS receipt_resend_cooldown_minutes;
UPDATE reconciliation_exceptions SET status = 'open' WHERE status = 'pending_approval';
ALTER TABLE reconciliation_exceptions DROP CONSTRAINT IF EXISTS reconciliation_exceptions_status_check;
ALTER TABLE reconciliation_exceptions ADD CONSTRAINT reconciliation_exceptions_status_check CHECK (status IN ('open', 'resolved'));
ALTER TABLE reconciliation_exceptions DROP COLUMN IF EXISTS requested_by, DROP COLUMN IF EXISTS requested_at,
    DROP COLUMN IF EXISTS decision_note;
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
