DROP INDEX IF EXISTS guardians_identity_user_idx;
UPDATE guardians SET identity_user_id = identity_user_id_unlinked WHERE identity_user_id_unlinked IS NOT NULL AND identity_user_id IS NULL;
ALTER TABLE guardians DROP COLUMN IF EXISTS identity_user_id_unlinked;
DROP TABLE IF EXISTS admission_refunds;
ALTER TABLE admission_periods DROP COLUMN IF EXISTS refund_admin_fee_idr, DROP COLUMN IF EXISTS refund_policy_bp;
ALTER TABLE admission_waves DROP COLUMN IF EXISTS entry_grade, DROP COLUMN IF EXISTS education_level;
ALTER TABLE applicants DROP COLUMN IF EXISTS custom_fields;
DROP TABLE IF EXISTS admission_form_fields;
