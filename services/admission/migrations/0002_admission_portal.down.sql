-- Reverse of 0002: drop the tables/columns added for the portal slice.

DROP INDEX IF EXISTS admission_cbt_school_idx;
DROP TABLE IF EXISTS admission_cbt_participants;
DROP TABLE IF EXISTS voucher_redemptions;

ALTER TABLE ranking_entries DROP CONSTRAINT IF EXISTS ranking_entries_recommendation_check;
UPDATE ranking_entries SET recommendation='waitlist' WHERE recommendation='waitlisted';
ALTER TABLE ranking_entries
    ADD CONSTRAINT ranking_entries_recommendation_check CHECK(recommendation IN('accepted','waitlist'));

ALTER TABLE student_conversions
    DROP COLUMN IF EXISTS guardian_link_id,
    DROP COLUMN IF EXISTS current_step;

ALTER TABLE audit_events DROP COLUMN IF EXISTS actor_name;

ALTER TABLE fee_schemes
    DROP COLUMN IF EXISTS registration_fee_idr,
    DROP COLUMN IF EXISTS reregistration_fee_idr,
    DROP COLUMN IF EXISTS installment_count;

ALTER TABLE discount_vouchers
    DROP COLUMN IF EXISTS status,
    DROP COLUMN IF EXISTS code_hint;

ALTER TABLE discount_rules
    DROP COLUMN IF EXISTS stackable,
    DROP COLUMN IF EXISTS status;

ALTER TABLE payment_submissions
    DROP COLUMN IF EXISTS file_name,
    DROP COLUMN IF EXISTS transfer_at,
    DROP COLUMN IF EXISTS bank_name,
    DROP COLUMN IF EXISTS account_name,
    DROP COLUMN IF EXISTS reference,
    DROP COLUMN IF EXISTS review_note;

ALTER TABLE document_requirements DROP COLUMN IF EXISTS description;

ALTER TABLE application_documents DROP COLUMN IF EXISTS file_name;

ALTER TABLE admission_tracks
    DROP COLUMN IF EXISTS description,
    DROP COLUMN IF EXISTS is_active;

ALTER TABLE admission_periods
    DROP COLUMN IF EXISTS registration_open_at,
    DROP COLUMN IF EXISTS registration_close_at;

ALTER TABLE applicants
    DROP COLUMN IF EXISTS nickname,
    DROP COLUMN IF EXISTS nik,
    DROP COLUMN IF EXISTS previous_school,
    DROP COLUMN IF EXISTS address,
    DROP COLUMN IF EXISTS guardian_relationship;
