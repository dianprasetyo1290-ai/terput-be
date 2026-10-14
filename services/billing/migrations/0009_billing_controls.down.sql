DROP TABLE IF EXISTS billing_scheme_changes;
ALTER TABLE billing_scheme_components DROP CONSTRAINT IF EXISTS billing_scheme_components_free_amount;
ALTER TABLE billing_scheme_components DROP COLUMN IF EXISTS is_free;
ALTER TABLE billing_schemes DROP CONSTRAINT IF EXISTS billing_schemes_effective_window;
ALTER TABLE billing_schemes DROP COLUMN IF EXISTS effective_from, DROP COLUMN IF EXISTS effective_to;
DROP TABLE IF EXISTS bill_followups;
DROP TABLE IF EXISTS bill_reminder_log;
DROP TABLE IF EXISTS billing_reminder_settings;
