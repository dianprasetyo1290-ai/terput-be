DROP INDEX IF EXISTS idx_announcements_publish_at;
ALTER TABLE announcements
    DROP CONSTRAINT IF EXISTS announcements_class_needs_unit,
    DROP CONSTRAINT IF EXISTS announcements_audience_required;
-- audience_all rows go back to "empty target_roles = everyone"; class/unit
-- limits and schedules cannot be represented by the old schema.
UPDATE announcements SET target_roles = '{}' WHERE audience_all;
ALTER TABLE announcements
    DROP COLUMN publish_at,
    DROP COLUMN target_class_names,
    DROP COLUMN target_class_ids,
    DROP COLUMN target_school_ids,
    DROP COLUMN audience_all;
