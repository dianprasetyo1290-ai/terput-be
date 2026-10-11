DROP TABLE IF EXISTS ppdb_announcements;
ALTER TABLE manual_discounts DROP COLUMN IF EXISTS decided_note, DROP COLUMN IF EXISTS decided_at,
    DROP COLUMN IF EXISTS decided_by_name, DROP COLUMN IF EXISTS decided_by, DROP COLUMN IF EXISTS created_by_name,
    DROP COLUMN IF EXISTS status;
ALTER TABLE admission_periods DROP COLUMN IF EXISTS max_discount_idr, DROP COLUMN IF EXISTS discount_mode;
ALTER TABLE discount_rules DROP COLUMN IF EXISTS applies_to, DROP COLUMN IF EXISTS quota,
    DROP COLUMN IF EXISTS valid_until, DROP COLUMN IF EXISTS valid_from;
ALTER TABLE document_requirements DROP COLUMN IF EXISTS stage;
ALTER TABLE charges DROP COLUMN IF EXISTS stage;
ALTER TABLE fee_schemes DROP COLUMN IF EXISTS min_reregistration_pct, DROP COLUMN IF EXISTS entrance_fee_idr,
    DROP COLUMN IF EXISTS wave_id;
