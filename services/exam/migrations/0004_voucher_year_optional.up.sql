-- A voucher's validity window can span academic years (or be issued before
-- one is picked at all), so it's no longer meaningfully tied to one.
ALTER TABLE exam_tokens ALTER COLUMN academic_year_id DROP NOT NULL;
